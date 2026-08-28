// Author: Lars Op den Kamp (lars@opdenkamp-it.nl)
// Copyright (c) 2026 Op den Kamp IT Solutions

package mxremote

import "encoding/binary"

// AmpSettings returns the amplifier zone settings for this bay, or nil.
func (b *Bay) AmpSettings() *AmpZoneSettings {
	r := b.dev.remote
	r.mu.Lock()
	defer r.mu.Unlock()
	if b.ampSettings == nil {
		return nil
	}
	v := *b.ampSettings
	return &v
}

func (b *Bay) setAmpSettings(s AmpZoneSettings) {
	changed := b.ampSettings == nil || *b.ampSettings != s
	b.ampSettings = &s
	if changed {
		r := b.dev.remote
		b.notify(pick(r.callbacks.OnAmpZoneSettingsChanged != nil, func() { r.callbacks.OnAmpZoneSettingsChanged(b, s) }))
	}
}

// Amp zone settings sizes and value range.
const (
	// ampZoneSettingsSize is sizeof(mxr_amp_zone_settings) on the wire.
	ampZoneSettingsSize = 56

	// AmpToneFlat is the neutral value for Bass, Treble and the EQ bands, which
	// are raw unsigned bytes despite the firmware header describing a signed
	// range.
	AmpToneFlat = 128

	// AmpToneHTTPMin and AmpToneHTTPMax are the bounds the amp's own HTTP API
	// enforces. Nothing enforces them here: the mesh receive path copies these
	// bytes straight through without a range check, so a peer can put any value
	// on the wire and the amp will take it. This library reports what arrived.
	// Clamping to these is imposing one device's HTTP policy on a mesh peer -
	// reasonable, but a caller's decision to make knowingly.
	AmpToneHTTPMin = 104
	AmpToneHTTPMax = 140
)

// ampZoneTarget resolves the device an amp-settings frame concerns: the payload
// UID, or the sender when that UID is zero.
//
// The two cases are different kinds of frame, not two spellings of one. An amp
// receiving 0x3D acts only when the payload target is its own uid, and it
// leaves that field zeroed when transmitting - so a zero-target frame is one no
// amp will ever accept, which makes it a status notification by construction
// and the header uid the only way to attribute it. A frame with the target set
// is a controller addressing one unit, and is a command rather than state.
//
// Both are cached here, since nothing in this library acts on a received frame
// and the mesh path applies commands unvalidated, so an addressed command is
// what that unit will end up holding.
func (r *Remote) ampZoneTarget(f *frame) *Device {
	uid := mustUUID(f, 0)
	if uid.Empty() {
		return r.devices[f.remoteID()]
	}
	return r.devices[uid]
}

func handleAmpZoneSettings(r *Remote, f *frame) {
	dev := r.ampZoneTarget(f)
	if dev == nil {
		return
	}
	p := f.payload()
	// The transmitter allocates sizeof(mxr_amp_zone_settings) and writes through
	// a struct pointer, so the wire image is the compiler's layout including its
	// padding and 2-byte tail: 56 bytes, not the 54 the fields occupy. The amp's
	// own receive path rejects anything shorter with the same constant.
	if len(p) < ampZoneSettingsSize {
		return
	}
	zone, ok := f.u16(16)
	if !ok {
		return
	}
	bay := dev.getByPortnumLocked(int(zone))
	if bay == nil {
		return
	}
	s := AmpZoneSettings{
		GainLeft:  int(p[18]),
		GainRight: int(p[19]),
		VolumeMin: int(p[20]),
		VolumeMax: int(p[21]),
		// mxr_amp_zone_settings is ALIGN(8), which is aligned(8) and not packed,
		// so the u32 delays cannot start at 22 - they pad to 24 and 28. The
		// rest of this record only decodes correctly with those two padding
		// bytes ahead of the delays rather than behind them: bass at 32,
		// power_auto_time at 40 and the eq bands at 44 and 49 all depend on it.
		//
		// Confirmed by offsetof compiled with the ProAmp8 toolchain. Both this
		// library and the Python reference previously read 22 and 26 and wrote
		// them there too, so a round trip agreed with itself while disagreeing
		// with the amp. It survived because the padding is zero: reading at 22
		// yields (delay & 0xFFFF) << 16, which is 0 for the 0 everybody had.
		DelayLeft:    binary.LittleEndian.Uint32(p[24:28]),
		DelayRight:   binary.LittleEndian.Uint32(p[28:32]),
		Bass:         int(p[32]),
		Treble:       int(p[33]),
		Bridged:      int(p[34]),
		PowerMode:    int(p[35]),
		PowerLevel:   int(p[36]),
		PowerTimeout: binary.LittleEndian.Uint32(p[40:44]),
	}
	for i := 0; i < 5; i++ {
		s.EQLeft[i] = int(p[44+i])
		s.EQRight[i] = int(p[49+i])
	}
	bay.setAmpSettings(s)
}

func handleAmpDolbySettings(r *Remote, f *frame) {
	dev := r.ampZoneTarget(f)
	if dev == nil {
		return
	}
	mode, ok := f.u8(16)
	if !ok {
		return
	}
	flags, _ := f.u8(17)
	dev.setDolbySettings(AmpDolbySettings{
		Mode:           int(mode),
		PCMUpmix:       flags&0x1 != 0,
		DolbyDetected:  flags&0x2 != 0,
		PCMUpmixActive: flags&0x4 != 0,
	})
}

// buildAmpZoneSettings assembles the 0x3D payload for the given target bay.
func buildAmpZoneSettings(deviceUID DeviceUID, port int, s AmpZoneSettings) []byte {
	p := make([]byte, 0, 56)
	p = append(p, deviceUID[:]...)
	p = append(p, byte(port), byte(port>>8))
	p = append(p, byte(s.GainLeft), byte(s.GainRight), byte(s.VolumeMin), byte(s.VolumeMax))
	p = append(p, 0, 0) // alignment ahead of the u32 delays, not behind them
	p = appendU32(p, s.DelayLeft)
	p = appendU32(p, s.DelayRight)
	p = append(p, byte(s.Bass), byte(s.Treble), byte(s.Bridged), byte(s.PowerMode), byte(s.PowerLevel))
	p = append(p, 0, 0, 0)
	p = appendU32(p, s.PowerTimeout)
	for i := 0; i < 5; i++ {
		p = append(p, byte(s.EQLeft[i]))
	}
	for i := 0; i < 5; i++ {
		p = append(p, byte(s.EQRight[i]))
	}
	p = append(p, 0, 0)
	return p
}

func appendU32(dst []byte, v uint32) []byte {
	return append(dst, byte(v), byte(v>>8), byte(v>>16), byte(v>>24))
}
