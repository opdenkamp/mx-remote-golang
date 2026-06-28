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
	Mode    uint16
	Refresh uint16
	Flags   uint8
}

// DeviceV2IPDetails is the local encoder/decoder configuration of a V2IP device.
type DeviceV2IPDetails struct {
	Video   V2IPStreamSource
	Audio   V2IPStreamSource
	Anc     V2IPStreamSource
	Arc     V2IPStreamSource
	TxRate  uint8
	Scaling V2IPScalingSettings
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

// AmpZoneSettings holds the per-zone settings of a ProAmp8 output.
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
