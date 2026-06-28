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
		IP:             ipBigEndian(d[132:136]),
		Querier:        ipBigEndian(d[136:140]),
		CableStatus:    parseCablePairs(d, 8, 20, 32, 44),
	}
	if protocol >= 0x21 {
		s.MACAddress = macString(d[140:146])
	}
	return s, true
}

// parseNetworkStatus decodes the network-port-status payload for protocol 0x22+.
func parseNetworkStatus(d []byte, protocol uint16) (NetworkPortStatus, bool) {
	if len(d) < 39 {
		return NetworkPortStatus{}, false
	}
	features := d[2]
	supportStatus := features&(1<<0) != 0
	supportCable := features&(1<<1) != 0
	supportIGMP := features&(1<<3) != 0
	portUplink := features&(1<<6) != 0

	s := NetworkPortStatus{
		Port:           int(binary.LittleEndian.Uint16(d[0:2])),
		Name:           cstr(d[4:20]),
		LinkSpeed:      UtpLinkSpeed(d[38] & 0x7),
		LinkFullDuplex: d[38]&(1<<3) != 0,
	}
	if portUplink && len(d) >= 32 {
		s.MACAddress = macString(d[21:27])
		s.IP = ipBigEndian(d[28:32])
		if supportIGMP && len(d) >= 36 {
			s.Querier = ipBigEndian(d[32:36])
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
