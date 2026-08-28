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
