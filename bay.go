// Author: Lars Op den Kamp (lars@opdenkamp-it.nl)
// Copyright (c) 2026 Op den Kamp IT Solutions

package mxremote

import "fmt"

// ARC (audio return channel) status strings.
const (
	ArcNone    = "Inactive"
	ArcHDMI    = "HDMI"
	ArcOptical = "optical"
	ArcAnalog  = "analog"
)

// Bay is a single input or output port on a Device.
//
// All exported methods are safe for concurrent use. Values returned are
// snapshots taken under the owning Remote's lock.
type Bay struct {
	dev *Device

	portNumber int
	portName   string
	userName   *string
	features   BayFeaturesMask
	mbayID     *int
	statusMask BayStatusMask

	videoSource *Bay
	audioSource *Bay

	powerStatus     *PowerStatus
	faulty          *bool
	hidden          *bool
	poePowered      *bool
	hdbtConnected   *bool
	signalDetected  *bool
	hpdDetected     *bool
	cecDetected     *bool
	decoderDisabled *bool
	encoderDisabled *bool
	signalType      *string
	arc             string
	audioVolume     *VolumeMuteStatus
	rcType          *int
	edidProfile     *int
	v2ipUID         DeviceUID
	mirror          BayMirrorStatus
	audioEndpoint   *AudioEndpoint
	ampSettings     *AmpZoneSettings

	callbacks []func(*Bay)
}

func newBay(dev *Device, cfg bayConfig) *Bay {
	return &Bay{
		dev:        dev,
		portNumber: cfg.port,
		portName:   cfg.bayName,
		features:   cfg.features,
		statusMask: cfg.status,
		arc:        ArcNone,
	}
}

// RegisterCallback registers fn to be called whenever this bay changes.
func (b *Bay) RegisterCallback(fn func(*Bay)) {
	b.dev.remote.mu.Lock()
	defer b.dev.remote.mu.Unlock()
	b.callbacks = append(b.callbacks, fn)
}

func (b *Bay) emitSelf() {
	r := b.dev.remote
	for _, fn := range b.callbacks {
		fn := fn
		r.emit(func() { fn(b) })
	}
}

// notify fires a specific callback (if non-nil), the generic OnBayUpdate
// callback, and any per-bay registered callbacks. This mirrors the reference
// library, where every specific callback fans into on_bay_update.
func (b *Bay) notify(specific func()) {
	r := b.dev.remote
	if specific != nil {
		r.emit(specific)
	}
	if r.callbacks.OnBayUpdate != nil {
		r.emit(func() { r.callbacks.OnBayUpdate(b) })
	}
	b.emitSelf()
}

// ---- identity / static properties ----

// Device returns the owning device.
func (b *Bay) Device() *Device { return b.dev }

// Port returns the port number used for routing operations.
func (b *Bay) Port() int { return b.portNumber }

// BayName returns the device-assigned port name.
func (b *Bay) BayName() string { return b.portName }

// Features returns the bay feature bitmask.
func (b *Bay) Features() BayFeaturesMask {
	r := b.dev.remote
	r.mu.Lock()
	defer r.mu.Unlock()
	return b.features
}

func (b *Bay) isInput() bool {
	return b.features.Has(BayHDMIIn) || b.features.Has(BayAudioDigIn) ||
		b.features.Has(BayAudioAnaIn) || b.isV2IPSource()
}

func (b *Bay) isOutput() bool {
	return b.features.Has(BayHDMIOut) || b.features.Has(BayAudioAmpOut) ||
		b.features.Has(BayAudioDigOut) || b.features.Has(BayAudioAnaOut) || b.isV2IPSink()
}

func (b *Bay) isHDMI() bool { return b.features.Has(BayHDMIIn) || b.features.Has(BayHDMIOut) }

func (b *Bay) isAudio() bool {
	if b.isHDMI() {
		return false
	}
	return b.features.Has(BayAudioAmpOut) || b.features.Has(BayAudioAnaIn) ||
		b.features.Has(BayAudioAnaOut) || b.features.Has(BayAudioDigIn) || b.features.Has(BayAudioDigOut)
}

func (b *Bay) isV2IPSource() bool {
	return b.features.Has(BayV2IPSourceLocal) || b.features.Has(BayV2IPSourceRemote)
}

func (b *Bay) isV2IPSink() bool {
	return b.features.Has(BayV2IPSinkLocal) || b.features.Has(BayV2IPSinkRemote)
}

func (b *Bay) isV2IPRemote() bool {
	return b.features.Has(BayV2IPSinkRemote) || b.features.Has(BayV2IPSourceRemote)
}

func (b *Bay) isLocal() bool { return !b.isV2IPRemote() }

func (b *Bay) hasVolumeControl() bool {
	return b.features.Has(BayAudioAnaOut) || b.features.Has(BayAudioAmpOut) ||
		b.features.Has(BayAudioAnaIn) || b.features.Has(BayAudioDigIn)
}

func (b *Bay) modeStr() string {
	if b.isOutput() {
		return "Output"
	}
	if b.isInput() {
		return "Input"
	}
	return "unknown"
}

// bayNum returns the bay number used by the device's API/topology.
func (b *Bay) bayNum() int {
	if b.mbayID != nil {
		return *b.mbayID
	}
	mode := b.modeStr()
	if len(b.portName) <= len(mode)+1 {
		return 0
	}
	var n int
	fmt.Sscanf(b.portName[len(mode)+1:], "%d", &n)
	return n
}

// IsInput reports whether this is a source bay.
func (b *Bay) IsInput() bool { return b.lockedBool(b.isInput) }

// IsOutput reports whether this is a sink bay.
func (b *Bay) IsOutput() bool { return b.lockedBool(b.isOutput) }

// IsHDMI reports whether this is an HDMI bay.
func (b *Bay) IsHDMI() bool { return b.lockedBool(b.isHDMI) }

// IsAudio reports whether this is an audio-only bay.
func (b *Bay) IsAudio() bool { return b.lockedBool(b.isAudio) }

// IsV2IPSource reports whether this bay is a V2IP source.
func (b *Bay) IsV2IPSource() bool { return b.lockedBool(b.isV2IPSource) }

// IsV2IPSink reports whether this bay is a V2IP sink.
func (b *Bay) IsV2IPSink() bool { return b.lockedBool(b.isV2IPSink) }

func (b *Bay) lockedBool(fn func() bool) bool {
	r := b.dev.remote
	r.mu.Lock()
	defer r.mu.Unlock()
	return fn()
}

// Mode returns "Input", "Output" or "unknown".
func (b *Bay) Mode() string {
	r := b.dev.remote
	r.mu.Lock()
	defer r.mu.Unlock()
	return b.modeStr()
}

// UserName returns the user-assigned name, falling back to the bay name.
func (b *Bay) UserName() string {
	r := b.dev.remote
	r.mu.Lock()
	defer r.mu.Unlock()
	return b.userNameLocked()
}

func (b *Bay) userNameLocked() string {
	if b.userName != nil {
		return *b.userName
	}
	return b.portName
}

// BayLabel returns "name (user name)" for logging.
func (b *Bay) BayLabel() string {
	r := b.dev.remote
	r.mu.Lock()
	defer r.mu.Unlock()
	un := b.userNameLocked()
	if un != b.portName {
		return fmt.Sprintf("%s (%s)", b.portName, un)
	}
	return b.portName
}

// SignalDetected reports whether a signal is present.
func (b *Bay) SignalDetected() bool { return b.lockedBoolPtr(&b.signalDetected) }

// Hidden reports whether the bay is hidden.
func (b *Bay) Hidden() bool { return b.lockedBoolPtr(&b.hidden) }

// Faulty reports whether the bay is faulty.
func (b *Bay) Faulty() bool { return b.lockedBoolPtr(&b.faulty) }

func (b *Bay) lockedBoolPtr(p **bool) bool {
	r := b.dev.remote
	r.mu.Lock()
	defer r.mu.Unlock()
	return *p != nil && **p
}

// SignalType returns the current signal type description.
func (b *Bay) SignalType() string {
	r := b.dev.remote
	r.mu.Lock()
	defer r.mu.Unlock()
	if b.signalType != nil {
		return *b.signalType
	}
	return "unknown"
}

// PowerStatusValue returns the computed CEC power status of the connected device.
func (b *Bay) PowerStatusValue() PowerStatus {
	r := b.dev.remote
	r.mu.Lock()
	defer r.mu.Unlock()
	if b.powerStatus != nil {
		return *b.powerStatus
	}
	return PowerUnknown
}

// VolumeStatus returns the current volume/mute state, or nil if unknown.
func (b *Bay) VolumeStatus() *VolumeMuteStatus {
	r := b.dev.remote
	r.mu.Lock()
	defer r.mu.Unlock()
	if b.audioVolume == nil {
		return nil
	}
	v := *b.audioVolume
	return &v
}

// VideoSource returns the currently routed video source bay, or nil.
func (b *Bay) VideoSource() *Bay {
	r := b.dev.remote
	r.mu.Lock()
	defer r.mu.Unlock()
	if !b.isOutput() {
		return nil
	}
	return b.videoSource
}

// AudioSource returns the currently routed audio source bay, or nil.
func (b *Bay) AudioSource() *Bay {
	r := b.dev.remote
	r.mu.Lock()
	defer r.mu.Unlock()
	if !b.isOutput() {
		return nil
	}
	if b.audioSource == nil {
		return b.videoSource
	}
	return b.audioSource
}

// V2IPSource returns the V2IP stream addresses advertised by this source bay.
func (b *Bay) V2IPSource() *V2IPStreamSources {
	r := b.dev.remote
	r.mu.Lock()
	defer r.mu.Unlock()
	return b.dev.v2ipSourceForLocked(b)
}

// EdidProfileValue returns the EDID profile of an HDMI input bay.
func (b *Bay) EdidProfileValue() (EdidProfile, bool) {
	r := b.dev.remote
	r.mu.Lock()
	defer r.mu.Unlock()
	if !b.isHDMI() || !b.isInput() || b.edidProfile == nil {
		return 0, false
	}
	return EdidProfile(*b.edidProfile), true
}

// RCTypeValue returns the remote-control type of an HDMI input bay.
func (b *Bay) RCTypeValue() (RCType, bool) {
	r := b.dev.remote
	r.mu.Lock()
	defer r.mu.Unlock()
	if !b.isHDMI() || !b.isInput() || b.rcType == nil {
		return 0, false
	}
	return RCType(*b.rcType), true
}

// ---- internal mutators (called with remote lock held) ----

// pick returns fn when cond is true, otherwise nil. Used to pass an optional
// specific callback to notify.
func pick(cond bool, fn func()) func() {
	if cond {
		return fn
	}
	return nil
}

func (b *Bay) setUserName(v string) {
	prev := b.userNameLocked()
	b.userName = &v
	if b.userNameLocked() != prev {
		r := b.dev.remote
		nv := b.userNameLocked()
		b.notify(pick(r.callbacks.OnNameChanged != nil, func() { r.callbacks.OnNameChanged(b, nv) }))
	}
}

func (b *Bay) setVideoSource(src *Bay) {
	if !b.isOutput() {
		return
	}
	if src == nil {
		b.videoSource = nil
		return
	}
	if b.videoSource == nil || src != b.videoSource {
		b.videoSource = src
		r := b.dev.remote
		b.notify(pick(r.callbacks.OnVideoSourceChanged != nil, func() { r.callbacks.OnVideoSourceChanged(b, src) }))
	}
}

func (b *Bay) setAudioSource(src *Bay) {
	if !b.isOutput() {
		return
	}
	if src == nil {
		b.audioSource = nil
		return
	}
	prev := b.audioSourceLocked()
	if b.audioSource == nil || src != b.audioSource {
		b.audioSource = src
	}
	if prev != b.audioSourceLocked() {
		r := b.dev.remote
		cur := b.audioSourceLocked()
		b.notify(pick(r.callbacks.OnAudioSourceChanged != nil, func() { r.callbacks.OnAudioSourceChanged(b, cur) }))
	}
}

func (b *Bay) audioSourceLocked() *Bay {
	if b.audioSource == nil {
		return b.videoSource
	}
	return b.audioSource
}

func (b *Bay) setBoolStatus(p **bool, val bool, cb func(*Bay, bool), riseFall bool) {
	prev := *p != nil && **p
	*p = &val
	if prev != val && (!riseFall || prev || val) {
		b.notify(pick(cb != nil, func() { cb(b, val) }))
	}
}

func (b *Bay) setSignalType(v string) {
	prev := "unknown"
	if b.signalType != nil {
		prev = *b.signalType
	}
	b.signalType = &v
	if prev != v {
		r := b.dev.remote
		b.notify(pick(r.callbacks.OnStatusSignalTypeChanged != nil, func() { r.callbacks.OnStatusSignalTypeChanged(b, v) }))
	}
}

func (b *Bay) setArc(v string) {
	if b.arc != v {
		b.arc = v
		r := b.dev.remote
		b.notify(pick(r.callbacks.OnStatusArcChanged != nil, func() { r.callbacks.OnStatusArcChanged(b, v) }))
	}
}

func (b *Bay) setPowerStatus(p PowerStatus) {
	prev := PowerUnknown
	if b.powerStatus != nil {
		prev = *b.powerStatus
	}
	b.powerStatus = &p
	if p != prev {
		r := b.dev.remote
		b.notify(pick(r.callbacks.OnPowerChanged != nil, func() { r.callbacks.OnPowerChanged(b, p) }))
	}
}

func (b *Bay) setEdidProfile(val int) {
	if !b.isHDMI() || !b.isInput() {
		return
	}
	if b.edidProfile == nil || *b.edidProfile != val {
		b.edidProfile = &val
		r := b.dev.remote
		b.notify(pick(r.callbacks.OnEdidProfileChanged != nil, func() { r.callbacks.OnEdidProfileChanged(b, EdidProfile(val)) }))
	}
}

func (b *Bay) setRCType(val int) {
	if !b.isHDMI() || !b.isInput() {
		return
	}
	if b.rcType == nil || *b.rcType != val {
		b.rcType = &val
		r := b.dev.remote
		b.notify(pick(r.callbacks.OnRcTypeChanged != nil, func() { r.callbacks.OnRcTypeChanged(b, RCType(val)) }))
	}
}

func (b *Bay) setVolumeStatus(other VolumeMuteStatus) {
	if !b.hasVolumeControl() {
		return
	}
	changed := false
	if b.audioVolume == nil {
		v := other
		b.audioVolume = &v
		changed = true
	} else {
		changed = b.audioVolume.update(other)
	}
	if changed {
		r := b.dev.remote
		cur := *b.audioVolume
		b.notify(pick(r.callbacks.OnVolumeChanged != nil, func() { r.callbacks.OnVolumeChanged(b, &cur) }))
	}
}

func (b *Bay) setMirroring(m BayMirrorStatus) {
	if !b.mirror.equal(m) {
		b.mirror = m
		r := b.dev.remote
		b.notify(pick(r.callbacks.OnMirrorStatusChanged != nil, func() { r.callbacks.OnMirrorStatusChanged(b, m) }))
	}
}

func (b *Bay) applyBayStatus(data BayStatusMask) {
	b.setBoolStatus(&b.faulty, data.Has(BayStatusFault), b.dev.remote.callbacks.OnStatusFaultyChanged, true)
	b.setBoolStatus(&b.hidden, data.Has(BayStatusHidden), b.dev.remote.callbacks.OnStatusHiddenChanged, true)
	b.setBoolStatus(&b.poePowered, data.Has(BayStatusPowered), b.dev.remote.callbacks.OnStatusPoePoweredChanged, false)
	b.setBoolStatus(&b.hdbtConnected, data.Has(BayStatusHDBTConnected), b.dev.remote.callbacks.OnStatusHdbtConnectedChanged, false)
	b.setBoolStatus(&b.hpdDetected, data.Has(BayStatusHPDDetected), b.dev.remote.callbacks.OnStatusHpdDetectedChanged, false)
	b.setBoolStatus(&b.cecDetected, data.Has(BayStatusCECDetected), b.dev.remote.callbacks.OnStatusCecDetectedChanged, false)
	b.setBoolStatus(&b.signalDetected, data.Has(BayStatusSignalDetected), b.dev.remote.callbacks.OnStatusSignalDetectedChanged, false)
	b.setBoolStatus(&b.encoderDisabled, data.Has(BayStatusEncoderDisable), b.onBayUpdateBool(), false)
	b.setBoolStatus(&b.decoderDisabled, data.Has(BayStatusDecoderDisable), b.onBayUpdateBool(), false)

	switch {
	case !data.Has(BayStatusCECDetected):
		b.setPowerStatus(PowerUnknown)
	case data.Has(BayStatusPoweredOn):
		b.setPowerStatus(PowerOn)
	case data.Has(BayStatusPoweredOff):
		b.setPowerStatus(PowerOff)
	default:
		b.setPowerStatus(PowerUnknown)
	}

	switch {
	case data.Has(BayStatusAudioARCHDMI):
		b.setArc(ArcHDMI)
	case data.Has(BayStatusAudioARCOptic):
		b.setArc(ArcOptical)
	case data.Has(BayStatusAudioARCAnalog):
		b.setArc(ArcAnalog)
	default:
		b.setArc(ArcNone)
	}
}

func (b *Bay) onBayUpdateBool() func(*Bay, bool) {
	r := b.dev.remote
	if r.callbacks.OnBayUpdate == nil {
		return nil
	}
	return func(bb *Bay, _ bool) { r.callbacks.OnBayUpdate(bb) }
}

func (b *Bay) applyBayConfig(cfg bayConfig) {
	b.features = cfg.features
	b.statusMask = cfg.status
	b.setUserName(cfg.userName)
	if b.mbayID == nil {
		v := cfg.bay
		b.mbayID = &v
	}
	b.applyBayStatus(cfg.status)
	if !cfg.status.Has(BayStatusSignalDetected) || !b.dev.isV2IP() {
		b.setSignalType(cfg.signalType)
	}
	if b.isOutput() {
		b.setVideoSource(b.dev.getByPortnumLocked(cfg.videoSource))
		b.setAudioSource(b.dev.getByPortnumLocked(cfg.audioSource))
	} else {
		b.setRCType(cfg.rcType)
		b.setEdidProfile(cfg.edidProfile)
	}
}

// BayNumber returns the bay number used by the device API/topology.
func (b *Bay) BayNumber() int {
	r := b.dev.remote
	r.mu.Lock()
	defer r.mu.Unlock()
	return b.bayNum()
}

// IsHDBaseT reports whether this is an HDBaseT output bay.
func (b *Bay) IsHDBaseT() bool { return b.lockedBool(b.isHdbaset) }

// IsV2IPRemote reports whether this bay is a remote (mesh-forwarded) V2IP bay.
func (b *Bay) IsV2IPRemote() bool { return b.lockedBool(b.isV2IPRemote) }

// IsLocal reports whether this bay is local (not a remote V2IP bay).
func (b *Bay) IsLocal() bool { return b.lockedBool(b.isLocal) }

// HPDDetected reports whether hotplug is detected.
func (b *Bay) HPDDetected() bool { return b.lockedBoolPtr(&b.hpdDetected) }

// CECDetected reports whether a CEC device is detected.
func (b *Bay) CECDetected() bool { return b.lockedBoolPtr(&b.cecDetected) }

// EncoderDisabled reports whether the V2IP encoder is disabled.
func (b *Bay) EncoderDisabled() bool { return b.lockedBoolPtr(&b.encoderDisabled) }

// DecoderDisabled reports whether the V2IP decoder is disabled.
func (b *Bay) DecoderDisabled() bool { return b.lockedBoolPtr(&b.decoderDisabled) }

// HDBTConnected reports whether the HDBaseT link is up.
func (b *Bay) HDBTConnected() bool { return b.lockedBoolPtr(&b.hdbtConnected) }

// PoEPowered reports whether PoE is supplying power on this bay.
func (b *Bay) PoEPowered() bool { return b.lockedBoolPtr(&b.poePowered) }

// ARC returns the audio-return-channel status string.
func (b *Bay) ARC() string {
	r := b.dev.remote
	r.mu.Lock()
	defer r.mu.Unlock()
	return b.arc
}

// Volume returns the current volume percentage. ok is false when unknown.
func (b *Bay) Volume() (int, bool) {
	r := b.dev.remote
	r.mu.Lock()
	defer r.mu.Unlock()
	vs := b.volumeStatusLocked()
	if vs == nil {
		return 0, false
	}
	return vs.Volume(), true
}

// Muted returns the mute state. ok is false when unknown.
func (b *Bay) Muted() (bool, bool) {
	r := b.dev.remote
	r.mu.Lock()
	defer r.mu.Unlock()
	vs := b.volumeStatusLocked()
	if vs == nil {
		return false, false
	}
	return vs.Muted()
}

// DolbyInput returns the input bay name providing the Dolby audio source, or "".
func (b *Bay) DolbyInput() string {
	r := b.dev.remote
	r.mu.Lock()
	defer r.mu.Unlock()
	return b.dolbyInputLocked()
}

// VideoRouteEndpoint returns the raw "ip:port" of an active V2IP sink video
// subscription that maps to no known source bay, or "".
func (b *Bay) VideoRouteEndpoint() string {
	r := b.dev.remote
	r.mu.Lock()
	defer r.mu.Unlock()
	return b.sinkRouteEndpoint(false)
}

// AudioRouteEndpoint returns the raw "ip:port" of an active V2IP sink audio
// subscription that maps to no known source bay, or "".
func (b *Bay) AudioRouteEndpoint() string {
	r := b.dev.remote
	r.mu.Lock()
	defer r.mu.Unlock()
	return b.sinkRouteEndpoint(true)
}

// Mirroring returns the bay's mirror status.
func (b *Bay) Mirroring() BayMirrorStatus {
	r := b.dev.remote
	r.mu.Lock()
	defer r.mu.Unlock()
	return b.mirror
}

// BayUID returns the cross-device unique identifier of this bay.
func (b *Bay) BayUID() BayUID {
	r := b.dev.remote
	r.mu.Lock()
	defer r.mu.Unlock()
	return b.bayUIDLocked()
}

// AudioEndpoint returns the audio endpoint associated with this bay, or nil.
func (b *Bay) AudioEndpoint() *AudioEndpoint {
	r := b.dev.remote
	r.mu.Lock()
	defer r.mu.Unlock()
	return b.audioEndpoint
}

func (b *Bay) setAudioEndpoint(ep *AudioEndpoint) {
	if ep == nil {
		return
	}
	if b.audioEndpoint != ep {
		b.audioEndpoint = ep
		ep.setBay(b)
		b.emitSelf()
	}
}

// V2IPUID returns the source device UID this V2IP bay maps to (from bay
// mappings), or the zero UID.
func (b *Bay) V2IPUID() DeviceUID {
	r := b.dev.remote
	r.mu.Lock()
	defer r.mu.Unlock()
	return b.v2ipUID
}

// Link returns the virtual link configured on this bay, or nil.
func (b *Bay) Link() *BayLink {
	r := b.dev.remote
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.links.getLocked(b)
}

// LinkConfigured reports whether this bay is linked to another bay.
func (b *Bay) LinkConfigured() bool {
	r := b.dev.remote
	r.mu.Lock()
	defer r.mu.Unlock()
	l := r.links.getLocked(b)
	return l != nil && l.linkedLocked()
}

// LinkedBay returns the bay linked to this one, or nil.
func (b *Bay) LinkedBay() *Bay {
	r := b.dev.remote
	r.mu.Lock()
	defer r.mu.Unlock()
	return b.linkedBayLocked()
}

func (b *Bay) isHdbaset() bool {
	return b.isHDMI() && b.isOutput() && b.bayNum() < b.dev.nbHdbtLocked()
}

func (b *Bay) dolbyInputLocked() string {
	if b.features.Has(BayDolby) {
		return "Input 9"
	}
	return ""
}

func (b *Bay) volumeStatusLocked() *VolumeMuteStatus {
	if !b.hasVolumeControl() {
		if lb := b.linkedBayLocked(); lb != nil && lb.hasVolumeControl() {
			return lb.volumeStatusLocked()
		}
		return nil
	}
	return b.audioVolume
}

func (b *Bay) sinkRouteEndpoint(audio bool) string {
	if !b.isOutput() || !b.isV2IPSink() {
		return ""
	}
	sink := b.dev.v2ipSink
	if sink == nil {
		return ""
	}
	stream := sink.Addresses.Video
	if audio {
		stream = sink.Addresses.Audio
	}
	if stream.Port == 0 {
		return ""
	}
	if b.dev.remote.getByStreamIPLocked(stream.IP, audio) != nil {
		return ""
	}
	return fmt.Sprintf("%s:%d", stream.IP, stream.Port)
}

func (b *Bay) bayUIDLocked() BayUID {
	if b.isV2IPSource() {
		var src DeviceUID
		if stream := b.dev.v2ipSourceForLocked(b); stream != nil && !stream.UID.Empty() {
			src = stream.UID
		} else if !b.v2ipUID.Empty() {
			src = b.v2ipUID
		}
		if !src.Empty() {
			return BayUID{Device: src, Port: 0}
		}
	}
	return BayUID{Device: b.dev.uid, Port: b.portNumber}
}

func (b *Bay) linkedBayLocked() *Bay {
	l := b.dev.remote.links.getLocked(b)
	if l == nil || !l.linkedLocked() {
		return nil
	}
	return l.linkedBayLocked()
}

func (b *Bay) String() string {
	return fmt.Sprintf("%s %s", b.dev.serialLocked(), b.portName)
}

// update merges other into v, returning whether anything changed.
func (v *VolumeMuteStatus) update(other VolumeMuteStatus) bool {
	changed := false
	if other.VolumeLeft >= 0 {
		changed = changed || v.VolumeLeft != other.VolumeLeft
		v.VolumeLeft = other.VolumeLeft
	}
	if other.VolumeRight >= 0 {
		changed = changed || v.VolumeRight != other.VolumeRight
		v.VolumeRight = other.VolumeRight
	}
	if other.MutedLeft != nil {
		changed = changed || v.MutedLeft == nil || *v.MutedLeft != *other.MutedLeft
		v.MutedLeft = other.MutedLeft
	}
	if other.MutedRight != nil {
		changed = changed || v.MutedRight == nil || *v.MutedRight != *other.MutedRight
		v.MutedRight = other.MutedRight
	}
	return changed
}
