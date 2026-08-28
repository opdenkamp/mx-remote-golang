// Author: Lars Op den Kamp (lars@opdenkamp-it.nl)
// Copyright (c) 2026 Op den Kamp IT Solutions

package mxremote

import (
	"encoding/binary"
	"fmt"
	"math"
	"net"
)

// Payloads of the command and notification opcodes.
//
// These frames are addressed to a device rather than reporting its state, so
// most surface as callbacks rather than cached state. A frame addressed to
// another unit still reaches every client on the group: the Target field says
// who it was for, and it is not necessarily this client or the sender.

// SetRouteRequest asks a device, addressed by serial, to switch a sink.
type SetRouteRequest struct {
	Serial    string
	SinkBay   int
	SourceBay int
	NoPowerOn bool
	AudioOnly bool // true when this arrived on AUDIO_SET_ROUTE rather than MX_SET_ROUTE
}

func (r SetRouteRequest) String() string {
	return fmt.Sprintf("set route on %s: sink=%d source=%d", r.Serial, r.SinkBay, r.SourceBay)
}

// EDIDRecord is one EDID block from a DEV_EDID reply. A reply carries one
// record per bay mode, so a combined reply produces two.
type EDIDRecord struct {
	// Output is true for a sink's EDID, false for a source's.
	Output bool
	// Data is a 256-byte EDID: a base block plus exactly one extension block.
	// A display publishing further extension blocks yields only the first.
	Data []byte
}

// EDIDRequest asks one device for its EDID.
type EDIDRequest struct {
	Target DeviceUID
	Output bool
}

// BayNameChange asks a device to rename one of its bays.
type BayNameChange struct {
	Target DeviceUID
	Port   int
	Name   string
}

// EDIDProfileChange asks a device to switch its input EDID profile.
type EDIDProfileChange struct {
	Target  DeviceUID
	Profile EdidProfile
}

// FactoryResetRequest asks peers to factory-reset.
type FactoryResetRequest struct {
	// All is set by the 0xFF broadcast form, which targets every peer.
	All bool
	// Target is set by the single-uid form, and nil otherwise. With neither
	// All nor Target the request addresses only the sender.
	Target *DeviceUID
}

// RebootRequest asks one device to reboot.
type RebootRequest struct {
	Target DeviceUID
}

// V2IPTilingConfig is the window a sink is currently told to show.
//
// This is the readable, pollable view of a sink's window. It is not the
// persisted video wall setting: on a sink running the v2ipwall module a write
// here is transient, because that module's reconciler pushes its own target
// window back within about a second. VideoWallCommand is what carries intent.
type V2IPTilingConfig struct {
	Target DeviceUID
	PosX   uint16
	PosY   uint16
	Width  uint16
	Height uint16
}

func (t V2IPTilingConfig) String() string {
	return fmt.Sprintf("tiling x=%d y=%d %dx%d", t.PosX, t.PosY, t.Width, t.Height)
}

// V2IPPowerSaveRequest asks a sink to enter or leave power save.
type V2IPPowerSaveRequest struct {
	// Target is nil on the broadcast form, which addresses every peer.
	Target  *DeviceUID
	Enabled bool
}

// RCSettings is the remote-control configuration of a source bay.
type RCSettings struct {
	Target DeviceUID
	// RCTarget selects the control method (rc_target_t). It is a single byte:
	// the enum is plain and Cortex-M builds with -fshort-enums, so three bytes
	// of padding follow it before IP. That padding is not zero - firmware
	// memcpys an uncleared stack local over the payload - so widening this to a
	// u32 makes one unchanged setting decode differently on every frame.
	RCTarget uint8
	// IP is the control target's address, empty when unset.
	IP         string
	CECEnabled bool
	CECAutoOn  bool
	ForwardRC  bool
	ForwardIR  bool
	// RCStatus is the driver state on the source (mxr_rc_status_t). A value
	// above the last one this library knows is passed through as-is rather than
	// clamped, so a firmware update cannot make it read as a known state.
	RCStatus uint8
	// StatusName is the driver-reported status string ("Sky", "Detecting"),
	// empty when unknown.
	StatusName string
}

// IRMeta is the raw-IR metadata shared by the IR capture and transmit frames.
type IRMeta struct {
	TimerResolution uint16
	Frequency       uint16
	NbTimings       uint16
	RepeatOffset    uint16
	Status          uint8
}

// IRCapture reports raw IR captured on a bay of the sending device.
type IRCapture struct {
	Port       int
	Timestamp  uint32
	LastChange uint32
	Meta       IRMeta
	// Timings is the raw on/off timing blob following the header.
	Timings []byte
}

// IRTransmitRequest asks one device to blast raw IR on one of its local bays.
type IRTransmitRequest struct {
	Target DeviceUID
	// LocalMode and LocalBay are in the target's own bay numbering, not a port.
	LocalMode uint8
	LocalBay  uint8
	Timestamp uint32
	Meta      IRMeta
	Timings   []byte
}

// KeyTransmitRequest asks one device to send a remote-control key on a bay.
type KeyTransmitRequest struct {
	Target   DeviceUID
	LocalBay int
	Key      RCKey
}

// ActionTransmitRequest asks one device to perform a remote-control action.
type ActionTransmitRequest struct {
	Target   DeviceUID
	LocalBay int
	Action   RCAction
}

// AudioClip reports that a bay detected audio clipping.
type AudioClip struct {
	Port int
	Clip uint8
}

// PDUState is the electrical state a PDU reports.
type PDUState struct {
	Current     float64
	Voltage     float64
	Power       float64
	Dissipation float64
	Frequency   float64
	Outlets     [8]uint8
}

func (p PDUState) String() string {
	return fmt.Sprintf("%.2fA %.2fV %.2fW", p.Current, p.Voltage, p.Power)
}

// V2IPBlacklistChange registers or unregisters a device on the source blacklist.
//
// The firmware guards this opcode behind V2IP_SUPPORT_BLACKLIST, which is 0 in
// shipping builds, so nothing in current firmware emits it.
type V2IPBlacklistChange struct {
	Target     DeviceUID
	Registered bool
}

// VideoWallOp is what a VideoWallCommand asks the sink to do with the window.
type VideoWallOp uint8

const (
	// VideoWallPreview applies the window without persisting it.
	VideoWallPreview VideoWallOp = 0
	// VideoWallStore persists the window as the sink's wall setting.
	VideoWallStore VideoWallOp = 1
	// VideoWallRevert restores the persisted setting and carries no window.
	VideoWallRevert VideoWallOp = 2
)

func (o VideoWallOp) String() string {
	switch o {
	case VideoWallPreview:
		return "preview"
	case VideoWallStore:
		return "store"
	case VideoWallRevert:
		return "revert"
	}
	return "unknown"
}

// VideoWallCommand asks one sink to crop its source to a wall window.
//
// This replaces the sink's window outright — unlike a V2IP device config, no
// field carries a validity marker, and a zero Width or Height is the wire
// spelling of "clear the wall and show the full frame" rather than "unset".
//
// The opcode belongs to the loadable v2ipwall module rather than MatrixOS, and
// a wall has no object of its own on the wire: it is a set of sinks each
// holding one rectangle, one frame each. It is a command with no reply, so
// nothing here is ever a status readback.
type VideoWallCommand struct {
	Target DeviceUID
	PosX   uint16
	PosY   uint16
	Width  uint16
	Height uint16
	// RasterW and RasterH are the active pixel dimensions the window was
	// authored against. They travel with the window because only the sender
	// knows what the installer drew against; a sink deriving them from what it
	// happens to be showing would store the window against the wrong picture.
	RasterW uint16
	RasterH uint16
	Op      VideoWallOp
}

// HasWindow reports whether the geometry in this command is meaningful.
//
// A revert zeroes the window and raster and the receiver ignores those bytes,
// so its zeros are not a clear.
func (v VideoWallCommand) HasWindow() bool { return v.Op != VideoWallRevert }

// Cleared reports a command that clears the wall and shows the full frame.
func (v VideoWallCommand) Cleared() bool {
	return v.HasWindow() && (v.Width == 0 || v.Height == 0)
}

func (v VideoWallCommand) String() string {
	if !v.HasWindow() {
		return "video wall revert"
	}
	if v.Cleared() {
		return fmt.Sprintf("video wall %s: clear", v.Op)
	}
	return fmt.Sprintf("video wall %s: %dx%d+%d+%d of %dx%d",
		v.Op, v.Width, v.Height, v.PosX, v.PosY, v.RasterW, v.RasterH)
}

// ---- receive handlers ----

func handleDiscoverRequest(r *Remote, f *frame) {
	dev := r.deviceFor(f)
	if dev == nil || r.callbacks.OnDiscoverRequest == nil {
		return
	}
	cb := r.callbacks.OnDiscoverRequest
	r.emit(func() { cb(dev) })
}

// handleEDID decodes a DEV_EDID frame. A 17-byte payload is a request; a
// 257-byte payload is one record, and 514 is two records concatenated — so the
// mode byte leads each record rather than one mode covering both.
func handleEDID(r *Remote, f *frame) {
	dev := r.deviceFor(f)
	if dev == nil {
		return
	}
	p := f.payload()
	const edidSize = 256
	switch len(p) {
	case 16 + 1:
		if r.callbacks.OnEDIDRequested == nil {
			return
		}
		var uid DeviceUID
		copy(uid[:], p[0:16])
		req := EDIDRequest{Target: uid, Output: p[16] != 0}
		cb := r.callbacks.OnEDIDRequested
		r.emit(func() { cb(dev, req) })
	case edidSize + 1, 2 * (edidSize + 1):
		if r.callbacks.OnEDIDReceived == nil {
			return
		}
		cb := r.callbacks.OnEDIDReceived
		for off := 0; off+edidSize+1 <= len(p); off += edidSize + 1 {
			rec := EDIDRecord{Output: p[off] != 0, Data: append([]byte(nil), p[off+1:off+1+edidSize]...)}
			r.emit(func() { cb(dev, rec) })
		}
	}
}

// routingRequest decodes mxr_routing_change_request. mbay_port_id is a u16, so
// the bays are two bytes each and no_power_on follows at 20.
func routingRequest(p []byte, audioOnly bool) (SetRouteRequest, bool) {
	need := 20
	if !audioOnly {
		need = 21
	}
	if len(p) < need {
		return SetRouteRequest{}, false
	}
	req := SetRouteRequest{
		Serial:    cstr(p[0:16]),
		SinkBay:   int(binary.LittleEndian.Uint16(p[16:18])),
		SourceBay: int(binary.LittleEndian.Uint16(p[18:20])),
		AudioOnly: audioOnly,
	}
	if !audioOnly {
		req.NoPowerOn = p[20] != 0
	}
	return req, true
}

func handleSetRoute(r *Remote, f *frame) {
	dev := r.deviceFor(f)
	req, ok := routingRequest(f.payload(), false)
	if dev == nil || !ok || r.callbacks.OnSetRouteRequested == nil {
		return
	}
	cb := r.callbacks.OnSetRouteRequested
	r.emit(func() { cb(dev, req) })
}

func handleAudioSetRoute(r *Remote, f *frame) {
	dev := r.deviceFor(f)
	req, ok := routingRequest(f.payload(), true)
	if dev == nil || !ok || r.callbacks.OnSetRouteRequested == nil {
		return
	}
	cb := r.callbacks.OnSetRouteRequested
	r.emit(func() { cb(dev, req) })
}

// handleIRCapture decodes mxr_ir_data. The struct is not packed, so the u32
// timestamp is aligned to 4 and the port's two bytes are followed by padding.
func handleIRCapture(r *Remote, f *frame) {
	dev := r.deviceFor(f)
	if dev == nil {
		return
	}
	p := f.payload()
	const irDataSize = 24
	if len(p) < irDataSize || f.protocol() < 0x19 {
		return
	}
	cap := IRCapture{
		Port:       int(binary.LittleEndian.Uint16(p[0:2])),
		Timestamp:  binary.LittleEndian.Uint32(p[4:8]),
		LastChange: binary.LittleEndian.Uint32(p[8:12]),
		Meta:       irMeta(p[12:21]),
		Timings:    append([]byte(nil), p[irDataSize:]...),
	}
	bay := dev.getByPortnumLocked(cap.Port)
	if bay == nil || r.callbacks.OnIRCaptured == nil {
		return
	}
	cb := r.callbacks.OnIRCaptured
	r.emit(func() { cb(bay, cap) })
}

func irMeta(p []byte) IRMeta {
	if len(p) < 9 {
		return IRMeta{}
	}
	return IRMeta{
		TimerResolution: binary.LittleEndian.Uint16(p[0:2]),
		Frequency:       binary.LittleEndian.Uint16(p[2:4]),
		NbTimings:       binary.LittleEndian.Uint16(p[4:6]),
		RepeatOffset:    binary.LittleEndian.Uint16(p[6:8]),
		Status:          p[8],
	}
}

// handleIRTransmit decodes mxr_tx_ir_data: the u32 timestamp is aligned to 4,
// so two padding bytes follow local_bay.
func handleIRTransmit(r *Remote, f *frame) {
	dev := r.deviceFor(f)
	if dev == nil {
		return
	}
	p := f.payload()
	// mxr_tx_ir_data is not packed: two bytes pad before the u32 timestamp, and
	// the struct's own 4-byte alignment pads the 9-byte meta block out to 36.
	// The firmware appends the timings at sizeof, so they start at 36 - taking
	// them from the end of the last field shifts every u16 timing by two bytes.
	const txIRHeader = 36
	if len(p) < txIRHeader || r.callbacks.OnIRTransmitRequested == nil {
		return
	}
	var uid DeviceUID
	copy(uid[:], p[0:16])
	req := IRTransmitRequest{
		Target:    uid,
		LocalMode: p[16],
		LocalBay:  p[17],
		Timestamp: binary.LittleEndian.Uint32(p[20:24]),
		Meta:      irMeta(p[24:33]),
		Timings:   append([]byte(nil), p[txIRHeader:]...),
	}
	cb := r.callbacks.OnIRTransmitRequested
	r.emit(func() { cb(dev, req) })
}

func handleVolumeStep(up bool) func(*Remote, *frame) {
	return func(r *Remote, f *frame) {
		dev := r.deviceFor(f)
		if dev == nil {
			return
		}
		port, ok := f.u8(0)
		if !ok {
			return
		}
		bay := dev.getByPortnumLocked(int(port))
		if bay == nil || r.callbacks.OnVolumeStep == nil {
			return
		}
		cb := r.callbacks.OnVolumeStep
		r.emit(func() { cb(bay, up) })
	}
}

// handleVolumeMute decodes mxr_volume_mute_data, the notification a device
// sends when its own volume changed. AUDIO_SET_VOLUME (0x14) is the request
// form and carries a target uid; this one does not.
func handleVolumeMute(r *Remote, f *frame) {
	dev := r.deviceFor(f)
	if dev == nil {
		return
	}
	p := f.payload()
	if len(p) < 4 {
		return
	}
	bay := dev.getByPortnumLocked(int(p[0]))
	if bay == nil {
		return
	}
	vol := VolumeMuteStatus{VolumeLeft: -1, VolumeRight: -1}
	if p[1] <= 100 {
		vol.VolumeLeft = int(p[1])
	}
	if p[2] <= 100 {
		vol.VolumeRight = int(p[2])
	}
	ms := MuteStatus(p[3])
	l, rr := ms.Left(), ms.Right()
	vol.MutedLeft, vol.MutedRight = &l, &rr
	bay.setVolumeStatus(vol)
}

func handleAudioClip(r *Remote, f *frame) {
	dev := r.deviceFor(f)
	if dev == nil {
		return
	}
	p := f.payload()
	if len(p) < 2 {
		return
	}
	bay := dev.getByPortnumLocked(int(p[0]))
	if bay == nil || r.callbacks.OnAudioClip == nil {
		return
	}
	clip := AudioClip{Port: int(p[0]), Clip: p[1]}
	cb := r.callbacks.OnAudioClip
	r.emit(func() { cb(bay, clip) })
}

func handlePDUState(r *Remote, f *frame) {
	dev := r.deviceFor(f)
	if dev == nil {
		return
	}
	p := f.payload()
	if len(p) < 32 {
		return
	}
	f32 := func(off int) float64 {
		v := math.Float32frombits(binary.LittleEndian.Uint32(p[off : off+4]))
		return math.Round(float64(v)*100) / 100
	}
	st := PDUState{
		Current: f32(0), Voltage: f32(4), Power: f32(8),
		Dissipation: f32(12), Frequency: f32(20),
	}
	copy(st.Outlets[:], p[24:32])
	dev.setPDUState(st)
}

func handleV2IPLinkRemote(r *Remote, f *frame) {
	dev := r.deviceFor(f)
	if dev == nil || r.callbacks.OnV2IPLinkChanged == nil {
		return
	}
	uid, ok := f.uuid(0)
	if !ok {
		return
	}
	cb := r.callbacks.OnV2IPLinkChanged
	r.emit(func() { cb(dev, uid) })
}

func handleDetectBays(r *Remote, f *frame) {
	dev := r.deviceFor(f)
	if dev == nil || r.callbacks.OnDetectBaysRequested == nil {
		return
	}
	cb := r.callbacks.OnDetectBaysRequested
	r.emit(func() { cb(dev) })
}

// handleChangeBayName decodes mxr_bay_name_data: uid, a u16 port, then a
// fixed-width name that carries no terminator when it fills the field.
func handleChangeBayName(r *Remote, f *frame) {
	dev := r.deviceFor(f)
	if dev == nil {
		return
	}
	p := f.payload()
	if len(p) < 18+deviceNameLen || r.callbacks.OnBayNameChangeRequested == nil {
		return
	}
	var uid DeviceUID
	copy(uid[:], p[0:16])
	ch := BayNameChange{
		Target: uid,
		Port:   int(binary.LittleEndian.Uint16(p[16:18])),
		Name:   cstr(p[18 : 18+deviceNameLen]),
	}
	cb := r.callbacks.OnBayNameChangeRequested
	r.emit(func() { cb(dev, ch) })
}

func handleReboot(r *Remote, f *frame) {
	dev := r.deviceFor(f)
	if dev == nil || r.callbacks.OnRebootRequested == nil {
		return
	}
	uid, ok := f.uuid(0)
	if !ok {
		return
	}
	cb := r.callbacks.OnRebootRequested
	r.emit(func() { cb(dev, RebootRequest{Target: uid}) })
}

func handleMonitoringPulse(r *Remote, f *frame) {
	dev := r.deviceFor(f)
	if dev == nil || r.callbacks.OnMonitoringPulse == nil {
		return
	}
	cb := r.callbacks.OnMonitoringPulse
	r.emit(func() { cb(dev) })
}

func handleUpgradeFPGA(r *Remote, f *frame) {
	dev := r.deviceFor(f)
	if dev == nil || r.callbacks.OnUpgradeFPGARequested == nil {
		return
	}
	cb := r.callbacks.OnUpgradeFPGARequested
	r.emit(func() { cb(dev) })
}

func handleEDIDProfile(r *Remote, f *frame) {
	dev := r.deviceFor(f)
	if dev == nil {
		return
	}
	p := f.payload()
	if len(p) < 18 || r.callbacks.OnEDIDProfileChangeRequested == nil {
		return
	}
	var uid DeviceUID
	copy(uid[:], p[0:16])
	ch := EDIDProfileChange{Target: uid, Profile: EdidProfile(binary.LittleEndian.Uint16(p[16:18]))}
	cb := r.callbacks.OnEDIDProfileChangeRequested
	r.emit(func() { cb(dev, ch) })
}

func handleSetupStatus(r *Remote, f *frame) {
	dev := r.deviceFor(f)
	if dev == nil {
		return
	}
	v, ok := f.u8(0)
	if !ok {
		return
	}
	dev.setSetupCompleted(v == 1)
}

func handleSetInstaller(r *Remote, f *frame) {
	dev := r.deviceFor(f)
	if dev == nil {
		return
	}
	id, ok := f.u16(0)
	if !ok {
		return
	}
	dev.setInstallerID(id)
}

// handleFilterStatus decodes the list of source devices filtered out of a
// sink's picker: a target uid followed by zero or more filtered uids.
func handleFilterStatus(r *Remote, f *frame) {
	dev := r.deviceFor(f)
	if dev == nil {
		return
	}
	p := f.payload()
	if len(p) < 16 || len(p)%16 != 0 {
		return
	}
	filtered := make([]DeviceUID, 0, len(p)/16-1)
	for off := 16; off+16 <= len(p); off += 16 {
		var uid DeviceUID
		copy(uid[:], p[off:off+16])
		filtered = append(filtered, uid)
	}
	out := dev.firstOutputLocked()
	if out == nil {
		return
	}
	out.setFiltered(filtered)
}

func handleFactoryReset(r *Remote, f *frame) {
	dev := r.deviceFor(f)
	if dev == nil || r.callbacks.OnFactoryResetRequested == nil {
		return
	}
	p := f.payload()
	var req FactoryResetRequest
	switch {
	case len(p) == 1 && p[0] == 0xFF:
		req.All = true
	case len(p) == 16:
		var uid DeviceUID
		copy(uid[:], p[0:16])
		req.Target = &uid
	}
	cb := r.callbacks.OnFactoryResetRequested
	r.emit(func() { cb(dev, req) })
}

func handleV2IPTiling(r *Remote, f *frame) {
	dev := r.deviceFor(f)
	if dev == nil {
		return
	}
	p := f.payload()
	if len(p) < 24 {
		return
	}
	var uid DeviceUID
	copy(uid[:], p[0:16])
	cfg := V2IPTilingConfig{
		Target: uid,
		PosX:   binary.LittleEndian.Uint16(p[16:18]),
		PosY:   binary.LittleEndian.Uint16(p[18:20]),
		Width:  binary.LittleEndian.Uint16(p[20:22]),
		Height: binary.LittleEndian.Uint16(p[22:24]),
	}
	if target := r.devices[uid]; target != nil {
		target.setTiling(cfg)
		return
	}
	if r.callbacks.OnTilingChanged != nil {
		cb := r.callbacks.OnTilingChanged
		r.emit(func() { cb(dev, cfg) })
	}
}

// handleV2IPPowerSave decodes both forms: a bare flag broadcast to every peer,
// and a uid-addressed flag for one unit.
func handleV2IPPowerSave(r *Remote, f *frame) {
	dev := r.deviceFor(f)
	if dev == nil || r.callbacks.OnPowerSaveRequested == nil {
		return
	}
	p := f.payload()
	var req V2IPPowerSaveRequest
	switch {
	case len(p) == 1:
		req.Enabled = p[0] == 1
	case len(p) == 17:
		var uid DeviceUID
		copy(uid[:], p[0:16])
		req.Target, req.Enabled = &uid, p[16] == 1
	default:
		return
	}
	cb := r.callbacks.OnPowerSaveRequested
	r.emit(func() { cb(dev, req) })
}

// handleRCSettings decodes mxr_rc_ctrl: a target uid then mxr_rc_config, whose
// rc_target_t and ip_addr_t are four bytes each ahead of the flag bits.
//
// Every meaningful bit sits in byte 24 alone - the four flags in the low
// nibble, rc_status in the high one. Byte 25 is dead space in the same bitfield
// container and 26..27 hold the reserved bits that open a second one, which is
// what puts status_name at 28. Reading 24..26 as one little-endian u16 and
// shifting happens to agree today and stops agreeing the moment reserved is
// spent.
func handleRCSettings(r *Remote, f *frame) {
	dev := r.deviceFor(f)
	if dev == nil {
		return
	}
	p := f.payload()
	if len(p) < 28 {
		return
	}
	var uid DeviceUID
	copy(uid[:], p[0:16])
	flags := p[24]
	s := RCSettings{
		Target:     uid,
		RCTarget:   p[16],
		CECEnabled: flags&(1<<0) != 0,
		CECAutoOn:  flags&(1<<1) != 0,
		ForwardRC:  flags&(1<<2) != 0,
		ForwardIR:  flags&(1<<3) != 0,
		RCStatus:   (flags >> 4) & 0xF,
	}
	// MXR_RC_STATUS_NAME_LEN is 15 in a 16-byte array, so unlike the device
	// name fields a full-length value here does carry its terminator.
	//
	// Offset 28 is not derivable from the struct alone - both plausible
	// bitfield packings give the same sizeof - and rests on compiled offsetof
	// assertions reported from elsewhere and, since, captured frames whose
	// firmware path requires an empty name for the target they were set to -
	// two independent lines, neither of them checked here.
	// It fails quietly: two bytes out lands inside the string and yields a
	// plausible truncated name rather than garbage, so a status name that is
	// merely odd - not obviously broken - is the symptom to re-check it on.
	if len(p) >= 28+16 {
		s.StatusName = cstr(p[28 : 28+16])
	}
	if ip := net.IPv4(p[20], p[21], p[22], p[23]); !ip.IsUnspecified() {
		s.IP = ip.String()
	}
	if target := r.devices[uid]; target != nil {
		target.setRCSettings(s)
		return
	}
	if r.callbacks.OnRCSettingsChanged != nil {
		cb := r.callbacks.OnRCSettingsChanged
		r.emit(func() { cb(dev, s) })
	}
}

func handleTxKey(r *Remote, f *frame) {
	dev := r.deviceFor(f)
	if dev == nil {
		return
	}
	p := f.payload()
	if len(p) < 20 || r.callbacks.OnKeyTransmitRequested == nil {
		return
	}
	var uid DeviceUID
	copy(uid[:], p[0:16])
	req := KeyTransmitRequest{
		Target:   uid,
		LocalBay: int(binary.LittleEndian.Uint16(p[16:18])),
		Key:      RCKey(binary.LittleEndian.Uint16(p[18:20])),
	}
	cb := r.callbacks.OnKeyTransmitRequested
	r.emit(func() { cb(dev, req) })
}

func handleTxAction(r *Remote, f *frame) {
	dev := r.deviceFor(f)
	if dev == nil {
		return
	}
	p := f.payload()
	if len(p) < 20 || r.callbacks.OnActionTransmitRequested == nil {
		return
	}
	var uid DeviceUID
	copy(uid[:], p[0:16])
	req := ActionTransmitRequest{
		Target:   uid,
		LocalBay: int(binary.LittleEndian.Uint16(p[16:18])),
		Action:   RCAction(binary.LittleEndian.Uint16(p[18:20])),
	}
	cb := r.callbacks.OnActionTransmitRequested
	r.emit(func() { cb(dev, req) })
}

func handleBlacklist(registered bool) func(*Remote, *frame) {
	return func(r *Remote, f *frame) {
		dev := r.deviceFor(f)
		if dev == nil || r.callbacks.OnBlacklistChanged == nil {
			return
		}
		uid, ok := f.uuid(0)
		if !ok {
			return
		}
		cb := r.callbacks.OnBlacklistChanged
		r.emit(func() { cb(dev, V2IPBlacklistChange{Target: uid, Registered: registered}) })
	}
}

// handleVideoWall decodes vw_mesh_frame. The struct is not packed and aligns to
// 4, so three zeroed bytes trail the op byte.
//
// This layout is the one decode here not derived from a source tree: the
// v2ipwall module owns the opcode and is not vendored alongside the firmware,
// so the offsets came second-hand.
//
// A shifted geometry field is self-evident - the window lands visibly wrong.
// The op byte at 28 is the quiet one: read at the wrong offset, a store behaves
// as a preview and looks entirely correct until the sink restarts and the wall
// reverts, or a revert reads as a preview and its zeroed window clears a wall
// that should have been restored. Suspect this layout on a wall that forgets
// its setting across a reboot, not on one that is visibly misplaced.
func handleVideoWall(r *Remote, f *frame) {
	dev := r.deviceFor(f)
	if dev == nil {
		return
	}
	p := f.payload()
	if len(p) < 32 || r.callbacks.OnVideoWallCommand == nil {
		return
	}
	var uid DeviceUID
	copy(uid[:], p[0:16])
	cmd := VideoWallCommand{
		Target:  uid,
		PosX:    binary.LittleEndian.Uint16(p[16:18]),
		PosY:    binary.LittleEndian.Uint16(p[18:20]),
		Width:   binary.LittleEndian.Uint16(p[20:22]),
		Height:  binary.LittleEndian.Uint16(p[22:24]),
		RasterW: binary.LittleEndian.Uint16(p[24:26]),
		RasterH: binary.LittleEndian.Uint16(p[26:28]),
		Op:      VideoWallOp(p[28]),
	}
	cb := r.callbacks.OnVideoWallCommand
	r.emit(func() { cb(dev, cmd) })
}
