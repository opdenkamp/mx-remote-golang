// Author: Lars Op den Kamp (lars@opdenkamp-it.nl)
// Copyright (c) 2026 Op den Kamp IT Solutions

package mxremote

import (
	"encoding/binary"
	"fmt"
	"net"
)

func vctStatus(v uint8) []string {
	rv := make([]string, 4)
	for i := 0; i < 4; i++ {
		if v&(1<<uint(i)) != 0 {
			rv[i] = "WARNING"
		} else {
			rv[i] = "healthy"
		}
	}
	return rv
}

func parseCablePairs(d []byte, offsets ...int) []UtpCableStatus {
	rv := make([]UtpCableStatus, 0, len(offsets))
	for _, o := range offsets {
		if o+12 > len(d) {
			break
		}
		rv = append(rv, UtpCableStatus{
			Polarity: d[o] == 1,
			Pair:     int(d[o+1]),
			Skew:     binary.LittleEndian.Uint32(d[o+4 : o+8]),
			Length:   binary.LittleEndian.Uint32(d[o+8 : o+12]),
		})
	}
	return rv
}

func ipBigEndian(d []byte) string {
	return net.IPv4(d[0], d[1], d[2], d[3]).String()
}

func macString(d []byte) string {
	return fmt.Sprintf("%02X:%02X:%02X:%02X:%02X:%02X", d[0], d[1], d[2], d[3], d[4], d[5])
}

// parseNetworkStatusPre22 decodes the network-port-status payload for protocol
// versions before 0x22.
func parseNetworkStatusPre22(d []byte, protocol uint16) (NetworkPortStatus, bool) {
	if len(d) < 146 {
		return NetworkPortStatus{}, false
	}
	errs := parseUtpLinkErrors(d[1])
	s := NetworkPortStatus{
		Port:           int(d[0]),
		Errors:         &errs,
		VCTStatus:      vctStatus(d[2]),
		LinkSpeed:      UtpLinkSpeed(d[3] & 0x7),
		LinkFullDuplex: d[3]&(1<<3) != 0,
		Name:           cstr(d[112:128]),
		CableStatus:    parseCablePairs(d, 8, 20, 32, 44),
	}
	// The legacy struct grew by appending, so each field only exists from the
	// version that added it: ip/querier at 0x12, mac at 0x21. Below those the
	// bytes belong to whatever follows the struct.
	if protocol >= 0x12 {
		s.IP = ipBigEndian(d[132:136])
		s.Querier = ipBigEndian(d[136:140])
	}
	if protocol >= 0x21 {
		s.MACAddress = macString(d[140:146])
	}
	return s, true
}

// netStatusLate reports whether a 0x22-stamped payload uses the later layout.
//
// Two layouts share the 0x22 stamp: port and features_status were widened from
// u8 to u16 without bumping any version. Only the fields ahead of ip move - mac
// ends at 24 or 26, and ip4_addr_t aligns to 4, so ip, querier and the status
// block sit at 28, 32 and 36 either way. The ambiguity is confined to bytes
// 0..27: port, features, name and mac. Both layouts are confirmed by compiled
// offsetof assertions, and both are 144 bytes - so neither the length nor the
// version can tell them apart, and the ground has to be the payload.
//
// Testing whether name looks like text does not work: an early-form name of
// three characters or more puts a printable byte at 4 and reads as late. What
// separates them is where the zero bytes fall. The later layout widened two
// small fields, so byte 1 is the high byte of port and byte 3 the high byte of
// features_status; the earlier layout has features at 1 and a name character
// at 3.
//
// That rests on features_status staying under 0x100. Only bits 0..6 are defined
// today (MX_NETLINK_SUPPORT_STATUS through MX_NETLINK_STATUS_UPLINK, max 0x7F),
// leaving nine free. If the field ever grows past a byte, byte 3 stops being
// zero and every late frame decodes as early - which would present as a decode
// bug rather than as a widened field, so check this first.
//
// A single-character early name also leaves byte 3 zero and is genuinely
// ambiguous; the later layout is the tie-break, being what every device on a
// live mesh was observed to emit, including units on much older firmware.
func netStatusLate(d []byte) bool {
	if len(d) < 4 {
		return true
	}
	return d[1] == 0 && d[3] == 0
}

// parseNetworkStatus decodes the network-port-status payload for protocol 0x22+.
func parseNetworkStatus(d []byte, protocol uint16) (NetworkPortStatus, bool) {
	if len(d) < 39 {
		return NetworkPortStatus{}, false
	}
	late := netStatusLate(d)
	// features_status is a u16 at 2 in the later layout and a u8 at 1 in the
	// earlier one; the flag bits live in its low byte either way
	features := d[1]
	if late {
		features = d[2]
	}
	supportStatus := features&(1<<0) != 0
	supportCable := features&(1<<1) != 0
	supportIGMP := features&(1<<3) != 0
	portUplink := features&(1<<6) != 0

	// name is mxr_device_name, char[MXR_DEVICE_NAME_LEN + 1], so unlike the
	// bare char[16] name fields elsewhere it always has room for a terminator
	nameOff, macOff := 4, 21
	port := int(binary.LittleEndian.Uint16(d[0:2]))
	if !late {
		nameOff, macOff = 2, 19
		port = int(d[0])
	}
	const ipOff, querierOff = 28, 32
	s := NetworkPortStatus{
		Port:           port,
		Name:           cstr(d[nameOff : nameOff+17]),
		LinkSpeed:      UtpLinkSpeed(d[38] & 0x7),
		LinkFullDuplex: d[38]&(1<<3) != 0,
	}
	if portUplink && len(d) >= querierOff {
		s.MACAddress = macString(d[macOff : macOff+6])
		s.IP = ipBigEndian(d[ipOff : ipOff+4])
		if supportIGMP && len(d) >= querierOff+4 {
			s.Querier = ipBigEndian(d[querierOff : querierOff+4])
		}
	}
	if supportStatus && len(d) >= 38 {
		errs := parseUtpLinkErrors(d[36])
		s.Errors = &errs
		s.VCTStatus = vctStatus(d[37])
	}
	if supportCable {
		s.CableStatus = parseCablePairs(d, 40, 52, 64, 76)
	}
	return s, true
}

// handleNetworkStatus (opcode 0x29) decodes a network port status report.
func handleNetworkStatus(r *Remote, f *frame) {
	dev := r.deviceFor(f)
	if dev == nil {
		return
	}
	p := f.payload()
	var (
		status NetworkPortStatus
		ok     bool
	)
	if f.protocol() < 0x22 {
		status, ok = parseNetworkStatusPre22(p, f.protocol())
	} else {
		status, ok = parseNetworkStatus(p, f.protocol())
	}
	if ok {
		dev.updateNetworkStatus(status)
	}
}
