// Author: Lars Op den Kamp (lars@opdenkamp-it.nl)
// Copyright (c) 2026 Op den Kamp IT Solutions

package mxremote

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

// helloInfo is the device advertisement parsed from a hello frame.
type helloInfo struct {
	supportedProtocol uint16
	name              string
	serial            string
	version           string
	features          DeviceFeature
	address           string
	hasFeatures       bool
}

func (h helloInfo) equal(o helloInfo) bool {
	return h.supportedProtocol == o.supportedProtocol &&
		h.name == o.name && h.serial == o.serial &&
		h.version == o.version && h.features == o.features
}

// Device is a discovered device on the MX Remote network (matrix, OneIP unit or
// amplifier). All exported methods are safe for concurrent use.
type Device struct {
	remote *Remote
	uid    DeviceUID

	hello         helloInfo
	bays          map[int]*Bay
	temperatures  []int
	online        bool
	haveConfig    bool
	rebooting     bool
	lastPing      time.Time
	helloReceived time.Time

	linkConfigReceived bool

	v2ipSources   []V2IPStreamSources
	v2ipDetails   *DeviceV2IPDetails
	pduState      *PDUState
	setupDone     *bool
	installerID   *uint16
	tiling        *V2IPTilingConfig
	rcSettings    *RCSettings
	audioSelect   *AudioChangeSource
	v2ipSink      *DeviceV2IPSink
	v2ipVersions  map[FirmwareType]FirmwareVersion
	meshMasterUID DeviceUID
	network       map[int]NetworkPortStatus
	sysStatus     *int
	sysMessage    *string
	topology      []TopologyEntry
	audio         *AudioEndpoints
	mv            *Multiviewer
	dolbySettings *AmpDolbySettings
	v2ipStats     *V2IPDeviceStats

	callbacks []func(*Device)
}

func newDevice(r *Remote, uid DeviceUID, h helloInfo) *Device {
	now := time.Now()
	return &Device{
		remote:        r,
		uid:           uid,
		hello:         h,
		bays:          map[int]*Bay{},
		v2ipVersions:  map[FirmwareType]FirmwareVersion{},
		network:       map[int]NetworkPortStatus{},
		online:        true,
		lastPing:      now,
		helloReceived: now,
	}
}

// RegisterCallback registers fn to be called whenever this device changes.
func (d *Device) RegisterCallback(fn func(*Device)) {
	d.remote.mu.Lock()
	defer d.remote.mu.Unlock()
	d.callbacks = append(d.callbacks, fn)
}

func (d *Device) emitSelf() {
	for _, fn := range d.callbacks {
		fn := fn
		d.remote.emit(func() { fn(d) })
	}
}

// notify fires a specific device callback (if non-nil), the generic
// OnDeviceUpdate callback, and any per-device registered callbacks.
func (d *Device) notify(specific func()) {
	r := d.remote
	if specific != nil {
		r.emit(specific)
	}
	if r.callbacks.OnDeviceUpdate != nil {
		r.emit(func() { r.callbacks.OnDeviceUpdate(d) })
	}
	d.emitSelf()
}

// ---- identity ----

// UID returns the device unique id.
func (d *Device) UID() DeviceUID { return d.uid }

// Serial returns the device serial number.
func (d *Device) Serial() string {
	d.remote.mu.Lock()
	defer d.remote.mu.Unlock()
	return d.serialLocked()
}

func (d *Device) serialLocked() string {
	if d.hello.serial == "" {
		return "Unknown"
	}
	return d.hello.serial
}

// Name returns the device name.
func (d *Device) Name() string {
	d.remote.mu.Lock()
	defer d.remote.mu.Unlock()
	return d.nameLocked()
}

func (d *Device) nameLocked() string {
	if d.hello.name == "" {
		return "Unknown"
	}
	if strings.TrimSpace(d.hello.name) == "" {
		return "<unnamed>"
	}
	return d.hello.name
}

// Address returns the device IP address.
func (d *Device) Address() string {
	d.remote.mu.Lock()
	defer d.remote.mu.Unlock()
	return d.hello.address
}

// Version returns the firmware version string.
func (d *Device) Version() string {
	d.remote.mu.Lock()
	defer d.remote.mu.Unlock()
	if d.hello.version == "" {
		return "Unknown"
	}
	return d.hello.version
}

// Protocol returns the protocol version the device supports.
func (d *Device) Protocol() int {
	d.remote.mu.Lock()
	defer d.remote.mu.Unlock()
	return int(d.hello.supportedProtocol)
}

// Features returns the device feature bitmask.
func (d *Device) Features() DeviceFeature {
	d.remote.mu.Lock()
	defer d.remote.mu.Unlock()
	return d.hello.features
}

// ---- type checks (locked internal) ----

func (d *Device) isV2IP() bool {
	return d.hello.features.Has(FeatureV2IPSink) || d.hello.features.Has(FeatureV2IPSource)
}

func (d *Device) isVideoMatrix() bool { return d.hello.features.Has(FeatureVideoRouting) }

func (d *Device) isAudioMatrix() bool {
	return d.hello.features.Has(FeatureAudioRouting) && !d.hello.features.Has(FeatureVideoRouting)
}

func (d *Device) isAmp() bool {
	return d.hello.features.Has(FeatureVolumeControl) &&
		d.hello.features.Has(FeatureAudioRouting) && !d.hello.features.Has(FeatureVideoRouting)
}

func (d *Device) isOneipMultiviewer() bool {
	return d.isV2IP() && d.hello.features.Has(FeatureMultiviewer)
}

// ErrProtocolTooOld is returned when a device's reported protocol version is
// below the floor of the opcode a call would have to send. Test for it with
// errors.Is to tell a refusal apart from a transmit failure.
var ErrProtocolTooOld = errors.New("device protocol too old for this opcode")

// requireOpcodeLocked reports whether this device can receive the opcode.
//
// A receiver drops any frame stamped above its own protocol version, silently
// and with no NAK, so sending an opcode whose floor exceeds what the device
// reports is futile rather than merely unsupported: the call would otherwise
// succeed and nothing would happen. A ProAmp8 on 4.1.1 reports 0x11, which is
// below the floor of several opcodes this library can send.
//
// A device that has not reported a version is allowed through - not knowing is
// not the same as knowing it is too old.
func (d *Device) requireOpcodeLocked(opcode uint16) error {
	have := d.hello.supportedProtocol
	need := protocolFor(opcode)
	if have == 0 || have >= need {
		return nil
	}
	return fmt.Errorf("%w: %s reports %#02x, opcode %#02x needs %#02x",
		ErrProtocolTooOld, d.serialLocked(), have, opcode, need)
}

func (d *Device) configInitialised() bool {
	return d.hello.features.Has(FeatureConfigInitialised)
}

func (d *Device) supportsVideoWall() bool {
	return d.hello.features.Has(FeatureVideoWall)
}

func (d *Device) hasLocalSource() bool {
	in := d.firstInputLocked()
	return in != nil && in.isLocal()
}

func (d *Device) hasLocalSink() bool {
	out := d.firstOutputLocked()
	return out != nil && out.isLocal()
}

func (d *Device) isOneipTx() bool { return d.isV2IP() && d.hasLocalSource() && !d.hasLocalSink() }
func (d *Device) isOneipRx() bool { return d.isV2IP() && d.hasLocalSink() && !d.hasLocalSource() }
func (d *Device) isOneipTz() bool { return d.isV2IP() && d.hasLocalSink() && d.hasLocalSource() }

// IsV2IP reports whether this is a OneIP/V2IP device.
func (d *Device) IsV2IP() bool { return d.lockedBool(d.isV2IP) }

// IsVideoMatrix reports whether this is a video matrix.
func (d *Device) IsVideoMatrix() bool { return d.lockedBool(d.isVideoMatrix) }

// IsAudioMatrix reports whether this is an audio-only matrix.
func (d *Device) IsAudioMatrix() bool { return d.lockedBool(d.isAudioMatrix) }

// IsAmp reports whether this is an amplifier.
func (d *Device) IsAmp() bool { return d.lockedBool(d.isAmp) }

// IsOneIPMultiviewer reports whether this is a OneIP multiviewer.
func (d *Device) IsOneIPMultiviewer() bool { return d.lockedBool(d.isOneipMultiviewer) }

// ConfigInitialised reports whether this device's firmware initialises the
// configuration it broadcasts.
//
// Firmware without it builds some frames over uninitialised stack, so these
// read as noise rather than as values: the scaling flags and, behind a
// spuriously set ScalingFlagModeValid, the scaling mode and refresh; bay 0's
// source addresses in the V2IP sources frame; and the padding beside RCTarget.
// Everything from a device reporting this bit is trustworthy.
func (d *Device) ConfigInitialised() bool { return d.lockedBool(d.configInitialised) }

// SupportsVideoWall reports whether this sink can crop its source to a video
// wall window.
func (d *Device) SupportsVideoWall() bool { return d.lockedBool(d.supportsVideoWall) }

// IsOneIPTx reports whether this is a OneIP transmitter.
func (d *Device) IsOneIPTx() bool { return d.lockedBool(d.isOneipTx) }

// IsOneIPRx reports whether this is a OneIP receiver.
func (d *Device) IsOneIPRx() bool { return d.lockedBool(d.isOneipRx) }

// IsOneIPTz reports whether this is a OneIP transceiver.
func (d *Device) IsOneIPTz() bool { return d.lockedBool(d.isOneipTz) }

func (d *Device) lockedBool(fn func() bool) bool {
	d.remote.mu.Lock()
	defer d.remote.mu.Unlock()
	return fn()
}

// ---- status ----

func (d *Device) onlineLocked() bool {
	limit := 120 * time.Second
	if d.hello.supportedProtocol >= 0x20 {
		limit = 15 * time.Second
	}
	return time.Since(d.lastPing) < limit
}

func (d *Device) rebootingLocked() bool {
	if d.rebooting {
		return true
	}
	return d.onlineLocked() && d.hello.features.Has(FeatureStatusReboot)
}

func (d *Device) bootingLocked() bool {
	return d.onlineLocked() && !d.rebootingLocked() && d.hello.features.Has(FeatureBooting)
}

func (d *Device) statusLocked() DeviceStatus {
	if d.onlineLocked() {
		if d.rebootingLocked() {
			return StatusRebooting
		}
		if d.bootingLocked() {
			return StatusBooting
		}
		return StatusOnline
	}
	return StatusOffline
}

// Status returns the high-level device status.
func (d *Device) Status() DeviceStatus {
	d.remote.mu.Lock()
	defer d.remote.mu.Unlock()
	return d.statusLocked()
}

// Online reports whether the device has pinged recently.
func (d *Device) Online() bool { return d.lockedBool(d.onlineLocked) }

// ---- bays ----

func (d *Device) sortedPorts() []int {
	ports := make([]int, 0, len(d.bays))
	for p := range d.bays {
		ports = append(ports, p)
	}
	sort.Ints(ports)
	return ports
}

// Bays returns all bays keyed by port number.
func (d *Device) Bays() map[int]*Bay {
	d.remote.mu.Lock()
	defer d.remote.mu.Unlock()
	out := make(map[int]*Bay, len(d.bays))
	for k, v := range d.bays {
		out[k] = v
	}
	return out
}

func (d *Device) getByPortnumLocked(portnum int) *Bay { return d.bays[portnum] }

func (d *Device) getByPortnameLocked(name string) *Bay {
	for _, p := range d.sortedPorts() {
		if d.bays[p].portName == name {
			return d.bays[p]
		}
	}
	return nil
}

func (d *Device) getByModeBayLocked(mode string, bay int) *Bay {
	for _, p := range d.sortedPorts() {
		b := d.bays[p]
		if b.modeStr() == mode && b.bayNum() == bay {
			return b
		}
	}
	return nil
}

// GetByPortnum returns the bay with the given port number, or nil.
func (d *Device) GetByPortnum(portnum int) *Bay {
	d.remote.mu.Lock()
	defer d.remote.mu.Unlock()
	return d.getByPortnumLocked(portnum)
}

// GetByPortname returns the bay with the given port name, or nil.
func (d *Device) GetByPortname(name string) *Bay {
	d.remote.mu.Lock()
	defer d.remote.mu.Unlock()
	return d.getByPortnameLocked(name)
}

func (d *Device) bayByUserNameLocked(name string) *Bay {
	for _, b := range d.inputsLocked() {
		if b.userNameLocked() == name {
			return b
		}
	}
	return nil
}

func (d *Device) firstInputLocked() *Bay {
	for _, p := range d.sortedPorts() {
		if d.bays[p].isInput() {
			return d.bays[p]
		}
	}
	return nil
}

func (d *Device) firstOutputLocked() *Bay {
	for _, p := range d.sortedPorts() {
		if d.bays[p].isOutput() {
			return d.bays[p]
		}
	}
	return nil
}

// FirstInput returns the first input bay, or nil.
func (d *Device) FirstInput() *Bay {
	d.remote.mu.Lock()
	defer d.remote.mu.Unlock()
	return d.firstInputLocked()
}

// FirstOutput returns the first output bay, or nil.
func (d *Device) FirstOutput() *Bay {
	d.remote.mu.Lock()
	defer d.remote.mu.Unlock()
	return d.firstOutputLocked()
}

func (d *Device) inputsLocked() []*Bay {
	var rv []*Bay
	for _, p := range d.sortedPorts() {
		b := d.bays[p]
		if b.isInput() && !(b.hidden != nil && *b.hidden) {
			rv = append(rv, b)
		}
	}
	return rv
}

func (d *Device) outputsLocked() []*Bay {
	var rv []*Bay
	for _, p := range d.sortedPorts() {
		b := d.bays[p]
		if b.isOutput() {
			rv = append(rv, b)
		}
	}
	return rv
}

// Inputs returns all visible input bays, ordered by port number.
func (d *Device) Inputs() []*Bay {
	d.remote.mu.Lock()
	defer d.remote.mu.Unlock()
	return d.inputsLocked()
}

// Outputs returns all output bays, ordered by port number.
func (d *Device) Outputs() []*Bay {
	d.remote.mu.Lock()
	defer d.remote.mu.Unlock()
	return d.outputsLocked()
}

// V2IPSources returns the V2IP stream sources advertised by this device, or nil.
func (d *Device) V2IPSources() []V2IPStreamSources {
	d.remote.mu.Lock()
	defer d.remote.mu.Unlock()
	if d.v2ipSources == nil {
		return nil
	}
	return append([]V2IPStreamSources(nil), d.v2ipSources...)
}

// V2IPDetails returns the device's V2IP encoder/decoder configuration, or nil.
func (d *Device) V2IPDetails() *DeviceV2IPDetails {
	d.remote.mu.Lock()
	defer d.remote.mu.Unlock()
	if d.v2ipDetails == nil {
		return nil
	}
	v := *d.v2ipDetails
	return &v
}

// V2IPSink returns the device's current sink subscription, or nil.
func (d *Device) V2IPSink() *DeviceV2IPSink {
	d.remote.mu.Lock()
	defer d.remote.mu.Unlock()
	if d.v2ipSink == nil {
		return nil
	}
	v := *d.v2ipSink
	return &v
}

func (d *Device) v2ipSourceForLocked(bay *Bay) *V2IPStreamSources {
	if !bay.isInput() || !d.isV2IP() || d.v2ipSources == nil {
		return nil
	}
	offset := 0
	if !d.hasLocalSource() {
		offset = 1
	}
	idx := bay.bayNum() - offset
	if idx < 0 || idx >= len(d.v2ipSources) {
		return nil
	}
	v := d.v2ipSources[idx]
	return &v
}

func (d *Device) hasBaysLocked() bool {
	nbIn := len(d.inputsLocked())
	nbOut := len(d.outputsLocked())
	return len(d.bays) >= (nbIn + nbOut)
}

func (d *Device) needLinkConfigLocked() bool {
	if d.isAmp() || d.isVideoMatrix() || d.isAudioMatrix() || d.isV2IP() {
		return !d.linkConfigReceived
	}
	return false
}

func (d *Device) configurationCompleteLocked() bool {
	if !d.hasBaysLocked() {
		return false
	}
	if d.isV2IP() && d.v2ipSources == nil {
		return false
	}
	return !d.needLinkConfigLocked()
}

// ConfigurationComplete reports whether all configuration has been received.
func (d *Device) ConfigurationComplete() bool {
	d.remote.mu.Lock()
	defer d.remote.mu.Unlock()
	return d.configurationCompleteLocked()
}

// ModelName returns a friendly model name.
func (d *Device) ModelName() string {
	d.remote.mu.Lock()
	defer d.remote.mu.Unlock()
	return d.modelNameLocked()
}

func (d *Device) modelNameLocked() string {
	if d.isV2IP() {
		switch {
		case d.isOneipMultiviewer():
			return "OneIP Multiviewer"
		case d.hasLocalSource() && d.hasLocalSink():
			return "OneIP Transceiver"
		case d.hasLocalSource():
			return "OneIP Transmitter"
		default:
			return "OneIP Receiver"
		}
	}
	models := map[string]string{
		"PROAMP8": "ProAmp8", "PROAMPv2": "ProAmp8 v2",
		"FFMB44": "neo:4 Bronze", "FFMS44": "neo:4 Silver", "FFMG44": "neo:4 Gold",
		"FF88SA": "neo:X", "FF88S": "neo:X", "FF88T": "neo:X", "FF88": "neo:8",
		"FF88A": "neo:8 Audio", "FF88A1": "neo:8 Audio",
		"FF66SA": "neo:6 X", "FF66A": "neo:6 Audio", "FF66A1": "neo:6 Audio",
		"FF64S": "neo:6", "SP14": "neo:4 Splitter", "SP142": "neo:4 Splitter",
	}
	if m, ok := models[d.hello.name]; ok {
		return m
	}
	return d.hello.name
}

func (d *Device) nbHdbtLocked() int {
	n := d.hello.name
	switch {
	case strings.HasPrefix(n, "FF88"):
		return 8
	case strings.HasPrefix(n, "FF66"):
		return 6
	case strings.HasPrefix(n, "FF64"):
		return 4
	case n == "FFMB44" || n == "FFMS44" || n == "FFMG44" || strings.HasPrefix(n, "SP14"):
		return 4
	}
	return 0
}

// Temperatures returns the named temperature sensor readings.
func (d *Device) Temperatures() map[string]int {
	d.remote.mu.Lock()
	defer d.remote.mu.Unlock()
	rv := map[string]int{}
	if d.isV2IP() {
		get := func(i int) int {
			if i < len(d.temperatures) {
				return d.temperatures[i]
			}
			return -1
		}
		rv["System"] = get(0)
		rv["FPGA"] = get(1)
		rv["Switch"] = get(2)
		return rv
	}
	for i, t := range d.temperatures {
		rv[fmt.Sprintf("Sensor %d", i+1)] = t
	}
	return rv
}

// NetworkStatus returns the network port status keyed by port id.
func (d *Device) NetworkStatus() map[int]NetworkPortStatus {
	d.remote.mu.Lock()
	defer d.remote.mu.Unlock()
	out := make(map[int]NetworkPortStatus, len(d.network))
	for k, v := range d.network {
		out[k] = v
	}
	return out
}

// MACAddress returns the first non-zero MAC address reported by a network port,
// or "".
func (d *Device) MACAddress() string {
	d.remote.mu.Lock()
	defer d.remote.mu.Unlock()
	for _, p := range d.sortedNetworkPorts() {
		mac := d.network[p].MACAddress
		if mac != "" && mac != "00:00:00:00:00:00" {
			return mac
		}
	}
	return ""
}

func (d *Device) sortedNetworkPorts() []int {
	ports := make([]int, 0, len(d.network))
	for p := range d.network {
		ports = append(ports, p)
	}
	sort.Ints(ports)
	return ports
}

func (d *Device) updateNetworkStatus(s NetworkPortStatus) {
	d.network[s.Port] = s
	d.emitSelf()
}

// V2IPStats returns the most recent V2IP statistics, or nil.
func (d *Device) V2IPStats() *V2IPDeviceStats {
	d.remote.mu.Lock()
	defer d.remote.mu.Unlock()
	if d.v2ipStats == nil {
		return nil
	}
	v := *d.v2ipStats
	return &v
}

func (d *Device) setV2IPStats(s V2IPDeviceStats) {
	d.v2ipStats = &s
	d.emitSelf()
}

// DolbySettings returns the amplifier Dolby settings, or nil.
func (d *Device) DolbySettings() *AmpDolbySettings {
	d.remote.mu.Lock()
	defer d.remote.mu.Unlock()
	if d.dolbySettings == nil {
		return nil
	}
	v := *d.dolbySettings
	return &v
}

func (d *Device) setDolbySettings(s AmpDolbySettings) {
	changed := d.dolbySettings == nil || *d.dolbySettings != s
	d.dolbySettings = &s
	if changed {
		r := d.remote
		d.notify(pick(r.callbacks.OnAmpDolbySettingsChanged != nil, func() { r.callbacks.OnAmpDolbySettingsChanged(d, s) }))
	}
}

// FirmwareVersions returns the V2IP firmware components keyed by type.
func (d *Device) FirmwareVersions() map[FirmwareType]FirmwareVersion {
	d.remote.mu.Lock()
	defer d.remote.mu.Unlock()
	out := make(map[FirmwareType]FirmwareVersion, len(d.v2ipVersions))
	for k, v := range d.v2ipVersions {
		out[k] = v
	}
	return out
}

func (d *Device) setFirmwareVersion(fv FirmwareVersion) {
	if cur, ok := d.v2ipVersions[fv.Type]; !ok || cur != fv {
		d.v2ipVersions[fv.Type] = fv
		d.emitSelf()
	}
}

// SystemStatus returns the device's numeric status code and message. ok is
// false when no system-status report has been received.
func (d *Device) SystemStatus() (status int, message string, ok bool) {
	d.remote.mu.Lock()
	defer d.remote.mu.Unlock()
	if d.sysStatus == nil {
		return 0, "", false
	}
	msg := ""
	if d.sysMessage != nil {
		msg = *d.sysMessage
	}
	return *d.sysStatus, msg, true
}

func (d *Device) setSystemStatus(status int, message string) {
	changed := d.sysStatus == nil || *d.sysStatus != status ||
		d.sysMessage == nil || *d.sysMessage != message
	d.sysStatus = &status
	d.sysMessage = &message
	if changed {
		d.emitSelf()
	}
}

// StatusMessage returns a human-readable health summary combining the device
// status, crash flag, and any system-status message.
func (d *Device) StatusMessage() string {
	d.remote.mu.Lock()
	defer d.remote.mu.Unlock()
	con := d.statusLocked()
	crashed := d.hello.features.Has(FeatureStatusCrashed)
	switch con {
	case StatusOffline, StatusRebooting, StatusInactive:
		return con.String()
	}
	if con == StatusOnline && !crashed {
		return "Healthy"
	}
	if con != StatusOnline && d.sysMessage == nil {
		return con.String()
	}
	msg := "Healthy"
	if crashed {
		msg = "Crashed Recently"
	}
	if d.sysMessage != nil {
		msg = *d.sysMessage
	}
	return fmt.Sprintf("%s - %s", con, msg)
}

// ---- mutation (called with remote lock held) ----

func (d *Device) applyHello(h helloInfo) {
	d.lastPing = time.Now()
	changed := !d.hello.equal(h)
	d.hello = h
	d.rebooting = false
	if changed {
		r := d.remote
		d.notify(pick(r.callbacks.OnDeviceConfigChanged != nil, func() { r.callbacks.OnDeviceConfigChanged(d) }))
	}
}

func (d *Device) checkConfigComplete() {
	if d.configurationCompleteLocked() && !d.haveConfig {
		d.haveConfig = true
		r := d.remote
		d.notify(pick(r.callbacks.OnDeviceConfigComplete != nil, func() { r.callbacks.OnDeviceConfigComplete(d) }))
	}
}

func (d *Device) applyBayConfig(cfg bayConfig) {
	d.lastPing = time.Now()
	bay := d.getByPortnumLocked(cfg.port)
	isNew := bay == nil
	if isNew {
		bay = newBay(d, cfg)
		d.bays[cfg.port] = bay
	}
	bay.applyBayConfig(cfg)
	if isNew {
		r := d.remote
		bay.notify(pick(r.callbacks.OnBayRegistered != nil, func() { r.callbacks.OnBayRegistered(bay) }))
		d.checkConfigComplete()
		d.emitSelf()
	}
}

func (d *Device) onLinkConfigReceived() {
	d.linkConfigReceived = true
	d.checkConfigComplete()
}

// PDUState returns the last PDU state this device reported, or nil.
func (d *Device) PDUState() *PDUState {
	d.remote.mu.Lock()
	defer d.remote.mu.Unlock()
	if d.pduState == nil {
		return nil
	}
	v := *d.pduState
	return &v
}

func (d *Device) setPDUState(st PDUState) {
	changed := d.pduState == nil || *d.pduState != st
	d.pduState = &st
	if changed {
		r := d.remote
		d.notify(pick(r.callbacks.OnPDUStateChanged != nil, func() { r.callbacks.OnPDUStateChanged(d, st) }))
	}
}

// SetupCompleted reports whether the mesh has marked this device set up. The
// second return is false until a setup-status frame has arrived.
func (d *Device) SetupCompleted() (bool, bool) {
	d.remote.mu.Lock()
	defer d.remote.mu.Unlock()
	if d.setupDone == nil {
		return false, false
	}
	return *d.setupDone, true
}

func (d *Device) setSetupCompleted(done bool) {
	changed := d.setupDone == nil || *d.setupDone != done
	d.setupDone = &done
	if changed {
		r := d.remote
		d.notify(pick(r.callbacks.OnSetupStatusChanged != nil, func() { r.callbacks.OnSetupStatusChanged(d, done) }))
	}
}

// InstallerID returns the installer id the mesh assigned, or nil.
func (d *Device) InstallerID() *uint16 {
	d.remote.mu.Lock()
	defer d.remote.mu.Unlock()
	if d.installerID == nil {
		return nil
	}
	v := *d.installerID
	return &v
}

func (d *Device) setInstallerID(id uint16) {
	changed := d.installerID == nil || *d.installerID != id
	d.installerID = &id
	if changed {
		r := d.remote
		d.notify(pick(r.callbacks.OnInstallerIDChanged != nil, func() { r.callbacks.OnInstallerIDChanged(d, id) }))
	}
}

// Tiling returns the window this sink is currently told to show, or nil.
func (d *Device) Tiling() *V2IPTilingConfig {
	d.remote.mu.Lock()
	defer d.remote.mu.Unlock()
	if d.tiling == nil {
		return nil
	}
	v := *d.tiling
	return &v
}

func (d *Device) setTiling(cfg V2IPTilingConfig) {
	changed := d.tiling == nil || *d.tiling != cfg
	d.tiling = &cfg
	if changed {
		r := d.remote
		d.notify(pick(r.callbacks.OnTilingChanged != nil, func() { r.callbacks.OnTilingChanged(d, cfg) }))
	}
}

// RCSettings returns this device's remote-control configuration, or nil.
func (d *Device) RCSettings() *RCSettings {
	d.remote.mu.Lock()
	defer d.remote.mu.Unlock()
	if d.rcSettings == nil {
		return nil
	}
	v := *d.rcSettings
	return &v
}

func (d *Device) setRCSettings(s RCSettings) {
	changed := d.rcSettings == nil || *d.rcSettings != s
	d.rcSettings = &s
	if changed {
		r := d.remote
		d.notify(pick(r.callbacks.OnRCSettingsChanged != nil, func() { r.callbacks.OnRCSettingsChanged(d, s) }))
	}
}

// AudioSourceSelection returns the last audio input-selection change this
// device reported, or nil. AudioSelectInput is the command that requests one.
func (d *Device) AudioSourceSelection() *AudioChangeSource {
	d.remote.mu.Lock()
	defer d.remote.mu.Unlock()
	if d.audioSelect == nil {
		return nil
	}
	v := *d.audioSelect
	return &v
}

func (d *Device) setAudioSelectInput(ch AudioChangeSource) {
	changed := d.audioSelect == nil || *d.audioSelect != ch
	d.audioSelect = &ch
	if changed {
		r := d.remote
		d.notify(pick(r.callbacks.OnAudioSelectInput != nil, func() { r.callbacks.OnAudioSelectInput(d, ch) }))
	}
}

// emitAudioParam fires the callback for a per-endpoint audio notification.
// Mute and trigger carry a boolean in the low bit of the same u32 that volume
// uses for a level.
func (d *Device) emitAudioParam(op uint16, endpoint int, value uint32) {
	r := d.remote
	switch op {
	case audioOpMute:
		if r.callbacks.OnAudioEndpointMute != nil {
			r.emit(func() { r.callbacks.OnAudioEndpointMute(d, endpoint, value != 0) })
		}
	case audioOpTrigger:
		if r.callbacks.OnAudioEndpointTrigger != nil {
			r.emit(func() { r.callbacks.OnAudioEndpointTrigger(d, endpoint, value != 0) })
		}
	case audioOpVolume:
		if r.callbacks.OnAudioEndpointVolume != nil {
			r.emit(func() { r.callbacks.OnAudioEndpointVolume(d, endpoint, value) })
		}
	}
	d.emitSelf()
}

func (d *Device) setV2IPSources(sources []V2IPStreamSources) {
	d.v2ipSources = sources
	d.emitSelf()
}

func (d *Device) setTemperatures(t []int) {
	changed := len(t) != len(d.temperatures)
	if !changed {
		for i := range t {
			if t[i] != d.temperatures[i] {
				changed = true
				break
			}
		}
	}
	d.temperatures = t
	if changed {
		r := d.remote
		d.notify(pick(r.callbacks.OnDeviceTemperatureChanged != nil, func() { r.callbacks.OnDeviceTemperatureChanged(d) }))
	}
}

// touch refreshes the device's last-seen time and fires an online transition if
// the device was previously considered offline.
func (d *Device) touch(ts time.Time) {
	d.lastPing = ts
	d.checkOnline()
}

// checkOnline fires the online-status-changed callback on a transition.
func (d *Device) checkOnline() {
	cur := d.onlineLocked()
	if cur != d.online {
		d.online = cur
		if !cur {
			d.haveConfig = false
		}
		r := d.remote
		d.notify(pick(r.callbacks.OnDeviceOnlineStatusChanged != nil, func() { r.callbacks.OnDeviceOnlineStatusChanged(d, cur) }))
	}
}

// IsMeshMaster reports whether this device is the mesh controller.
func (d *Device) IsMeshMaster() bool {
	return d.lockedBool(func() bool { return d.hello.features.Has(FeatureMeshMaster) })
}

// IsMeshMember reports whether this device is a mesh member.
func (d *Device) IsMeshMember() bool {
	return d.lockedBool(func() bool { return d.hello.features.Has(FeatureMeshMember) })
}

// MeshMaster returns the mesh controller device. For non-V2IP devices or when
// the master is unknown it returns the device itself.
func (d *Device) MeshMaster() *Device {
	d.remote.mu.Lock()
	defer d.remote.mu.Unlock()
	if !d.isV2IP() || d.meshMasterUID.Empty() {
		return d
	}
	if m := d.remote.devices[d.meshMasterUID]; m != nil {
		return m
	}
	return d
}

func (d *Device) setMeshMaster(uid DeviceUID) {
	if d.meshMasterUID != uid {
		d.meshMasterUID = uid
		d.emitSelf()
	}
}

// AmpDolbyChannels returns the number of Dolby-capable channels on an amplifier.
func (d *Device) AmpDolbyChannels() int {
	d.remote.mu.Lock()
	defer d.remote.mu.Unlock()
	return d.ampDolbyChannelsLocked()
}

func (d *Device) ampDolbyChannelsLocked() int {
	n := 0
	for _, p := range d.sortedPorts() {
		if d.bays[p].dolbyInputLocked() != "" {
			n++
		}
	}
	return n
}

// Topology returns the most recent topology report from this device.
func (d *Device) Topology() []TopologyEntry {
	d.remote.mu.Lock()
	defer d.remote.mu.Unlock()
	return append([]TopologyEntry(nil), d.topology...)
}

func (d *Device) setTopology(t []TopologyEntry) {
	if !topologyEqual(d.topology, t) {
		d.topology = t
		d.emitSelf()
	}
}

func topologyEqual(a, b []TopologyEntry) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// AudioEndpoints returns the device's audio endpoint collection, or nil.
func (d *Device) AudioEndpoints() *AudioEndpoints {
	d.remote.mu.Lock()
	defer d.remote.mu.Unlock()
	return d.audio
}

// AudioEndpointByID returns the audio endpoint with the given id, or nil.
func (d *Device) AudioEndpointByID(id int) *AudioEndpoint {
	d.remote.mu.Lock()
	defer d.remote.mu.Unlock()
	return d.audio.Get(id)
}

func (d *Device) setAudioEndpoints(eps *AudioEndpoints) {
	if d.audio.equal(eps) {
		d.audio = eps
		return
	}
	d.audio = eps
	switch {
	case d.isOneipTz() || d.isOneipTx():
		if in := d.firstInputLocked(); in != nil {
			in.setAudioEndpoint(eps.treeFirstOutput())
		}
		if out := d.firstOutputLocked(); out != nil {
			out.setAudioEndpoint(eps.treeFirstInput())
		}
	case d.isAmp():
		for _, ep := range eps.List() {
			var bay *Bay
			if ep.ID < 10 {
				bay = d.getByModeBayLocked("Input", ep.ID)
			} else {
				bay = d.getByModeBayLocked("Output", ep.ID-10)
			}
			if bay != nil {
				bay.setAudioEndpoint(ep)
			}
		}
	}
	d.emitSelf()
}

func (d *Device) applyAudioLinks(links []AudioLink) {
	if d.audio == nil {
		return
	}
	for _, l := range links {
		if ep := d.audio.Get(l.Endpoint); ep != nil {
			ep.linkedUID = l.LinkDevice
			le := l.LinkEndpoint
			ep.linkedEP = &le
		}
	}
}

// V2IPSourceLocal returns the V2IP stream sources of this device's local input,
// or nil.
func (d *Device) V2IPSourceLocal() *V2IPStreamSources {
	d.remote.mu.Lock()
	defer d.remote.mu.Unlock()
	in := d.firstInputLocked()
	if in == nil {
		return nil
	}
	return d.v2ipSourceForLocked(in)
}

func (d *Device) String() string {
	return fmt.Sprintf("(%s %s)", d.serialLocked(), d.nameLocked())
}
