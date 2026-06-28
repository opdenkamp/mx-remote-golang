// Author: Lars Op den Kamp (lars@opdenkamp-it.nl)
// Copyright (c) 2026 Op den Kamp IT Solutions

package mxremote

import (
	"encoding/binary"
	"testing"
	"time"
)

func feeder(r *Remote, sender DeviceUID) func(uint16, []byte) {
	return func(op uint16, payload []byte) {
		r.processFrame(buildFrame(sender, op, ProtocolVersion, payload), "10.8.8.9", time.Now())
	}
}

func TestTopologyParse(t *testing.T) {
	r := newTestRemote(Callbacks{})
	sender := uidN(3)
	feed := feeder(r, sender)
	feed(opSysHello, helloPayload(0x27, "FF88", "TP0001", "4.7.9", FeatureVideoRouting))

	other := uidN(8)
	rec := make([]byte, 20)
	copy(rec[0:16], other[:])
	binary.LittleEndian.PutUint32(rec[16:20], 0xABCD)
	feed(opTopology, rec)

	topo := r.GetByUID(sender).Topology()
	if len(topo) != 1 || topo[0].UID != other || topo[0].Mask != 0xABCD {
		t.Fatalf("topology = %+v", topo)
	}
}

func TestMultiviewerParse(t *testing.T) {
	r := newTestRemote(Callbacks{})
	sender := uidN(4)
	feed := feeder(r, sender)
	feed(opSysHello, helloPayload(0x27, "ONEIP-MV", "MV0001", "4.7.9", FeatureV2IPSink|FeatureMultiviewer))

	p := make([]byte, 190)
	// p[0:16] target, p[16] = STATUS opcode (0), p[17:24] pad
	p[169] = byte(MVViewPIP)      // view_mode
	p[171] = byte(MVPipSizeLarge) // pip_size
	p[180] = 42                   // audio volume
	p[182] = byte(MVSource1)      // screen 0 video source
	p[179] = 0                    // audio source -> +1 => SOURCE_1
	su := uidN(4)
	copy(p[24:40], su[:]) // settings uid

	feed(opV2IPMultiviewer, p)

	dev := r.GetByUID(sender)
	if !dev.IsOneIPMultiviewer() {
		t.Fatal("device should be a multiviewer")
	}
	mv := dev.Multiviewer()
	if mv.ViewMode() != MVViewPIP {
		t.Fatalf("view mode = %v", mv.ViewMode())
	}
	if mv.PipSize() != MVPipSizeLarge {
		t.Fatalf("pip size = %v", mv.PipSize())
	}
	if mv.AudioVolume() != 42 {
		t.Fatalf("audio volume = %d", mv.AudioVolume())
	}
	if mv.VideoSource(0) != MVSource1 {
		t.Fatalf("screen 0 source = %v", mv.VideoSource(0))
	}
	if mv.AudioSource() != MVSource1 {
		t.Fatalf("audio source = %v", mv.AudioSource())
	}
}

func TestAudioFeaturesParse(t *testing.T) {
	r := newTestRemote(Callbacks{})
	sender := uidN(5)
	feed := feeder(r, sender)
	feed(opSysHello, helloPayload(0x27, "ONEIP", "AU0001", "4.7.9", FeatureV2IPSource|FeatureV2IPSink))

	p := make([]byte, 68)
	// op u16 @0 = FEATURES(0); uid @2
	binary.LittleEndian.PutUint16(p[28:30], 2) // nb_endpoints
	// entry 0 @36: id=0 type=ENDPOINT features=input|v2ip_tx
	p[36] = 0
	p[37] = audioEntryEndpoint
	binary.LittleEndian.PutUint32(p[44:48], uint32(audioFeatureInput|audioFeatureV2IPTx))
	// entry 1 @52: id=1 type=ENDPOINT features=output
	p[52] = 1
	p[53] = audioEntryEndpoint
	binary.LittleEndian.PutUint32(p[60:64], uint32(audioFeatureOutput))

	feed(opV2IPAudio, p)

	dev := r.GetByUID(sender)
	eps := dev.AudioEndpoints()
	if eps == nil || len(eps.List()) != 2 {
		t.Fatalf("expected 2 endpoints, got %v", eps)
	}
	if ep := dev.AudioEndpointByID(0); ep == nil || !ep.IsInput() || !ep.IsV2IP() {
		t.Fatalf("endpoint 0 wrong: %+v", ep)
	}
	if ep := dev.AudioEndpointByID(1); ep == nil || !ep.IsOutput() {
		t.Fatalf("endpoint 1 wrong: %+v", ep)
	}
}

func TestAmpAndStatsAndNetwork(t *testing.T) {
	var dolbyEvents, ampEvents int
	r := newTestRemote(Callbacks{
		OnAmpDolbySettingsChanged: func(d *Device, s AmpDolbySettings) { dolbyEvents++ },
		OnAmpZoneSettingsChanged:  func(b *Bay, s AmpZoneSettings) { ampEvents++ },
	})
	sender := uidN(7)
	feed := feeder(r, sender)
	// amp: volume control + audio routing, no video routing
	feed(opSysHello, helloPayload(0x27, "PROAMP8", "AM0001", "4.7.9", FeatureVolumeControl|FeatureAudioRouting))
	feed(opSysBayConfig, bayConfigRec(3, 1, 0, "Output 1", "Zone 1", 0, BayAudioAmpOut))
	dev := r.GetByUID(sender)
	if !dev.IsAmp() {
		t.Fatal("device should be an amp")
	}

	// amp zone settings for port 3
	az := make([]byte, 54)
	copy(az[0:16], sender[:])
	binary.LittleEndian.PutUint16(az[16:18], 3) // zone = port 3
	az[18], az[19] = 10, 11                     // gain l/r
	binary.LittleEndian.PutUint32(az[40:44], 300)
	feed(opAmpZoneSettings, az)
	bay := dev.GetByPortnum(3)
	if s := bay.AmpSettings(); s == nil || s.GainLeft != 10 || s.GainRight != 11 || s.PowerTimeout != 300 {
		t.Fatalf("amp settings = %+v", bay.AmpSettings())
	}
	if ampEvents == 0 {
		t.Fatal("amp zone callback not fired")
	}

	// dolby settings: mode 2, flags upmix+detected
	dolby := make([]byte, 18)
	copy(dolby[0:16], sender[:])
	dolby[16] = 2
	dolby[17] = 0x1 | 0x2
	feed(opAmpDolbyState, dolby)
	if ds := dev.DolbySettings(); ds == nil || ds.Mode != 2 || !ds.PCMUpmix || !ds.DolbyDetected || ds.PCMUpmixActive {
		t.Fatalf("dolby = %+v", dev.DolbySettings())
	}
	if dolbyEvents == 0 {
		t.Fatal("dolby callback not fired")
	}

	// v2ip stats (128-byte body)
	stats := make([]byte, 128)
	binary.LittleEndian.PutUint32(stats[0:4], 1000)   // tx video
	binary.LittleEndian.PutUint32(stats[40:44], 5000) // rx video total
	stats[80] = byte(DecoderHealthy)                  // rx decoder_state @40 within rx block (40+40)
	feed(opV2IPStats, stats)
	if st := dev.V2IPStats(); st == nil || st.Tx.Video != 1000 || st.Rx.VideoTotal != 5000 {
		t.Fatalf("stats = %+v", dev.V2IPStats())
	}

	// network status (0x22+): port 0, uplink + status features
	const protoModern = 0x22
	net := make([]byte, 88)
	binary.LittleEndian.PutUint16(net[0:2], 1) // port 1
	net[2] = (1 << 0) | (1 << 6)               // support_status + port_uplink
	copy(net[4:20], "eth0")
	net[21], net[22], net[23], net[24], net[25], net[26] = 0xAA, 0xBB, 0xCC, 0xDD, 0xEE, 0xFF
	net[28], net[29], net[30], net[31] = 10, 8, 8, 1 // ip
	net[38] = 0x4 | (1 << 3)                         // 1G + full duplex
	frameBytes := buildFrame(sender, opNetLinkStatus, protoModern, net)
	r.processFrame(frameBytes, "10.8.8.7", time.Now())
	ns := dev.NetworkStatus()
	if p, ok := ns[1]; !ok || p.Name != "eth0" || p.IP != "10.8.8.1" || !p.LinkFullDuplex || p.LinkSpeed != LinkSpeed1G {
		t.Fatalf("network status = %+v", ns)
	}
	if dev.MACAddress() != "AA:BB:CC:DD:EE:FF" {
		t.Fatalf("mac = %q", dev.MACAddress())
	}
}

func TestOnlineLivenessRefresh(t *testing.T) {
	var onlineEvents []bool
	r := newTestRemote(Callbacks{
		OnDeviceOnlineStatusChanged: func(d *Device, online bool) { onlineEvents = append(onlineEvents, online) },
	})
	sender := uidN(11)
	now := time.Now()

	// Hello received 20s ago: a V2IP device (proto >= 0x20) uses a 15s window,
	// so it must read offline.
	r.processFrame(buildFrame(sender, opSysHello, ProtocolVersion,
		helloPayload(0x27, "ONEIP", "LV0001", "4.7.9", FeatureV2IPSink)), "10.8.8.9", now.Add(-20*time.Second))
	dev := r.GetByUID(sender)
	if dev.Online() {
		t.Fatal("device should be offline after 20s of silence")
	}

	// Any later frame (even an unhandled opcode) refreshes liveness.
	r.processFrame(buildFrame(sender, opSysMonitoringPulse, ProtocolVersion, nil), "10.8.8.9", now)
	if !dev.Online() {
		t.Fatal("device should be back online after a fresh frame")
	}
	if len(onlineEvents) == 0 || onlineEvents[len(onlineEvents)-1] != true {
		t.Fatalf("expected an online transition event, got %v", onlineEvents)
	}
}

func TestFirmwareAndSystemStatus(t *testing.T) {
	r := newTestRemote(Callbacks{})
	sender := uidN(10)
	feed := feeder(r, sender)
	feed(opSysHello, helloPayload(0x27, "ONEIP", "FW0001", "4.7.9", FeatureV2IPSink))

	// firmware version: type FPGA(1), hash @4, timestamp @8, version str @12
	fw := make([]byte, 28)
	fw[0] = byte(FirmwareFPGA)
	binary.LittleEndian.PutUint32(fw[4:8], 0xCAFE)
	binary.LittleEndian.PutUint32(fw[8:12], 1700000000)
	copy(fw[12:], "1.2.3")
	feed(opFirmwareVersion, fw)

	dev := r.GetByUID(sender)
	versions := dev.FirmwareVersions()
	v, ok := versions[FirmwareFPGA]
	if !ok || v.Version != "1.2.3" || v.Hash != 0xCAFE || v.Timestamp != 1700000000 {
		t.Fatalf("firmware = %+v", versions)
	}

	// system status: code @16, message @18
	ss := make([]byte, 18)
	binary.LittleEndian.PutUint16(ss[16:18], 7)
	ss = append(ss, []byte("overheating")...)
	feed(opSysStatus, ss)
	code, msg, ok := dev.SystemStatus()
	if !ok || code != 7 || msg != "overheating" {
		t.Fatalf("system status = %d %q ok=%v", code, msg, ok)
	}
}

func TestSvdAndSignalStatus(t *testing.T) {
	if s, ok := LookupSvd(1); !ok || s.HorizontalActive != 640 || s.VerticalActive != 480 {
		t.Fatalf("svd 1 = %+v ok=%v", s, ok)
	}

	r := newTestRemote(Callbacks{})
	sender := uidN(6)
	feed := feeder(r, sender)
	feed(opSysHello, helloPayload(0x27, "FF88", "SG0001", "4.7.9", FeatureVideoRouting))
	feed(opSysBayConfig, bayConfigRec(2, 1, 0, "Output 1", "TV", 0, BayHDMIOut))

	p := make([]byte, 112)
	p[2] = 1 << 1 // support_flags: stream_valid
	// video details at 40: svd id, colour_space, colour_depth, ... frame_rate@48
	p[40] = 16                                   // svd id 16 (1920x1080)
	p[41] = 0                                    // RGB
	p[42] = 8                                    // 8 bpp
	p[48] = 60                                   // frame rate
	binary.LittleEndian.PutUint16(p[100:102], 2) // port number

	feed(opBaySignalStatus, p)

	bay := r.GetByUID(sender).GetByPortnum(2)
	if !bay.SignalDetected() {
		t.Fatal("signal should be detected")
	}
	want, _ := LookupSvd(16)
	st := bay.SignalType()
	if st == "No Signal" || st == "unknown" {
		t.Fatalf("signal type = %q (svd16=%v)", st, want)
	}
}
