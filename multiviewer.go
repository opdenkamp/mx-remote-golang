// Author: Lars Op den Kamp (lars@opdenkamp-it.nl)
// Copyright (c) 2026 Op den Kamp IT Solutions

package mxremote

import "fmt"

// Multiviewer view layout modes.
type MultiviewerViewMode int

const (
	MVViewUnknown          MultiviewerViewMode = 0
	MVViewSingle           MultiviewerViewMode = 1
	MVViewPIP              MultiviewerViewMode = 2
	MVViewTwoScreenLarge   MultiviewerViewMode = 3
	MVViewTwoScreenSmall   MultiviewerViewMode = 4
	MVViewThreeScreenLarge MultiviewerViewMode = 5
	MVViewThreeScreenSmall MultiviewerViewMode = 6
	MVViewFourScreenLarge  MultiviewerViewMode = 7
	MVViewFourScreenSmall  MultiviewerViewMode = 8
)

// Multiviewer picture-in-picture position.
type MultiviewerPipPosition int

const (
	MVPipPosUnknown     MultiviewerPipPosition = 0
	MVPipPosLeftTop     MultiviewerPipPosition = 1
	MVPipPosLeftBottom  MultiviewerPipPosition = 2
	MVPipPosRightTop    MultiviewerPipPosition = 3
	MVPipPosRightBottom MultiviewerPipPosition = 4
)

// Multiviewer picture-in-picture size.
type MultiviewerPipSize int

const (
	MVPipSizeUnknown MultiviewerPipSize = 0
	MVPipSizeSmall   MultiviewerPipSize = 1
	MVPipSizeMedium  MultiviewerPipSize = 2
	MVPipSizeLarge   MultiviewerPipSize = 3
)

// Multiviewer output resolution/refresh mode.
type MultiviewerOutputMode int

const (
	MVOutUnknown        MultiviewerOutputMode = 0
	MVOut4096x2160P60   MultiviewerOutputMode = 1
	MVOut4096x2160P50   MultiviewerOutputMode = 2
	MVOut3840x2160P60   MultiviewerOutputMode = 3
	MVOut3840x2160P50   MultiviewerOutputMode = 4
	MVOut3840x2160P30   MultiviewerOutputMode = 5
	MVOut3840x2160P25   MultiviewerOutputMode = 6
	MVOut1920x1200P60RB MultiviewerOutputMode = 7
	MVOut1920x1080P60   MultiviewerOutputMode = 8
	MVOut1920x1080P50   MultiviewerOutputMode = 9
	MVOut1360x768P60    MultiviewerOutputMode = 10
	MVOut1280x800P60    MultiviewerOutputMode = 11
	MVOut1280x720P60    MultiviewerOutputMode = 12
	MVOut1280x720P50    MultiviewerOutputMode = 13
	MVOut1024x768P60    MultiviewerOutputMode = 14
)

// Multiviewer HDCP mode.
type MultiviewerHDCPMode int

const (
	MVHDCPUnknown MultiviewerHDCPMode = 0
	MVHDCPV14     MultiviewerHDCPMode = 1
	MVHDCPV22     MultiviewerHDCPMode = 2
)

// Multiviewer IT-content mode.
type MultiviewerITCMode int

const (
	MVITCUnknown MultiviewerITCMode = 0
	MVITCVideo   MultiviewerITCMode = 1
	MVITCPC      MultiviewerITCMode = 2
)

// Multiviewer EDID template.
type MultiviewerEDIDTemplate int

const (
	MVEdidUnknown MultiviewerEDIDTemplate = 0
)

// Multiviewer aspect ratio.
type MultiviewerAspectRatio int

const (
	MVAspectUnknown MultiviewerAspectRatio = 0
	MVAspectFull    MultiviewerAspectRatio = 1
	MVAspect16x9    MultiviewerAspectRatio = 2
)

// MultiviewerBool is a tri-state on/off/unknown setting.
type MultiviewerBool int

const (
	MVBoolOff     MultiviewerBool = 0
	MVBoolOn      MultiviewerBool = 1
	MVBoolUnknown MultiviewerBool = 0xFF
)

// Multiviewer input source selection.
type MultiviewerSource int

const (
	MVSourceUnknown MultiviewerSource = 0
	MVSource1       MultiviewerSource = 1
	MVSource2       MultiviewerSource = 2
	MVSource3       MultiviewerSource = 3
	MVSource4       MultiviewerSource = 4
)

// multiviewer sub-command opcodes.
const (
	mvOpStatus        = 0
	mvOpViewMode      = 1
	mvOpVideoSource   = 2
	mvOpAudioSource   = 3
	mvOpAudioVolume   = 4
	mvOpEdidTemplate  = 5
	mvOpRouteRC       = 6
	mvOpPipSize       = 7
	mvOpPipPosition   = 8
	mvOpAspect        = 9
	mvOpAutoSwitch    = 10
	mvOpOutputMode    = 11
	mvOpOutputITCMode = 12
	mvOpHDCPMode      = 13
	mvOpConfigSource  = 14
	mvOpAutoRoute     = 15
)

// mvConfig is the parsed multiviewer status. It is comparable so the device can
// detect changes.
type mvConfig struct {
	uid           DeviceUID
	mappings      [4]DeviceUID
	mcuVersion    string
	scalerVersion string
	hwViewMode    uint8
	viewMode      MultiviewerViewMode
	pipPosition   MultiviewerPipPosition
	pipSize       MultiviewerPipSize
	outputMode    MultiviewerOutputMode
	hdcpMode      MultiviewerHDCPMode
	outputITC     MultiviewerITCMode
	edidTemplate  MultiviewerEDIDTemplate
	aspectRatio   MultiviewerAspectRatio
	autoSwitch    MultiviewerBool
	audioSource   MultiviewerSource
	audioVolume   int
	audioMuted    MultiviewerBool
	videoSources  [4]MultiviewerSource
	remoteControl MultiviewerSource
}

func parseMVConfig(f *frame) mvConfig {
	u8 := func(idx int) uint8 { v, _ := f.u8(idx); return v }
	uuid := func(idx int) DeviceUID { v, _ := f.uuid(idx); return v }
	str := func(idx, n int) string { v, _ := f.str(idx, n); return v }

	// A value this library has no name for is passed through as it arrived. The
	// alternative - folding it to zero - reports it as whichever mode happens
	// to be zero, so a firmware that adds a mode would make a driver read a
	// confidently wrong value rather than an unrecognised one.
	raw := func(idx int) int { return int(u8(idx)) }

	// Bound only where the field is a numeric range rather than an enum.
	clampPercent := func(idx int) int {
		v := int(u8(idx))
		if v > 100 {
			return 0
		}
		return v
	}
	c := mvConfig{
		uid:           uuid(24),
		mcuVersion:    str(40+4*16, 32),
		scalerVersion: str(40+6*16, 32),
		hwViewMode:    u8(168),
		viewMode:      MultiviewerViewMode(raw(169)),
		pipPosition:   MultiviewerPipPosition(raw(170)),
		pipSize:       MultiviewerPipSize(raw(171)),
		outputMode:    MultiviewerOutputMode(raw(172)),
		hdcpMode:      MultiviewerHDCPMode(raw(173)),
		outputITC:     MultiviewerITCMode(raw(174)),
		edidTemplate:  MultiviewerEDIDTemplate(raw(175)),
		aspectRatio:   MultiviewerAspectRatio(raw(177)),
		audioVolume:   clampPercent(180),
		remoteControl: mvSourcePlus(u8(186)),
		audioSource:   mvSourcePlus(u8(179)),
		autoSwitch:    mvBool(u8(178)),
		audioMuted:    mvBool(u8(181)),
	}
	for i := 0; i < 4; i++ {
		c.mappings[i] = uuid(40 + i*16)
		c.videoSources[i] = MultiviewerSource(raw(182 + i))
	}
	return c
}

func mvSourcePlus(v uint8) MultiviewerSource {
	if v > 3 {
		return MVSourceUnknown
	}
	return MultiviewerSource(v + 1)
}

func mvBool(v uint8) MultiviewerBool {
	if v > 1 {
		return MVBoolUnknown
	}
	return MultiviewerBool(v)
}

// Multiviewer provides read access and control for a OneIP multiviewer device.
// Obtain one from Device.Multiviewer. All methods are safe for concurrent use;
// control methods are no-ops (returning an error) on non-multiviewer devices.
type Multiviewer struct {
	dev    *Device
	config *mvConfig
}

func (d *Device) multiviewer() *Multiviewer {
	if d.mv == nil {
		d.mv = &Multiviewer{dev: d}
	}
	return d.mv
}

// Multiviewer returns the multiviewer controller for this device.
func (d *Device) Multiviewer() *Multiviewer {
	d.remote.mu.Lock()
	defer d.remote.mu.Unlock()
	return d.multiviewer()
}

func (d *Device) updateMultiviewer(c mvConfig) {
	mv := d.multiviewer()
	if mv.config == nil || *mv.config != c {
		mv.config = &c
		d.emitSelf()
	}
}

func (m *Multiviewer) read() (*mvConfig, bool) {
	r := m.dev.remote
	r.mu.Lock()
	defer r.mu.Unlock()
	if !m.dev.isOneipMultiviewer() || m.config == nil {
		return nil, false
	}
	return m.config, true
}

// ViewMode returns the current screen layout mode.
func (m *Multiviewer) ViewMode() MultiviewerViewMode {
	if c, ok := m.read(); ok {
		return c.viewMode
	}
	return MVViewUnknown
}

// VideoSource returns the source assigned to the given screen index.
func (m *Multiviewer) VideoSource(screen int) MultiviewerSource {
	if c, ok := m.read(); ok && screen >= 0 && screen < 4 {
		return c.videoSources[screen]
	}
	return MVSourceUnknown
}

// AudioSource returns the screen whose audio is selected.
func (m *Multiviewer) AudioSource() MultiviewerSource {
	if c, ok := m.read(); ok {
		return c.audioSource
	}
	return MVSourceUnknown
}

// AudioVolume returns the output volume (0-100), or -1 when unknown.
func (m *Multiviewer) AudioVolume() int {
	if c, ok := m.read(); ok {
		return c.audioVolume
	}
	return -1
}

// AudioMuted returns the output mute state.
func (m *Multiviewer) AudioMuted() MultiviewerBool {
	if c, ok := m.read(); ok {
		return c.audioMuted
	}
	return MVBoolUnknown
}

// EdidTemplate returns the active EDID template.
func (m *Multiviewer) EdidTemplate() MultiviewerEDIDTemplate {
	if c, ok := m.read(); ok {
		return c.edidTemplate
	}
	return MVEdidUnknown
}

// RemoteControl returns the screen receiving remote-control passthrough.
func (m *Multiviewer) RemoteControl() MultiviewerSource {
	if c, ok := m.read(); ok {
		return c.remoteControl
	}
	return MVSourceUnknown
}

// PipSize returns the picture-in-picture window size.
func (m *Multiviewer) PipSize() MultiviewerPipSize {
	if c, ok := m.read(); ok {
		return c.pipSize
	}
	return MVPipSizeUnknown
}

// PipPosition returns the picture-in-picture window position.
func (m *Multiviewer) PipPosition() MultiviewerPipPosition {
	if c, ok := m.read(); ok {
		return c.pipPosition
	}
	return MVPipPosUnknown
}

// ScreenAspect returns the output aspect ratio.
func (m *Multiviewer) ScreenAspect() MultiviewerAspectRatio {
	if c, ok := m.read(); ok {
		return c.aspectRatio
	}
	return MVAspectUnknown
}

// AutoSwitch returns whether auto-switching is enabled.
func (m *Multiviewer) AutoSwitch() MultiviewerBool {
	if c, ok := m.read(); ok {
		return c.autoSwitch
	}
	return MVBoolUnknown
}

// OutputMode returns the output resolution/refresh mode.
func (m *Multiviewer) OutputMode() MultiviewerOutputMode {
	if c, ok := m.read(); ok {
		return c.outputMode
	}
	return MVOutUnknown
}

// OutputITCMode returns the output IT-content mode.
func (m *Multiviewer) OutputITCMode() MultiviewerITCMode {
	if c, ok := m.read(); ok {
		return c.outputITC
	}
	return MVITCUnknown
}

// HDCPMode returns the HDCP mode.
func (m *Multiviewer) HDCPMode() MultiviewerHDCPMode {
	if c, ok := m.read(); ok {
		return c.hdcpMode
	}
	return MVHDCPUnknown
}

// ConnectedSource returns the device UID mapped to the given multiviewer input.
func (m *Multiviewer) ConnectedSource(input int) (DeviceUID, bool) {
	if c, ok := m.read(); ok && input >= 0 && input < 4 {
		return c.mappings[input], true
	}
	return DeviceUID{}, false
}

// MCUVersion returns the multiviewer MCU firmware version.
func (m *Multiviewer) MCUVersion() string {
	if c, ok := m.read(); ok {
		return c.mcuVersion
	}
	return ""
}

// ScalerVersion returns the multiviewer scaler firmware version.
func (m *Multiviewer) ScalerVersion() string {
	if c, ok := m.read(); ok {
		return c.scalerVersion
	}
	return ""
}

// ---- control ----

// mvCmdPayload assembles a multiviewer command body: target uid + opcode + 7
// pad + args.
// MultiviewerCommand is one V2IP_MULTIVIEWER sub-command as it arrived.
//
// The opcode multiplexes sixteen sub-commands on the byte at offset 16 behind a
// uniform envelope: target uid, sub-opcode, seven pad, then parameters. Only
// STATUS reports state; the other fifteen are requests, and what they change
// comes back on the following STATUS.
//
// The parameters are exposed as raw bytes on purpose. The opcode is owned by
// the multiviewer module rather than MatrixOS, so beyond the envelope there is
// no firmware source here to pin per-sub-command field semantics against.
type MultiviewerCommand struct {
	Target DeviceUID
	// Op is the sub-opcode. Values this library has no name for still arrive.
	Op byte
	// Params is everything after the envelope, empty when the frame carries none.
	Params []byte
}

func (c MultiviewerCommand) String() string {
	return fmt.Sprintf("multiviewer command %d for %s (%d param bytes)", c.Op, c.Target, len(c.Params))
}

func mvCmdPayload(target DeviceUID, op byte, args ...byte) []byte {
	payload := make([]byte, 0, 24+len(args))
	payload = append(payload, target[:]...)
	payload = append(payload, op)
	payload = append(payload, 0, 0, 0, 0, 0, 0, 0)
	payload = append(payload, args...)
	return payload
}

func (m *Multiviewer) cmd(op byte, args ...byte) error {
	r := m.dev.remote
	r.mu.Lock()
	if err := m.dev.requireOpcodeLocked(opV2IPMultiviewer); err != nil {
		r.mu.Unlock()
		return err
	}
	if !m.dev.isOneipMultiviewer() {
		r.mu.Unlock()
		return fmt.Errorf("%s is not a multiviewer", m.dev.serialLocked())
	}
	target, uid := m.dev.uid, r.uid
	r.mu.Unlock()
	// stamped above this opcode's 0x16 minimum, matching the reference library.
	// MatrixOS has no handler for it - the multiviewer module owns the opcode - so
	// the format an 0x16..0x1F receiver expects here is unverified.
	_, err := r.transmit(buildFrame(uid, opV2IPMultiviewer, 0x20, mvCmdPayload(target, op, args...)))
	return err
}

// SetViewMode sets the screen layout mode.
func (m *Multiviewer) SetViewMode(v MultiviewerViewMode) error {
	return m.cmd(mvOpViewMode, byte(v))
}

// SetVideoSource assigns a source to a screen.
func (m *Multiviewer) SetVideoSource(screen int, source MultiviewerSource) error {
	return m.cmd(mvOpVideoSource, byte(screen), byte(source))
}

// SetAudioSource selects which screen's audio to output.
func (m *Multiviewer) SetAudioSource(source MultiviewerSource) error {
	return m.cmd(mvOpAudioSource, byte(int(source)-1))
}

// SetAudioVolume sets the output volume and mute state.
func (m *Multiviewer) SetAudioVolume(volume int, muted bool) error {
	mb := byte(0)
	if muted {
		mb = 1
	}
	return m.cmd(mvOpAudioVolume, byte(volume), mb)
}

// SetEdidTemplate sets the EDID template.
func (m *Multiviewer) SetEdidTemplate(e MultiviewerEDIDTemplate) error {
	return m.cmd(mvOpEdidTemplate, byte(e))
}

// SetRemoteControl sets which screen receives remote-control passthrough.
func (m *Multiviewer) SetRemoteControl(source MultiviewerSource) error {
	return m.cmd(mvOpRouteRC, byte(int(source)-1))
}

// SetPipSize sets the PIP window size.
func (m *Multiviewer) SetPipSize(s MultiviewerPipSize) error { return m.cmd(mvOpPipSize, byte(s)) }

// SetPipPosition sets the PIP window position.
func (m *Multiviewer) SetPipPosition(p MultiviewerPipPosition) error {
	return m.cmd(mvOpPipPosition, byte(p))
}

// SetScreenAspect sets the output aspect ratio.
func (m *Multiviewer) SetScreenAspect(a MultiviewerAspectRatio) error {
	return m.cmd(mvOpAspect, byte(a))
}

// SetAutoSwitch enables or disables auto-switching.
func (m *Multiviewer) SetAutoSwitch(enable bool) error {
	b := byte(0)
	if enable {
		b = 1
	}
	return m.cmd(mvOpAutoSwitch, b)
}

// SetOutputMode sets the output resolution/refresh mode.
func (m *Multiviewer) SetOutputMode(o MultiviewerOutputMode) error {
	return m.cmd(mvOpOutputMode, byte(o))
}

// SetOutputITCMode sets the output IT-content mode.
func (m *Multiviewer) SetOutputITCMode(o MultiviewerITCMode) error {
	return m.cmd(mvOpOutputITCMode, byte(o))
}

// SetHDCPMode sets the HDCP mode.
func (m *Multiviewer) SetHDCPMode(h MultiviewerHDCPMode) error {
	return m.cmd(mvOpHDCPMode, byte(h))
}

// SetConnectedSource maps a source device UID to a multiviewer input. Pass the
// zero UID to clear the mapping.
func (m *Multiviewer) SetConnectedSource(input int, source DeviceUID) error {
	args := make([]byte, 0, 24)
	args = append(args, source[:]...)
	args = append(args, byte(input))
	args = append(args, 0, 0, 0, 0, 0, 0, 0)
	return m.cmd(mvOpConfigSource, args...)
}

// AutoRoute triggers automatic source routing.
func (m *Multiviewer) AutoRoute() error { return m.cmd(mvOpAutoRoute) }
