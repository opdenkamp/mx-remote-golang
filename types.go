// Author: Lars Op den Kamp (lars@opdenkamp-it.nl)
// Copyright (c) 2026 Op den Kamp IT Solutions

package mxremote

import (
	"encoding/binary"
	"fmt"
	"net"
)

// DeviceStatus is the high-level state of a device or bay on the network.
type DeviceStatus int

const (
	StatusOnline    DeviceStatus = 0
	StatusOffline   DeviceStatus = 1
	StatusRebooting DeviceStatus = 2
	StatusBooting   DeviceStatus = 3
	StatusInactive  DeviceStatus = 4
)

func (s DeviceStatus) String() string {
	switch s {
	case StatusOnline:
		return "Online"
	case StatusOffline:
		return "Offline"
	case StatusRebooting:
		return "Rebooting"
	case StatusBooting:
		return "Booting"
	case StatusInactive:
		return "Inactive"
	}
	return "Unknown"
}

// PowerStatus is the CEC power state of a device connected to a bay.
type PowerStatus int

const (
	PowerUnknown PowerStatus = 0
	PowerOn      PowerStatus = 1
	PowerOff     PowerStatus = 2
)

func (p PowerStatus) String() string {
	switch p {
	case PowerOn:
		return "on"
	case PowerOff:
		return "off"
	}
	return "unknown"
}

// ConnectStatus is the connect / signal-detect state reported for a bay.
type ConnectStatus int

const (
	ConnectUnknown ConnectStatus = 0
	Connected      ConnectStatus = 1
	Disconnected   ConnectStatus = 2
)

func (c ConnectStatus) String() string {
	switch c {
	case Connected:
		return "connected"
	case Disconnected:
		return "disconnected"
	}
	return "unknown"
}

// HiddenStatus is the visibility state of a bay.
type HiddenStatus int

const (
	HiddenUnknown HiddenStatus = 0
	Hidden        HiddenStatus = 1
	Visible       HiddenStatus = 2
)

func (h HiddenStatus) String() string {
	switch h {
	case Hidden:
		return "hidden"
	case Visible:
		return "visible"
	}
	return "unknown"
}

// audioDontChange is what a settable audio field carries when the sender is
// leaving it alone. Volume rejects it by exceeding 100, but as a mute bitfield
// it reads as both channels muted, so mute has to test for it.
const audioDontChange = 0xFF

// MuteStatus decodes the per-channel mute bitfield (bit0=left, bit1=right).
type MuteStatus uint8

func (m MuteStatus) Left() bool  { return m&(1<<0) != 0 }
func (m MuteStatus) Right() bool { return m&(1<<1) != 0 }
func (m MuteStatus) Muted() bool { return m != 0 }

// VolumeMuteStatus holds the volume and mute state of a bay. Left/right values
// of -1 mean "unknown".
type VolumeMuteStatus struct {
	VolumeLeft  int
	VolumeRight int
	MutedLeft   *bool
	MutedRight  *bool
}

// Volume returns the combined left/right volume percentage.
func (v VolumeMuteStatus) Volume() int {
	if v.VolumeRight < 0 {
		if v.VolumeLeft < 0 {
			return 0
		}
		return v.VolumeLeft
	}
	if v.VolumeLeft < 0 {
		return v.VolumeRight
	}
	return (v.VolumeLeft + v.VolumeRight) / 2
}

// Muted reports the combined mute state. The second result is false when the
// mute state is unknown.
func (v VolumeMuteStatus) Muted() (bool, bool) {
	if v.MutedLeft == nil && v.MutedRight == nil {
		return false, false
	}
	return (v.MutedLeft != nil && *v.MutedLeft) || (v.MutedRight != nil && *v.MutedRight), true
}

// wire encodes the 3-byte [volume_left, volume_right, muted] field.
func (v VolumeMuteStatus) wire() []byte {
	vl := v.VolumeLeft
	if vl < 0 {
		vl = v.Volume()
	}
	vr := v.VolumeRight
	if vr < 0 {
		vr = v.Volume()
	}
	return []byte{byte(vl), byte(vr), v.mutedValue()}
}

func (v VolumeMuteStatus) mutedValue() byte {
	muted, known := v.Muted()
	if !known || !muted {
		return 0
	}
	ml := v.MutedLeft != nil && *v.MutedLeft
	mr := v.MutedRight != nil && *v.MutedRight
	switch {
	case ml && mr:
		return 3
	case ml:
		return 1
	default:
		return 2
	}
}

// V2IPStreamSource is a single multicast stream address (video, audio, or ancillary).
type V2IPStreamSource struct {
	Label string
	IP    string
	Port  int
}

func (s V2IPStreamSource) String() string {
	return fmt.Sprintf("%s=%s:%d", s.Label, s.IP, s.Port)
}

// parseStreamSource decodes a v2ip_stream_source: 4 big-endian IP bytes followed
// by a little-endian u16 port. data must be at least 6 bytes.
func parseStreamSource(label string, data []byte) V2IPStreamSource {
	ip := net.IPv4(data[0], data[1], data[2], data[3]).String()
	port := int(data[4]) | int(data[5])<<8
	return V2IPStreamSource{Label: label, IP: ip, Port: port}
}

// V2IPStreamSources groups the video, audio, ancillary and (optional) ARC
// streams advertised by a single V2IP source, with the originating device UID
// when known (zero UID means unknown).
type V2IPStreamSources struct {
	UID   DeviceUID
	Video V2IPStreamSource
	Audio V2IPStreamSource
	Anc   V2IPStreamSource
	Arc   *V2IPStreamSource
}

func (s V2IPStreamSources) String() string {
	return fmt.Sprintf("video:%s audio:%s anc:%s", s.Video, s.Audio, s.Anc)
}

// V2IPAudioFormat overrides the sample rate and channel count of a V2IP audio
// stream. Zero values mean "use the firmware default".
type V2IPAudioFormat struct {
	SampleRate uint32
	Channels   uint8
}

func (f V2IPAudioFormat) wire() []byte {
	b := make([]byte, 8)
	binary.LittleEndian.PutUint32(b, f.SampleRate)
	b[4] = f.Channels
	return b
}

func parseAudioFormat(data []byte) (V2IPAudioFormat, bool) {
	if len(data) < 8 {
		return V2IPAudioFormat{}, false
	}
	return V2IPAudioFormat{
		SampleRate: binary.LittleEndian.Uint32(data[0:4]),
		Channels:   data[4],
	}, true
}

func (f V2IPAudioFormat) String() string {
	return fmt.Sprintf("%dHz/%dch", f.SampleRate, f.Channels)
}

// V2IPScalingSettings holds a V2IP output's scaling mode, refresh rate and flags.
type V2IPScalingSettings struct {
	Mode    MxrSignalType
	Refresh uint16
	Flags   uint8
}

// Scaling config flags. The two validity bits say which half of a scaling
// config a frame actually carries. Only these three bits are defined: bits
// 2..6 carry no meaning and are not reliably zero on the wire.
const (
	ScalingFlagModeValid    uint8 = 1 << 0
	ScalingFlagOptionsValid uint8 = 1 << 1
	ScalingFlagAutoScaling  uint8 = 1 << 7
)

// merge folds a received scaling config onto the cached one, field by field.
//
// A write carries the mode or the options alone, so taking the block wholesale
// would drop whichever half was not being written. The options branch replaces
// the option bit rather than adding to it, which is what lets an options-only
// write clear ScalingFlagAutoScaling.
//
// Only bit 7 is carried, not the whole top nibble. Firmware predating the fix
// builds this frame from an uninitialised stack local and ORs its flags onto
// whatever was there, so bits 2..6 are undefined noise on any receiver-capable
// unit. Copying the nibble - which is what the firmware's own receiver does -
// would cache that noise as though it meant something.
func (s V2IPScalingSettings) merge(previous V2IPScalingSettings) V2IPScalingSettings {
	out := previous
	if s.Flags&ScalingFlagModeValid != 0 {
		out.Mode = s.Mode
		out.Refresh = s.Refresh
		out.Flags |= ScalingFlagModeValid
	}
	if s.Flags&ScalingFlagOptionsValid != 0 {
		out.Flags &^= ScalingFlagAutoScaling
		out.Flags |= ScalingFlagOptionsValid
		out.Flags |= s.Flags & ScalingFlagAutoScaling
	}
	return out
}

// V2IPDscpConfig is the per-stream DSCP marking in a V2IP device configuration.
//
// A stream whose wire byte carries no V2IPDscpSet bit reads back as nil here.
// Firmware treats the marking as all-or-nothing: it applies one only when all
// three streams carry a value and otherwise falls back to V2IPDscpDefault, so
// Complete reports which case a frame is in.
type V2IPDscpConfig struct {
	Video *uint8
	Audio *uint8
	Anc   *uint8
}

// Complete reports whether all three streams carry a marking, which is what
// firmware requires before it applies one.
func (d V2IPDscpConfig) Complete() bool {
	return d.Video != nil && d.Audio != nil && d.Anc != nil
}

func (d V2IPDscpConfig) String() string {
	if !d.Complete() {
		return "no marking"
	}
	return fmt.Sprintf("video:%d audio:%d anc:%d", *d.Video, *d.Audio, *d.Anc)
}

// parseDscp decodes one dscp byte from a V2IP device-config options word, or
// nil when the byte carries no marking.
func parseDscp(raw uint8) *uint8 {
	if raw&V2IPDscpSet == 0 {
		return nil
	}
	v := raw & V2IPDscpMax
	return &v
}

// DeviceV2IPDetails is the local encoder/decoder configuration of a V2IP device.
type DeviceV2IPDetails struct {
	Video V2IPStreamSource
	Audio V2IPStreamSource
	Anc   V2IPStreamSource
	Arc   V2IPStreamSource

	// TxRate is the encoder rate in units of 10Mb/s, or nil when the sender
	// offered no rate. A rate-only write carries the rate on its own; every
	// other controller write puts a value outside V2IPSourceRateMin..Max here,
	// which firmware drops as invalid so that address-only and scaling writes
	// leave the peer's rate alone.
	TxRate *uint8

	Dscp    V2IPDscpConfig
	Scaling V2IPScalingSettings
}

// Valid reports whether a stream source carries a usable address: a multicast
// IP and a non-zero port, both — the firmware's own validity test.
func (s V2IPStreamSource) Valid() bool {
	ip := net.ParseIP(s.IP)
	return ip != nil && ip.To4() != nil && ip.IsMulticast() && s.Port != 0
}

// sourceValid reports whether the source block carries usable addresses.
// Firmware requires video and anc; audio is optional and is carried with them.
func (v DeviceV2IPDetails) sourceValid() bool {
	return v.Video.Valid() && v.Anc.Valid()
}

// merge folds a received device configuration onto the cached one.
//
// Every field here is optional behind its own validity marker: the frame's
// payload is zeroed before a sender fills in the one field it is writing, so a
// controller writing a TX rate sends zeroed addresses and a controller writing
// addresses sends an out-of-range rate. Firmware applies each field only behind
// its own test, so replacing the whole cached config on every frame would make
// the peer read back with its addresses, rate or marking gone.
func (v DeviceV2IPDetails) merge(previous *DeviceV2IPDetails) DeviceV2IPDetails {
	if previous == nil {
		return v
	}
	if !v.sourceValid() {
		v.Video, v.Audio, v.Anc = previous.Video, previous.Audio, previous.Anc
	}
	if !v.Arc.Valid() {
		v.Arc = previous.Arc
	}
	if v.TxRate == nil {
		v.TxRate = previous.TxRate
	}
	// firmware gates all three dscp bytes on the video byte's set bit alone,
	// and stores whatever the other two carry
	if v.Dscp.Video == nil {
		v.Dscp = previous.Dscp
	}
	v.Scaling = v.Scaling.merge(previous.Scaling)
	return v
}

// BaySignalDetails is what a bay signal status report carries beyond the
// signal-detected flag and the human-readable signal type.
type BaySignalDetails struct {
	// FrameRate is in Hz, already corrected for a 1000/1001 clock.
	FrameRate float64

	// TmdsClock is the TMDS clock rate in Hz.
	TmdsClock uint32

	// Status is the bay status word from the report's bay block.
	Status BayStatusMask

	// Scaling is the signal type the bay is scaling to.
	Scaling MxrSignalType

	// ClockRate is the video clock rate in Hz.
	ClockRate uint32
}

// MxrSignalType is the 2-byte signal type carried in scaling configs and bay
// signal reports.
//
// Byte 0 is the CTA-861 svd (0 when the signal is not HDMI); byte 1 packs
// color:4 in the low nibble, then non_int:1 and bpp:3 in the top bits.
type MxrSignalType uint16

// signal type bpp indices; the field is an index, not a bit depth.
const (
	sigBppUnknown = 0
	sigBppUnset   = 5
)

// Svd returns the CTA-861 short video descriptor, 0 when the signal is not HDMI.
func (t MxrSignalType) Svd() int { return int(t & 0xFF) }

// ColourSpace returns the colour space (see VideoColourSpace).
func (t MxrSignalType) ColourSpace() int { return int(t>>8) & 0xF }

// NonInteger reports a 1000/1001 frame rate.
func (t MxrSignalType) NonInteger() bool { return t&(1<<12) != 0 }

// BppIndex returns the raw bpp index as carried on the wire.
func (t MxrSignalType) BppIndex() int { return int(t>>13) & 0x7 }

// Bpp returns the bit depth the bpp index stands for, 0 when unknown or unset.
func (t MxrSignalType) Bpp() int {
	switch t.BppIndex() {
	case 1:
		return 8
	case 2:
		return 10
	case 3:
		return 12
	case 4:
		return 16
	}
	return sigBppUnknown
}

// IsSet reports whether the signal type carries anything but the unset sentinel.
func (t MxrSignalType) IsSet() bool { return t.BppIndex() != sigBppUnset }

func (t MxrSignalType) String() string {
	if !t.IsSet() {
		return "unset"
	}
	if bpp := t.Bpp(); bpp != 0 {
		return fmt.Sprintf("svd %d, color %d, %dbpp", t.Svd(), t.ColourSpace(), bpp)
	}
	return fmt.Sprintf("svd %d, color %d", t.Svd(), t.ColourSpace())
}

// DeviceV2IPSink is the sink-side route a V2IP device is currently subscribed to.
type DeviceV2IPSink struct {
	Addresses V2IPStreamSources
	AudioFmt  *V2IPAudioFormat
}

// FirmwareVersion describes a firmware component reported by a device.
type FirmwareVersion struct {
	Type      FirmwareType
	Timestamp int64
	Version   string
	Hash      uint32
}

func (f FirmwareVersion) String() string {
	return fmt.Sprintf("firmware %s version %s hash %d", f.Type, f.Version, f.Hash)
}

// BayMirrorStatus describes whether an output bay mirrors another device's
// output, and which one. The zero value means "not mirroring".
type BayMirrorStatus struct {
	Target *BayUID
}

// IsMirroring reports whether this bay mirrors another.
func (m BayMirrorStatus) IsMirroring() bool { return m.Target != nil }

func (m BayMirrorStatus) equal(o BayMirrorStatus) bool {
	if m.Target == nil || o.Target == nil {
		return m.Target == nil && o.Target == nil
	}
	return *m.Target == *o.Target
}

// TopologyEntry is one device in a topology report: a device UID and the
// bitmask of devices it is connected to.
type TopologyEntry struct {
	UID  DeviceUID
	Mask uint32
}

// UtpLinkErrors is the decoded link-error bitmask for a network port.
type UtpLinkErrors struct {
	InError       bool
	InFCSError    bool
	InCollision   bool
	OutDeferred   bool
	OutExcessive  bool
	PolarityError bool
	SkewWarning   bool
	LengthWarning bool
}

func parseUtpLinkErrors(v uint8) UtpLinkErrors {
	return UtpLinkErrors{
		InError:       v&(1<<0) != 0,
		InFCSError:    v&(1<<1) != 0,
		InCollision:   v&(1<<2) != 0,
		OutDeferred:   v&(1<<3) != 0,
		OutExcessive:  v&(1<<4) != 0,
		PolarityError: v&(1<<5) != 0,
		SkewWarning:   v&(1<<6) != 0,
		LengthWarning: v&(1<<7) != 0,
	}
}

// UtpCableStatus is the diagnostic status of a single UTP cable pair.
type UtpCableStatus struct {
	Polarity bool
	Pair     int
	Skew     uint32
	Length   uint32
}

// NetworkPortStatus describes the link state and diagnostics of a network port.
// Optional fields (Errors, VCTStatus, CableStatus) are nil when the port or
// firmware does not report them.
type NetworkPortStatus struct {
	Port           int
	Name           string
	LinkSpeed      UtpLinkSpeed
	LinkFullDuplex bool
	IP             string
	Querier        string
	MACAddress     string
	Errors         *UtpLinkErrors
	VCTStatus      []string
	CableStatus    []UtpCableStatus
}

// AmpZoneSettings is a ProAmp8 zone's gain, delay, tone and power settings.
//
// Bass, Treble and the EQ bands are raw unsigned bytes with AmpToneFlat as
// neutral, not signed values - the firmware header's "-104 to 152" is wrong at
// both ends. AmpToneHTTPMin and AmpToneHTTPMax bound what the amp's HTTP API
// accepts; the mesh path validates nothing, so any byte can arrive here. Gains
// are int16 inside the amp but a byte on the mesh, so a frame cannot carry the
// amp's full internal range.
type AmpZoneSettings struct {
	GainLeft     int
	GainRight    int
	VolumeMin    int
	VolumeMax    int
	DelayLeft    uint32
	DelayRight   uint32
	Bass         int
	Treble       int
	Bridged      int
	PowerMode    int
	PowerLevel   int
	PowerTimeout uint32
	EQLeft       [5]int
	EQRight      [5]int
}

// AmpDolbySettings holds the Dolby processing state of a ProAmp8.
type AmpDolbySettings struct {
	Mode           int
	PCMUpmix       bool
	DolbyDetected  bool
	PCMUpmixActive bool
}
