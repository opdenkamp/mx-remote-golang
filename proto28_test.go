// Author: Lars Op den Kamp (lars@opdenkamp-it.nl)
// Copyright (c) 2026 Op den Kamp IT Solutions

package mxremote

import (
	"encoding/binary"
	"strings"
	"testing"
	"time"
)

// v2ipCfg is a v2ip_device_config_update payload builder. Every field goes in
// verbatim so a caller can send the zeroed blocks and out-of-range rate a
// controller writing one field does.
type v2ipCfg struct {
	uid                            DeviceUID
	videoIP, audioIP, ancIP, arcIP string
	videoPort, audioPort, ancPort  int
	arcPort                        int
	rate, dscpVideo, dscpAudio     byte
	dscpAnc                        byte
	mode, refresh                  uint16
	flags                          byte
}

func (c v2ipCfg) bytes() []byte {
	p := make([]byte, 88)
	copy(p[0:16], c.uid[:])
	put := func(off int, ip string, port int) {
		if ip == "" {
			return
		}
		copy(p[off:off+4], ip4Bytes(ip))
		binary.LittleEndian.PutUint16(p[off+4:off+6], uint16(port))
	}
	put(16, c.videoIP, c.videoPort)
	put(24, c.audioIP, c.audioPort)
	put(32, c.ancIP, c.ancPort)
	p[40] = c.rate
	p[41] = c.dscpVideo
	p[42] = c.dscpAudio
	p[43] = c.dscpAnc
	put(48, c.arcIP, c.arcPort)
	binary.LittleEndian.PutUint16(p[56:58], c.mode)
	binary.LittleEndian.PutUint16(p[58:60], c.refresh)
	p[60] = c.flags
	return p
}

// addresses is a source block that passes mxr_v2ip_av_source_valid.
func addresses(uid DeviceUID, base string) v2ipCfg {
	return v2ipCfg{
		uid:     uid,
		videoIP: base, videoPort: V2IPPortVideo,
		audioIP: base, audioPort: V2IPPortAudio,
		ancIP: base, ancPort: V2IPPortANC,
	}
}

func dscpByte(v byte) byte { return V2IPDscpSet | v }

func v2ipRemote(t *testing.T, n byte) (*Remote, DeviceUID, func(uint16, []byte)) {
	t.Helper()
	r := newTestRemote(Callbacks{})
	sender := uidN(n)
	feed := feeder(r, sender)
	feed(opSysHello, helloPayload(0x28, "ONEIP-TX", "TX0001", "4.8.0", FeatureV2IPSource))
	return r, sender, feed
}

func TestV2IPDeviceCfgDscpAndRate(t *testing.T) {
	r, sender, feed := v2ipRemote(t, 20)

	full := addresses(sender, "239.1.2.3")
	full.rate = 40
	full.dscpVideo, full.dscpAudio, full.dscpAnc = dscpByte(34), dscpByte(46), dscpByte(0)
	feed(opV2IPDeviceCfg, full.bytes())

	det := r.GetByUID(sender).V2IPDetails()
	if det.TxRate == nil || *det.TxRate != 40 {
		t.Fatalf("tx rate = %v, want 40", det.TxRate)
	}
	if !det.Dscp.Complete() {
		t.Fatalf("dscp = %v, want complete", det.Dscp)
	}
	// DSCP 0 (CS0) is a legal marking: the set bit, not the value, says present
	if *det.Dscp.Video != 34 || *det.Dscp.Audio != 46 || *det.Dscp.Anc != 0 {
		t.Fatalf("dscp = %v, want video:34 audio:46 anc:0", det.Dscp)
	}

	// an address-only write: zeroed rate and dscp bytes, which must leave the
	// cached rate and marking alone rather than clearing them
	feed(opV2IPDeviceCfg, addresses(sender, "239.9.9.9").bytes())

	det = r.GetByUID(sender).V2IPDetails()
	if det.Video.IP != "239.9.9.9" {
		t.Fatalf("video ip = %q, want 239.9.9.9", det.Video.IP)
	}
	if det.TxRate == nil || *det.TxRate != 40 {
		t.Fatalf("tx rate after address-only write = %v, want 40", det.TxRate)
	}
	if !det.Dscp.Complete() || *det.Dscp.Audio != 46 {
		t.Fatalf("dscp after address-only write = %v, want video:34 audio:46 anc:0", det.Dscp)
	}
}

func TestV2IPDeviceCfgRateOnlyKeepsAddresses(t *testing.T) {
	r, sender, feed := v2ipRemote(t, 21)
	feed(opV2IPDeviceCfg, addresses(sender, "239.1.2.3").bytes())

	// a rate-only write zeroes every address block; the peer keeps the ones it
	// already had, so reporting 0.0.0.0 here would be wrong
	feed(opV2IPDeviceCfg, v2ipCfg{uid: sender, rate: 40}.bytes())

	det := r.GetByUID(sender).V2IPDetails()
	if det.Video.IP != "239.1.2.3" || det.Anc.IP != "239.1.2.3" {
		t.Fatalf("addresses after rate-only write = %v / %v", det.Video, det.Anc)
	}
	if det.TxRate == nil || *det.TxRate != 40 {
		t.Fatalf("tx rate = %v, want 40", det.TxRate)
	}
}

func TestV2IPStreamSourceValidity(t *testing.T) {
	cases := []struct {
		name string
		src  V2IPStreamSource
		want bool
	}{
		{"multicast with port", V2IPStreamSource{IP: "239.1.2.3", Port: V2IPPortVideo}, true},
		{"unicast", V2IPStreamSource{IP: "10.8.8.9", Port: V2IPPortVideo}, false},
		{"multicast port 0", V2IPStreamSource{IP: "239.1.2.3", Port: 0}, false},
		{"zero", V2IPStreamSource{IP: "0.0.0.0", Port: 0}, false},
	}
	for _, c := range cases {
		if got := c.src.Valid(); got != c.want {
			t.Fatalf("%s: Valid() = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestV2IPDeviceCfgRejectsUnusableAddresses(t *testing.T) {
	r, sender, feed := v2ipRemote(t, 22)
	feed(opV2IPDeviceCfg, addresses(sender, "239.1.2.3").bytes())

	// a unicast video address fails mxr_v2ip_stream_valid, so the whole source
	// block is dropped rather than half-applied
	unicast := addresses(sender, "239.4.5.6")
	unicast.videoIP = "10.8.8.9"
	feed(opV2IPDeviceCfg, unicast.bytes())
	if got := r.GetByUID(sender).V2IPDetails().Anc.IP; got != "239.1.2.3" {
		t.Fatalf("anc ip after unicast video = %q, want 239.1.2.3", got)
	}

	// so does a multicast address with no port
	noPort := addresses(sender, "239.4.5.6")
	noPort.ancPort = 0
	feed(opV2IPDeviceCfg, noPort.bytes())
	if got := r.GetByUID(sender).V2IPDetails().Video.IP; got != "239.1.2.3" {
		t.Fatalf("video ip after port-0 anc = %q, want 239.1.2.3", got)
	}
}

func TestV2IPScalingMergesPerField(t *testing.T) {
	r, sender, feed := v2ipRemote(t, 23)

	both := addresses(sender, "239.1.2.3")
	both.mode, both.refresh = 16, 60
	both.flags = ScalingFlagModeValid | ScalingFlagOptionsValid | ScalingFlagAutoScaling
	feed(opV2IPDeviceCfg, both.bytes())

	// an options-only write carries no mode, so the peer keeps its resolution
	optionsOnly := v2ipCfg{uid: sender, flags: ScalingFlagOptionsValid}
	feed(opV2IPDeviceCfg, optionsOnly.bytes())

	sc := r.GetByUID(sender).V2IPDetails().Scaling
	if sc.Mode.Svd() != 16 || sc.Refresh != 60 {
		t.Fatalf("scaling after options-only write = %v / %d", sc.Mode, sc.Refresh)
	}
	// the options branch replaces the whole top nibble, so this does clear
	// auto-scaling
	if sc.Flags&ScalingFlagAutoScaling != 0 {
		t.Fatalf("auto scaling still set after an options-only write clearing it")
	}

	// a mode-only write leaves the options nibble alone
	modeOnly := v2ipCfg{uid: sender, mode: 31, refresh: 50, flags: ScalingFlagModeValid}
	feed(opV2IPDeviceCfg, modeOnly.bytes())

	sc = r.GetByUID(sender).V2IPDetails().Scaling
	if sc.Mode.Svd() != 31 || sc.Refresh != 50 {
		t.Fatalf("scaling after mode-only write = %v / %d", sc.Mode, sc.Refresh)
	}
	if sc.Flags&ScalingFlagOptionsValid == 0 {
		t.Fatalf("options-valid lost after a mode-only write: flags = %#02x", sc.Flags)
	}
}

func TestV2IPDeviceCfgPartialDscpIsNoMarking(t *testing.T) {
	r, sender, feed := v2ipRemote(t, 24)

	// firmware stores all three bytes behind the video byte's set bit, but
	// applies a marking only when all three carry one
	c := addresses(sender, "239.1.2.3")
	c.dscpVideo, c.dscpAudio = dscpByte(34), dscpByte(46)
	feed(opV2IPDeviceCfg, c.bytes())

	det := r.GetByUID(sender).V2IPDetails()
	if det.Dscp.Complete() {
		t.Fatalf("dscp = %v, want incomplete", det.Dscp)
	}
	if det.Dscp.Anc != nil {
		t.Fatalf("anc dscp = %d, want nil", *det.Dscp.Anc)
	}
}

func TestBayConfigPagesMerge(t *testing.T) {
	r := newTestRemote(Callbacks{})
	sender := uidN(22)
	feed := feeder(r, sender)
	feed(opSysHello, helloPayload(0x28, "FF88", "PG0001", "4.8.0", FeatureVideoRouting))

	// a device splits its bays across frames sized against the MTU; the second
	// page must not displace the first
	feed(opSysBayConfig, append(bayConfigRec(1, 0, 0, "Input 1", "Apple TV", 0, BayHDMIIn),
		bayConfigRec(2, 0, 1, "Input 2", "Blu-ray", 0, BayHDMIIn)...))
	feed(opSysBayConfig, bayConfigRec(3, 1, 0, "Output 1", "TV", 0, BayHDMIOut))

	dev := r.GetByUID(sender)
	for _, port := range []int{1, 2, 3} {
		if dev.GetByPortnum(port) == nil {
			t.Fatalf("bay on port %d missing after paged config", port)
		}
	}
}

func TestLinkPagesMerge(t *testing.T) {
	r := newTestRemote(Callbacks{})
	sender := uidN(23)
	feed := feeder(r, sender)
	feed(opSysHello, helloPayload(0x28, "FF88", "PG0002", "4.8.0", FeatureVideoRouting))
	feed(opSysBayConfig, append(bayConfigRec(1, 0, 0, "Input 1", "Apple TV", 0, BayHDMIIn),
		bayConfigRec(2, 1, 0, "Output 1", "TV", 0, BayHDMIOut)...))

	linkRec := func(port int, serial, bay string) []byte {
		rec := make([]byte, 38)
		rec[0] = byte(port)
		copy(rec[2:18], serial)
		copy(rec[18:34], bay)
		return rec
	}
	feed(opSysLinks, linkRec(1, "AMP00001", "Zone 1"))
	feed(opSysLinks, linkRec(2, "AMP00001", "Zone 2"))

	dev := r.GetByUID(sender)
	if l := dev.GetByPortnum(1).Link(); l == nil || l.LinkedBayName() != "Zone 1" {
		t.Fatalf("port 1 link lost after a second page: %v", l)
	}
	if l := dev.GetByPortnum(2).Link(); l == nil || l.LinkedBayName() != "Zone 2" {
		t.Fatalf("port 2 link = %v", l)
	}
}

func TestFixedWidthNameFillsField(t *testing.T) {
	r := newTestRemote(Callbacks{})
	sender := uidN(24)
	feed := feeder(r, sender)

	// a 16-character name fills MXR_DEVICE_NAME_LEN with no room for a
	// terminator; reading it must stop at the field edge, not run into the
	// serial that follows
	full := "0123456789ABCDEF"
	feed(opSysHello, helloPayload(0x28, full, "SN0001", "4.8.0", FeatureVideoRouting))

	dev := r.GetByUID(sender)
	if got := dev.Name(); got != full {
		t.Fatalf("name = %q, want %q", got, full)
	}
	if got := dev.Serial(); got != "SN0001" {
		t.Fatalf("serial = %q, want SN0001", got)
	}
}

func TestNonAsciiNameCostsOneCharacter(t *testing.T) {
	p := []byte("Zone \xff1")
	if got, want := cstr(p), "Zone �1"; got != want {
		t.Fatalf("cstr = %q, want %q", got, want)
	}
	if strings.ContainsRune(cstr(make([]byte, 16)), 0) {
		t.Fatal("an all-NUL field should decode to the empty string")
	}
}

func TestSignalStatusVideoAndBayBlock(t *testing.T) {
	r := newTestRemote(Callbacks{})
	sender := uidN(25)
	feed := feeder(r, sender)
	feed(opSysHello, helloPayload(0x28, "ONEIP-RX", "SS0001", "4.8.0", FeatureVideoRouting))
	feed(opSysBayConfig, bayConfigRec(2, 1, 0, "Output 1", "TV", 0, BayHDMIOut))

	p := make([]byte, avDetailsSize)
	p[2] = 1 << 1 // support_flags: stream_valid
	p[40] = 16    // av_details_video.svd
	p[41] = 0     // colour space: RGB
	p[42] = 8     // colour depth
	// frame_rate is a uint16 at offset 8 of the video block, so a rate above
	// 255Hz must not wrap into the low byte
	binary.LittleEndian.PutUint16(p[48:50], 300)
	binary.LittleEndian.PutUint32(p[50:54], 594000000) // tmds_clock
	binary.LittleEndian.PutUint16(p[100:102], 2)       // av_details_bay.portnum
	binary.LittleEndian.PutUint32(p[102:106], uint32(BayStatusSignalDetected))
	// mxr_signal_type: svd 16, colour 1, bpp index 2 (= 10 bits)
	p[106] = 16
	p[107] = 1 | (2 << 5)
	binary.LittleEndian.PutUint32(p[108:112], 148500000) // clock_rate

	feed(opBaySignalStatus, p)

	d := r.GetByUID(sender).GetByPortnum(2).SignalDetails()
	if d == nil {
		t.Fatal("no signal details")
	}
	if d.FrameRate != 300 {
		t.Fatalf("frame rate = %v, want 300", d.FrameRate)
	}
	if d.TmdsClock != 594000000 {
		t.Fatalf("tmds clock = %d, want 594000000", d.TmdsClock)
	}
	if d.ClockRate != 148500000 {
		t.Fatalf("clock rate = %d, want 148500000", d.ClockRate)
	}
	if !d.Status.Has(BayStatusSignalDetected) {
		t.Fatalf("status = %v, want signal detected", d.Status)
	}
	if d.Scaling.Svd() != 16 || d.Scaling.ColourSpace() != 1 {
		t.Fatalf("scaling = %v", d.Scaling)
	}
	// bpp is an index, not a bit depth: 2 stands for 10
	if got := d.Scaling.Bpp(); got != 10 {
		t.Fatalf("bpp = %d, want 10", got)
	}
	if !d.Scaling.IsSet() {
		t.Fatalf("scaling %v should be set", d.Scaling)
	}
}

func TestSignalStatusShortReportDropped(t *testing.T) {
	r := newTestRemote(Callbacks{})
	sender := uidN(26)
	feed := feeder(r, sender)
	feed(opSysHello, helloPayload(0x28, "ONEIP-RX", "SS0002", "4.8.0", FeatureVideoRouting))
	feed(opSysBayConfig, bayConfigRec(2, 1, 0, "Output 1", "TV", 0, BayHDMIOut))

	// 68..111 bytes is the old AV_DETAILS_MIN_SIZE range: the bay block naming
	// the reporting bay is not there, so the report cannot be attributed
	feed(opBaySignalStatus, make([]byte, 68))

	if d := r.GetByUID(sender).GetByPortnum(2).SignalDetails(); d != nil {
		t.Fatalf("short report was decoded: %+v", d)
	}
}

func TestSignalTypeUnsetSentinel(t *testing.T) {
	unset := MxrSignalType(uint16(sigBppUnset) << 13)
	if unset.IsSet() {
		t.Fatal("bpp index 5 is the unset sentinel")
	}
	if got := unset.String(); got != "unset" {
		t.Fatalf("String = %q, want %q", got, "unset")
	}
	for idx, want := range map[int]int{0: 0, 1: 8, 2: 10, 3: 12, 4: 16} {
		st := MxrSignalType(uint16(idx) << 13)
		if got := st.Bpp(); got != want {
			t.Fatalf("bpp index %d = %d, want %d", idx, got, want)
		}
	}
}

func TestOpcodeProtocolStaysUnderAmpCap(t *testing.T) {
	// a ProAmp8 caps at MXR_PROTOCOL_VERSION 0x22 and drops any frame stamped
	// above its own cap, so the opcodes it handles must stay under it
	for _, op := range []uint16{opSysHello, opSysDiscover, opRCSettings, opV2IPDeviceCfg,
		opV2IPManualSrcSwitch, opAudioSetVolume, opAmpZoneSettings, opAmpDolbyState} {
		if got := protocolFor(op); got > 0x22 {
			t.Fatalf("opcode %#02x stamps protocol %#02x, above the 0x22 cap", op, got)
		}
	}
	if got := protocolFor(opV2IPVideoWall); got != 0x28 {
		t.Fatalf("video wall protocol = %#02x, want 0x28", got)
	}
}

func TestPayloadBoundedByDeclaredLength(t *testing.T) {
	// a datagram carrying more than its header declares must not yield phantom
	// records to a per-record loop
	data := buildFrame(uidN(27), opSysBayConfig, 0x01, make([]byte, bayConfigSize))
	f, err := parseFrame(append(data, make([]byte, bayConfigSize)...), "10.8.8.9", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if got := len(f.payload()); got != bayConfigSize {
		t.Fatalf("payload len = %d, want %d", got, bayConfigSize)
	}
}
