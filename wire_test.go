// Author: Lars Op den Kamp (lars@opdenkamp-it.nl)
// Copyright (c) 2026 Op den Kamp IT Solutions

package mxremote

import (
	"encoding/hex"
	"testing"
)

var testUID = func() DeviceUID {
	var u DeviceUID
	for i := range u {
		u[i] = byte(i)
	}
	return u
}()

func hexOf(b []byte) string { return hex.EncodeToString(b) }

func TestUIDString(t *testing.T) {
	if got, want := testUID.String(), "03020100.07060504.0b0a0908.0f0e0d0c"; got != want {
		t.Fatalf("String() = %q, want %q", got, want)
	}
	parsed, err := ParseDeviceUID(testUID.String())
	if err != nil {
		t.Fatal(err)
	}
	if parsed != testUID {
		t.Fatalf("round trip = %x, want %x", parsed, testUID)
	}
}

func TestHelloFrame(t *testing.T) {
	payload := make([]byte, 0, 54)
	payload = append(payload, byte(ProtocolVersion&0xFF), byte(ProtocolVersion>>8))
	payload = appendFixedStr(payload, "TestApp", 16)
	payload = appendFixedStr(payload, "P9SN00000000", 16)
	payload = appendFixedStr(payload, Version, 16)
	feat := uint32(FeatureManager)
	payload = append(payload, byte(feat), byte(feat>>8), byte(feat>>16), byte(feat>>24))
	got := hexOf(buildFrame(testUID, opSysHello, protocolFor(opSysHello), payload))
	want := "50380100000102030405060708090a0b0c0d0e0f000036002900546573744170700000000000000000005039534e303030303030303000000000322e312e33000000000000000000000000000800"
	if got != want {
		t.Fatalf("hello\n got=%s\nwant=%s", got, want)
	}
}

func TestDiscoverFrame(t *testing.T) {
	got := hexOf(buildFrame(testUID, opSysDiscover, protocolFor(opSysDiscover), nil))
	want := "50380100000102030405060708090a0b0c0d0e0f01000000"
	if got != want {
		t.Fatalf("discover\n got=%s\nwant=%s", got, want)
	}
}

func TestSetNameFrame(t *testing.T) {
	port := 5
	payload := append([]byte(nil), testUID[:]...)
	payload = append(payload, byte(port), byte(port>>8))
	payload = appendFixedStr(payload, "Living Room", 16)
	got := hexOf(buildFrame(testUID, opChangeBayName, protocolFor(opChangeBayName), payload))
	want := "50380600000102030405060708090a0b0c0d0e0f22002200000102030405060708090a0b0c0d0e0f05004c6976696e6720526f6f6d0000000000"
	if got != want {
		t.Fatalf("setname\n got=%s\nwant=%s", got, want)
	}
}

func TestVolumeSetFrame(t *testing.T) {
	port := 5
	f := false
	vol := VolumeMuteStatus{VolumeLeft: 40, VolumeRight: 40, MutedLeft: &f, MutedRight: &f}
	if got, want := hexOf(vol.wire()), "282800"; got != want {
		t.Fatalf("vol.wire = %s, want %s", got, want)
	}
	payload := append([]byte(nil), testUID[:]...)
	payload = append(payload, byte(port), byte(port>>8))
	payload = append(payload, vol.wire()...)
	payload = append(payload, 0, 0, 0)
	got := hexOf(buildFrame(testUID, opAudioSetVolume, protocolFor(opAudioSetVolume), payload))
	want := "50381100000102030405060708090a0b0c0d0e0f14001800000102030405060708090a0b0c0d0e0f0500282800000000"
	if got != want {
		t.Fatalf("volset\n got=%s\nwant=%s", got, want)
	}
}

func TestManualSourceSwitchFrame(t *testing.T) {
	fmt := &V2IPAudioFormat{SampleRate: 48000, Channels: 2}
	payload := buildV2IPManualSourceSwitch(testUID, "239.1.2.3", 50020, "239.1.2.4", 50022, "0.0.0.0", 0, fmt)
	got := hexOf(buildFrame(testUID, opV2IPManualSrcSwitch, protocolFor(opV2IPManualSrcSwitch), payload))
	want := "50380700000102030405060708090a0b0c0d0e0f24003000000102030405060708090a0b0c0d0e0fef01020364c30000ef01020466c30000000000000000000080bb000002000000"
	if got != want {
		t.Fatalf("manualsw\n got=%s\nwant=%s", got, want)
	}
}

func TestEdidFrame(t *testing.T) {
	profile := Edid4KHDR71
	payload := append([]byte(nil), testUID[:]...)
	payload = append(payload, byte(profile), byte(profile>>8), 0, 0, 0, 0, 0, 0)
	got := hexOf(buildFrame(testUID, opBayEDIDProfile, protocolFor(opBayEDIDProfile), payload))
	want := "50380800000102030405060708090a0b0c0d0e0f34001800000102030405060708090a0b0c0d0e0f0800000000000000"
	if got != want {
		t.Fatalf("edid\n got=%s\nwant=%s", got, want)
	}
}

func TestRebootFrame(t *testing.T) {
	got := hexOf(buildFrame(testUID, opSysReboot, protocolFor(opSysReboot), testUID[:]))
	want := "50380100000102030405060708090a0b0c0d0e0f28001000000102030405060708090a0b0c0d0e0f"
	if got != want {
		t.Fatalf("reboot\n got=%s\nwant=%s", got, want)
	}
}

func TestRCActionFrame(t *testing.T) {
	port, action := 5, ActionPowerOn
	payload := append([]byte(nil), testUID[:]...)
	payload = append(payload, byte(port), byte(port>>8), byte(action), byte(uint16(action)>>8))
	got := hexOf(buildFrame(testUID, opRCTxAction, protocolFor(opRCTxAction), payload))
	want := "50380c00000102030405060708090a0b0c0d0e0f0e001400000102030405060708090a0b0c0d0e0f05000100"
	if got != want {
		t.Fatalf("rcaction\n got=%s\nwant=%s", got, want)
	}
}

func TestMultiviewerTxFrames(t *testing.T) {
	vm := hexOf(buildFrame(testUID, opV2IPMultiviewer, 0x20, mvCmdPayload(testUID, mvOpViewMode, byte(MVViewPIP))))
	if want := "50382000000102030405060708090a0b0c0d0e0f42001900000102030405060708090a0b0c0d0e0f010000000000000002"; vm != want {
		t.Fatalf("mv viewmode\n got=%s\nwant=%s", vm, want)
	}
	av := hexOf(buildFrame(testUID, opV2IPMultiviewer, 0x20, mvCmdPayload(testUID, mvOpAudioVolume, 42, 1)))
	if want := "50382000000102030405060708090a0b0c0d0e0f42001a00000102030405060708090a0b0c0d0e0f04000000000000002a01"; av != want {
		t.Fatalf("mv audiovol\n got=%s\nwant=%s", av, want)
	}
}

func TestAudioTxFrames(t *testing.T) {
	mute := hexOf(buildFrame(testUID, opV2IPAudio, protocolFor(opV2IPAudio), append(audioCmdHeader(audioOpMute, testUID), audioParam(3, 1)...)))
	if want := "50381a00000102030405060708090a0b0c0d0e0f43001c0001000000000102030405060708090a0b0c0d0e0f0300000001000000"; mute != want {
		t.Fatalf("audio mute\n got=%s\nwant=%s", mute, want)
	}
	vol := hexOf(buildFrame(testUID, opV2IPAudio, protocolFor(opV2IPAudio), append(audioCmdHeader(audioOpVolume, testUID), audioParam(5, 80)...)))
	if want := "50381a00000102030405060708090a0b0c0d0e0f43001c0004000000000102030405060708090a0b0c0d0e0f0500000050000000"; vol != want {
		t.Fatalf("audio vol\n got=%s\nwant=%s", vol, want)
	}
}

// The expected bytes here are laid out from the C declaration field by field,
// not produced by buildAmpZoneSettings - a vector taken from the builder only
// proves the builder agrees with itself, which is how the delays sat at the
// wrong offset in two libraries at once.
func TestAmpZoneSettingsTxFrame(t *testing.T) {
	s := AmpZoneSettings{
		GainLeft: 10, GainRight: 11, VolumeMin: 0, VolumeMax: 100,
		DelayLeft: 5, DelayRight: 6, Bass: 3, Treble: 4, Bridged: 1,
		PowerMode: 2, PowerLevel: 7, PowerTimeout: 300,
		EQLeft:  [5]int{1, 2, 3, 4, 5},
		EQRight: [5]int{6, 7, 8, 9, 10},
	}
	got := hexOf(buildFrame(testUID, opAmpZoneSettings, protocolFor(opAmpZoneSettings), buildAmpZoneSettings(testUID, 5, s)))
	want := "50381c00000102030405060708090a0b0c0d0e0f3d003800000102030405060708090a0b0c0d0e0f05000a0b00640000050000000600000003040102070000002c0100000102030405060708090a0000"
	if got != want {
		t.Fatalf("ampzone\n got=%s\nwant=%s", got, want)
	}
}

func TestStatsRequestTxFrame(t *testing.T) {
	payload := append(append([]byte(nil), testUID[:]...), 1)
	got := hexOf(buildFrame(testUID, opV2IPStats, protocolFor(opV2IPStats), payload))
	want := "50381300000102030405060708090a0b0c0d0e0f3f001100000102030405060708090a0b0c0d0e0f01"
	if got != want {
		t.Fatalf("statsreq\n got=%s\nwant=%s", got, want)
	}
}

func TestNewConnUnknownInterface(t *testing.T) {
	c, err := newConn(MulticastIP, 0, "", "no_such_iface0")
	if err == nil {
		c.close()
		t.Fatal("expected error for unknown interface, got nil")
	}
}

func TestBayConfigRoundTrip(t *testing.T) {
	// Build a 61-byte output bay record and verify field extraction.
	p := make([]byte, bayConfigSize)
	p[0] = 7 // port
	p[1] = 1 // output
	p[2] = 2 // bay
	p[3] = 3 // video source
	p[4] = 4 // audio source
	copy(p[5:21], "Output 3")
	copy(p[21:37], "Kitchen")
	copy(p[37:53], "1080p60")
	// status bit3 (signal detected) + features HDMI_OUT
	p[53] = 1 << 3
	p[57] = 1 << 0
	cfg := parseBayConfig(p)
	if cfg.port != 7 || cfg.modenum != 1 || cfg.bay != 2 {
		t.Fatalf("bad header fields: %+v", cfg)
	}
	if cfg.bayName != "Output 3" || cfg.userName != "Kitchen" || cfg.signalType != "1080p60" {
		t.Fatalf("bad strings: %+v", cfg)
	}
	if !cfg.status.Has(BayStatusSignalDetected) || !cfg.features.Has(BayHDMIOut) {
		t.Fatalf("bad masks: status=%x features=%x", cfg.status, cfg.features)
	}
}
