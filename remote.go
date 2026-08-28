// Author: Lars Op den Kamp (lars@opdenkamp-it.nl)
// Copyright (c) 2026 Op den Kamp IT Solutions

package mxremote

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Config configures a Remote.
type Config struct {
	// TargetIP is the multicast/broadcast destination. Empty uses the default
	// (multicast 224.8.8.8, or the interface broadcast address when Broadcast).
	TargetIP string
	// Port is the UDP port. Zero uses the default for the selected mode.
	Port int
	// LocalIP selects the interface by address: it becomes the multicast egress
	// interface and the membership interface, so it decides which NIC frames
	// leave by and which one they are accepted on. Empty picks the first
	// non-loopback IPv4 the host enumerates, which on a multi-homed host is
	// arbitrary - set this or Interface whenever more than one NIC exists. The
	// symptom of picking wrong is one-sided: periodic broadcasts still arrive,
	// so discovery looks healthy while every request this library sends leaves
	// by the wrong NIC and is never answered.
	LocalIP string
	// Interface is the network interface name (e.g. "br_v8") for multicast
	// discovery. When set it takes precedence over LocalIP and works even on an
	// interface with no IPv4 address (tagged VLANs): membership and egress are
	// keyed by interface index. Linux only; on macOS/Windows the interface must
	// have an IPv4 address. Empty keeps the LocalIP behaviour.
	Interface string
	// Broadcast uses broadcast instead of multicast.
	Broadcast bool
	// Name is advertised on the network.
	Name string
	// Callbacks receives state-change events.
	Callbacks Callbacks
}

// Remote is the main entry point. It manages the UDP connection, discovers
// devices, and maintains the device registry. Create one with New and start it
// with Start.
type Remote struct {
	mu        sync.Mutex
	devices   map[DeviceUID]*Device
	links     *bayLinks
	pending   []func()
	callbacks Callbacks

	uid  DeviceUID
	name string

	targetIP  string
	port      int
	localIP   string
	iface     string
	broadcast bool

	conn            *conn
	txTap           func([]byte)
	closing         bool
	wg              sync.WaitGroup
	lastHello       time.Time
	helloInterval   time.Duration
	discoverTimeout time.Time
}

// New creates a Remote from cfg. Call Start to begin discovery.
func New(cfg Config) *Remote {
	name := cfg.Name
	if name == "" {
		name = "MXR Go"
	}
	r := &Remote{
		devices:   map[DeviceUID]*Device{},
		callbacks: cfg.Callbacks,
		name:      name,
		targetIP:  cfg.TargetIP,
		port:      cfg.Port,
		localIP:   cfg.LocalIP,
		iface:     cfg.Interface,
		broadcast: cfg.Broadcast,
	}
	r.links = newBayLinks(r)
	r.helloInterval = nextHelloInterval()
	return r
}

func (r *Remote) effectiveTargetIP() string {
	if r.targetIP != "" {
		return r.targetIP
	}
	if !r.broadcast {
		return MulticastIP
	}
	if b, err := broadcastAddress(r.localIP); err == nil && b != "" {
		return b
	}
	return MulticastIP
}

func (r *Remote) effectivePort() int {
	if r.port != 0 {
		return r.port
	}
	if r.broadcast {
		return BroadcastPort
	}
	return MulticastPort
}

// emit queues a callback to run after the lock is released. Must hold r.mu.
func (r *Remote) emit(fn func()) {
	if fn != nil {
		r.pending = append(r.pending, fn)
	}
}

// runLocked runs fn under the lock, then fires any queued callbacks.
func (r *Remote) runLocked(fn func()) {
	r.mu.Lock()
	fn()
	pending := r.pending
	r.pending = nil
	r.mu.Unlock()
	for _, cb := range pending {
		cb()
	}
}

// Start loads the client UID, opens the connection and begins discovery. It
// returns once the listener is running; discovery continues in the background
// until Close. The ctx bounds the background goroutines' lifetime.
func (r *Remote) Start(ctx context.Context) error {
	if err := r.loadUID(); err != nil {
		return err
	}
	c, err := newConn(r.effectiveTargetIP(), r.effectivePort(), r.localIP, r.iface)
	if err != nil {
		return err
	}
	r.conn = c

	r.wg.Add(1)
	go r.receiveLoop(c)

	r.txHello()
	r.txDiscover()

	r.wg.Add(1)
	go r.backgroundProbe(ctx)
	return nil
}

// Close stops discovery and closes the connection.
func (r *Remote) Close() error {
	r.mu.Lock()
	if r.closing {
		r.mu.Unlock()
		return nil
	}
	r.closing = true
	c := r.conn
	r.mu.Unlock()

	if c != nil {
		c.close()
	}
	r.wg.Wait()
	return nil
}

func (r *Remote) loadUID() error {
	if !r.uid.Empty() {
		return nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	path := filepath.Join(home, ".mxr-uid")
	if b, err := os.ReadFile(path); err == nil && len(b) >= 16 {
		copy(r.uid[:], b[:16])
		return nil
	}
	if _, err := rand.Read(r.uid[:]); err != nil {
		return err
	}
	_ = os.WriteFile(path, r.uid[:], 0o600)
	return nil
}

// UID returns this client's unique id.
func (r *Remote) UID() DeviceUID { return r.uid }

// Name returns the advertised name.
func (r *Remote) Name() string { return r.name }

// Devices returns a snapshot of all discovered devices.
func (r *Remote) Devices() []*Device {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]*Device, 0, len(r.devices))
	for _, d := range r.devices {
		out = append(out, d)
	}
	return out
}

// GetByUID returns the device with the given UID, or nil.
func (r *Remote) GetByUID(uid DeviceUID) *Device {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.devices[uid]
}

// GetBySerial returns the device with the given serial number, or nil.
func (r *Remote) GetBySerial(serial string) *Device {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.getBySerialLocked(serial)
}

func (r *Remote) getBySerialLocked(serial string) *Device {
	for _, d := range r.devices {
		if d.hello.serial == serial {
			return d
		}
	}
	return nil
}

// GetByUIDString resolves a device by its dotted-hex UID string, falling back to
// a serial-number match, mirroring the reference get_by_uid.
func (r *Remote) GetByUIDString(s string) *Device {
	if uid, err := ParseDeviceUID(s); err == nil {
		if d := r.GetByUID(uid); d != nil {
			return d
		}
	}
	return r.GetBySerial(s)
}

// GetBayByPortnum returns the bay on the given device (by UID) and port, or nil.
func (r *Remote) GetBayByPortnum(uid DeviceUID, port int) *Bay {
	r.mu.Lock()
	defer r.mu.Unlock()
	d := r.devices[uid]
	if d == nil {
		return nil
	}
	return d.getByPortnumLocked(port)
}

func (r *Remote) getBayByPortnameLocked(serial, portname string) *Bay {
	d := r.getBySerialLocked(serial)
	if d == nil {
		return nil
	}
	return d.getByPortnameLocked(portname)
}

// ValidAddresses returns the non-loopback IPv4 addresses usable as LocalIP.
func ValidAddresses() []string { return validAddresses() }

// UpdateConfig changes the local interface and/or multicast/broadcast mode at
// runtime, reconnecting if the network parameters changed.
func (r *Remote) UpdateConfig(localIP string, broadcast bool) error {
	r.mu.Lock()
	changed := r.localIP != localIP || r.broadcast != broadcast
	if !changed {
		r.mu.Unlock()
		return nil
	}
	r.localIP = localIP
	r.broadcast = broadcast
	old := r.conn
	iface := r.iface
	target, port := r.effectiveTargetIP(), r.effectivePort()
	r.mu.Unlock()

	if old != nil {
		old.close()
	}
	c, err := newConn(target, port, localIP, iface)
	if err != nil {
		return err
	}
	r.mu.Lock()
	r.conn = c
	r.mu.Unlock()

	r.wg.Add(1)
	go r.receiveLoop(c)
	r.txHello()
	r.txDiscover()
	return nil
}

func (r *Remote) getByStreamIPLocked(ip string, audio bool) *Bay {
	for _, d := range r.devices {
		if !d.isV2IP() || d.v2ipSources == nil {
			continue
		}
		in := d.firstInputLocked()
		if in == nil {
			continue
		}
		src := d.v2ipSourceForLocked(in)
		if src == nil {
			continue
		}
		if !audio && src.Video.IP == ip {
			return in
		}
		if audio && src.Audio.IP == ip {
			return in
		}
	}
	return nil
}

// transmit sends raw bytes to the target. Safe to call without the lock.
// transmit sends a frame, refusing one the target cannot receive.
//
// target names the device the frame is addressed to, or nil for a broadcast
// with no single recipient. It is a parameter rather than something derived
// here because the addressed device is a payload uid at an offset that differs
// per opcode, while the opcode itself is at a fixed place in the header.
//
// The check lives here rather than at each call site on purpose: every frame
// reaches the wire through this function and every frame is built by
// buildFrame, so a send that skips the gate cannot be written by accident. An
// earlier per-site version missed the one method that built its own frame
// instead of delegating to a gated sibling.
func (r *Remote) transmit(target *Device, data []byte) (int, error) {
	r.mu.Lock()
	c := r.conn
	tap := r.txTap
	var err error
	if target != nil && len(data) >= headerLen {
		err = target.requireOpcodeLocked(binary.LittleEndian.Uint16(data[20:22]))
	}
	r.mu.Unlock()
	if err != nil {
		return 0, err
	}
	// A frame that passes the gate is what this library would put on the wire,
	// so tests round-trip it back through the decoder from here. Most payloads
	// are assembled inside their control method and cannot be reached any other
	// way.
	if tap != nil {
		tap(data)
	}
	if c == nil {
		return 0, fmt.Errorf("connection closed")
	}
	return c.transmit(data)
}

func (r *Remote) txDiscover() {
	r.mu.Lock()
	r.discoverTimeout = time.Now()
	uid := r.uid
	r.mu.Unlock()
	_, _ = r.transmit(nil, buildFrame(uid, opSysDiscover, protocolFor(opSysDiscover), nil))
}

// Hello announcement cadence, matching the firmware's own: a 2.5s base plus up
// to 2.5s of jitter, re-drawn after each send so a mesh full of clients does
// not fall into step.
//
// The probe loop ticks once a second, so the interval actually observed is the
// draw rounded up to the next tick — effectively 3, 4 or 5 seconds rather than
// a continuous 2.5 to 5. Coarser than the firmware, and left that way: the
// jitter exists to stop announcers colliding, and three values across clients
// whose ticks start at different moments is enough for that.
const (
	helloBaseInterval   = 2500 * time.Millisecond
	helloJitterInterval = 2500 * time.Millisecond
)

func nextHelloInterval() time.Duration {
	var b [2]byte
	if _, err := rand.Read(b[:]); err != nil {
		return helloBaseInterval + helloJitterInterval/2
	}
	jitter := time.Duration(uint16(b[0])<<8|uint16(b[1])) * helloJitterInterval / 65536
	return helloBaseInterval + jitter
}

// helloDueLocked reports whether it is time to announce again.
//
// This is a timer, not a response to traffic: a device announces itself on a
// schedule whether or not anything is talking to it, and a client that only
// re-announced when a datagram arrived would go silent on a quiet network and
// stay unknown to every peer that started after it.
func (r *Remote) helloDueLocked(now time.Time) bool {
	return !r.closing && now.Sub(r.lastHello) >= r.helloInterval
}

func (r *Remote) txHello() {
	r.mu.Lock()
	uid := r.uid
	name := r.name
	r.mu.Unlock()

	payload := make([]byte, 0, 54)
	payload = append(payload, byte(ProtocolVersion&0xFF), byte(ProtocolVersion>>8))
	payload = appendFixedStr(payload, name, 16)
	payload = appendFixedStr(payload, "P9SN00000000", 16)
	payload = appendFixedStr(payload, Version, 16)
	feat := uint32(FeatureManager)
	payload = append(payload, byte(feat), byte(feat>>8), byte(feat>>16), byte(feat>>24))
	// Re-arm only once the frame is actually away, as the firmware does — it
	// resets hello_timeout inside the branch where mxr_transmit succeeded. A
	// send that fails is retried on the next tick rather than costing a whole
	// interval of silence, which matters most at startup and after a network
	// blip: exactly when being heard is worth the most.
	if n, err := r.transmit(nil, buildFrame(uid, opSysHello, protocolFor(opSysHello), payload)); err != nil || n <= 0 {
		return
	}
	r.mu.Lock()
	r.lastHello = time.Now()
	r.helloInterval = nextHelloInterval()
	r.mu.Unlock()
}

func (r *Remote) receiveLoop(c *conn) {
	defer r.wg.Done()
	buf := make([]byte, 65535)
	for {
		n, addr, err := c.read(buf)
		if err != nil {
			return
		}
		data := make([]byte, n)
		copy(data, buf[:n])
		r.processDatagram(data, addr)
	}
}

func (r *Remote) processDatagram(data []byte, addr string) {
	r.processFrame(data, addr, time.Now())
}

func (r *Remote) processFrame(data []byte, addr string, ts time.Time) {
	f, err := parseFrame(data, addr, ts)
	if err != nil {
		return
	}
	if f.remoteID() == r.uid {
		return
	}
	handler := frameHandlers[f.opcode()]
	r.runLocked(func() {
		if handler != nil {
			handler(r, f)
		}
		// Any frame from a known device proves it is alive — refresh its
		// liveness so online detection doesn't rely on hello frames alone
		// (V2IP devices use a 15s window but only hello every 30s).
		if d := r.devices[f.remoteID()]; d != nil {
			d.touch(f.timestamp)
		}
	})
}

func (r *Remote) backgroundProbe(ctx context.Context) {
	defer r.wg.Done()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.mu.Lock()
			if r.closing {
				r.mu.Unlock()
				return
			}
			txDiscover := false
			hasComplete := false
			for _, d := range r.devices {
				if d.configurationCompleteLocked() {
					hasComplete = true
				}
			}
			if !hasComplete {
				txDiscover = true
			} else {
				for _, d := range r.devices {
					d.checkOnline()
					if !d.configurationCompleteLocked() && time.Since(d.helloReceived) <= 15*time.Second {
						// still within the grace period
					} else if !d.configurationCompleteLocked() {
						txDiscover = true
					}
				}
			}
			due := time.Since(r.discoverTimeout) >= 5*time.Second
			helloDue := r.helloDueLocked(time.Now())
			pending := r.pending
			r.pending = nil
			r.mu.Unlock()
			if helloDue {
				r.txHello()
			}
			for _, cb := range pending {
				cb()
			}
			if txDiscover && due {
				r.txDiscover()
			}
		}
	}
}

// onHello registers or updates a device from a hello frame.
func (r *Remote) onHello(h helloInfo, uid DeviceUID) {
	d := r.devices[uid]
	if d == nil {
		d = newDevice(r, uid, h)
		r.devices[uid] = d
	}
	d.applyHello(h)
}

// appendFixedStr appends value (ASCII, truncated to size) padded with NULs to size.
func appendFixedStr(dst []byte, value string, size int) []byte {
	b := []byte(value)
	if len(b) > size {
		b = b[:size]
	}
	dst = append(dst, b...)
	for i := len(b); i < size; i++ {
		dst = append(dst, 0)
	}
	return dst
}
