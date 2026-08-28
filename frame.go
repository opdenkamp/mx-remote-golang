// Author: Lars Op den Kamp (lars@opdenkamp-it.nl)
// Copyright (c) 2026 Op den Kamp IT Solutions

package mxremote

import (
	"encoding/binary"
	"fmt"
	"time"
)

const (
	headerLen = 24
	magic0    = 0x50 // 'P'
	magic1    = 0x38 // '8'

	// deviceNameLen and fwVersionLen are the widths of the fixed-size name
	// fields on the wire (MXR_DEVICE_NAME_LEN, MXR_FW_VERSION_LEN). A value
	// that fills the field leaves no room for a terminator, so read exactly the
	// field width and only then cut at a NUL - scanning on runs into the
	// neighbouring struct member.
	deviceNameLen = 16
	fwVersionLen  = 128
)

// frame is a decoded MX Remote wire frame: a 24-byte header followed by an
// opcode-specific payload.
//
// Wire layout:
//
//	[0]      0x50 'P'
//	[1]      0x38 '8'
//	[2:4]    protocol version (u16 LE)
//	[4:20]   sender device UID (16 bytes)
//	[20:22]  opcode (u16 LE)
//	[22:24]  payload length (u16 LE)
//	[24:]    payload
type frame struct {
	data      []byte
	addr      string
	timestamp time.Time
}

func parseFrame(data []byte, addr string, ts time.Time) (*frame, error) {
	if len(data) < headerLen {
		return nil, fmt.Errorf("invalid mx_remote frame (length = %d)", len(data))
	}
	if data[0] != magic0 || data[1] != magic1 {
		return nil, fmt.Errorf("invalid mx_remote frame (header = %d:%d)", data[0], data[1])
	}
	return &frame{data: data, addr: addr, timestamp: ts}, nil
}

// buildFrame assembles a frame for transmission. payload may be nil.
func buildFrame(uid DeviceUID, opcode uint16, protocol uint16, payload []byte) []byte {
	out := make([]byte, headerLen+len(payload))
	out[0] = magic0
	out[1] = magic1
	binary.LittleEndian.PutUint16(out[2:], protocol)
	copy(out[4:20], uid[:])
	binary.LittleEndian.PutUint16(out[20:], opcode)
	binary.LittleEndian.PutUint16(out[22:], uint16(len(payload)))
	copy(out[24:], payload)
	return out
}

func (f *frame) protocol() uint16 { return binary.LittleEndian.Uint16(f.data[2:4]) }

func (f *frame) remoteID() DeviceUID {
	var uid DeviceUID
	copy(uid[:], f.data[4:20])
	return uid
}

func (f *frame) opcode() uint16     { return binary.LittleEndian.Uint16(f.data[20:22]) }
func (f *frame) payloadLen() uint16 { return binary.LittleEndian.Uint16(f.data[22:24]) }

// payload returns the frame payload, bounded by both the length the header
// declares and the bytes that actually arrived: a truncated datagram can claim
// more than it carries, and a padded one can carry more than it claims.
func (f *frame) payload() []byte {
	if len(f.data) <= headerLen {
		return nil
	}
	end := headerLen + int(f.payloadLen())
	if end > len(f.data) {
		end = len(f.data)
	}
	return f.data[headerLen:end]
}

// Payload accessors. idx is relative to the start of the payload. Each returns
// ok=false when the payload is too short.

func (f *frame) u8(idx int) (uint8, bool) {
	i := headerLen + idx
	if i < 0 || i >= len(f.data) {
		return 0, false
	}
	return f.data[i], true
}

func (f *frame) boolean(idx int) bool {
	v, ok := f.u8(idx)
	return ok && v == 1
}

func (f *frame) u16(idx int) (uint16, bool) {
	i := headerLen + idx
	if i < 0 || i+1 >= len(f.data) {
		return 0, false
	}
	return binary.LittleEndian.Uint16(f.data[i:]), true
}

func (f *frame) u32(idx int) (uint32, bool) {
	i := headerLen + idx
	if i < 0 || i+3 >= len(f.data) {
		return 0, false
	}
	return binary.LittleEndian.Uint32(f.data[i:]), true
}

// str reads an ASCII string from a fixed-width field. length<=0 reads to the
// end of the payload; otherwise it slices exactly length bytes and cuts there,
// so a value filling its field never runs on into the next struct member.
func (f *frame) str(idx, length int) (string, bool) {
	start := headerLen + idx
	if start < 0 || start > len(f.data) {
		return "", false
	}
	var raw []byte
	if length > 0 {
		end := start + length
		if end > len(f.data) {
			return "", false
		}
		raw = f.data[start:end]
	} else {
		raw = f.data[start:]
	}
	return cstr(raw), true
}

func (f *frame) uuid(idx int) (DeviceUID, bool) {
	var uid DeviceUID
	start := headerLen + idx
	if start < 0 || start+16 > len(f.data) {
		return uid, false
	}
	copy(uid[:], f.data[start:start+16])
	return uid, true
}

func (f *frame) bytesFrom(idx int) []byte {
	start := headerLen + idx
	if start < 0 || start >= len(f.data) {
		return nil
	}
	return f.data[start:]
}
