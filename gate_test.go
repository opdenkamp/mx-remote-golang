// Author: Lars Op den Kamp (lars@opdenkamp-it.nl)
// Copyright (c) 2026 Op den Kamp IT Solutions

package mxremote

import (
	"context"
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
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
// one level below it at processFrame. Keeping a test at this level matters even
// though the wrapper is currently thin: it was not always, and the behaviour it
// used to hold — re-announcing hello — was wrong precisely because it lived
// here, driven by arriving traffic rather than by a clock.
func TestProcessDatagramProcessesFrames(t *testing.T) {
	r := newTestRemote(Callbacks{})
	r.uid = uidN(200)
	peer := uidN(201)

	var sent [][]byte
	r.mu.Lock()
	r.txTap = func(b []byte) { sent = append(sent, append([]byte(nil), b...)) }
	r.mu.Unlock()

	r.processDatagram(buildFrame(peer, opSysHello, protocolFor(opSysHello),
		helloPayload(0x28, "Peer", "PR0001", "4.8.0", FeatureVideoRouting)), "10.8.8.9")
	if r.GetByUID(peer) == nil {
		t.Fatal("the datagram's frame was not processed")
	}
	// arriving traffic must not itself trigger an announcement
	for _, f := range sent {
		if len(f) >= headerLen && binary.LittleEndian.Uint16(f[20:22]) == opSysHello {
			t.Fatal("a received datagram triggered a hello; announcement is a timer, not a reply")
		}
	}
}

// A device announces itself on a schedule whether or not anything is talking to
// it. A client that only re-announced on arriving traffic went silent on a
// quiet network and stayed unknown to every peer that started after it.
func TestHelloIsAnnouncedOnATimer(t *testing.T) {
	r := newTestRemote(Callbacks{})
	r.uid = uidN(202)

	r.mu.Lock()
	// as though we had just announced
	r.lastHello = time.Now()
	r.helloInterval = 3 * time.Second
	now := r.lastHello
	r.mu.Unlock()

	if r.helloDueLocked(now.Add(2 * time.Second)) {
		t.Fatal("announced early")
	}
	// no traffic has arrived, and it is still due once the interval elapses
	if !r.helloDueLocked(now.Add(3 * time.Second)) {
		t.Fatal("not due at the interval; a silent network would never announce")
	}
	if !r.helloDueLocked(now.Add(time.Hour)) {
		t.Fatal("not due long after the interval")
	}

	r.mu.Lock()
	r.closing = true
	r.mu.Unlock()
	if r.helloDueLocked(now.Add(time.Hour)) {
		t.Fatal("announced while closing")
	}
}

// The interval is re-drawn on each send, so a mesh full of clients started
// together does not stay in step.
func TestHelloIntervalIsJittered(t *testing.T) {
	r := newTestRemote(Callbacks{})
	r.uid = uidN(203)

	seen := map[time.Duration]int{}
	for i := 0; i < 200; i++ {
		d := nextHelloInterval()
		if d < helloBaseInterval || d > helloBaseInterval+helloJitterInterval {
			t.Fatalf("interval %v outside %v..%v", d, helloBaseInterval, helloBaseInterval+helloJitterInterval)
		}
		seen[d]++
	}
	if len(seen) < 50 {
		t.Fatalf("only %d distinct intervals in 200 draws; the jitter is not varying", len(seen))
	}

	// A send that fails must not consume the interval: the firmware re-arms
	// only inside the branch where the transmit succeeded, so a failure is
	// retried on the next tick rather than costing a whole interval of
	// silence. This Remote has no socket, so the send fails.
	r.mu.Lock()
	r.lastHello = time.Time{}
	r.helloInterval = 0
	r.mu.Unlock()
	r.txHello()
	r.mu.Lock()
	interval, last := r.helloInterval, r.lastHello
	r.mu.Unlock()
	if interval != 0 || !last.IsZero() {
		t.Fatalf("a failed send re-armed the timer (interval %v, last %v); it should retry next tick",
			interval, last)
	}
}

// The decision and the send are tested above; this drives the loop that joins
// them. Without it, deleting the call from the probe leaves every other hello
// test green — the pieces work and nothing announces.
func TestProbeLoopAnnouncesHello(t *testing.T) {
	r := newTestRemote(Callbacks{})
	r.uid = uidN(204)

	got := make(chan struct{}, 1)
	r.mu.Lock()
	r.lastHello = time.Now().Add(-time.Hour) // long overdue
	r.helloInterval = time.Millisecond
	r.txTap = func(b []byte) {
		if len(b) >= headerLen && binary.LittleEndian.Uint16(b[20:22]) == opSysHello {
			select {
			case got <- struct{}{}:
			default:
			}
		}
	}
	r.mu.Unlock()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	r.wg.Add(1)
	go r.backgroundProbe(ctx)

	select {
	case <-got:
	case <-time.After(5 * time.Second):
		t.Fatal("the probe loop never announced, though the hello was overdue")
	}
	cancel()
	r.wg.Wait()
}

// The fourth question, after "does the pattern still match", "does every match
// get read" and "does every site get matched": is the thing being guarded still
// the only way through?
//
// Every argument for the protocol gate rests on two structural claims — one
// function builds every frame, and one function sends every frame. Neither is
// enforced by anything. A second frame builder, or a second call to the
// socket, would bypass the gate entirely, and the scan above would not notice
// because it only inspects the sends it can already see.
func TestTheChokePointsAreStillChokePoints(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	// No subpackage can hide a send: this is a single flat package by design,
	// examples/ is a separate module path that cannot reach unexported ones,
	// and docs/ is prose, which the Go files check below holds it to. Asserted
	// as the exact expected set rather than "not more than two", because the
	// latter also passes when the search stops matching anything.
	// filepath.Glob has no trailing-slash directory form, unlike a shell, so
	// this reads entries and filters.
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	var dirs []string
	for _, e := range entries {
		if e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
			dirs = append(dirs, e.Name())
		}
	}
	want := []string{"docs", "examples"}
	if !reflect.DeepEqual(dirs, want) {
		t.Errorf("subdirectories = %v, want exactly %v; this scan only reads "+
			"the package root, so anything else could hold a send it never sees", dirs, want)
	}
	// docs/ is exempt from the scan because it is prose. That is only true for
	// as long as it holds no Go.
	if goFiles, err := filepath.Glob("docs/*.go"); err != nil {
		t.Fatal(err)
	} else if len(goFiles) != 0 {
		t.Errorf("docs/ holds Go files %v; the scan never reads them", goFiles)
	}

	var buildSites, sendSites []string
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for i, line := range strings.Split(string(src), "\n") {
			// writing the magic byte is what makes a frame a frame
			if strings.Contains(line, "= magic0") && !strings.Contains(line, "magic0 ") {
				buildSites = append(buildSites, fmt.Sprintf("%s:%d", f, i+1))
			}
			// the conn-level write is the last point before the wire
			if strings.Contains(line, ".pc.WriteToUDP(") {
				sendSites = append(sendSites, fmt.Sprintf("%s:%d", f, i+1))
			}
		}
	}
	if len(buildSites) != 1 {
		t.Errorf("frames are built in %d places (%v); the gate assumes exactly one, "+
			"and a second builder is not reached by the send scan", len(buildSites), buildSites)
	}
	if len(sendSites) != 1 {
		t.Errorf("the socket is written from %d places (%v); the gate assumes exactly one, "+
			"and a second write bypasses it entirely", len(sendSites), sendSites)
	}
}
