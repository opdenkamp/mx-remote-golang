// Author: Lars Op den Kamp (lars@opdenkamp-it.nl)
// Copyright (c) 2026 Op den Kamp IT Solutions

package mxremote

import "encoding/binary"

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
		signalType:  cstr(p[37:53]),
		status:      BayStatusMask(binary.LittleEndian.Uint32(p[53:57])),
		features:    BayFeaturesMask(binary.LittleEndian.Uint32(p[57:61])),
	}
}

// cstr returns the NUL-terminated ASCII prefix of b.
func cstr(b []byte) string {
	for i, c := range b {
		if c == 0 {
			return string(b[:i])
		}
	}
	return string(b)
}
