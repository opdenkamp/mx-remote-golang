// Author: Lars Op den Kamp (lars@opdenkamp-it.nl)
// Copyright (c) 2026 Op den Kamp IT Solutions

package mxremote

import (
	"encoding/binary"
	"fmt"
	"strconv"
	"strings"
)

// DeviceUID is the 16-byte unique identifier of an MX Remote device on the
// network. The zero value is the empty (all-zero) UID.
type DeviceUID [16]byte

// ParseDeviceUID parses the human-readable "aaaaaaaa.bbbbbbbb.cccccccc.dddddddd"
// form (four little-endian 32-bit words printed big-endian in hex) into a UID.
func ParseDeviceUID(s string) (DeviceUID, error) {
	var uid DeviceUID
	parts := strings.Split(s, ".")
	if len(parts) < 4 {
		return uid, fmt.Errorf("invalid uid %q", s)
	}
	for i := 0; i < 4; i++ {
		v, err := strconv.ParseUint(parts[i], 16, 32)
		if err != nil {
			return uid, fmt.Errorf("invalid uid %q: %w", s, err)
		}
		binary.LittleEndian.PutUint32(uid[i*4:], uint32(v))
	}
	return uid, nil
}

// DeviceUIDFromBytes builds a UID from raw bytes. An empty slice yields the zero
// UID; any other length shorter than 16 is an error.
func DeviceUIDFromBytes(b []byte) (DeviceUID, error) {
	var uid DeviceUID
	if len(b) == 0 {
		return uid, nil
	}
	if len(b) < 16 {
		return uid, fmt.Errorf("invalid uid length %d", len(b))
	}
	copy(uid[:], b[:16])
	return uid, nil
}

// Empty reports whether the UID is all zero.
func (u DeviceUID) Empty() bool {
	return u == DeviceUID{}
}

// Bytes returns the raw 16-byte value.
func (u DeviceUID) Bytes() []byte {
	b := make([]byte, 16)
	copy(b, u[:])
	return b
}

// String returns the human-readable dotted-hex form.
func (u DeviceUID) String() string {
	var b strings.Builder
	for i := 0; i < 4; i++ {
		if i > 0 {
			b.WriteByte('.')
		}
		word := u[i*4 : i*4+4]
		for j := 3; j >= 0; j-- {
			fmt.Fprintf(&b, "%02x", word[j])
		}
	}
	return b.String()
}

// BayUID identifies a single bay (port) by its owning device and port number.
type BayUID struct {
	Device DeviceUID
	Port   int
}

func (b BayUID) String() string {
	return fmt.Sprintf("%s:%d", b.Device, b.Port)
}
