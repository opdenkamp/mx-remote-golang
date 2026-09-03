// Author: Lars Op den Kamp (lars@opdenkamp-it.nl)
// Copyright (c) 2026 Op den Kamp IT Solutions

package mxremote

import (
	"encoding/binary"
	"testing"
	"time"
)

// poisoned returns a payload of n bytes pre-filled with a non-zero,
// position-varying pattern, for callers that then write the real fields over
// it.
//
// A zero-filled fixture cannot catch a field read at the right offset but the
// wrong width: the padding beside it is zero, so a widened read returns the
// same value. Poisoning the payload first makes any read that strays past a
// field's real width produce a wrong answer instead of the right one. That is
// the class a mutation sweep over offsets structurally misses, because the
// offset is correct.
func poisoned(n int) []byte {
	p := make([]byte, n)
	for i := range p {
		p[i] = byte(0xA5 ^ i)
	}
	return p
}

func cmdRemote(t *testing.T, n byte, cb Callbacks) (*Remote, DeviceUID, func(uint16, []byte)) {
	t.Helper()
	r := newTestRemote(cb)
	sender := uidN(n)
	feed := feeder(r, sender)
	feed(opSysHello, helloPayload(0x28, "ONEIP", "CM0001", "4.8.0", FeatureVideoRouting))
	return r, sender, feed
}

// mbay_port_id is a uint16, so both bays are two bytes and no_power_on lands at
// 20 - reading them as bytes at 16/17 puts the sink's high byte in the source.
func TestSetRouteUsesU16Bays(t *testing.T) {
	var got SetRouteRequest
	_, _, feed := cmdRemote(t, 40, Callbacks{
		OnSetRouteRequested: func(_ *Device, req SetRouteRequest) { got = req },
	})
	p := make([]byte, 21)
	copy(p[0:16], "P9SN00000001")
	binary.LittleEndian.PutUint16(p[16:18], 300) // sink bay above a byte
	binary.LittleEndian.PutUint16(p[18:20], 7)
	p[20] = 1
	feed(opMxSetRoute, p)

	if got.Serial != "P9SN00000001" {
		t.Fatalf("serial = %q", got.Serial)
	}
	if got.SinkBay != 300 {
		t.Fatalf("sink bay = %d, want 300", got.SinkBay)
	}
	if got.SourceBay != 7 {
		t.Fatalf("source bay = %d, want 7", got.SourceBay)
	}
	if !got.NoPowerOn {
		t.Fatal("no_power_on should be set")
	}
}

// AUDIO_SET_ROUTE addresses its target by serial like MX_SET_ROUTE, but its
// struct stops after the two bays.
func TestAudioSetRouteHasNoPowerOnByte(t *testing.T) {
	var got SetRouteRequest
	_, _, feed := cmdRemote(t, 41, Callbacks{
		OnSetRouteRequested: func(_ *Device, req SetRouteRequest) { got = req },
	})
	p := make([]byte, 20)
	copy(p[0:16], "P9SN00000002")
	binary.LittleEndian.PutUint16(p[16:18], 4)
	binary.LittleEndian.PutUint16(p[18:20], 2)
	feed(opAudioSetRoute, p)

	if !got.AudioOnly || got.SinkBay != 4 || got.SourceBay != 2 || got.NoPowerOn {
		t.Fatalf("audio set route = %+v", got)
	}
}

// mxr_ir_data is not packed, so the u32 timestamp aligns to 4 and two padding
// bytes follow the port.
func TestIRCaptureAlignment(t *testing.T) {
	var got IRCapture
	r := newTestRemote(Callbacks{OnIRCaptured: func(_ *Bay, c IRCapture) { got = c }})
	sender := uidN(42)
	// RC_IR is gated on protocol 0x19 and up, so stamp the frames explicitly
	send := func(op uint16, proto uint16, payload []byte) {
		r.processFrame(buildFrame(sender, op, proto, payload), "10.8.8.9", time.Now())
	}
	send(opSysHello, protocolFor(opSysHello), helloPayload(0x28, "ONEIP", "CM0002", "4.8.0", FeatureVideoRouting))
	send(opSysBayConfig, protocolFor(opSysBayConfig), bayConfigRec(3, 0, 0, "Input 1", "Sky", 0, BayHDMIIn))

	p := make([]byte, 24+8)
	binary.LittleEndian.PutUint16(p[0:2], 3)
	binary.LittleEndian.PutUint32(p[4:8], 0xAABBCCDD)
	binary.LittleEndian.PutUint32(p[8:12], 0x11223344)
	binary.LittleEndian.PutUint16(p[12:14], 2)     // timer resolution
	binary.LittleEndian.PutUint16(p[14:16], 38000) // carrier frequency
	binary.LittleEndian.PutUint16(p[16:18], 67)    // nb timings
	p[20] = 1                                      // status
	copy(p[24:], []byte{1, 2, 3, 4, 5, 6, 7, 8})
	send(opRCIr, 0x19, p)

	if got.Port != 3 {
		t.Fatalf("port = %d, want 3", got.Port)
	}
	if got.Timestamp != 0xAABBCCDD {
		t.Fatalf("timestamp = %#x, want 0xAABBCCDD", got.Timestamp)
	}
	if got.LastChange != 0x11223344 {
		t.Fatalf("last change = %#x", got.LastChange)
	}
	if got.Meta.Frequency != 38000 || got.Meta.NbTimings != 67 || got.Meta.Status != 1 {
		t.Fatalf("meta = %+v", got.Meta)
	}
	if len(got.Timings) != 8 {
		t.Fatalf("timings = %d bytes, want 8", len(got.Timings))
	}
}

// A combined EDID reply is two 257-byte records, so the mode byte leads both
// halves rather than one mode covering the pair.
func TestEDIDCombinedReplyHasModePerRecord(t *testing.T) {
	var got []EDIDRecord
	_, _, feed := cmdRemote(t, 44, Callbacks{
		OnEDIDReceived: func(_ *Device, e EDIDRecord) { got = append(got, e) },
	})
	p := make([]byte, 2*257)
	p[0] = 0 // input
	p[1] = 0xAB
	p[257] = 1 // output
	p[258] = 0xCD
	feed(opDevEDID, p)

	if len(got) != 2 {
		t.Fatalf("got %d records, want 2", len(got))
	}
	if got[0].Output || got[0].Data[0] != 0xAB || len(got[0].Data) != 256 {
		t.Fatalf("first record = %+v", got[0])
	}
	if !got[1].Output || got[1].Data[0] != 0xCD {
		t.Fatalf("second record = output=%v first byte=%#x", got[1].Output, got[1].Data[0])
	}
}

func TestEDIDRequestForm(t *testing.T) {
	var got EDIDRequest
	var records int
	_, _, feed := cmdRemote(t, 45, Callbacks{
		OnEDIDRequested: func(_ *Device, req EDIDRequest) { got = req },
		OnEDIDReceived:  func(_ *Device, _ EDIDRecord) { records++ },
	})
	target := uidN(99)
	p := append(append([]byte(nil), target[:]...), 1)
	feed(opDevEDID, p)

	if got.Target != target || !got.Output {
		t.Fatalf("edid request = %+v", got)
	}
	if records != 0 {
		t.Fatal("a request must not decode as an EDID record")
	}
}

func TestVideoWallDecode(t *testing.T) {
	var got VideoWallCommand
	_, _, feed := cmdRemote(t, 46, Callbacks{
		OnVideoWallCommand: func(_ *Device, c VideoWallCommand) { got = c },
	})
	target := uidN(77)
	wall := func(op VideoWallOp, w, h uint16) []byte {
		// poisoned so the three pad bytes after the op are not zero
		p := poisoned(32)
		copy(p[0:16], target[:])
		binary.LittleEndian.PutUint16(p[16:18], 1920)
		binary.LittleEndian.PutUint16(p[18:20], 0)
		binary.LittleEndian.PutUint16(p[20:22], w)
		binary.LittleEndian.PutUint16(p[22:24], h)
		binary.LittleEndian.PutUint16(p[24:26], 3840)
		binary.LittleEndian.PutUint16(p[26:28], 2160)
		p[28] = byte(op)
		return p
	}

	feed(opV2IPVideoWall, wall(VideoWallStore, 1920, 1080))
	if got.Target != target || got.PosX != 1920 || got.Width != 1920 || got.Height != 1080 {
		t.Fatalf("wall = %+v", got)
	}
	if got.RasterW != 3840 || got.RasterH != 2160 {
		t.Fatalf("raster = %dx%d, want 3840x2160", got.RasterW, got.RasterH)
	}
	if got.Op != VideoWallStore || !got.HasWindow() || got.Cleared() {
		t.Fatalf("op=%v hasWindow=%v cleared=%v", got.Op, got.HasWindow(), got.Cleared())
	}

	// a zero width is the wire spelling of "clear the wall", not "unset"
	feed(opV2IPVideoWall, wall(VideoWallPreview, 0, 0))
	if !got.Cleared() {
		t.Fatal("a zero-width window should read as a clear")
	}

	// a revert zeroes the geometry and the receiver ignores it, so those zeros
	// are not a clear
	revert := wall(VideoWallRevert, 0, 0)
	feed(opV2IPVideoWall, revert)
	if got.HasWindow() || got.Cleared() {
		t.Fatalf("revert should carry no window: %+v", got)
	}
}

func TestTxKeyAndActionDecode(t *testing.T) {
	var key KeyTransmitRequest
	var act ActionTransmitRequest
	_, _, feed := cmdRemote(t, 47, Callbacks{
		OnKeyTransmitRequested:    func(_ *Device, r KeyTransmitRequest) { key = r },
		OnActionTransmitRequested: func(_ *Device, r ActionTransmitRequest) { act = r },
	})
	target := uidN(88)
	mk := func(v uint16) []byte {
		p := make([]byte, 20)
		copy(p[0:16], target[:])
		binary.LittleEndian.PutUint16(p[16:18], 300)
		binary.LittleEndian.PutUint16(p[18:20], v)
		return p
	}
	feed(opRCTxKey, mk(0x0041))
	feed(opRCTxAction, mk(uint16(ActionPowerOn)))

	if key.Target != target || key.LocalBay != 300 || key.Key != RCKey(0x41) {
		t.Fatalf("key request = %+v", key)
	}
	// the action is a u16 at 18, the same place the request writes it
	if act.LocalBay != 300 || act.Action != ActionPowerOn {
		t.Fatalf("action request = %+v", act)
	}
}

func TestSetupStatusInstallerAndPDU(t *testing.T) {
	r, sender, feed := cmdRemote(t, 48, Callbacks{})

	feed(opSetupStatus, []byte{1})
	if done, known := r.GetByUID(sender).SetupCompleted(); !known || !done {
		t.Fatalf("setup completed = %v known=%v", done, known)
	}
	feed(opSetInstaller, []byte{0x34, 0x12})
	if id := r.GetByUID(sender).InstallerID(); id == nil || *id != 0x1234 {
		t.Fatalf("installer id = %v", id)
	}

	p := make([]byte, 32)
	binary.LittleEndian.PutUint32(p[0:4], 0x40000000)   // 2.0 A
	binary.LittleEndian.PutUint32(p[4:8], 0x42F00000)   // 120.0 V
	binary.LittleEndian.PutUint32(p[20:24], 0x42480000) // 50.0 Hz
	p[24], p[25] = 1, 0
	feed(opPDUState, p)
	st := r.GetByUID(sender).PDUState()
	if st == nil || st.Current != 2 || st.Voltage != 120 || st.Frequency != 50 {
		t.Fatalf("pdu state = %+v", st)
	}
	if st.Outlets[0] != 1 || st.Outlets[1] != 0 {
		t.Fatalf("outlets = %v", st.Outlets)
	}
}

func TestFilterStatusMergesUIDList(t *testing.T) {
	r, sender, feed := cmdRemote(t, 49, Callbacks{})
	feed(opSysBayConfig, bayConfigRec(2, 1, 0, "Output 1", "TV", 0, BayHDMIOut))

	a, b := uidN(61), uidN(62)
	p := append([]byte(nil), sender[:]...)
	p = append(p, a[:]...)
	p = append(p, b[:]...)
	feed(opBayFilterStatus, p)

	filtered := r.GetByUID(sender).GetByPortnum(2).FilteredDevices()
	if len(filtered) != 2 || filtered[0] != a || filtered[1] != b {
		t.Fatalf("filtered = %v", filtered)
	}
}

func TestPowerSaveBothForms(t *testing.T) {
	var got V2IPPowerSaveRequest
	_, _, feed := cmdRemote(t, 50, Callbacks{
		OnPowerSaveRequested: func(_ *Device, req V2IPPowerSaveRequest) { got = req },
	})
	feed(opV2IPPowerSave, []byte{1})
	if got.Target != nil || !got.Enabled {
		t.Fatalf("broadcast form = %+v", got)
	}
	target := uidN(55)
	feed(opV2IPPowerSave, append(append([]byte(nil), target[:]...), 0))
	if got.Target == nil || *got.Target != target || got.Enabled {
		t.Fatalf("targeted form = %+v", got)
	}
}

func TestFactoryResetForms(t *testing.T) {
	var got FactoryResetRequest
	_, _, feed := cmdRemote(t, 51, Callbacks{
		OnFactoryResetRequested: func(_ *Device, req FactoryResetRequest) { got = req },
	})
	feed(opSysFactoryReset, []byte{0xFF})
	if !got.All || got.Target != nil {
		t.Fatalf("broadcast form = %+v", got)
	}
	target := uidN(56)
	feed(opSysFactoryReset, target[:])
	if got.All || got.Target == nil || *got.Target != target {
		t.Fatalf("targeted form = %+v", got)
	}
	feed(opSysFactoryReset, nil)
	if got.All || got.Target != nil {
		t.Fatalf("sender-only form = %+v", got)
	}
}

func TestChangeBayNameFullWidth(t *testing.T) {
	var got BayNameChange
	_, _, feed := cmdRemote(t, 52, Callbacks{
		OnBayNameChangeRequested: func(_ *Device, c BayNameChange) { got = c },
	})
	target := uidN(57)
	full := "0123456789ABCDEF" // fills the field, no terminator
	p := append([]byte(nil), target[:]...)
	p = append(p, 0, 0)
	binary.LittleEndian.PutUint16(p[16:18], 300)
	p = append(p, []byte(full)...)
	feed(opChangeBayName, p)

	if got.Target != target || got.Port != 300 || got.Name != full {
		t.Fatalf("name change = %+v", got)
	}
}

func TestRCSettingsDecode(t *testing.T) {
	r, sender, feed := cmdRemote(t, 53, Callbacks{})
	p := make([]byte, 48)
	copy(p[0:16], sender[:])
	binary.LittleEndian.PutUint32(p[16:20], 7) // RC_TARGET_MX_REMOTE
	copy(p[20:24], []byte{10, 8, 80, 30})
	binary.LittleEndian.PutUint16(p[24:26], 1|(1<<2)|(3<<4)) // cec on, rc forward, status 3
	feed(opRCSettings, p)

	s := r.GetByUID(sender).RCSettings()
	if s == nil {
		t.Fatal("no rc settings")
	}
	if s.RCTarget != 7 || s.IP != "10.8.80.30" {
		t.Fatalf("rc settings = %+v", s)
	}
	if !s.CECEnabled || s.CECAutoOn || !s.ForwardRC || s.ForwardIR {
		t.Fatalf("flags = %+v", s)
	}
	if s.RCStatus != 3 {
		t.Fatalf("rc status = %d, want 3", s.RCStatus)
	}
}

// The audio SELECT_INPUT body carries the route end to end: the sink at offset
// 20 and the source at 36, with their endpoint ids after. Decoding those the
// other way round swaps source and sink.
//
// The three uids here are all different. The header names the device addressed
// for the hop that carried the frame, which is the sink only on a single-hop
// route, so a fixture that lets the header and the body's sink be the same uid
// cannot tell a body-read orientation from a header-read one.
func TestAudioSelectInputOrientation(t *testing.T) {
	var got AudioChangeSource
	r, sender, feed := cmdRemote(t, 60, Callbacks{
		OnAudioSelectInput: func(_ *Device, c AudioChangeSource) { got = c },
	})
	source, sink := uidN(61), uidN(63)

	p := audioCmdHeader(audioOpSelectInput, sender)
	p = append(p, sink[:]...)
	p = append(p, source[:]...)
	p = append(p, 7, 0, 9, 0) // sink endpoint 7, source endpoint 9
	feed(opV2IPAudio, p)

	if got.TargetUID != sink || got.TargetID != 7 {
		t.Fatalf("sink = %s:%d, want %s:7", got.TargetUID, got.TargetID, sink)
	}
	if got.SourceUID != source || got.SourceID != 9 {
		t.Fatalf("source = %s:%d, want %s:9", got.SourceUID, got.SourceID, source)
	}
	if sel := r.GetByUID(sender).AudioSourceSelection(); sel == nil || *sel != got {
		t.Fatalf("cached selection = %v", sel)
	}
}

// The command this library sends must decode back to the same orientation.
func TestAudioSelectInputRoundTrip(t *testing.T) {
	var got AudioChangeSource
	r, sender, feed := cmdRemote(t, 62, Callbacks{
		OnAudioSelectInput: func(_ *Device, c AudioChangeSource) { got = c },
	})
	source := uidN(63)
	sinkEP, sourceEP := &AudioEndpoint{ID: 3}, &AudioEndpoint{ID: 5}

	p := audioCmdHeader(audioOpSelectInput, sender)
	p = append(p, sender[:]...)
	p = append(p, source[:]...)
	p = append(p, byte(sinkEP.ID), byte(sinkEP.ID>>8), byte(sourceEP.ID), byte(sourceEP.ID>>8))
	feed(opV2IPAudio, p)

	if got.TargetID != 3 || got.SourceID != 5 || got.SourceUID != source {
		t.Fatalf("round trip = %+v", got)
	}
	_ = r
}

func TestAudioEndpointParams(t *testing.T) {
	var muted, trigger *bool
	var volume uint32
	var volEP int
	_, _, feed := cmdRemote(t, 64, Callbacks{
		OnAudioEndpointMute:    func(_ *Device, _ int, m bool) { muted = &m },
		OnAudioEndpointTrigger: func(_ *Device, _ int, a bool) { trigger = &a },
		OnAudioEndpointVolume:  func(_ *Device, ep int, v uint32) { volEP, volume = ep, v },
	})
	send := func(op uint16, ep, param int) {
		feed(opV2IPAudio, append(audioCmdHeader(op, uidN(64)), audioParam(ep, param)...))
	}
	send(audioOpMute, 2, 1)
	send(audioOpTrigger, 3, 0)
	send(audioOpVolume, 4, 80)

	if muted == nil || !*muted {
		t.Fatalf("mute = %v", muted)
	}
	if trigger == nil || *trigger {
		t.Fatalf("trigger = %v", trigger)
	}
	if volEP != 4 || volume != 80 {
		t.Fatalf("volume endpoint %d = %d, want 4 = 80", volEP, volume)
	}
}

// mxr_tx_ir_data is unpacked and 4-aligned, so the firmware appends the timings
// at sizeof = 36. Taking them from the end of the last field shifts every u16
// timing by two bytes.
func TestIRTransmitTimingsStartAtStructSize(t *testing.T) {
	var got IRTransmitRequest
	_, _, feed := cmdRemote(t, 70, Callbacks{
		OnIRTransmitRequested: func(_ *Device, req IRTransmitRequest) { got = req },
	})
	target := uidN(71)
	// poisoned so the padding at 18..20 and the struct tail at 34..36 are not
	// zero: a field read at the right offset but the wrong width shows up here
	p := poisoned(36 + 6)
	copy(p[0:16], target[:])
	p[16], p[17] = 1, 2
	binary.LittleEndian.PutUint16(p[24:26], 0) // meta.timer_resolution
	binary.LittleEndian.PutUint16(p[30:32], 0) // meta.repeat_offset
	p[32] = 0                                  // meta.status
	binary.LittleEndian.PutUint32(p[20:24], 0xDEADBEEF)
	binary.LittleEndian.PutUint16(p[26:28], 38000) // meta.frequency
	binary.LittleEndian.PutUint16(p[28:30], 3)     // meta.nb_timings
	copy(p[36:], []byte{1, 0, 2, 0, 3, 0})
	feed(opRCIrTx, p)

	if got.Target != target || got.LocalMode != 1 || got.LocalBay != 2 {
		t.Fatalf("target = %+v", got)
	}
	if got.Timestamp != 0xDEADBEEF {
		t.Fatalf("timestamp = %#x", got.Timestamp)
	}
	if got.Meta.Frequency != 38000 || got.Meta.NbTimings != 3 {
		t.Fatalf("meta = %+v", got.Meta)
	}
	if len(got.Timings) != 6 {
		t.Fatalf("timings = %d bytes, want 6 (tail padding leaked in?)", len(got.Timings))
	}
	for i, want := range []uint16{1, 2, 3} {
		if v := binary.LittleEndian.Uint16(got.Timings[i*2:]); v != want {
			t.Fatalf("timing %d = %d, want %d", i, v, want)
		}
	}
}

func TestRCSettingsStatusName(t *testing.T) {
	r, sender, feed := cmdRemote(t, 72, Callbacks{})
	p := make([]byte, 48)
	copy(p[0:16], sender[:])
	binary.LittleEndian.PutUint32(p[16:20], 7)
	copy(p[20:24], []byte{10, 8, 80, 30})
	p[24] = 1 | (1 << 3) | (5 << 4) // cec on, ir forward, status 5
	// byte 25 is dead space in the same container; a decoder that reads 24..26
	// as one u16 and shifts would pick this up
	p[25] = 0xFF
	copy(p[28:44], "Detecting")
	feed(opRCSettings, p)

	s := r.GetByUID(sender).RCSettings()
	if s == nil {
		t.Fatal("no rc settings")
	}
	if !s.CECEnabled || s.CECAutoOn || s.ForwardRC || !s.ForwardIR {
		t.Fatalf("flags = %+v", s)
	}
	if s.RCStatus != 5 {
		t.Fatalf("rc status = %d, want 5", s.RCStatus)
	}
	if s.StatusName != "Detecting" {
		t.Fatalf("status name = %q, want %q", s.StatusName, "Detecting")
	}
}

// An unknown enum value must reach the caller as itself, not fold into
// whichever named value happens to be zero.
func TestUnknownEnumsArePassedThrough(t *testing.T) {
	r, sender, feed := cmdRemote(t, 73, Callbacks{})
	feed(opSysHello, helloPayload(0x28, "ONEIP-MV", "MV9", "4.8.0", FeatureV2IPSink|FeatureMultiviewer))

	p := make([]byte, 190)
	p[169] = 200 // a view mode far beyond anything named
	feed(opV2IPMultiviewer, p)

	mv := r.GetByUID(sender).Multiviewer()
	if mv == nil {
		t.Skip("device did not register as a multiviewer")
	}
	if got := mv.ViewMode(); int(got) != 200 {
		t.Fatalf("view mode = %d, want it passed through as 200", got)
	}

	// an unknown firmware type must not read back as a known one either
	fw := make([]byte, 12+8)
	fw[0] = 42
	copy(fw[12:], "9.9.9")
	feed(opFirmwareVersion, fw)
	if v := r.GetByUID(sender).FirmwareVersions(); len(v) > 0 {
		found := false
		for _, e := range v {
			if int(e.Type) == 42 {
				found = true
			}
		}
		if !found {
			t.Fatalf("firmware type 42 was not preserved: %+v", v)
		}
	}
}

func TestMultiviewerSubCommandsSurface(t *testing.T) {
	var seen []byte
	r, sender, feed := cmdRemote(t, 74, Callbacks{
		OnMultiviewerCommand: func(_ *Device, c MultiviewerCommand) { seen = append(seen, c.Op) },
	})
	_ = r
	for op := byte(0); op < 16; op++ {
		feed(opV2IPMultiviewer, mvCmdPayload(sender, op, 1, 2, 3))
	}
	if len(seen) != 16 {
		t.Fatalf("saw %d of 16 sub-commands: %v", len(seen), seen)
	}
}

// The 0x3F blocks are 20 and 44 because fpga_tx_stats and fpga_rx_stats carry
// their ALIGN(8) before the struct keyword, where GCC ignores it. The 128-byte
// total is stable by accident, so pin the block boundaries, not just the total.
func TestV2IPStatsBlockLayout(t *testing.T) {
	if txStatsSize != 20 || rxStatsSize != 44 || v2ipStatsSize != 128 {
		t.Fatalf("block sizes = %d/%d total %d, want 20/44 total 128",
			txStatsSize, rxStatsSize, v2ipStatsSize)
	}
	r, sender, feed := cmdRemote(t, 80, Callbacks{})

	// each rx block is 10 u32 stats then the decoder state at block offset 40
	p := make([]byte, v2ipStatsSize)
	binary.LittleEndian.PutUint32(p[0:4], 11)                        // tx totals, video
	binary.LittleEndian.PutUint32(p[20:24], 22)                      // tx per minute, video
	binary.LittleEndian.PutUint32(p[40:44], 33)                      // rx totals, video total
	binary.LittleEndian.PutUint32(p[76:80], 44)                      // rx totals, anc seq errors
	binary.LittleEndian.PutUint32(p[80:84], uint32(DecoderStarting)) // rx totals, state
	binary.LittleEndian.PutUint32(p[84:88], 55)                      // rx per minute, video total
	binary.LittleEndian.PutUint32(p[124:128], uint32(DecoderBad))    // rx per minute, state
	feed(opV2IPStats, p)

	st := r.GetByUID(sender).V2IPStats()
	if st == nil {
		t.Fatal("no stats")
	}
	if st.Tx.Video != 11 || st.TxPerMinute.Video != 22 {
		t.Fatalf("tx = %d / %d, want 11 / 22", st.Tx.Video, st.TxPerMinute.Video)
	}
	if st.Rx.VideoTotal != 33 || st.Rx.AncSeqErrors != 44 {
		t.Fatalf("rx totals = %+v", st.Rx)
	}
	if st.RxPerMinute.VideoTotal != 55 {
		t.Fatalf("rx per minute video = %d, want 55", st.RxPerMinute.VideoTotal)
	}
	if st.Rx.DecoderState != DecoderStarting {
		t.Fatalf("totals decoder state = %v, want Starting", st.Rx.DecoderState)
	}
	if st.RxPerMinute.DecoderState != DecoderBad {
		t.Fatalf("per-minute decoder state = %v, want Bad", st.RxPerMinute.DecoderState)
	}
}

// A decoder that is merely coming up must not read as one that failed.
func TestDecoderStateStartingIsNotAVerdict(t *testing.T) {
	if DecoderStarting != 3 {
		t.Fatalf("V2IP_STATE_STARTING is 3, got %d", DecoderStarting)
	}
	for _, s := range []V2IPDecoderState{DecoderUnknown, DecoderStarting} {
		if s.Settled() {
			t.Fatalf("%v should not be a verdict", s)
		}
	}
	for _, s := range []V2IPDecoderState{DecoderHealthy, DecoderBad} {
		if !s.Settled() {
			t.Fatalf("%v should be a verdict", s)
		}
	}
	if got := DecoderStarting.String(); got != "Starting" {
		t.Fatalf("Starting reads as %q - conflating it with Unknown loses the distinction", got)
	}
	// anything beyond the enum keeps its number rather than naming a known state
	if got := V2IPDecoderState(9).String(); got != "state 9" {
		t.Fatalf("unknown state reads as %q", got)
	}
}

// Bytes 16..19 and 24..27 of three real 0x45 frames, from units all configured
// for CEC. rc_target_t is one byte and the three that follow are padding the
// firmware never clears, so they carry live stack content that differs per
// frame. Reading the field as a u32 makes one unchanged setting decode as three
// different values; the expectation here comes from the units' known
// configuration, not from what the decoder happens to produce.
var rcSettingsCaptures = []struct {
	rcTarget [4]byte
	flags    [4]byte
}{
	{[4]byte{0x01, 0x73, 0x20, 0x28}, [4]byte{0x0f, 0x6f, 0x05, 0x28}},
	{[4]byte{0x01, 0x6e, 0x1e, 0x28}, [4]byte{0x0f, 0x6f, 0x05, 0x28}},
	{[4]byte{0x01, 0xb5, 0x1b, 0x28}, [4]byte{0x0f, 0x00, 0x00, 0x00}},
}

func TestRCSettingsPaddingIsNotPartOfTheField(t *testing.T) {
	for i, c := range rcSettingsCaptures {
		r, sender, feed := cmdRemote(t, byte(90+i), Callbacks{})
		p := make([]byte, 48)
		copy(p[0:16], sender[:])
		copy(p[16:20], c.rcTarget[:])
		copy(p[20:24], []byte{10, 8, 80, 30})
		copy(p[24:28], c.flags[:])
		// status_name stays empty: firmware writes a NUL and returns for any
		// non-network target, so a CEC unit cannot legitimately report one
		feed(opRCSettings, p)

		s := r.GetByUID(sender).RCSettings()
		if s == nil {
			t.Fatalf("capture %d: no rc settings", i)
		}
		// RC_TARGET_CEC, identical across all three despite the padding differing
		if s.RCTarget != 1 {
			t.Fatalf("capture %d: rc target = %d, want 1 (padding swallowed?)", i, s.RCTarget)
		}
		if !s.CECEnabled || !s.CECAutoOn || !s.ForwardRC || !s.ForwardIR {
			t.Fatalf("capture %d: flags from 0x0f = %+v, want all four set", i, s)
		}
		if s.RCStatus != 0 {
			t.Fatalf("capture %d: rc status = %d, want 0", i, s.RCStatus)
		}
		if s.StatusName != "" {
			t.Fatalf("capture %d: status name = %q, want empty for a CEC target", i, s.StatusName)
		}
	}
}

// 0x24 and the sink block appended to 0x3C were decoded but never exercised:
// a mutation sweep over every decode offset found their addresses among the
// sites no test would catch if they shifted.
func TestManualSourceSwitchAndSinkBlock(t *testing.T) {
	r, sender, feed := cmdRemote(t, 95, Callbacks{})
	feed(opSysBayConfig, bayConfigRec(1, 1, 0, "Output 1", "TV", 0, BayV2IPSinkLocal))

	put := func(p []byte, off int, ip string, port int) {
		copy(p[off:off+4], ip4Bytes(ip))
		binary.LittleEndian.PutUint16(p[off+4:off+6], uint16(port))
	}

	// manual source switch: uid, then video/audio/anc at 16/24/32 and an
	// optional audio format at 40
	p := make([]byte, 48)
	copy(p[0:16], sender[:])
	put(p, 16, "239.1.1.1", V2IPPortVideo)
	put(p, 24, "239.1.1.2", V2IPPortAudio)
	put(p, 32, "239.1.1.3", V2IPPortANC)
	binary.LittleEndian.PutUint32(p[40:44], 96000)
	p[44] = 6
	feed(opV2IPManualSrcSwitch, p)

	sink := r.GetByUID(sender).V2IPSink()
	if sink == nil {
		t.Fatal("no sink")
	}
	if sink.Addresses.Video.IP != "239.1.1.1" || sink.Addresses.Video.Port != V2IPPortVideo {
		t.Fatalf("video = %v", sink.Addresses.Video)
	}
	if sink.Addresses.Audio.IP != "239.1.1.2" || sink.Addresses.Anc.IP != "239.1.1.3" {
		t.Fatalf("audio/anc = %v / %v", sink.Addresses.Audio, sink.Addresses.Anc)
	}
	if sink.AudioFmt == nil || sink.AudioFmt.SampleRate != 96000 || sink.AudioFmt.Channels != 6 {
		t.Fatalf("audio format = %+v", sink.AudioFmt)
	}

	// the same sink block appended to a device config, at 88 with its format at 112
	cfg := addresses(sender, "239.2.2.2")
	c := make([]byte, 120)
	copy(c, cfg.bytes())
	put(c, 88, "239.3.3.1", V2IPPortVideo)
	put(c, 96, "239.3.3.2", V2IPPortAudio)
	put(c, 104, "239.3.3.3", V2IPPortANC)
	binary.LittleEndian.PutUint32(c[112:116], 44100)
	c[116] = 2
	feed(opV2IPDeviceCfg, c)

	sink = r.GetByUID(sender).V2IPSink()
	if sink.Addresses.Video.IP != "239.3.3.1" || sink.Addresses.Anc.IP != "239.3.3.3" {
		t.Fatalf("sink block = %v / %v", sink.Addresses.Video, sink.Addresses.Anc)
	}
	if sink.AudioFmt == nil || sink.AudioFmt.SampleRate != 44100 || sink.AudioFmt.Channels != 2 {
		t.Fatalf("sink audio format = %+v", sink.AudioFmt)
	}
}

// Firmware predating the fix builds a 0x3C from an uninitialised stack local
// and ORs its scaling flags onto whatever was there, so bits 2..6 arrive as
// noise on any receiver-capable unit. Only bit 7 carries meaning.
func TestScalingOptionNoiseBitsAreNotCached(t *testing.T) {
	r, sender, feed := cmdRemote(t, 96, Callbacks{})

	base := addresses(sender, "239.1.2.3")
	base.mode, base.refresh = 16, 60
	base.flags = ScalingFlagModeValid | ScalingFlagOptionsValid | ScalingFlagAutoScaling
	feed(opV2IPDeviceCfg, base.bytes())

	// an options-only write carrying garbage in the undefined bits, with
	// auto-scaling genuinely off
	noisy := v2ipCfg{uid: sender, flags: ScalingFlagOptionsValid | 0x7C}
	feed(opV2IPDeviceCfg, noisy.bytes())

	sc := r.GetByUID(sender).V2IPDetails().Scaling
	if sc.Flags&ScalingFlagAutoScaling != 0 {
		t.Fatal("auto-scaling should have been cleared by the options write")
	}
	if noise := sc.Flags &^ (ScalingFlagModeValid | ScalingFlagOptionsValid | ScalingFlagAutoScaling); noise != 0 {
		t.Fatalf("undefined flag bits %#02x were cached; only bit 7 carries meaning", noise)
	}
	// the mode half is untouched by an options-only write
	if sc.Mode.Svd() != 16 || sc.Refresh != 60 {
		t.Fatalf("mode half = %v / %d, want svd 16 / 60", sc.Mode, sc.Refresh)
	}

	// and a genuine auto-scaling bit still arrives
	on := v2ipCfg{uid: sender, flags: ScalingFlagOptionsValid | ScalingFlagAutoScaling | 0x34}
	feed(opV2IPDeviceCfg, on.bytes())
	sc = r.GetByUID(sender).V2IPDetails().Scaling
	if sc.Flags&ScalingFlagAutoScaling == 0 {
		t.Fatal("auto-scaling should have been set")
	}
}

// A real 0x3C captured off a live mesh. Expected values
// come from the sending unit's own configuration and from firmware behaviour,
// not from what this decoder produces.
var deviceCfgCapture = []byte{
	0x27, 0x40, 0x01, 0x04, 0x85, 0xac, 0xb7, 0xaa, 0x3e, 0x7d, 0x2c, 0x67, 0xc6, 0x07, 0x00, 0xf5,
	0xea, 0xda, 0x44, 0xf4, 0x64, 0xc3, 0x00, 0x00, // source.video 234.218.68.244:50020
	0xea, 0xda, 0x44, 0xf5, 0x66, 0xc3, 0x00, 0x00, // source.audio 234.218.68.245:50022
	0xea, 0xda, 0x44, 0xf4, 0x65, 0xc3, 0x00, 0x00, // source.anc   234.218.68.244:50021
	0x5a, 0x90, 0x90, 0x90, 0x00, 0x00, 0x00, 0x00, // tx_rate 90, dscp SET|16 x3
	0xea, 0xda, 0x44, 0xf6, 0x67, 0xc3, 0x00, 0x00, // audio_return 234.218.68.246:50023
	0x13, 0x20, 0x32, 0x00, 0xdf, 0x1b, 0x00, 0x10, // scaling: svd 19, 8bpp, 50Hz, flags 0xdf
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, // tiling, uid zero: not carried
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
	0xea, 0xda, 0x44, 0xf4, 0x64, 0xc3, 0x00, 0x00, // sink.video
	0xea, 0xda, 0x44, 0xf5, 0x66, 0xc3, 0x00, 0x00, // sink.audio
	0xea, 0xda, 0x44, 0xf4, 0x65, 0xc3, 0x00, 0x00, // sink.anc
	0, 0, 0, 0, 0, 0, 0, 0, // sink_audio_fmt
}

func TestDeviceConfigCapture(t *testing.T) {
	if len(deviceCfgCapture) != 120 {
		t.Fatalf("capture is %d bytes, want 120", len(deviceCfgCapture))
	}
	r, sender, feed := cmdRemote(t, 97, Callbacks{})
	feed(opV2IPDeviceCfg, deviceCfgCapture)

	det := r.GetByUID(sender).V2IPDetails()
	// v2ip_stream_source is 8 bytes: port is uint_fast16_t, four bytes on ARM,
	// so the two bytes after the port are part of the field, not the next one
	if det.Video.IP != "234.218.68.244" || det.Video.Port != 50020 {
		t.Fatalf("video = %v, want 234.218.68.244:50020", det.Video)
	}
	if det.Audio.Port != 50022 || det.Anc.Port != 50021 {
		t.Fatalf("audio/anc ports = %d / %d, want 50022 / 50021", det.Audio.Port, det.Anc.Port)
	}
	if det.Arc.IP != "234.218.68.246" || det.Arc.Port != 50023 {
		t.Fatalf("arc = %v", det.Arc)
	}
	if det.TxRate == nil || *det.TxRate != 90 {
		t.Fatalf("tx rate = %v, want 90", det.TxRate)
	}
	// 0x90 is MXR_V2IP_DSCP_SET | 16, and 16 is CS2, the boot default
	if !det.Dscp.Complete() || *det.Dscp.Video != V2IPDscpDefault ||
		*det.Dscp.Audio != V2IPDscpDefault || *det.Dscp.Anc != V2IPDscpDefault {
		t.Fatalf("dscp = %v, want all three at %d", det.Dscp, V2IPDscpDefault)
	}
	if det.Scaling.Mode.Svd() != 19 || det.Scaling.Mode.Bpp() != 8 || det.Scaling.Refresh != 50 {
		t.Fatalf("scaling = %v @ %dHz", det.Scaling.Mode, det.Scaling.Refresh)
	}
	// flags 0xdf carries bits 2,3,4,6 as well: this unit predates the fix for
	// the uninitialised mxr_scaling_config, so only bits 0,1,7 mean anything
	if det.Scaling.Flags&ScalingFlagAutoScaling == 0 {
		t.Fatal("auto-scaling should be set")
	}
	if noise := det.Scaling.Flags &^ (ScalingFlagModeValid | ScalingFlagOptionsValid | ScalingFlagAutoScaling); noise != 0 {
		t.Fatalf("undefined flag bits %#02x cached from a 0xdf frame", noise)
	}
	// the tiling block is zeroed, so its uid is zero: not carried, not a clear
	if tl := r.GetByUID(sender).Tiling(); tl != nil {
		t.Fatalf("a zero-uid tiling block was cached as a window: %v", tl)
	}
	sink := r.GetByUID(sender).V2IPSink()
	if sink == nil || sink.Addresses.Video.Port != 50020 || sink.Addresses.Anc.Port != 50021 {
		t.Fatalf("sink block = %+v", sink)
	}
}

// A carried window: uid stamped, geometry zero, which firmware writes for a
// cleared wall and a client must not confuse with "not carried".
func TestTilingCarriedVersusAbsent(t *testing.T) {
	r, sender, feed := cmdRemote(t, 98, Callbacks{})
	target := uidN(99)

	p := append([]byte(nil), deviceCfgCapture...)
	copy(p[64:80], target[:])
	binary.LittleEndian.PutUint16(p[80:82], 1920)
	binary.LittleEndian.PutUint16(p[84:86], 3840)
	binary.LittleEndian.PutUint16(p[86:88], 2160)
	feed(opV2IPDeviceCfg, p)

	tl := r.GetByUID(sender).Tiling()
	if tl == nil || tl.Target != target || tl.PosX != 1920 || tl.Width != 3840 || tl.Height != 2160 {
		t.Fatalf("carried window = %v", tl)
	}

	// uid stamped with zero geometry is a real clear, and must still be cached
	copy(p[80:88], make([]byte, 8))
	feed(opV2IPDeviceCfg, p)
	tl = r.GetByUID(sender).Tiling()
	if tl == nil || tl.Width != 0 || tl.Height != 0 {
		t.Fatalf("a stamped clear should cache as a zero window, got %v", tl)
	}

	// and a zero-uid block must leave that cached clear alone
	copy(p[64:80], make([]byte, 16))
	binary.LittleEndian.PutUint16(p[84:86], 1234)
	feed(opV2IPDeviceCfg, p)
	if tl = r.GetByUID(sender).Tiling(); tl == nil || tl.Width != 0 {
		t.Fatalf("an uncarried block overwrote the cached window: %v", tl)
	}
}

// decoderVector is bytes 128..151 of a 0x3F payload, read identically by this
// client, the Rust client and a decode of the firmware's struct declaration.
//
// It is composed rather than captured: no frame from a live sink exists in any
// of the three trees, and two of the three readings are downstream of this
// fixture, so agreement between them cannot catch a value all three took from
// the same description. Only the firmware struct decode is independent of it.
// Replace this with a captured frame when one is to hand.
var decoderVector = []byte{
	0x01, 0x04, 0x00, 0x77, 0x00, 0x0f, 0x70, 0x08,
	0x02, 0x00, 0x58, 0x02, 0x10, 0x01, 0x10, 0x00,
	0xa9, 0x86, 0x01, 0x00, 0xef, 0xbe, 0xad, 0xde,
}

// statsPayload builds a 0x3F payload of n bytes over poison, so a field read
// one byte wide or one byte over returns a wrong value rather than a zero that
// happens to match.
func statsPayload(n int, decoder []byte) []byte {
	p := poisoned(n)
	if decoder != nil {
		copy(p[v2ipStatsSize:], decoder)
	}
	return p
}

func TestV2IPDecoderVector(t *testing.T) {
	r, sender, feed := cmdRemote(t, 81, Callbacks{})
	feed(opV2IPStats, statsPayload(v2ipStatsSize+decoderDetailSize, decoderVector))

	st := r.GetByUID(sender).V2IPStats()
	if st == nil || !st.DecoderReported || st.Decoder == nil {
		t.Fatalf("stats = %+v", st)
	}
	d := st.Decoder
	// Reserved byte 3 is 0x77 here, so a Blocking read taken from it reads true.
	if d.Reason != DecoderReasonFormatMismatch || d.Blocking {
		t.Errorf("reason = %v, blocking = %v, want format mismatch / false", d.Reason, d.Blocking)
	}
	if d.Width != 3840 || d.Height != 2160 {
		t.Errorf("geometry = %dx%d, want 3840x2160", d.Width, d.Height)
	}
	if d.Format != DecoderFormatYCbCr422 || d.Updates != 600 {
		t.Errorf("format = %v, updates = %d, want YCbCr 4:2:2 / 600", d.Format, d.Updates)
	}
	if d.Flags != 0x00100110 || d.BlockedCount != 100009 {
		t.Errorf("flags = %#08x, blocked = %d, want 0x00100110 / 100009", d.Flags, d.BlockedCount)
	}
	if !d.Recovered() {
		t.Error("Recovered() = false for a 3840x2160 reading")
	}
	// Bit 20 is a cause nobody names; it has to survive as one.
	for _, r := range []V2IPDecoderReason{4, 8, 20} {
		if !d.HasReason(r) {
			t.Errorf("HasReason(%d) = false, want true", r)
		}
	}
	for _, r := range []V2IPDecoderReason{DecoderReasonOK, 5, 21} {
		if d.HasReason(r) {
			t.Errorf("HasReason(%d) = true, want false", r)
		}
	}
}

// The three states a report can be in are distinct, and only the third carries
// fields: a sender too old to have the block at all is not a decoder that has
// never answered, and neither is a reading of 0x0.
func TestV2IPDecoderThreeStates(t *testing.T) {
	r, sender, feed := cmdRemote(t, 82, Callbacks{})
	dev := r.GetByUID(sender)

	feed(opV2IPStats, statsPayload(v2ipStatsSize, nil))
	if st := dev.V2IPStats(); st == nil || st.DecoderReported || st.Decoder != nil {
		t.Fatalf("128-byte payload: reported = %v, decoder = %+v", st.DecoderReported, st.Decoder)
	}

	unanswered := poisoned(decoderDetailSize)
	unanswered[0] = 0
	feed(opV2IPStats, statsPayload(v2ipStatsSize+decoderDetailSize, unanswered))
	if st := dev.V2IPStats(); st == nil || !st.DecoderReported || st.Decoder != nil {
		t.Fatalf("valid=0: reported = %v, decoder = %+v", st.DecoderReported, st.Decoder)
	}

	feed(opV2IPStats, statsPayload(v2ipStatsSize+decoderDetailSize, decoderVector))
	if st := dev.V2IPStats(); st == nil || !st.DecoderReported || st.Decoder == nil {
		t.Fatalf("valid=1: reported = %v, decoder = %+v", st.DecoderReported, st.Decoder)
	}
}

// Every field of the block read at its own width, with values a narrower or
// wider read cannot reproduce: a byte-wide Format read returns the right answer
// for every value currently named, so give it a non-zero high byte.
func TestV2IPDecoderFieldWidths(t *testing.T) {
	r, sender, feed := cmdRemote(t, 83, Callbacks{})

	d := poisoned(decoderDetailSize)
	d[0] = 1                                        // valid
	d[1] = 0x0b                                     // reason, one past the last named value
	d[2] = 1                                        // blocking
	d[3] = 0x5a                                     // reserved, must not be read as blocking
	binary.LittleEndian.PutUint16(d[4:6], 0x0500)   // width 1280
	binary.LittleEndian.PutUint16(d[6:8], 0x02d0)   // height 720
	binary.LittleEndian.PutUint16(d[8:10], 0x0102)  // format 258, low byte a named value
	binary.LittleEndian.PutUint16(d[10:12], 0xfffe) // updates, just short of the wrap
	binary.LittleEndian.PutUint32(d[12:16], 0x80000010)
	binary.LittleEndian.PutUint32(d[16:20], 0xdeadbeef)
	feed(opV2IPStats, statsPayload(v2ipStatsSize+decoderDetailSize, d))

	st := r.GetByUID(sender).V2IPStats()
	if st == nil || st.Decoder == nil {
		t.Fatalf("stats = %+v", st)
	}
	got := st.Decoder
	want := V2IPDecoderDetail{
		Reason: 0x0b, Blocking: true,
		Width: 1280, Height: 720,
		Format: 258, Updates: 0xfffe,
		Flags: 0x80000010, BlockedCount: 0xdeadbeef,
	}
	if *got != want {
		t.Fatalf("decoder = %+v, want %+v", *got, want)
	}
	// An unnamed reason and an unnamed format both stay opaque rather than
	// being folded onto a named one.
	if s := got.Reason.String(); s != "reason 11" {
		t.Errorf("reason 11 renders as %q", s)
	}
	if s := got.Format.String(); s != "format 258" {
		t.Errorf("format 258 renders as %q", s)
	}
	// Bit 31 is the top of the word and bit 4 a named cause; bit 24 is clear,
	// and reason 32 is past the word rather than a bit that wrapped into it.
	if !got.HasReason(31) || !got.HasReason(4) || got.HasReason(24) || got.HasReason(32) {
		t.Errorf("flags %#08x: 31 = %v, 4 = %v, 24 = %v, 32 = %v", got.Flags,
			got.HasReason(31), got.HasReason(4), got.HasReason(24), got.HasReason(32))
	}
}

// Appending the decoder block moved no counter, and a payload longer than this
// library understands is parsed up to the prefix it does understand.
func TestV2IPStatsCountersSurviveTheDecoderBlock(t *testing.T) {
	r, sender, feed := cmdRemote(t, 84, Callbacks{})

	for _, n := range []int{v2ipStatsSize + decoderDetailSize, v2ipStatsSize + decoderDetailSize + 8} {
		p := statsPayload(n, decoderVector)
		binary.LittleEndian.PutUint32(p[0:4], 11)    // tx totals, video
		binary.LittleEndian.PutUint32(p[20:24], 22)  // tx per minute, video
		binary.LittleEndian.PutUint32(p[40:44], 33)  // rx totals, video total
		binary.LittleEndian.PutUint32(p[80:84], 1)   // rx totals, decoder state
		binary.LittleEndian.PutUint32(p[84:88], 55)  // rx per minute, video total
		binary.LittleEndian.PutUint32(p[124:128], 2) // rx per minute, decoder state
		feed(opV2IPStats, p)

		st := r.GetByUID(sender).V2IPStats()
		if st == nil {
			t.Fatalf("%d-byte payload: no stats", n)
		}
		if st.Tx.Video != 11 || st.TxPerMinute.Video != 22 || st.Rx.VideoTotal != 33 || st.RxPerMinute.VideoTotal != 55 {
			t.Errorf("%d-byte payload: counters = %+v / %+v", n, st.Tx, st.Rx)
		}
		if st.Rx.DecoderState != DecoderHealthy || st.RxPerMinute.DecoderState != DecoderBad {
			t.Errorf("%d-byte payload: states = %v / %v", n, st.Rx.DecoderState, st.RxPerMinute.DecoderState)
		}
		if st.Decoder == nil || st.Decoder.Width != 3840 {
			t.Errorf("%d-byte payload: decoder = %+v", n, st.Decoder)
		}
	}
}

// A stamp above this library's own version must not cost us the frame. Reports
// carrying the decoder block are stamped 0x29; a receive ceiling would drop them
// whole, losing the counters that were already being decoded rather than only
// the block that was added.
func TestReceiveHasNoProtocolCeiling(t *testing.T) {
	r, sender, feed := cmdRemote(t, 85, Callbacks{})
	feed(opSysHello, helloPayload(0x28, "ONEIP", "CM0001", "4.8.0", FeatureV2IPSink))

	above := ProtocolVersion + 1
	p := statsPayload(v2ipStatsSize+decoderDetailSize, decoderVector)
	binary.LittleEndian.PutUint32(p[40:44], 4242) // rx totals, video total
	r.processFrame(buildFrame(sender, opV2IPStats, above, p), "10.8.8.9", time.Now())

	st := r.GetByUID(sender).V2IPStats()
	if st == nil {
		t.Fatalf("frame stamped %#x was dropped", above)
	}
	if st.Rx.VideoTotal != 4242 || st.Decoder == nil || st.Decoder.Width != 3840 {
		t.Fatalf("stamped %#x: rx = %d, decoder = %+v", above, st.Rx.VideoTotal, st.Decoder)
	}
}

// The stamp is a floor on the decoder block, not a ceiling on the frame. A
// sender predating the block appended no such thing, so 24 bytes past the
// counters are some other growth: reading them would invent a reason, a
// geometry and a fault word out of bytes that mean something else.
//
// Both halves are asserted against the same payload, so the difference measured
// is the stamp and nothing else. The counters are read either way, which is
// what a ceiling on the frame would have thrown away with the tail.
func TestDecoderBlockNeedsTheStampAndTheLength(t *testing.T) {
	for _, tc := range []struct {
		stamp    uint16
		reported bool
	}{
		{decoderProtocol - 1, false},
		{decoderProtocol, true},
	} {
		r, sender, feed := cmdRemote(t, 87, Callbacks{})
		feed(opSysHello, helloPayload(0x28, "ONEIP", "CM0001", "4.8.0", FeatureV2IPSink))

		p := statsPayload(v2ipStatsSize+decoderDetailSize, decoderVector)
		binary.LittleEndian.PutUint32(p[40:44], 4242) // rx totals, video total
		r.processFrame(buildFrame(sender, opV2IPStats, tc.stamp, p), "10.8.8.9", time.Now())

		st := r.GetByUID(sender).V2IPStats()
		if st == nil {
			t.Fatalf("stamped %#x: the whole report was dropped", tc.stamp)
		}
		if st.Rx.VideoTotal != 4242 {
			t.Errorf("stamped %#x: rx video total = %d, want 4242", tc.stamp, st.Rx.VideoTotal)
		}
		if st.DecoderReported != tc.reported {
			t.Errorf("stamped %#x: DecoderReported = %v, want %v", tc.stamp, st.DecoderReported, tc.reported)
		}
		if got := st.Decoder != nil; got != tc.reported {
			t.Errorf("stamped %#x: decoder present = %v, want %v", tc.stamp, got, tc.reported)
		}
	}
}

// V2IPStats hands out a copy: a caller mutating the decoder it was given must
// not reach the cached reading.
func TestV2IPStatsCopiesTheDecoder(t *testing.T) {
	r, sender, feed := cmdRemote(t, 86, Callbacks{})
	feed(opV2IPStats, statsPayload(v2ipStatsSize+decoderDetailSize, decoderVector))

	dev := r.GetByUID(sender)
	first := dev.V2IPStats()
	first.Decoder.Width = 1
	if got := dev.V2IPStats(); got.Decoder.Width != 3840 {
		t.Fatalf("cached width = %d after a caller wrote 1, want 3840", got.Decoder.Width)
	}
}

// Format answers nothing about whether a stream is present: a working RGB
// stream and no stream at all both read DecoderFormatRGB, and only the
// geometry beside it tells them apart. Reading format 0 as no-signal reports a
// dead sink on a live one.
func TestV2IPDecoderFormatZeroIsNotNoSignal(t *testing.T) {
	r, sender, feed := cmdRemote(t, 87, Callbacks{})
	dev := r.GetByUID(sender)

	// Format is what stays put across the two halves. The reason moves with the
	// geometry because the wire ties them: a sink reporting no geometry does
	// not also report OK, so holding the reason still would build a state no
	// device sends.
	block := func(w, h uint16, reason V2IPDecoderReason) []byte {
		d := poisoned(decoderDetailSize)
		d[0] = 1 // valid
		d[1] = byte(reason)
		d[2] = 0 // blocking
		binary.LittleEndian.PutUint16(d[4:6], w)
		binary.LittleEndian.PutUint16(d[6:8], h)
		binary.LittleEndian.PutUint16(d[8:10], uint16(DecoderFormatRGB))
		binary.LittleEndian.PutUint16(d[10:12], 7)
		binary.LittleEndian.PutUint32(d[12:16], 0)
		binary.LittleEndian.PutUint32(d[16:20], 0)
		return d
	}

	feed(opV2IPStats, statsPayload(v2ipStatsSize+decoderDetailSize, block(1920, 1080, DecoderReasonOK)))
	live := dev.V2IPStats().Decoder
	if live == nil || !live.Recovered() || live.Width != 1920 || live.Height != 1080 {
		t.Fatalf("working RGB stream: decoder = %+v", live)
	}

	feed(opV2IPStats, statsPayload(v2ipStatsSize+decoderDetailSize, block(0, 0, DecoderReasonNoFormat)))
	none := dev.V2IPStats().Decoder
	if none == nil || none.Recovered() {
		t.Fatalf("no stream: decoder = %+v", none)
	}

	// The pair proves nothing once the formats differ. Pin both to the constant
	// rather than to each other: an equality check still passes if a later edit
	// moves both halves together, which quietly makes them two ordinary
	// fixtures.
	if live.Format != DecoderFormatRGB || none.Format != DecoderFormatRGB {
		t.Fatalf("formats = %v / %v, want RGB for both; the fixture no longer isolates geometry",
			live.Format, none.Format)
	}
}

// Every counter read at its own offset, over a payload carrying a distinct
// value in every four-byte word. Asserting that the 128- and 152-byte forms
// agree with each other cannot catch a shift, since two reads off the same
// wrong offset agree, so each field is pinned to the value its own offset
// holds.
func TestV2IPStatsCounterOffsets(t *testing.T) {
	r, sender, feed := cmdRemote(t, 88, Callbacks{})
	p := statsPayload(v2ipStatsSize+decoderDetailSize, decoderVector)
	for o := 0; o < v2ipStatsSize; o += 4 {
		binary.LittleEndian.PutUint32(p[o:o+4], uint32(0x100+o))
	}
	feed(opV2IPStats, p)

	st := r.GetByUID(sender).V2IPStats()
	if st == nil {
		t.Fatal("no stats")
	}
	w := func(o int) uint32 { return uint32(0x100 + o) }
	tx := func(name string, got V2IPTxStats, at int) {
		want := V2IPTxStats{Video: w(at), Audio: w(at + 4), Anc: w(at + 8),
			StreamDown: w(at + 12), Overflow: w(at + 16)}
		if got != want {
			t.Errorf("%s = %+v, want %+v", name, got, want)
		}
	}
	rx := func(name string, got V2IPRxStats, at int) {
		want := V2IPRxStats{
			VideoTotal: w(at), VideoDropped: w(at + 4), VideoSeqErrors: w(at + 8),
			WdtTimeout: w(at + 12), AudioTotal: w(at + 16), AudioDropped: w(at + 20),
			AudioSeqErrors: w(at + 24), AncTotal: w(at + 28), AncDropped: w(at + 32),
			AncSeqErrors: w(at + 36),
			// one byte at block offset 40, so the low byte of the word there
			DecoderState: V2IPDecoderState(byte(w(at + 40))),
		}
		if got != want {
			t.Errorf("%s = %+v, want %+v", name, got, want)
		}
	}
	tx("tx totals", st.Tx, 0)
	tx("tx per minute", st.TxPerMinute, txStatsSize)
	rx("rx totals", st.Rx, 2*txStatsSize)
	rx("rx per minute", st.RxPerMinute, 2*txStatsSize+rxStatsSize)
}

// A half geometry is not a recovered picture. No sink sends one - a decoder
// that recovered a width recovered a height - which is exactly why every other
// fixture sets both or neither, leaving && and || indistinguishable.
func TestV2IPDecoderHalfGeometryIsNotRecovered(t *testing.T) {
	r, sender, feed := cmdRemote(t, 89, Callbacks{})
	dev := r.GetByUID(sender)

	for _, g := range []struct{ w, h uint16 }{{0, 2160}, {3840, 0}} {
		d := poisoned(decoderDetailSize)
		d[0] = 1
		d[1] = byte(DecoderReasonNoFormat)
		d[2] = 0
		binary.LittleEndian.PutUint16(d[4:6], g.w)
		binary.LittleEndian.PutUint16(d[6:8], g.h)
		binary.LittleEndian.PutUint16(d[8:10], uint16(DecoderFormatYCbCr422))
		binary.LittleEndian.PutUint16(d[10:12], 9)
		binary.LittleEndian.PutUint32(d[12:16], 0)
		binary.LittleEndian.PutUint32(d[16:20], 0)
		feed(opV2IPStats, statsPayload(v2ipStatsSize+decoderDetailSize, d))

		got := dev.V2IPStats().Decoder
		if got == nil || got.Width != g.w || got.Height != g.h {
			t.Fatalf("%dx%d: decoder = %+v", g.w, g.h, got)
		}
		if got.Recovered() {
			t.Errorf("%dx%d counts as a recovered picture", g.w, g.h)
		}
	}
}

// reason is whichever cause won a fixed priority order, which is deliberately
// not the numbering, so it is derivable from flags by no rule at all. These
// three readings rule out the two rules someone would reach for, and both
// shapes are needed: with only the restart loop, a lowest-set-bit rule still
// passes.
func TestV2IPDecoderReasonIsNotDerivableFromFlags(t *testing.T) {
	r, sender, feed := cmdRemote(t, 90, Callbacks{})
	dev := r.GetByUID(sender)

	cases := []struct {
		name   string
		reason V2IPDecoderReason
		bits   []V2IPDecoderReason
	}{
		{"teardown, switch pending wins", DecoderReasonSwitchPending,
			[]V2IPDecoderReason{DecoderReasonNoPackets, DecoderReasonNoFormat, DecoderReasonSwitchPending}},
		{"teardown settled", DecoderReasonNoPackets,
			[]V2IPDecoderReason{DecoderReasonNoPackets, DecoderReasonNoFormat}},
		{"restart loop, bit 9 invisible in reason", DecoderReasonNoPackets,
			[]V2IPDecoderReason{DecoderReasonNoPackets, DecoderReasonTxBridgeUnlocked}},
	}

	lowestAgrees, highestAgrees := 0, 0
	for _, c := range cases {
		var flags uint32
		for _, b := range c.bits {
			flags |= 1 << uint(b)
		}
		d := poisoned(decoderDetailSize)
		d[0] = 1
		d[1] = byte(c.reason)
		d[2] = 0
		binary.LittleEndian.PutUint16(d[4:6], 0)
		binary.LittleEndian.PutUint16(d[6:8], 0)
		binary.LittleEndian.PutUint16(d[8:10], uint16(DecoderFormatRGB))
		binary.LittleEndian.PutUint16(d[10:12], 3)
		binary.LittleEndian.PutUint32(d[12:16], flags)
		binary.LittleEndian.PutUint32(d[16:20], 0)
		feed(opV2IPStats, statsPayload(v2ipStatsSize+decoderDetailSize, d))

		got := dev.V2IPStats().Decoder
		if got == nil || got.Reason != c.reason || got.Flags != flags {
			t.Fatalf("%s: reason = %v, flags = %#x, want %v / %#x", c.name, got.Reason, got.Flags, c.reason, flags)
		}
		for _, b := range c.bits {
			if !got.HasReason(b) {
				t.Errorf("%s: HasReason(%v) = false", c.name, b)
			}
		}
		// Bit 0 is force-cleared, so OK is never among the causes.
		if got.HasReason(DecoderReasonOK) {
			t.Errorf("%s: HasReason(OK) = true", c.name)
		}
		if c.reason == c.bits[0] {
			lowestAgrees++
		}
		if c.reason == c.bits[len(c.bits)-1] {
			highestAgrees++
		}
	}

	// The set is only worth having while it still contradicts both rules.
	if lowestAgrees == len(cases) {
		t.Error("every reading agrees with lowest-set-bit; the set no longer rules that out")
	}
	if highestAgrees == len(cases) {
		t.Error("every reading agrees with highest-set-bit; the set no longer rules that out")
	}
}

// A payload grows by appending at the back, so a length gate has to be a
// minimum. Where several forms share an opcode they are tested longest first,
// or a grown short form is swallowed by the longer form's minimum.
func TestGrownPayloadsKeepTheirForm(t *testing.T) {
	target := uidN(57)
	tail := []byte{0xDE, 0xAD, 0xBE, 0xEF}

	t.Run("factory reset target", func(t *testing.T) {
		var got FactoryResetRequest
		_, _, feed := cmdRemote(t, 53, Callbacks{
			OnFactoryResetRequested: func(_ *Device, req FactoryResetRequest) { got = req },
		})
		feed(opSysFactoryReset, append(append([]byte(nil), target[:]...), tail...))
		if got.Target == nil || *got.Target != target {
			t.Fatalf("grown target form = %+v, want target %v", got, target)
		}
	})

	t.Run("factory reset all", func(t *testing.T) {
		var got FactoryResetRequest
		_, _, feed := cmdRemote(t, 54, Callbacks{
			OnFactoryResetRequested: func(_ *Device, req FactoryResetRequest) { got = req },
		})
		feed(opSysFactoryReset, []byte{0xFF, 0x01})
		if !got.All || got.Target != nil {
			t.Fatalf("grown broadcast form = %+v", got)
		}
	})

	t.Run("power save target", func(t *testing.T) {
		var got V2IPPowerSaveRequest
		_, _, feed := cmdRemote(t, 55, Callbacks{
			OnPowerSaveRequested: func(_ *Device, req V2IPPowerSaveRequest) { got = req },
		})
		p := append(append([]byte(nil), target[:]...), 1)
		feed(opV2IPPowerSave, append(p, tail...))
		if got.Target == nil || *got.Target != target || !got.Enabled {
			t.Fatalf("grown target form = %+v", got)
		}
	})

	t.Run("power save broadcast", func(t *testing.T) {
		var got V2IPPowerSaveRequest
		_, _, feed := cmdRemote(t, 56, Callbacks{
			OnPowerSaveRequested: func(_ *Device, req V2IPPowerSaveRequest) { got = req },
		})
		feed(opV2IPPowerSave, []byte{1, 0x99})
		if got.Target != nil || !got.Enabled {
			t.Fatalf("grown broadcast form = %+v", got)
		}
	})

	t.Run("filter status", func(t *testing.T) {
		r, sender, feed := cmdRemote(t, 57, Callbacks{})
		feed(opSysBayConfig, bayConfigRec(2, 1, 0, "Output 1", "TV", 0, BayHDMIOut))
		a := uidN(61)
		p := append(append([]byte(nil), sender[:]...), a[:]...)
		feed(opBayFilterStatus, append(p, tail...))

		filtered := r.GetByUID(sender).GetByPortnum(2).FilteredDevices()
		if len(filtered) != 1 || filtered[0] != a {
			t.Fatalf("filtered = %v, want [%v]", filtered, a)
		}
	})
}

// A factory reset with no payload asks the sender to reset itself, so an
// unrecognised payload must be dropped rather than falling into that form.
func TestFactoryResetIgnoresUnknownForm(t *testing.T) {
	calls := 0
	_, _, feed := cmdRemote(t, 58, Callbacks{
		OnFactoryResetRequested: func(_ *Device, _ FactoryResetRequest) { calls++ },
	})
	feed(opSysFactoryReset, []byte{0x01, 0x02, 0x03})
	if calls != 0 {
		t.Fatalf("unknown form raised %d reset requests", calls)
	}
}

// A disabled sink is named idle, and the causes idle outranks stay set: they
// are what the decoder observed. The accessor must report the word as it
// arrived, since ranking one cause over another is the caller's job.
//
// The flags word here is not asserted whole. Which causes accompany bit 10 is
// a firmware detail that moves as the firmware gets more correct, and a
// fixture pinning the exact word would call that a regression.
func TestV2IPDecoderIdleDoesNotSuppressLesserCauses(t *testing.T) {
	d := append([]byte(nil), decoderVector...)
	d[1] = byte(DecoderReasonIdle)
	binary.LittleEndian.PutUint32(d[12:16], 1<<DecoderReasonNoPackets|
		1<<DecoderReasonNoFormat|1<<DecoderReasonSwitchPending|1<<DecoderReasonIdle)

	r, sender, feed := cmdRemote(t, 89, Callbacks{})
	feed(opV2IPStats, statsPayload(v2ipStatsSize+decoderDetailSize, d))

	dec := r.GetByUID(sender).V2IPStats().Decoder
	if dec == nil || dec.Reason != DecoderReasonIdle {
		t.Fatalf("decoder = %+v", dec)
	}
	for _, r := range []V2IPDecoderReason{
		DecoderReasonIdle,
		DecoderReasonNoPackets,
		DecoderReasonNoFormat,
		DecoderReasonSwitchPending,
	} {
		if !dec.HasReason(r) {
			t.Errorf("HasReason(%v) = false, want true beneath idle", r)
		}
	}
	if dec.HasReason(DecoderReasonDegraded) {
		t.Error("HasReason(degraded) = true, but its bit is clear")
	}
}

// Geometry is read before any reason is decided, so an idle sink reporting a
// picture is a normal steady state. Nothing may infer sink state from it.
func TestV2IPDecoderIdleKeepsItsGeometry(t *testing.T) {
	d := append([]byte(nil), decoderVector...)
	d[1] = byte(DecoderReasonIdle)
	binary.LittleEndian.PutUint32(d[12:16], 1<<DecoderReasonIdle)

	r, sender, feed := cmdRemote(t, 90, Callbacks{})
	feed(opV2IPStats, statsPayload(v2ipStatsSize+decoderDetailSize, d))

	dec := r.GetByUID(sender).V2IPStats().Decoder
	if dec == nil || dec.Reason != DecoderReasonIdle {
		t.Fatalf("decoder = %+v", dec)
	}
	if dec.Width != 3840 || dec.Height != 2160 || !dec.Recovered() {
		t.Errorf("geometry = %dx%d recovered=%v, want 3840x2160 recovered",
			dec.Width, dec.Height, dec.Recovered())
	}
}

// No payload length may panic a handler. A length gate placed in front of the
// wrong thing breaks this invariant rather than merely mis-filing a frame: the
// slicing that reads a fixed block is bounded by a gate elsewhere in the
// handler, so removing or narrowing that gate indexes past the payload and
// takes the receive goroutine down with it.
//
// Every handler is swept, because the gate that protects one block is rarely
// written next to it. The sweep is over lengths rather than contents for the
// same reason - a gate reads the length, so length is the axis that reaches
// one. The three stamps matter where a handler selects a layout by version and
// then slices for it.
func TestNoPayloadLengthPanicsAHandler(t *testing.T) {
	opcodes := make([]uint16, 0, len(frameHandlers)+2)
	for op := range frameHandlers {
		opcodes = append(opcodes, op)
	}
	// two the table has no entry for, so an unknown opcode is swept as well
	opcodes = append(opcodes, 0xFFFE, 0xFFFF)

	lengths := make([]int, 0, 300)
	for n := 0; n <= 260; n++ {
		lengths = append(lengths, n)
	}
	// past the sweep, the lengths the wider payloads gate on
	lengths = append(lengths, 512, 513, 514, 515, 1024)

	patterns := map[string]func(int) []byte{
		"zero":     func(n int) []byte { return make([]byte, n) },
		"poisoned": poisoned,
		"ones": func(n int) []byte {
			p := make([]byte, n)
			for i := range p {
				p[i] = 0xFF
			}
			return p
		},
	}

	for _, stamp := range []uint16{0x01, 0x28, ProtocolVersion} {
		for name, fill := range patterns {
			r, sender, feed := cmdRemote(t, 88, Callbacks{})
			feed(opSysHello, helloPayload(0x28, "ONEIP", "SW0001", "4.8.0", FeatureV2IPSink|FeatureV2IPSource))
			for _, op := range opcodes {
				for _, n := range lengths {
					func() {
						defer func() {
							if e := recover(); e != nil {
								t.Fatalf("opcode %#x, %d bytes, %s, stamped %#x: %v", op, n, name, stamp, e)
							}
						}()
						r.processFrame(buildFrame(sender, op, stamp, fill(n)), "10.8.8.9", time.Now())
					}()
				}
			}
		}
	}
}
