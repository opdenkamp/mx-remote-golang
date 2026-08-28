// Author: Lars Op den Kamp (lars@opdenkamp-it.nl)
// Copyright (c) 2026 Op den Kamp IT Solutions

package mxremote

import (
	"encoding/binary"
	"testing"
)

// Coverage over the opcode handlers that had none.
//
// A handler nothing exercises has every field unpinned by construction, which
// neither an offset sweep nor a poisoned fixture reports: both measure tests
// that run, and these did not. `go test -cover` over the handlers is what finds
// this class.

func bayStateRemote(t *testing.T, n byte, cb Callbacks) (*Remote, DeviceUID, func(uint16, []byte)) {
	t.Helper()
	r := newTestRemote(cb)
	sender := uidN(n)
	feed := feeder(r, sender)
	feed(opSysHello, helloPayload(0x28, "FF88", "HD0001", "4.8.0", FeatureVideoRouting))
	feed(opSysBayConfig, append(
		bayConfigRec(1, 0, 0, "Input 1", "Apple TV", 0, BayHDMIIn),
		bayConfigRec(2, 1, 0, "Output 1", "TV", 0, BayHDMIOut|BayAudioAmpOut)...))
	return r, sender, feed
}

func TestBayTargetedHandlers(t *testing.T) {
	var key RCKey
	var action RCAction
	var clip AudioClip
	r, sender, feed := bayStateRemote(t, 110, Callbacks{
		OnKeyPressed:     func(_ *Bay, k RCKey) { key = k },
		OnActionReceived: func(_ *Bay, a RCAction) { action = a },
		OnAudioClip:      func(_ *Bay, c AudioClip) { clip = c },
	})
	dev := r.GetByUID(sender)
	in, out := dev.GetByPortnum(1), dev.GetByPortnum(2)

	// 0x04 DEV_CONNECT: an input reports signal, an output reports HPD
	feed(opDevConnect, []byte{1, 1})
	if !in.SignalDetected() {
		t.Fatal("connect status did not set signal detected on an input")
	}

	// 0x05 DEV_POWER_CHANGE
	feed(opDevPowerChange, []byte{2, 1})
	if out.PowerStatusValue() != PowerOn {
		t.Fatalf("power = %v, want on", out.PowerStatusValue())
	}

	// 0x27 BAY_HIDE, addressed by uid then a u16 port
	hide := append(append([]byte(nil), sender[:]...), 2, 0, 1)
	feed(opBayHide, hide)
	if !out.Hidden() {
		t.Fatal("bay should be hidden")
	}

	// 0x14 AUDIO_SET_VOLUME: uid, u16 port, then volume/mute
	vol := append(append([]byte(nil), sender[:]...), 2, 0, 40, 45, 0, 0, 0, 0)
	feed(opAudioSetVolume, vol)
	if v := out.VolumeStatus(); v == nil || v.VolumeLeft != 40 || v.VolumeRight != 45 {
		t.Fatalf("volume set = %+v", v)
	}

	// 0x12 AUDIO_VOLUME_MUTE: the notification form, port as a single byte
	feed(opAudioVolumeMute, []byte{2, 55, 60, 0})
	if v := out.VolumeStatus(); v == nil || v.VolumeLeft != 55 || v.VolumeRight != 60 {
		t.Fatalf("volume notification = %+v", v)
	}

	// 0x0B RC_KEY and 0x0D RC_ACTION, both u16 port at protocol >= 6
	feed(opRCKey, []byte{1, 0, 0x41, 0x00})
	if key != RCKey(0x41) {
		t.Fatalf("key = %v", key)
	}
	feed(opRCAction, []byte{1, 0, byte(ActionPowerOn), 0})
	if action != ActionPowerOn {
		t.Fatalf("action = %v", action)
	}

	// 0x11 AUDIO_CLIP
	feed(opAudioClip, []byte{2, 1})
	if clip.Port != 2 || clip.Clip != 1 {
		t.Fatalf("clip = %+v", clip)
	}

	// 0x39 BAY_STATUS
	st := make([]byte, 28)
	st[0] = 1
	binary.LittleEndian.PutUint32(st[20:24], uint32(BayStatusSignalDetected))
	binary.LittleEndian.PutUint32(st[24:28], uint32(BayHDMIIn))
	feed(opBayStatus, st)
	if !in.SignalDetected() {
		t.Fatal("bay status should report signal detected")
	}
}

func TestRoutingAndDeviceHandlers(t *testing.T) {
	r, sender, feed := bayStateRemote(t, 111, Callbacks{})
	dev := r.GetByUID(sender)

	// 0x08 MX_ROUTE: sink port, then video and audio sources two bytes apart
	feed(opMxRoute, []byte{2, 0, 1, 0, 1, 0})
	if v := dev.GetByPortnum(2).VideoSource(); v == nil || v.Port() != 1 {
		t.Fatalf("video source = %v", v)
	}

	// 0x15 SYS_TEMPERATURE: a count then that many readings
	feed(opSysTemperature, []byte{2, 41, 43})
	if temps := dev.Temperatures(); len(temps) != 2 {
		t.Fatalf("temperatures = %v", temps)
	}

	// 0x44 V2IP_BAY_MAPPINGS: count<<1|isInput, first bay, then uids from 8
	mapped := uidN(112)
	bm := make([]byte, 8+16)
	binary.LittleEndian.PutUint16(bm[0:2], (1<<1)|1) // one input mapping
	binary.LittleEndian.PutUint16(bm[2:4], 0)
	copy(bm[8:24], mapped[:])
	feed(opV2IPBayMappings, bm)
	if got := dev.GetByPortnum(1).V2IPUID(); got != mapped {
		t.Fatalf("bay mapping uid = %v, want %v", got, mapped)
	}

	// 0x40 V2IP_TILING addressed at a known device caches as its window
	tl := append([]byte(nil), sender[:]...)
	tl = append(tl, make([]byte, 8)...)
	binary.LittleEndian.PutUint16(tl[16:18], 640)
	binary.LittleEndian.PutUint16(tl[20:22], 1920)
	binary.LittleEndian.PutUint16(tl[22:24], 1080)
	feed(opV2IPTiling, tl)
	if w := dev.Tiling(); w == nil || w.PosX != 640 || w.Width != 1920 || w.Height != 1080 {
		t.Fatalf("tiling = %v", w)
	}
}

func TestCommandHandlersReachTheirCallbacks(t *testing.T) {
	var seen []string
	var reboot RebootRequest
	var profile EDIDProfileChange
	var link DeviceUID
	r, sender, feed := bayStateRemote(t, 113, Callbacks{
		OnDiscoverRequest:            func(*Device) { seen = append(seen, "discover") },
		OnDetectBaysRequested:        func(*Device) { seen = append(seen, "detect") },
		OnUpgradeFPGARequested:       func(*Device) { seen = append(seen, "fpga") },
		OnMonitoringPulse:            func(*Device) { seen = append(seen, "pulse") },
		OnRebootRequested:            func(_ *Device, q RebootRequest) { reboot = q },
		OnEDIDProfileChangeRequested: func(_ *Device, c EDIDProfileChange) { profile = c },
		OnV2IPLinkChanged:            func(_ *Device, u DeviceUID) { link = u },
	})
	_ = r
	target := uidN(114)

	feed(opSysDiscover, nil)
	feed(opV2IPDetectBays, nil)
	feed(opV2IPUpgradeFPGA, nil)
	feed(opSysMonitoringPulse, nil)
	if len(seen) != 4 {
		t.Fatalf("payload-free commands seen = %v, want all four", seen)
	}

	feed(opSysReboot, target[:])
	if reboot.Target != target {
		t.Fatalf("reboot target = %v", reboot.Target)
	}

	ep := append(append([]byte(nil), target[:]...), byte(Edid4K), 0, 0, 0, 0, 0, 0, 0)
	feed(opBayEDIDProfile, ep)
	if profile.Target != target || profile.Profile != Edid4K {
		t.Fatalf("edid profile change = %+v", profile)
	}

	feed(opV2IPLinkRemote, target[:])
	if link != target {
		t.Fatalf("v2ip link target = %v", link)
	}
	_ = sender
}

// 0x29 has four layouts across three version gates, plus two that share the
// 0x22 stamp because port and features_status were widened without a bump.
func TestNetworkStatusLayouts(t *testing.T) {
	// the later 0x22 form, as measured on a live mesh: name at 4, mac at 21
	late := make([]byte, 144)
	binary.LittleEndian.PutUint16(late[0:2], 4)
	binary.LittleEndian.PutUint16(late[2:4], 1<<3|1<<6) // igmp + uplink
	copy(late[4:21], "UTP PoE+")
	copy(late[21:27], []byte{0x00, 0x15, 0x82, 0x13, 0x89, 0xae})
	copy(late[28:32], []byte{10, 8, 83, 228})
	copy(late[32:36], []byte{10, 8, 8, 254})

	s, ok := parseNetworkStatus(late, 0x22)
	if !ok {
		t.Fatal("late form did not parse")
	}
	if s.Port != 4 || s.Name != "UTP PoE+" {
		t.Fatalf("late: port %d name %q", s.Port, s.Name)
	}
	if s.MACAddress != "00:15:82:13:89:AE" {
		t.Fatalf("late mac = %q", s.MACAddress)
	}
	if s.IP != "10.8.83.228" || s.Querier != "10.8.8.254" {
		t.Fatalf("late ip/querier = %q / %q", s.IP, s.Querier)
	}

	// the earlier 0x22 form: everything ahead of ip shifts down two, but ip and
	// querier do not move because ip4_addr_t aligns to 4
	early := make([]byte, 144)
	early[0] = 4
	early[1] = 1<<3 | 1<<6
	copy(early[2:19], "eth0")
	copy(early[19:25], []byte{0x00, 0x15, 0x82, 0x13, 0x89, 0xae})
	copy(early[28:32], []byte{10, 8, 83, 229})
	copy(early[32:36], []byte{10, 8, 8, 254})

	s, ok = parseNetworkStatus(early, 0x22)
	if !ok {
		t.Fatal("early form did not parse")
	}
	if s.Port != 4 || s.Name != "eth0" {
		t.Fatalf("early: port %d name %q", s.Port, s.Name)
	}
	if s.MACAddress != "00:15:82:13:89:AE" || s.IP != "10.8.83.229" {
		t.Fatalf("early mac/ip = %q / %q", s.MACAddress, s.IP)
	}
}

// The legacy struct grew by appending, so a field only exists from the version
// that added it: ip and querier at 0x12, mac at 0x21.
func TestNetworkStatusLegacyGating(t *testing.T) {
	d := make([]byte, 146)
	copy(d[112:128], "UTP PoE+")
	copy(d[132:136], []byte{10, 8, 83, 228})
	copy(d[136:140], []byte{10, 8, 8, 254})
	copy(d[140:146], []byte{0x00, 0x15, 0x82, 0x13, 0x89, 0xae})

	s, ok := parseNetworkStatusPre22(d, 0x21)
	if !ok || s.Name != "UTP PoE+" || s.IP != "10.8.83.228" || s.MACAddress == "" {
		t.Fatalf("0x21 form = %+v", s)
	}
	// at 0x12 there is no mac: offset 140 is whatever follows the struct
	s, _ = parseNetworkStatusPre22(d, 0x12)
	if s.IP != "10.8.83.228" {
		t.Fatalf("0x12 should still carry ip, got %q", s.IP)
	}
	if s.MACAddress != "" {
		t.Fatalf("0x12 predates mac, but decoded %q", s.MACAddress)
	}
	// at 0x06 there is no ip or querier either
	s, _ = parseNetworkStatusPre22(d, 0x06)
	if s.IP != "" || s.Querier != "" || s.MACAddress != "" {
		t.Fatalf("0x06 predates ip/querier/mac, got %q / %q / %q", s.IP, s.Querier, s.MACAddress)
	}
	if s.Name != "UTP PoE+" {
		t.Fatalf("0x06 name = %q", s.Name)
	}
}

// 0x1F V2IP_SOURCE_SWITCH: a sink is told which stream IPs to subscribe to, and
// the sources are resolved back to the bays advertising those addresses.
func TestV2IPSourceSwitch(t *testing.T) {
	r := newTestRemote(Callbacks{})
	src := uidN(120)
	sf := feeder(r, src)
	sf(opSysHello, helloPayload(0x28, "ONEIP-TX", "TX9", "4.8.0", FeatureV2IPSource))
	sf(opSysBayConfig, bayConfigRec(1, 0, 0, "Input 1", "Apple TV", 0, BayHDMIIn))
	sf(opSysBayV2IPSources, streamRec(src, "239.5.5.1", "239.5.5.2", "239.5.5.3", V2IPPortVideo))

	sink := uidN(121)
	kf := feeder(r, sink)
	kf(opSysHello, helloPayload(0x28, "ONEIP-RX", "RX9", "4.8.0", FeatureV2IPSink))
	kf(opSysBayConfig, bayConfigRec(1, 1, 0, "Output 1", "TV", 0, BayV2IPSinkLocal))

	p := append([]byte(nil), sink[:]...)
	p = append(p, 239, 5, 5, 1) // video group
	p = append(p, 239, 5, 5, 2) // audio group
	kf(opV2IPSourceSwitch, p)

	out := r.GetByUID(sink).GetByPortnum(1)
	if v := out.VideoSource(); v == nil || v.Device().UID() != src {
		t.Fatalf("video source did not resolve to the advertising device: %v", v)
	}
}
