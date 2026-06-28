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

// ampZoneTarget resolves the device an amp-settings frame applies to: the
// payload UID, or the sender when that UID is zero.
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
	if len(p) < 54 {
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
		GainLeft:     int(p[18]),
		GainRight:    int(p[19]),
		VolumeMin:    int(p[20]),
		VolumeMax:    int(p[21]),
		DelayLeft:    binary.LittleEndian.Uint32(p[22:26]),
		DelayRight:   binary.LittleEndian.Uint32(p[26:30]),
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
	p = appendU32(p, s.DelayLeft)
	p = appendU32(p, s.DelayRight)
	p = append(p, 0, 0)
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
