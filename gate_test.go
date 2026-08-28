// Author: Lars Op den Kamp (lars@opdenkamp-it.nl)
// Copyright (c) 2026 Op den Kamp IT Solutions

package mxremote

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// broadcastOpcodes are the sends with no single recipient, and so the only ones
// entitled to pass a nil target.
var broadcastOpcodes = map[string]bool{
	"opSysDiscover":        true,
	"opSysHello":           true,
	"opSysMonitoringPulse": true,
}

var transmitCall = regexp.MustCompile(`r\.transmit\(\s*([A-Za-z_][\w.]*)\s*,\s*buildFrame\(\s*\w+\s*,\s*(op\w+)`)

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
		for _, m := range transmitCall.FindAllStringSubmatch(string(src), -1) {
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
