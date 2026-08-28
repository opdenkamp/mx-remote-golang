// Author: Lars Op den Kamp (lars@opdenkamp-it.nl)
// Copyright (c) 2026 Op den Kamp IT Solutions

package mxremote

import (
	"fmt"
	"net"
)

func ip4Bytes(ip string) []byte {
	if ip == "" {
		return []byte{0, 0, 0, 0}
	}
	p := net.ParseIP(ip).To4()
	if p == nil {
		return []byte{0, 0, 0, 0}
	}
	return []byte{p[0], p[1], p[2], p[3]}
}

func appendStreamAddr(dst []byte, ip string, port int) []byte {
	dst = append(dst, ip4Bytes(ip)...)
	return append(dst, byte(port), byte(port>>8), 0, 0)
}

// buildV2IPSourceSwitch builds the 0x1F payload: sink uid + video/audio source IPs.
func buildV2IPSourceSwitch(sink DeviceUID, videoIP, audioIP string) []byte {
	p := make([]byte, 0, 24)
	p = append(p, sink[:]...)
	p = append(p, ip4Bytes(videoIP)...)
	p = append(p, ip4Bytes(audioIP)...)
	return p
}

// buildV2IPManualSourceSwitch builds the 0x24 payload.
func buildV2IPManualSourceSwitch(target DeviceUID, videoIP string, videoPort int, audioIP string, audioPort int, ancIP string, ancPort int, fmt *V2IPAudioFormat) []byte {
	p := make([]byte, 0, 48)
	p = append(p, target[:]...)
	p = appendStreamAddr(p, videoIP, videoPort)
	p = appendStreamAddr(p, audioIP, audioPort)
	p = appendStreamAddr(p, ancIP, ancPort)
	if fmt != nil {
		p = append(p, fmt.wire()...)
	}
	return p
}

// SelectVideoSource routes the video source of this output bay to the given
// source port. Only V2IP sinks are supported.
func (b *Bay) SelectVideoSource(port int) error {
	r := b.dev.remote
	r.mu.Lock()
	if !b.isOutput() {
		r.mu.Unlock()
		return fmt.Errorf("%s is not an output bay", b.portName)
	}
	if !b.isV2IPSink() {
		r.mu.Unlock()
		return fmt.Errorf("video routing is only supported on V2IP sinks")
	}
	src := b.dev.getByPortnumLocked(port)
	if src == nil {
		r.mu.Unlock()
		return fmt.Errorf("source port %d not found", port)
	}
	stream := b.dev.v2ipSourceForLocked(src)
	if stream == nil {
		r.mu.Unlock()
		return fmt.Errorf("v2ip addresses for source port %d not known", port)
	}
	payload := buildV2IPSourceSwitch(b.dev.uid, stream.Video.IP, "")
	uid := r.uid
	r.mu.Unlock()
	_, err := r.transmit(buildFrame(uid, opV2IPSourceSwitch, protocolFor(opV2IPSourceSwitch), payload))
	return err
}

// SelectAudioSource routes the audio source of this output bay to the given
// source port. Only V2IP sinks are supported.
func (b *Bay) SelectAudioSource(port int) error {
	r := b.dev.remote
	r.mu.Lock()
	if !b.isV2IPSink() {
		r.mu.Unlock()
		return fmt.Errorf("audio routing is only supported on V2IP sinks")
	}
	src := b.dev.getByPortnumLocked(port)
	if src == nil {
		r.mu.Unlock()
		return fmt.Errorf("source port %d not found", port)
	}
	stream := b.dev.v2ipSourceForLocked(src)
	if stream == nil {
		r.mu.Unlock()
		return fmt.Errorf("v2ip addresses for source port %d not known", port)
	}
	payload := buildV2IPSourceSwitch(b.dev.uid, "", stream.Audio.IP)
	uid := r.uid
	r.mu.Unlock()
	_, err := r.transmit(buildFrame(uid, opV2IPSourceSwitch, protocolFor(opV2IPSourceSwitch), payload))
	return err
}

// SelectAudioSourceAddr routes the audio of this V2IP sink to a raw multicast
// address, optionally overriding the audio format. A zero port defaults to the
// standard V2IP audio port. video and ancillary streams are left unchanged.
func (b *Bay) SelectAudioSourceAddr(audioIP string, audioPort int, fmt *V2IPAudioFormat) error {
	r := b.dev.remote
	r.mu.Lock()
	if !b.isV2IPSink() {
		r.mu.Unlock()
		return errNotV2IPSink
	}
	target := b.dev.uid
	uid := r.uid
	r.mu.Unlock()
	if audioPort == 0 {
		audioPort = V2IPPortAudio
	}
	payload := buildV2IPManualSourceSwitch(target, "0.0.0.0", 0, audioIP, audioPort, "0.0.0.0", 0, fmt)
	_, err := r.transmit(buildFrame(uid, opV2IPManualSrcSwitch, protocolFor(opV2IPManualSrcSwitch), payload))
	return err
}

var errNotV2IPSink = fmt.Errorf("not a V2IP sink")

// SetName sets the user-assigned name of this bay.
func (b *Bay) SetName(name string) error {
	if len(name) > 16 {
		name = name[:16]
	}
	r := b.dev.remote
	r.mu.Lock()
	payload := make([]byte, 0, 34)
	payload = append(payload, b.dev.uid[:]...)
	payload = append(payload, byte(b.portNumber), byte(b.portNumber>>8))
	payload = appendFixedStr(payload, name, 16)
	uid := r.uid
	r.mu.Unlock()
	if _, err := r.transmit(buildFrame(uid, opChangeBayName, protocolFor(opChangeBayName), payload)); err != nil {
		return err
	}
	r.runLocked(func() { b.setUserName(name) })
	return nil
}

// SetHidden hides or shows this bay.
func (b *Bay) SetHidden(hidden bool) error {
	r := b.dev.remote
	r.mu.Lock()
	payload := make([]byte, 0, 24)
	payload = append(payload, b.dev.uid[:]...)
	payload = append(payload, byte(b.portNumber), byte(b.portNumber>>8))
	if hidden {
		payload = append(payload, 1)
	} else {
		payload = append(payload, 0)
	}
	payload = append(payload, 0, 0, 0, 0, 0)
	uid := r.uid
	r.mu.Unlock()
	if _, err := r.transmit(buildFrame(uid, opBayHide, protocolFor(opBayHide), payload)); err != nil {
		return err
	}
	status := Visible
	if hidden {
		status = Hidden
	}
	r.runLocked(func() { b.applyHidden(status) })
	return nil
}

// SelectEdidProfile sets the EDID profile of this input bay.
func (b *Bay) SelectEdidProfile(profile EdidProfile) error {
	r := b.dev.remote
	r.mu.Lock()
	payload := make([]byte, 0, 24)
	payload = append(payload, b.dev.uid[:]...)
	payload = append(payload, byte(profile), byte(profile>>8))
	payload = append(payload, 0, 0, 0, 0, 0, 0)
	uid := r.uid
	r.mu.Unlock()
	if _, err := r.transmit(buildFrame(uid, opBayEDIDProfile, protocolFor(opBayEDIDProfile), payload)); err != nil {
		return err
	}
	r.runLocked(func() { b.setEdidProfile(int(profile)) })
	return nil
}

// TxAction sends a remote-control action targeting this bay.
func (b *Bay) TxAction(action RCAction) error {
	r := b.dev.remote
	r.mu.Lock()
	payload := make([]byte, 0, 20)
	payload = append(payload, b.dev.uid[:]...)
	payload = append(payload, byte(b.portNumber), byte(b.portNumber>>8), byte(action), byte(uint16(action)>>8))
	uid := r.uid
	r.mu.Unlock()
	_, err := r.transmit(buildFrame(uid, opRCTxAction, protocolFor(opRCTxAction), payload))
	return err
}

// VolumeSet sets the volume (0-100) and optional mute state of this bay.
func (b *Bay) VolumeSet(volume int, muted *bool) error {
	r := b.dev.remote
	r.mu.Lock()
	if !b.hasVolumeControl() {
		r.mu.Unlock()
		return fmt.Errorf("volume control not supported by %s", b.portName)
	}
	vol := VolumeMuteStatus{VolumeLeft: volume, VolumeRight: volume}
	if muted != nil {
		m := *muted
		vol.MutedLeft = &m
		vol.MutedRight = &m
	}
	payload := make([]byte, 0, 24)
	payload = append(payload, b.dev.uid[:]...)
	payload = append(payload, byte(b.portNumber), byte(b.portNumber>>8))
	payload = append(payload, vol.wire()...)
	payload = append(payload, 0, 0, 0)
	uid := r.uid
	r.mu.Unlock()
	if _, err := r.transmit(buildFrame(uid, opAudioSetVolume, protocolFor(opAudioSetVolume), payload)); err != nil {
		return err
	}
	r.runLocked(func() { b.setVolumeStatus(vol) })
	return nil
}

// PowerOn sends a power-on action to the device connected to this bay.
func (b *Bay) PowerOn() error {
	if err := b.TxAction(ActionPowerOn); err != nil {
		return err
	}
	b.dev.remote.runLocked(func() { b.setPowerStatus(PowerOn) })
	return nil
}

// PowerOff sends a power-off action to the device connected to this bay.
func (b *Bay) PowerOff() error {
	if err := b.TxAction(ActionPowerOff); err != nil {
		return err
	}
	b.dev.remote.runLocked(func() { b.setPowerStatus(PowerOff) })
	return nil
}

// VolumeUp raises the volume by one step.
func (b *Bay) VolumeUp() error {
	v, ok := b.Volume()
	if !ok {
		return fmt.Errorf("volume unknown")
	}
	return b.VolumeSet(v+1, nil)
}

// VolumeDown lowers the volume by one step.
func (b *Bay) VolumeDown() error {
	v, ok := b.Volume()
	if !ok {
		return fmt.Errorf("volume unknown")
	}
	return b.VolumeSet(v-1, nil)
}

// MuteSet sets the mute state, keeping the current volume.
func (b *Bay) MuteSet(mute bool) error {
	v, ok := b.Volume()
	if !ok {
		return fmt.Errorf("volume unknown")
	}
	return b.VolumeSet(v, &mute)
}

// SelectVideoSourceByUserName routes video from the input bay with the given
// user-assigned name. Only V2IP sinks are supported.
func (b *Bay) SelectVideoSourceByUserName(name string) error {
	r := b.dev.remote
	r.mu.Lock()
	src := b.dev.bayByUserNameLocked(name)
	r.mu.Unlock()
	if src == nil {
		return fmt.Errorf("source %q not found", name)
	}
	return b.SelectVideoSource(src.portNumber)
}

// SelectAudioSourceByName routes audio from the input bay with the given
// user-assigned name. When fmt is non-nil the manual switch frame is used so the
// receiver's sample rate / channel count can be overridden. V2IP sinks only.
func (b *Bay) SelectAudioSourceByName(name string, format *V2IPAudioFormat) error {
	r := b.dev.remote
	r.mu.Lock()
	if !b.isV2IPSink() {
		r.mu.Unlock()
		return errNotV2IPSink
	}
	src := b.dev.bayByUserNameLocked(name)
	if src == nil {
		r.mu.Unlock()
		return fmt.Errorf("source %q not found", name)
	}
	stream := b.dev.v2ipSourceForLocked(src)
	if stream == nil {
		r.mu.Unlock()
		return fmt.Errorf("v2ip addresses for source %q not known", name)
	}
	target, uid := b.dev.uid, r.uid
	audioIP, audioPort := stream.Audio.IP, stream.Audio.Port
	r.mu.Unlock()

	if format != nil {
		payload := buildV2IPManualSourceSwitch(target, "0.0.0.0", 0, audioIP, audioPort, "0.0.0.0", 0, format)
		_, err := r.transmit(buildFrame(uid, opV2IPManualSrcSwitch, protocolFor(opV2IPManualSrcSwitch), payload))
		return err
	}
	payload := buildV2IPSourceSwitch(target, "", audioIP)
	_, err := r.transmit(buildFrame(uid, opV2IPSourceSwitch, protocolFor(opV2IPSourceSwitch), payload))
	return err
}

func audioCmdHeader(opcode uint16, target DeviceUID) []byte {
	h := make([]byte, 0, 20)
	h = append(h, byte(opcode), byte(opcode>>8), 0, 0)
	h = append(h, target[:]...)
	return h
}

func audioParam(endpointID, param int) []byte {
	return []byte{
		byte(endpointID), byte(endpointID >> 8), 0, 0,
		byte(param), byte(param >> 8), byte(param >> 16), byte(param >> 24),
	}
}

func (d *Device) sendAudio(payload []byte) error {
	r := d.remote
	r.mu.Lock()
	uid := r.uid
	r.mu.Unlock()
	_, err := r.transmit(buildFrame(uid, opV2IPAudio, protocolFor(opV2IPAudio), payload))
	return err
}

// AudioMute sets the mute state of an audio endpoint on this device.
func (d *Device) AudioMute(endpointID int, mute bool) error {
	v := 0
	if mute {
		v = 1
	}
	return d.sendAudio(append(audioCmdHeader(audioOpMute, d.uid), audioParam(endpointID, v)...))
}

// AudioTrigger sets the trigger state of an audio endpoint on this device.
func (d *Device) AudioTrigger(endpointID int, trigger bool) error {
	v := 0
	if trigger {
		v = 1
	}
	return d.sendAudio(append(audioCmdHeader(audioOpTrigger, d.uid), audioParam(endpointID, v)...))
}

// AudioVolumeSet sets the volume of an audio endpoint on this device.
func (d *Device) AudioVolumeSet(endpointID, volume int) error {
	return d.sendAudio(append(audioCmdHeader(audioOpVolume, d.uid), audioParam(endpointID, volume)...))
}

// AudioSelectInput routes a source endpoint (on the given source device) to a
// sink endpoint on this device.
func (d *Device) AudioSelectInput(sinkEP *AudioEndpoint, source DeviceUID, sourceEP *AudioEndpoint) error {
	if sinkEP == nil || sourceEP == nil {
		return fmt.Errorf("nil endpoint")
	}
	p := audioCmdHeader(audioOpSelectInput, d.uid)
	p = append(p, d.uid[:]...)
	p = append(p, source[:]...)
	p = append(p, byte(sinkEP.ID), byte(sinkEP.ID>>8), byte(sourceEP.ID), byte(sourceEP.ID>>8))
	return d.sendAudio(p)
}

// SetZoneSettings applies amplifier zone settings to this bay.
func (b *Bay) SetZoneSettings(s AmpZoneSettings) error {
	r := b.dev.remote
	r.mu.Lock()
	payload := buildAmpZoneSettings(b.dev.uid, b.portNumber, s)
	uid := r.uid
	r.mu.Unlock()
	if _, err := r.transmit(buildFrame(uid, opAmpZoneSettings, protocolFor(opAmpZoneSettings), payload)); err != nil {
		return err
	}
	r.runLocked(func() { b.setAmpSettings(s) })
	return nil
}

// ReadStats enables or disables V2IP statistics reporting on the device.
func (d *Device) ReadStats(enable bool) error {
	r := d.remote
	r.mu.Lock()
	payload := append([]byte(nil), d.uid[:]...)
	if enable {
		payload = append(payload, 1)
	} else {
		payload = append(payload, 0)
	}
	uid := r.uid
	r.mu.Unlock()
	_, err := r.transmit(buildFrame(uid, opV2IPStats, protocolFor(opV2IPStats), payload))
	return err
}

// SendMonitoringPulse broadcasts a monitoring pulse (opcode 0x2B), asking peers
// on the network to report their monitoring data immediately.
func (r *Remote) SendMonitoringPulse() error {
	r.mu.Lock()
	uid := r.uid
	r.mu.Unlock()
	_, err := r.transmit(buildFrame(uid, opSysMonitoringPulse, protocolFor(opSysMonitoringPulse), nil))
	return err
}

// Reboot reboots the device.
func (d *Device) Reboot() error {
	r := d.remote
	r.mu.Lock()
	payload := append([]byte(nil), d.uid[:]...)
	uid := r.uid
	d.rebooting = true
	r.mu.Unlock()
	_, err := r.transmit(buildFrame(uid, opSysReboot, protocolFor(opSysReboot), payload))
	return err
}
