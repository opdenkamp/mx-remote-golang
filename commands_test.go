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

// The audio SELECT_INPUT body names the sink twice - once as the command
// header's target and again at offset 20 - with the source at 36. Decoding
// those the other way round swaps source and sink.
func TestAudioSelectInputOrientation(t *testing.T) {
	var got AudioChangeSource
	r, sender, feed := cmdRemote(t, 60, Callbacks{
		OnAudioSelectInput: func(_ *Device, c AudioChangeSource) { got = c },
	})
	source := uidN(61)

	p := audioCmdHeader(audioOpSelectInput, sender)
	p = append(p, sender[:]...) // sink, repeated after the header
	p = append(p, source[:]...)
	p = append(p, 7, 0, 9, 0) // sink endpoint 7, source endpoint 9
	feed(opV2IPAudio, p)

	if got.TargetUID != sender || got.TargetID != 7 {
		t.Fatalf("sink = %s:%d, want %s:7", got.TargetUID, got.TargetID, sender)
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

// A real 0x3C from a 10.12.32-1 unit, captured off a live mesh. Expected values
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
