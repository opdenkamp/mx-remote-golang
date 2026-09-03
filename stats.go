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

	// decoderDetailSize is 20 bytes of fields rounded up by ALIGN(8), so the
	// last four bytes are padding a sender clears rather than a field.
	decoderDetailSize = 24
)

// V2IPDecoderReason is the primary cause a decoder reports for its current
// state. Values are added by firmware updates, so an unrecognised one is kept
// as it arrived rather than folded onto a named cause.
type V2IPDecoderReason uint8

const (
	DecoderReasonOK V2IPDecoderReason = 0
	// DecoderReasonNoPackets means no codestream is arriving at all.
	DecoderReasonNoPackets V2IPDecoderReason = 1
	// DecoderReasonDegraded means packets arrive but the stream is impaired.
	DecoderReasonDegraded V2IPDecoderReason = 2
	// DecoderReasonNoFormat means no format could be recovered from the
	// codestream.
	DecoderReasonNoFormat V2IPDecoderReason = 3
	// DecoderReasonFormatMismatch means the recovered format differs from the
	// one configured.
	DecoderReasonFormatMismatch V2IPDecoderReason = 4
	// DecoderReasonFormatRejected means the configured output format was
	// refused.
	DecoderReasonFormatRejected V2IPDecoderReason = 5
	// DecoderReasonDecoderBlocked means the converter watchdog is holding the
	// stream back.
	DecoderReasonDecoderBlocked V2IPDecoderReason = 6
	// DecoderReasonSwitchPending is a source switch in progress. It is not a
	// fault.
	DecoderReasonSwitchPending V2IPDecoderReason = 7
	// DecoderReasonPTPUnlocked affects audio only; the picture is fine.
	DecoderReasonPTPUnlocked V2IPDecoderReason = 8
	// DecoderReasonTxBridgeUnlocked is the HDMI transmitter unlocked while the
	// pipeline rebuilds, and it ranks below every input-side cause - a sink
	// rebuilding in a loop names an input cause in Reason and carries this in
	// Flags alone, so look for it there rather than in Reason.
	//
	// Three things the name does not carry. It is debounced, so it never
	// reports a transient and the picture has been down several seconds by the
	// time it appears. The debounce restarts each time it elapses, so the flag
	// staying set across reports means a restart loop rather than one long
	// event. And it is not evaluated across a format change: it holds its
	// previous value and clears on the first reading after things settle,
	// which Updates cannot tell apart from a fresh reading.
	DecoderReasonTxBridgeUnlocked V2IPDecoderReason = 9
	// DecoderReasonIdle is a configured sink that is not enabled. Devices are
	// not observed to report it: a sink switched off deliberately reports
	// DecoderReasonNoPackets and holds there, indistinguishable from one whose
	// source is dead. Nothing in this block says a sink was switched off on
	// purpose - ask V2IP_DEVICE_CFG or the device HTTP status instead.
	DecoderReasonIdle V2IPDecoderReason = 10
)

func (r V2IPDecoderReason) String() string {
	switch r {
	case DecoderReasonOK:
		return "OK"
	case DecoderReasonNoPackets:
		return "no packets"
	case DecoderReasonDegraded:
		return "packets degraded"
	case DecoderReasonNoFormat:
		return "no format recovered"
	case DecoderReasonFormatMismatch:
		return "format mismatch"
	case DecoderReasonFormatRejected:
		return "format rejected"
	case DecoderReasonDecoderBlocked:
		return "decoder blocked"
	case DecoderReasonSwitchPending:
		return "switch pending"
	case DecoderReasonPTPUnlocked:
		return "PTP unlocked"
	case DecoderReasonTxBridgeUnlocked:
		return "TX bridge unlocked"
	case DecoderReasonIdle:
		return "idle"
	}
	return fmt.Sprintf("reason %d", uint8(r))
}

// V2IPDecoderFormat is the colour space a decoder recovered from a codestream.
//
// This is not the colour space of a signal report: MxrSignalType.ColourSpace
// is a 4-bit field whose 0xF means unknown, whereas here the same idea is 255.
// The two must not be converted into one another.
type V2IPDecoderFormat uint16

const (
	DecoderFormatRGB      V2IPDecoderFormat = 0
	DecoderFormatYCbCr444 V2IPDecoderFormat = 1
	DecoderFormatYCbCr422 V2IPDecoderFormat = 2
	DecoderFormatYCbCr420 V2IPDecoderFormat = 3
	// DecoderFormatUnnamed means the decoder cannot name the format.
	DecoderFormatUnnamed V2IPDecoderFormat = 255
)

func (f V2IPDecoderFormat) String() string {
	switch f {
	case DecoderFormatRGB:
		return "RGB"
	case DecoderFormatYCbCr444:
		return "YCbCr 4:4:4"
	case DecoderFormatYCbCr422:
		return "YCbCr 4:2:2"
	case DecoderFormatYCbCr420:
		return "YCbCr 4:2:0"
	case DecoderFormatUnnamed:
		return "unnamed"
	}
	return fmt.Sprintf("format %d", uint16(f))
}

// V2IPDecoderDetail is what a sink's decoder recovered from the codestream it
// is being given, as opposed to what came out of the scaler after it.
//
// Colour depth is deliberately absent and stays absent: it is not recovered
// from the codestream, so it was withheld rather than shipped as a number that
// looks like a measurement. Assert bit depth at the encoder's input bay
// instead.
type V2IPDecoderDetail struct {
	// Reason is for display, not for classification: it is whichever cause won
	// a fixed priority order that is deliberately not the numbering, so a value
	// can be appended without reordering the rest. It is therefore neither the
	// lowest nor the highest bit of Flags and cannot be derived from it at all.
	// Classify on Flags, via HasReason.
	Reason V2IPDecoderReason

	// Blocking reports that the converter watchdog is holding the stream back.
	Blocking bool

	// Width and Height are recovered from the codestream, pre-scaler and
	// unrounded, and are 0 when nothing was recovered. They, not Format, are
	// what says whether the decoder recovered anything - see Recovered.
	Width  uint16
	Height uint16

	// Format is the recovered colour space. It is never a no-signal indicator
	// at any value: with no stream it reads DecoderFormatRGB, which a real RGB
	// reading is indistinguishable from.
	Format V2IPDecoderFormat

	// Updates counts readings actually stored, so a stalled video processor
	// leaves it still rather than implying a refresh. Monotonic, never reset,
	// wraps at 65535 (about 36 hours).
	//
	// The processor is read every 2s and the report goes out every 1s, latched
	// between reads, so roughly every other report repeats a reading already
	// seen. After changing what a sink is pointed at, wait for Updates to
	// advance by two before trusting the geometry: the counter ticks when a
	// reply lands rather than when a query is sent, so a single tick can carry
	// an answer read fractionally before the switch.
	Updates uint16

	// Flags has bit N set for reason N, so it carries every cause that applies
	// where Reason carries only the one that won on priority. Bits with no
	// named reason are causes this library has no name for yet - see HasReason.
	//
	// Three invariants hold: bit 0 is force-cleared, so an empty word means
	// nothing applies; DecoderReasonNoFormat and DecoderReasonFormatMismatch
	// are the two arms of one decision and never both appear; and the priority
	// ranks behind Reason are stable when a value is appended.
	Flags uint32

	// BlockedCount is how many times the converter watchdog has triggered.
	BlockedCount uint32
}

// Recovered reports whether the decoder recovered a picture from the
// codestream. Geometry is what answers that; Format cannot, since it reads
// DecoderFormatRGB when there is no stream at all.
func (d *V2IPDecoderDetail) Recovered() bool { return d.Width != 0 && d.Height != 0 }

// HasReason reports whether cause r is among those that apply, including a
// cause this library has no name for. DecoderReasonOK is never among them:
// the flags word carries causes, and bit 0 is unused.
func (d *V2IPDecoderDetail) HasReason(r V2IPDecoderReason) bool {
	return r != DecoderReasonOK && r < 32 && d.Flags&(1<<uint(r)) != 0
}

// V2IPDeviceStats holds the cumulative and per-minute TX/RX statistics.
type V2IPDeviceStats struct {
	Tx          V2IPTxStats
	TxPerMinute V2IPTxStats
	Rx          V2IPRxStats
	RxPerMinute V2IPRxStats

	// DecoderReported is true when the report carried the decoder block at
	// all. A sender predating the block sends a 128-byte payload and carries
	// none, which is a different thing from a decoder that has never answered.
	DecoderReported bool

	// Decoder is the decoder's reading, nil while the decoder has never
	// answered - including when DecoderReported is false. Its fields carry
	// nothing in that state, so there is no zeroed struct to mistake for a
	// reading of 0x0.
	Decoder *V2IPDecoderDetail
}

// clone deep-copies the stats so a caller cannot reach the cached Decoder.
func (s V2IPDeviceStats) clone() V2IPDeviceStats {
	if s.Decoder != nil {
		d := *s.Decoder
		s.Decoder = &d
	}
	return s
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

// parseDecoderDetail decodes the decoder block, returning nil when the decoder
// has never answered and every field behind the valid marker is meaningless.
// Byte 3 is reserved and bytes 20..23 are alignment padding; neither is read.
func parseDecoderDetail(d []byte) *V2IPDecoderDetail {
	if d[0] == 0 {
		return nil
	}
	u16 := func(o int) uint16 { return binary.LittleEndian.Uint16(d[o : o+2]) }
	return &V2IPDecoderDetail{
		Reason:       V2IPDecoderReason(d[1]),
		Blocking:     d[2] != 0,
		Width:        u16(4),
		Height:       u16(6),
		Format:       V2IPDecoderFormat(u16(8)),
		Updates:      u16(10),
		Flags:        binary.LittleEndian.Uint32(d[12:16]),
		BlockedCount: binary.LittleEndian.Uint32(d[16:20]),
	}
}

// handleV2IPStats (opcode 0x3F) decodes encoder/decoder statistics. Frames of
// length 17 are enable/disable requests and carry no statistics.
//
// The decoder block appended at 128 is present or absent by payload length,
// not by the frame's protocol stamp: length survives a further block being
// appended after it, where a stamp comparison would reject the longer payload
// whole. Parse the prefix understood here and ignore any tail.
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
	stats := V2IPDeviceStats{
		Tx:          parseTxStats(p[0:txStatsSize]),
		TxPerMinute: parseTxStats(p[txStatsSize : 2*txStatsSize]),
		Rx:          parseRxStats(p[rx : rx+rxStatsSize]),
		RxPerMinute: parseRxStats(p[rx+rxStatsSize : rx+2*rxStatsSize]),
	}
	if len(p) >= v2ipStatsSize+decoderDetailSize {
		stats.DecoderReported = true
		stats.Decoder = parseDecoderDetail(p[v2ipStatsSize : v2ipStatsSize+decoderDetailSize])
	}
	dev.setV2IPStats(stats)
}
