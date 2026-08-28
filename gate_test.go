// Author: Lars Op den Kamp (lars@opdenkamp-it.nl)
// Copyright (c) 2026 Op den Kamp IT Solutions

package mxremote

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

// broadcastOpcodes are the sends with no single recipient, and so the only ones
// entitled to pass a nil target.
var broadcastOpcodes = map[string]bool{
	"opSysDiscover":        true,
	"opSysHello":           true,
	"opSysMonitoringPulse": true,
}

// Every send, however written, and then the readable form of one. Splitting
// them is the point: a site the detailed pattern cannot read must fail loudly
// rather than be skipped, or a send in an unexpected form goes unchecked while
// the scan stays green. A minimum count catches the pattern failing wholesale;
// it does not catch one site dropping out of it.
var (
	anyTransmit  = regexp.MustCompile(`r\.transmit\(`)
	transmitCall = regexp.MustCompile(`r\.transmit\(\s*([A-Za-z_][\w.]*)\s*,\s*buildFrame\(\s*\w+\s*,\s*(op\w+)`)
)

// TestEveryTargetedSendPassesItsTarget closes the gap the compiler cannot.
//
// transmit takes the target as a parameter, so omitting the decision is a build
// error — but passing nil for a frame that does have a recipient compiles fine
// and silently skips the protocol gate. Nothing at the type level distinguishes
// "no recipient" from "recipient I did not bother to name", so this asserts it
// against the source instead.
func TestEveryTargetedSendPassesItsTarget(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	seen := 0
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		text := string(src)
		// every site the loose pattern finds must also be readable by the
		// detailed one; anything else is reported, not skipped
		readable := map[int]bool{}
		for _, loc := range transmitCall.FindAllStringIndex(text, -1) {
			readable[loc[0]] = true
		}
		for _, loc := range anyTransmit.FindAllStringIndex(text, -1) {
			if !readable[loc[0]] {
				t.Errorf("%s:%d: a send here could not be read for its target and opcode, "+
					"so it is unchecked; rewrite it in the usual form or widen the pattern deliberately",
					f, strings.Count(text[:loc[0]], "\n")+1)
			}
		}
		for _, m := range transmitCall.FindAllStringSubmatch(text, -1) {
			target, opcode := m[1], m[2]
			seen++
			if target == "nil" && !broadcastOpcodes[opcode] {
				t.Errorf("%s: %s is sent with a nil target, so it skips the protocol gate; "+
					"pass the addressed device, or add it to broadcastOpcodes if it truly has none",
					f, opcode)
			}
			if target != "nil" && broadcastOpcodes[opcode] {
				t.Errorf("%s: %s is listed as a broadcast but is sent to %s", f, opcode, target)
			}
		}
	}
	// A regex that matches nothing would report every file clean.
	if seen < 15 {
		t.Fatalf("only matched %d transmit calls, expected every send site; the pattern has drifted", seen)
	}
}

// Round-tripping the library's own builders through its own decoders is the
// only instrument here that tests *meaning* rather than position.
//
// A field decoded from the right offset but attributed to the wrong thing —
// source read as target, left delay as right — passes every positional check,
// because nothing about the byte layout is wrong. A round trip catches it when
// the builder and the decoder disagree about what a field means, which is what
// a real orientation bug looks like: two sides of one library, written at
// different times, disagreeing.
//
// The limit is worth stating: where builder and decoder are wrong *together*,
// a round trip is clean. That case needs an external reference — a captured
// frame or the firmware struct — and no round trip can substitute for it.
func TestBuilderDecoderRoundTrip(t *testing.T) {
	r := newTestRemote(Callbacks{})
	sender := uidN(180)
	feed := feeder(r, sender)
	feed(opSysHello, helloPayload(0x28, "ProAmp8", "RT0001", "4.8.0",
		FeatureAudioAmplifier|FeatureV2IPSink))
	feed(opSysBayConfig, bayConfigRec(3, 1, 0, "Zone 3", "Study", 0,
		BayAudioAmpOut|BayV2IPSinkLocal))

	t.Run("amp zone settings", func(t *testing.T) {
		// every field distinct, and the two delays deliberately unequal so
		// swapping them is visible
		want := AmpZoneSettings{
			GainLeft: 190, GainRight: 191, VolumeMin: 12, VolumeMax: 220,
			DelayLeft: 96000, DelayRight: 144000,
			Bass: 130, Treble: 131, Bridged: 1, PowerMode: 2, PowerLevel: 33,
			PowerTimeout: 900,
			EQLeft:       [5]int{120, 121, 122, 123, 124},
			EQRight:      [5]int{140, 141, 142, 143, 144},
		}
		feed(opAmpZoneSettings, buildAmpZoneSettings(sender, 3, want))
		got := r.GetByUID(sender).GetByPortnum(3).AmpSettings()
		if got == nil {
			t.Fatal("what the builder produced did not decode at all")
		}
		if *got != want {
			t.Fatalf("round trip changed the settings:\n sent %+v\n got  %+v", want, *got)
		}
	})

	t.Run("manual source switch", func(t *testing.T) {
		// three distinct addresses and ports, so any pair being swapped shows
		payload := buildV2IPManualSourceSwitch(sender,
			"239.10.0.1", 50020, "239.10.0.2", 50022, "239.10.0.3", 50021,
			&V2IPAudioFormat{SampleRate: 96000, Channels: 6})
		feed(opV2IPManualSrcSwitch, payload)

		sink := r.GetByUID(sender).V2IPSink()
		if sink == nil {
			t.Fatal("what the builder produced did not decode at all")
		}
		for _, c := range []struct {
			what string
			got  V2IPStreamSource
			ip   string
			port int
		}{
			{"video", sink.Addresses.Video, "239.10.0.1", 50020},
			{"audio", sink.Addresses.Audio, "239.10.0.2", 50022},
			{"anc", sink.Addresses.Anc, "239.10.0.3", 50021},
		} {
			if c.got.IP != c.ip || c.got.Port != c.port {
				t.Errorf("%s round-tripped as %s:%d, sent %s:%d", c.what, c.got.IP, c.got.Port, c.ip, c.port)
			}
		}
		if sink.AudioFmt == nil || sink.AudioFmt.SampleRate != 96000 || sink.AudioFmt.Channels != 6 {
			t.Errorf("audio format round trip = %+v", sink.AudioFmt)
		}
	})
}

// Round-tripping the control methods themselves, not just the payload builders
// that happen to be separate functions. Most payloads are assembled inline in
// the method that sends them, so capturing what actually goes to the wire is
// the only way to feed it back through the decoder.
//
// Attempting this is itself an audit, and it surfaced two things about this
// library that no other test here states. A captured frame carries our own uid
// as its sender and processFrame drops those as echoes, so the harness has to
// rewrite the sender to a peer — the library genuinely cannot decode its own
// sends. And once it does, the handlers split: some act on the device named in
// the payload, others on whoever sent the frame, so a round trip only proves
// something if it asserts against the one the handler actually updates.
func TestControlMethodsRoundTrip(t *testing.T) {
	r := newTestRemote(Callbacks{})
	target, peer := uidN(190), uidN(191)
	bays := append(
		bayConfigRec(1, 0, 0, "Input 1", "Apple TV", 0, BayHDMIIn),
		bayConfigRec(2, 1, 0, "Output 1", "TV", 0, BayHDMIOut|BayAudioAmpOut)...)
	for _, uid := range []DeviceUID{target, peer} {
		f := feeder(r, uid)
		f(opSysHello, helloPayload(0x28, "FF88", "RT"+string(rune('A'+uid[0]%26)), "4.8.0",
			FeatureVideoRouting|FeatureAudioAmplifier))
		f(opSysBayConfig, bays)
	}

	var sent [][]byte
	r.mu.Lock()
	r.txTap = func(b []byte) { sent = append(sent, append([]byte(nil), b...)) }
	r.mu.Unlock()

	dev := r.GetByUID(target)
	in, out := dev.GetByPortnum(1), dev.GetByPortnum(2)
	peerOut := r.GetByUID(peer).GetByPortnum(2)

	// send, then replay the captured frame as though a peer controller had sent
	// it: our own uid would be dropped as an echo
	roundTrip := func(t *testing.T, do func() error) {
		t.Helper()
		sent = sent[:0]
		if err := do(); err != nil && err.Error() != "connection closed" {
			t.Fatalf("send failed for a reason other than the missing socket: %v", err)
		}
		if len(sent) != 1 {
			t.Fatalf("expected exactly one frame, captured %d", len(sent))
		}
		frame := sent[0]
		copy(frame[4:20], peer[:])
		r.processFrame(frame, "10.8.8.9", time.Now())
	}

	// addressed by the uid in the payload
	t.Run("SetName", func(t *testing.T) {
		var got BayNameChange
		r.mu.Lock()
		r.callbacks.OnBayNameChangeRequested = func(_ *Device, c BayNameChange) { got = c }
		r.mu.Unlock()
		roundTrip(t, func() error { return out.SetName("Kitchen Amp") })
		if got.Target != target || got.Port != 2 || got.Name != "Kitchen Amp" {
			t.Fatalf("name round-tripped as %+v", got)
		}
	})

	t.Run("SetHidden", func(t *testing.T) {
		roundTrip(t, func() error { return out.SetHidden(true) })
		if !out.Hidden() {
			t.Fatal("hidden did not round trip onto the addressed bay")
		}
	})

	t.Run("SelectEdidProfile", func(t *testing.T) {
		var got EDIDProfileChange
		r.mu.Lock()
		r.callbacks.OnEDIDProfileChangeRequested = func(_ *Device, c EDIDProfileChange) { got = c }
		r.mu.Unlock()
		roundTrip(t, func() error { return in.SelectEdidProfile(Edid4KHDR71) })
		if got.Target != target || got.Profile != Edid4KHDR71 {
			t.Fatalf("edid profile round-tripped as %+v", got)
		}
	})

	t.Run("TxAction", func(t *testing.T) {
		var got ActionTransmitRequest
		r.mu.Lock()
		r.callbacks.OnActionTransmitRequested = func(_ *Device, q ActionTransmitRequest) { got = q }
		r.mu.Unlock()
		roundTrip(t, func() error { return out.TxAction(ActionPowerOn) })
		if got.Target != target || got.Action != ActionPowerOn || got.LocalBay != 2 {
			t.Fatalf("action round-tripped as %+v", got)
		}
	})

	// keyed off whoever sent the frame, so it lands on the peer's bay
	t.Run("VolumeSet", func(t *testing.T) {
		f := false
		roundTrip(t, func() error { return out.VolumeSet(37, &f) })
		v := peerOut.VolumeStatus()
		if v == nil || v.VolumeLeft != 37 {
			t.Fatalf("volume round-tripped onto the sender's bay as %+v", v)
		}
		if ov := out.VolumeStatus(); ov != nil && ov.VolumeLeft == 37 {
			t.Fatal("volume also landed on the addressed bay; this handler is sender-keyed")
		}
	})
}

// processDatagram is the real receive entry point; every other test here enters
// one level below it at processFrame, which is where the Python port found the
// same shape hiding its echo skip.
//
// The layer is not empty: it is the only thing that re-announces this client's
// hello. The background probe sends discover and never hello, so if this stops
// working the client goes silent to peers that started after it did, and no
// test below this level would notice.
func TestProcessDatagramReannouncesHello(t *testing.T) {
	r := newTestRemote(Callbacks{})
	r.uid = uidN(200)
	peer := uidN(201)

	var sent [][]byte
	r.mu.Lock()
	r.txTap = func(b []byte) { sent = append(sent, append([]byte(nil), b...)) }
	r.mu.Unlock()

	helloCount := func() int {
		n := 0
		for _, f := range sent {
			if len(f) >= headerLen && binary.LittleEndian.Uint16(f[20:22]) == opSysHello {
				n++
			}
		}
		return n
	}

	datagram := buildFrame(peer, opSysDiscover, protocolFor(opSysDiscover), nil)

	// a recent hello: an arriving datagram must not trigger another
	r.mu.Lock()
	r.lastHello = time.Now()
	r.mu.Unlock()
	sent = sent[:0]
	r.processDatagram(datagram, "10.8.8.9")
	if n := helloCount(); n != 0 {
		t.Fatalf("re-announced %d times with a fresh hello, want 0", n)
	}

	// once the hello has aged past its window, the next datagram re-announces
	r.mu.Lock()
	r.lastHello = time.Now().Add(-31 * time.Second)
	r.mu.Unlock()
	sent = sent[:0]
	r.processDatagram(datagram, "10.8.8.9")
	if n := helloCount(); n != 1 {
		t.Fatalf("re-announced %d times with a stale hello, want 1", n)
	}

	// and the frame itself is still processed, not swallowed by the re-announce
	r.mu.Lock()
	r.lastHello = time.Now()
	r.mu.Unlock()
	r.processDatagram(buildFrame(peer, opSysHello, protocolFor(opSysHello),
		helloPayload(0x28, "Peer", "PR0001", "4.8.0", FeatureVideoRouting)), "10.8.8.9")
	if r.GetByUID(peer) == nil {
		t.Fatal("the datagram's own frame was not processed")
	}
}
