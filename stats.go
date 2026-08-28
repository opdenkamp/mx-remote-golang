// Author: Lars Op den Kamp (lars@opdenkamp-it.nl)
// Copyright (c) 2026 Op den Kamp IT Solutions

package mxremote

import (
	"encoding/binary"
	"fmt"
)

// V2IPTxStats holds transmitter stream statistics.
type V2IPTxStats struct {
	Video      uint32
	Audio      uint32
	Anc        uint32
	StreamDown uint32
	Overflow   uint32
}

// V2IPDecoderState is the health state of a V2IP decoder.
type V2IPDecoderState int

const (
	DecoderUnknown V2IPDecoderState = 0
	DecoderHealthy V2IPDecoderState = 1
	DecoderBad     V2IPDecoderState = 2
	// DecoderStarting is a decoder still coming up, which any sink subscribed
	// to during a route change reports.
	DecoderStarting V2IPDecoderState = 3
)

// Settled reports whether the decoder has reached a verdict.
//
// Only Healthy and Bad are verdicts; Unknown and Starting both mean the sink
// has not said yet. Testing for failure as "not Healthy" reads a receiver that
// is merely coming up as one that failed to decode, which is what a sink
// reports for a moment after every route change.
func (s V2IPDecoderState) Settled() bool {
	return s == DecoderHealthy || s == DecoderBad
}

func (s V2IPDecoderState) String() string {
	switch s {
	case DecoderUnknown:
		return "Unknown"
	case DecoderHealthy:
		return "Healthy"
	case DecoderBad:
		return "Bad"
	case DecoderStarting:
		return "Starting"
	}
	return fmt.Sprintf("state %d", int(s))
}

// V2IPRxStats holds receiver stream statistics.
type V2IPRxStats struct {
	VideoTotal     uint32
	VideoDropped   uint32
	VideoSeqErrors uint32
	WdtTimeout     uint32
	AudioTotal     uint32
	AudioDropped   uint32
	AudioSeqErrors uint32
	AncTotal       uint32
	AncDropped     uint32
	AncSeqErrors   uint32
	DecoderState   V2IPDecoderState
}

// Block sizes of the 0x3F payload. fpga_tx_stats and fpga_rx_stats are 20 and
// 44 rather than 24 and 48 because their ALIGN(8) sits before the struct
// keyword, where GCC ignores it. The 128-byte total is therefore stable by
// accident: correcting those declarations would shift every block after the
// first while changing nothing a reader of the header could detect, so pin the
// sizes rather than only the field offsets.
const (
	txStatsSize   = 20
	rxStatsSize   = 44
	v2ipStatsSize = 2*txStatsSize + 2*rxStatsSize
)

// V2IPDeviceStats holds the cumulative and per-minute TX/RX statistics.
type V2IPDeviceStats struct {
	Tx          V2IPTxStats
	TxPerMinute V2IPTxStats
	Rx          V2IPRxStats
	RxPerMinute V2IPRxStats
}

func parseTxStats(d []byte) V2IPTxStats {
	return V2IPTxStats{
		Video:      binary.LittleEndian.Uint32(d[0:4]),
		Audio:      binary.LittleEndian.Uint32(d[4:8]),
		Anc:        binary.LittleEndian.Uint32(d[8:12]),
		StreamDown: binary.LittleEndian.Uint32(d[12:16]),
		Overflow:   binary.LittleEndian.Uint32(d[16:20]),
	}
}

func parseRxStats(d []byte) V2IPRxStats {
	u := func(o int) uint32 { return binary.LittleEndian.Uint32(d[o : o+4]) }
	return V2IPRxStats{
		VideoTotal:     u(0),
		VideoDropped:   u(4),
		VideoSeqErrors: u(8),
		WdtTimeout:     u(12),
		AudioTotal:     u(16),
		AudioDropped:   u(20),
		AudioSeqErrors: u(24),
		AncTotal:       u(28),
		AncDropped:     u(32),
		AncSeqErrors:   u(36),
		DecoderState:   V2IPDecoderState(d[40]),
	}
}

// handleV2IPStats (opcode 0x3F) decodes encoder/decoder statistics. Frames of
// length 17 are enable/disable requests and carry no statistics.
func handleV2IPStats(r *Remote, f *frame) {
	dev := r.deviceFor(f)
	if dev == nil {
		return
	}
	p := f.payload()
	if len(p) == 17 || len(p) < v2ipStatsSize {
		return
	}
	rx := 2 * txStatsSize
	dev.setV2IPStats(V2IPDeviceStats{
		Tx:          parseTxStats(p[0:txStatsSize]),
		TxPerMinute: parseTxStats(p[txStatsSize : 2*txStatsSize]),
		Rx:          parseRxStats(p[rx : rx+rxStatsSize]),
		RxPerMinute: parseRxStats(p[rx+rxStatsSize : rx+2*rxStatsSize]),
	})
}
