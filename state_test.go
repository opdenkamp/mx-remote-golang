// Author: Lars Op den Kamp (lars@opdenkamp-it.nl)
// Copyright (c) 2026 Op den Kamp IT Solutions

package mxremote

import (
	"encoding/binary"
	"testing"
	"time"
)

func uidN(n byte) DeviceUID {
	var u DeviceUID
	u[0] = n
	u[15] = 0xAA
	return u
}

func helloPayload(proto uint16, name, serial, version string, feat DeviceFeature) []byte {
	p := make([]byte, 0, 54)
	p = append(p, byte(proto), byte(proto>>8))
	p = appendFixedStr(p, name, 16)
	p = appendFixedStr(p, serial, 16)
	p = appendFixedStr(p, version, 16)
	p = append(p, byte(feat), byte(feat>>8), byte(feat>>16), byte(feat>>24))
	return p
}

func bayConfigRec(port, mode, bay int, name, user string, status BayStatusMask, feat BayFeaturesMask) []byte {
	p := make([]byte, bayConfigSize)
	p[0] = byte(port)
	p[1] = byte(mode)
	p[2] = byte(bay)
	copy(p[5:21], name)
	copy(p[21:37], user)
	binary.LittleEndian.PutUint32(p[53:57], uint32(status))
	binary.LittleEndian.PutUint32(p[57:61], uint32(feat))
	return p
}

func streamRec(uid DeviceUID, videoIP, audioIP, ancIP string, port int) []byte {
	rec := make([]byte, 40)
	copy(rec[0:16], uid[:])
	put := func(off int, ip string) {
		copy(rec[off:off+4], ip4Bytes(ip))
		rec[off+4] = byte(port)
		rec[off+5] = byte(port >> 8)
	}
	put(16, videoIP)
	put(24, audioIP)
	put(32, ancIP)
	return rec
}

// newTestRemote returns a Remote that is not network-connected; frames are fed
// directly via processFrame.
func newTestRemote(cb Callbacks) *Remote {
	r := New(Config{Callbacks: cb})
	return r
}

func TestDiscoveryAndV2IPSources(t *testing.T) {
	var completed []string
	r := newTestRemote(Callbacks{
		OnDeviceConfigComplete: func(d *Device) { completed = append(completed, d.Serial()) },
	})
	sender := uidN(1)
	srcUID := uidN(9)
	feed := func(op uint16, payload []byte) {
		r.processFrame(buildFrame(sender, op, ProtocolVersion, payload), "10.8.8.5", time.Now())
	}

	feed(opSysHello, helloPayload(0x27, "FF88", "AB1234", "4.7.9", FeatureV2IPSource|FeatureV2IPSink))
	dev := r.GetByUID(sender)
	if dev == nil {
		t.Fatal("device not registered")
	}
	if dev.Serial() != "AB1234" || !dev.IsV2IP() {
		t.Fatalf("bad device: serial=%s v2ip=%v", dev.Serial(), dev.IsV2IP())
	}
	if dev.Address() != "10.8.8.5" {
		t.Fatalf("address = %q", dev.Address())
	}

	// input bay (port 0, v2ip source local) + output bay (port 1, v2ip sink local)
	in := bayConfigRec(0, 0, 0, "Input 1", "Apple TV", 0, BayV2IPSourceLocal)
	out := bayConfigRec(1, 1, 0, "Output 1", "Living Room", 0, BayV2IPSinkLocal)
	feed(opSysBayConfig, append(append([]byte{}, in...), out...))

	if got := len(dev.Bays()); got != 2 {
		t.Fatalf("want 2 bays, got %d", got)
	}
	inBay := dev.GetByPortnum(0)
	if inBay == nil || !inBay.IsInput() || !inBay.IsV2IPSource() {
		t.Fatalf("input bay wrong: %+v", inBay)
	}
	if inBay.UserName() != "Apple TV" {
		t.Fatalf("user name = %q", inBay.UserName())
	}

	// v2ip sources
	feed(opSysBayV2IPSources, streamRec(srcUID, "239.1.1.1", "239.1.1.2", "239.1.1.3", 50020))

	src := inBay.V2IPSource()
	if src == nil || src.Video.IP != "239.1.1.1" || src.Audio.IP != "239.1.1.2" {
		t.Fatalf("v2ip source wrong: %+v", src)
	}
	// bay_uid of a v2ip source resolves to (sourceUID, 0)
	if bu := inBay.BayUID(); bu.Device != srcUID || bu.Port != 0 {
		t.Fatalf("bay uid = %s, want %s:0", bu, srcUID)
	}
	// config complete should have fired (has bays + v2ip sources, no link config needed... )
	// link config is required for v2ip; send it to complete.
	feed(opSysLinks, make([]byte, 0))
	if !dev.ConfigurationComplete() {
		t.Fatal("device should be configuration-complete")
	}
	if len(completed) == 0 || completed[0] != "AB1234" {
		t.Fatalf("config-complete callback not fired: %v", completed)
	}
}

func TestMirrorAndMesh(t *testing.T) {
	var mirrorEvents int
	r := newTestRemote(Callbacks{
		OnMirrorStatusChanged: func(b *Bay, m BayMirrorStatus) { mirrorEvents++ },
	})
	sender := uidN(2)
	master := uidN(7)
	feed := func(op uint16, payload []byte) {
		r.processFrame(buildFrame(sender, op, ProtocolVersion, payload), "10.8.8.6", time.Now())
	}
	feed(opSysHello, helloPayload(0x27, "ONEIP", "RX0001", "4.7.9", FeatureV2IPSink|FeatureMesh))
	feed(opSysBayConfig, bayConfigRec(0, 1, 0, "Output 1", "TV", 0, BayV2IPSinkLocal))
	dev := r.GetByUID(sender)
	out := dev.GetByPortnum(0)
	if out == nil {
		t.Fatal("output bay missing")
	}

	// mirror status: target=sender (is_own), master=master uid
	mp := make([]byte, 32)
	copy(mp[0:16], sender[:])
	copy(mp[16:32], master[:])
	feed(opBayMirrorStatus, mp)
	if !out.Mirroring().IsMirroring() {
		t.Fatal("expected mirroring")
	}
	if tgt := out.Mirroring().Target; tgt == nil || tgt.Device != master {
		t.Fatalf("mirror target wrong: %+v", out.Mirroring().Target)
	}
	if mirrorEvents == 0 {
		t.Fatal("mirror callback not fired")
	}

	// mesh REPORT_MEMBERSHIP: op=0xFF, target uid at offset 4
	meshp := make([]byte, 36)
	meshp[0] = 0xFF
	copy(meshp[4:20], master[:])
	feed(opMeshOperation, meshp)
	if mm := dev.MeshMaster(); mm == nil || mm.UID() != master {
		// master device not registered, so MeshMaster falls back to self
		if mm != dev {
			t.Fatalf("mesh master = %v", mm)
		}
	}
}
