// Author: Lars Op den Kamp (lars@opdenkamp-it.nl)
// Copyright (c) 2026 Op den Kamp IT Solutions

package mxremote

import (
	"bytes"
	"encoding/binary"
	"strings"
	"unicode/utf8"
)

const bayConfigSize = 61

// bayConfig is a single 61-byte bay descriptor from a bay-config frame.
type bayConfig struct {
	port        int
	modenum     int
	bay         int
	videoSource int
	audioSource int
	edidProfile int
	rcType      int
	bayName     string
	userName    string
	signalType  string
	signalMode  MxrSignalType
	status      BayStatusMask
	features    BayFeaturesMask
}

func parseBayConfig(p []byte) bayConfig {
	return bayConfig{
		port:        int(p[0]),
		modenum:     int(p[1]),
		bay:         int(p[2]),
		videoSource: int(p[3]),
		audioSource: int(p[4]),
		edidProfile: (int(p[4]&0x0F) << 8) | int(p[3]),
		rcType:      int(p[4]>>4) & 0x0F,
		bayName:     cstr(p[5:21]),
		userName:    cstr(p[21:37]),
		// mxr_cfg_signal is a 14-byte description followed by a 2-byte
		// mxr_signal_type, not a 16-byte string: a description filling its
		// field would otherwise run into the type bytes
		signalType: cstr(p[37:51]),
		signalMode: MxrSignalType(binary.LittleEndian.Uint16(p[51:53])),
		status:     BayStatusMask(binary.LittleEndian.Uint32(p[53:57])),
		features:   BayFeaturesMask(binary.LittleEndian.Uint32(p[57:61])),
	}
}

// cstr decodes a fixed-width ASCII wire field: the bytes up to the first NUL,
// or all of b when the value fills the field and leaves no room for one. A peer
// on older firmware can still put junk in such a field, so a non-ASCII byte
// costs one character rather than the whole name.
func cstr(b []byte) string {
	if i := bytes.IndexByte(b, 0); i >= 0 {
		b = b[:i]
	}
	var sb strings.Builder
	sb.Grow(len(b))
	for _, c := range b {
		if c > 0x7F {
			sb.WriteRune(utf8.RuneError)
			continue
		}
		sb.WriteByte(c)
	}
	return sb.String()
}
