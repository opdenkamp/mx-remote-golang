// Author: Lars Op den Kamp (lars@opdenkamp-it.nl)
// Copyright (c) 2026 Op den Kamp IT Solutions

package mxremote

import (
	"encoding/binary"
	"net"
)

// frameHandlers maps an opcode to its receive handler. Handlers run with the
// Remote lock held and queue callbacks via Remote.emit.
var frameHandlers = map[uint16]func(*Remote, *frame){
	opSysHello:              handleHello,
	opSysLinks:              handleLinks,
	opSysBayConfig:          handleBayConfig,
	opSysBayConfigSecondary: handleBayConfig,
	opDevConnect:            handleConnectStatus,
	opDevPowerChange:        handlePowerChange,
	opMxRoute:               handleRoutingChange,
	opRCKey:                 handleRCKey,
	opRCAction:              handleRCAction,
	opAudioSetVolume:        handleVolumeSet,
	opSysTemperature:        handleTemperature,
	opFirmwareVersion:       handleFirmwareVersion,
	opSysStatus:             handleSystemStatus,
	opNetLinkStatus:         handleNetworkStatus,
	opAmpZoneSettings:       handleAmpZoneSettings,
	opAmpDolbyState:         handleAmpDolbySettings,
	opV2IPStats:             handleV2IPStats,
	opTopology:              handleTopology,
	opBaySignalStatus:       handleSignalStatusNew,
	opV2IPMultiviewer:       handleMultiviewer,
	opV2IPAudio:             handleAudio,
	opV2IPSourceSwitch:      handleV2IPSourceSwitch,
	opV2IPManualSrcSwitch:   handleV2IPManualSourceSwitch,
	opSysBayV2IPSources:     handleV2IPSources,
	opBayHide:               handleBayHide,
	opBayMirrorStatus:       handleMirrorStatus,
	opBayStatus:             handleBayStatus,
	opMeshOperation:         handleMeshOperation,
	opV2IPDeviceCfg:         handleV2IPDeviceConfiguration,
	opV2IPBayMappings:       handleV2IPBayMapping,
	opSysDiscover:           handleDiscoverRequest,
	opDevEDID:               handleEDID,
	opMxSetRoute:            handleSetRoute,
	opAudioSetRoute:         handleAudioSetRoute,
	opRCIr:                  handleIRCapture,
	opAudioVolumeUp:         handleVolumeStep(true),
	opAudioVolumeDown:       handleVolumeStep(false),
	opAudioVolumeMute:       handleVolumeMute,
	opAudioClip:             handleAudioClip,
	opPDUState:              handlePDUState,
	opV2IPLinkRemote:        handleV2IPLinkRemote,
	opV2IPDetectBays:        handleDetectBays,
	opChangeBayName:         handleChangeBayName,
	opSysReboot:             handleReboot,
	opSysMonitoringPulse:    handleMonitoringPulse,
	opV2IPUpgradeFPGA:       handleUpgradeFPGA,
	opV2IPBlistRegister:     handleBlacklist(true),
	opV2IPBlistUnregister:   handleBlacklist(false),
	opBayEDIDProfile:        handleEDIDProfile,
	opSetupStatus:           handleSetupStatus,
	opSetInstaller:          handleSetInstaller,
	opBayFilterStatus:       handleFilterStatus,
	opSysFactoryReset:       handleFactoryReset,
	opV2IPTiling:            handleV2IPTiling,
	opV2IPPowerSave:         handleV2IPPowerSave,
	opRCSettings:            handleRCSettings,
	opRCTxKey:               handleTxKey,
	opRCTxAction:            handleTxAction,
	opRCIrTx:                handleIRTransmit,
	opV2IPVideoWall:         handleVideoWall,
}

func (r *Remote) deviceFor(f *frame) *Device { return r.devices[f.remoteID()] }

func handleHello(r *Remote, f *frame) {
	proto, _ := f.u16(0)
	name, _ := f.str(2, 16)
	serial, _ := f.str(18, 16)
	version, _ := f.str(34, 16)
	h := helloInfo{
		supportedProtocol: proto,
		name:              name,
		serial:            serial,
		version:           version,
		address:           f.addr,
	}
	if feat, ok := f.u32(50); ok {
		h.features = DeviceFeature(feat)
		h.hasFeatures = true
	}
	r.onHello(h, f.remoteID())
}

// handleBayConfig merges one page of bay descriptors into the device cache.
//
// A device pages its bays across several frames: firmware sizes each page
// against mxr_max_payload_len() and shrinks it further on OOM, so the record
// count varies from frame to frame and no single frame holds the whole list.
// Merge records rather than replacing the cache, and never read the record
// count as the device's bay count.
func handleBayConfig(r *Remote, f *frame) {
	dev := r.deviceFor(f)
	if dev == nil {
		return
	}
	p := f.payload()
	for off := 0; off+bayConfigSize <= len(p); off += bayConfigSize {
		dev.applyBayConfig(parseBayConfig(p[off : off+bayConfigSize]))
	}
}

func handleConnectStatus(r *Remote, f *frame) {
	dev := r.deviceFor(f)
	if dev == nil {
		return
	}
	port, ok := f.u8(0)
	if !ok {
		return
	}
	bay := dev.getByPortnumLocked(int(port))
	if bay == nil {
		return
	}
	status := Disconnected
	if f.boolean(1) {
		status = Connected
	}
	bay.applyConnectStatus(status)
}

func handlePowerChange(r *Remote, f *frame) {
	dev := r.deviceFor(f)
	if dev == nil {
		return
	}
	port, ok := f.u8(0)
	if !ok {
		return
	}
	bay := dev.getByPortnumLocked(int(port))
	if bay == nil {
		return
	}
	if f.boolean(1) {
		bay.setPowerStatus(PowerOn)
	} else {
		bay.setPowerStatus(PowerOff)
	}
}

func handleRoutingChange(r *Remote, f *frame) {
	dev := r.deviceFor(f)
	if dev == nil {
		return
	}
	sinkPort, ok := f.u8(0)
	if !ok {
		return
	}
	sink := dev.getByPortnumLocked(int(sinkPort))
	if sink == nil {
		return
	}
	var video, audio *Bay
	if v, ok := f.u8(2); ok {
		video = dev.getByPortnumLocked(int(v))
	}
	if a, ok := f.u8(4); ok {
		audio = dev.getByPortnumLocked(int(a))
	}
	sink.applySelected(video, audio)
}

func handleRCAction(r *Remote, f *frame) {
	dev := r.deviceFor(f)
	if dev == nil {
		return
	}
	var port int
	if dev.hello.supportedProtocol >= 6 {
		v, ok := f.u16(0)
		if !ok {
			return
		}
		port = int(v)
	} else {
		v, ok := f.u8(0)
		if !ok {
			return
		}
		port = int(v)
	}
	bay := dev.getByPortnumLocked(port)
	action, ok := f.u8(2)
	if bay == nil || !ok {
		return
	}
	bay.emitAction(RCAction(action))
}

func handleVolumeSet(r *Remote, f *frame) {
	dev := r.deviceFor(f)
	if dev == nil {
		return
	}
	port, ok := f.u16(16)
	if !ok {
		return
	}
	bay := dev.getByPortnumLocked(int(port))
	if bay == nil {
		return
	}
	vol := VolumeMuteStatus{VolumeLeft: -1, VolumeRight: -1}
	if vl, ok := f.u8(18); ok && vl <= 100 {
		vol.VolumeLeft = int(vl)
	}
	if vr, ok := f.u8(19); ok && vr <= 100 {
		vol.VolumeRight = int(vr)
	}
	if m, ok := f.u8(20); ok {
		ms := MuteStatus(m)
		l, rr := ms.Left(), ms.Right()
		vol.MutedLeft = &l
		vol.MutedRight = &rr
	}
	bay.setVolumeStatus(vol)
}

func handleTemperature(r *Remote, f *frame) {
	dev := r.deviceFor(f)
	if dev == nil {
		return
	}
	nb, ok := f.u8(0)
	if !ok {
		return
	}
	temps := make([]int, 0, nb)
	for i := 1; i <= int(nb); i++ {
		if v, ok := f.u8(i); ok {
			temps = append(temps, int(v))
		}
	}
	dev.setTemperatures(temps)
}

func handleV2IPSources(r *Remote, f *frame) {
	dev := r.deviceFor(f)
	if dev == nil {
		return
	}
	p := f.payload()
	var sources []V2IPStreamSources
	for off := 0; off+40 <= len(p); off += 40 {
		rec := p[off : off+40]
		var uid DeviceUID
		copy(uid[:], rec[0:16])
		sources = append(sources, V2IPStreamSources{
			UID:   uid,
			Video: parseStreamSource("video", rec[16:22]),
			Audio: parseStreamSource("audio", rec[24:30]),
			Anc:   parseStreamSource("anc", rec[32:38]),
		})
	}
	dev.setV2IPSources(sources)
}

func handleV2IPSourceSwitch(r *Remote, f *frame) {
	target := r.devices[mustUUID(f, 0)]
	if target == nil {
		return
	}
	p := f.payload()
	if len(p) < 24 {
		return
	}
	videoIP := net.IPv4(p[16], p[17], p[18], p[19]).String()
	audioIP := net.IPv4(p[20], p[21], p[22], p[23]).String()
	sink := target.firstOutputLocked()
	if sink == nil {
		return
	}
	sink.applySelected(r.getByStreamIPLocked(videoIP, false), r.getByStreamIPLocked(audioIP, true))
}

func handleV2IPManualSourceSwitch(r *Remote, f *frame) {
	target := r.devices[mustUUID(f, 0)]
	if target == nil {
		return
	}
	p := f.payload()
	if len(p) < 38 {
		return
	}
	video := parseStreamSource("video", p[16:22])
	audio := parseStreamSource("audio", p[24:30])
	anc := parseStreamSource("anc", p[32:38])
	sink := target.firstOutputLocked()
	if sink != nil {
		sink.applySelected(r.getByStreamIPLocked(video.IP, false), r.getByStreamIPLocked(audio.IP, true))
	}
	s := DeviceV2IPSink{Addresses: V2IPStreamSources{Video: video, Audio: audio, Anc: anc}}
	if len(p) >= 48 {
		if fmt, ok := parseAudioFormat(p[40:48]); ok {
			s.AudioFmt = &fmt
		}
	}
	target.setV2IPSink(s)
}

func handleV2IPDeviceConfiguration(r *Remote, f *frame) {
	dev := r.deviceFor(f)
	if dev == nil {
		return
	}
	p := f.payload()
	if len(p) < 61 {
		return
	}
	details := DeviceV2IPDetails{
		Video: parseStreamSource("video", p[16:22]),
		Audio: parseStreamSource("audio", p[24:30]),
		Anc:   parseStreamSource("anc", p[32:38]),
		Arc:   parseStreamSource("arc", p[48:54]),
		Dscp: V2IPDscpConfig{
			Video: parseDscp(p[41]),
			Audio: parseDscp(p[42]),
			Anc:   parseDscp(p[43]),
		},
		Scaling: V2IPScalingSettings{
			Mode:    MxrSignalType(uint16(p[56]) | uint16(p[57])<<8),
			Refresh: uint16(p[58]) | uint16(p[59])<<8,
			// only bits 0, 1 and 7 are defined; firmware predating the fix
			// builds this from an uninitialised stack local, so the rest is
			// noise and must not reach the cache even on a first frame
			Flags: p[60] & (ScalingFlagModeValid | ScalingFlagOptionsValid | ScalingFlagAutoScaling),
		},
	}
	if p[40] >= V2IPSourceRateMin && p[40] <= V2IPSourceRateMax {
		rate := p[40]
		details.TxRate = &rate
	}
	dev.setV2IPDetails(details)

	// The tiling block carries no validity flag of its own; its uid is the
	// marker. Every path that produces a real window stamps it, and a
	// controller writing any other field sends the block zeroed - so uid zero
	// means "not carried" while uid set with zero geometry is a real clear.
	if len(p) >= 88 {
		var uid DeviceUID
		copy(uid[:], p[64:80])
		if !uid.Empty() {
			dev.setTiling(V2IPTilingConfig{
				Target: uid,
				PosX:   binary.LittleEndian.Uint16(p[80:82]),
				PosY:   binary.LittleEndian.Uint16(p[82:84]),
				Width:  binary.LittleEndian.Uint16(p[84:86]),
				Height: binary.LittleEndian.Uint16(p[86:88]),
			})
		}
	}

	if len(p) >= 120 {
		sink := DeviceV2IPSink{
			Addresses: V2IPStreamSources{
				Video: parseStreamSource("video", p[88:94]),
				Audio: parseStreamSource("audio", p[96:102]),
				Anc:   parseStreamSource("anc", p[104:110]),
			},
		}
		if fmt, ok := parseAudioFormat(p[112:120]); ok {
			sink.AudioFmt = &fmt
		}
		dev.setV2IPSink(sink)
	}
}

func handleBayHide(r *Remote, f *frame) {
	target := r.devices[mustUUID(f, 0)]
	if target == nil {
		return
	}
	port, ok := f.u16(16)
	if !ok {
		return
	}
	bay := target.getByPortnumLocked(int(port))
	if bay == nil {
		return
	}
	if f.boolean(18) {
		bay.applyHidden(Hidden)
	} else {
		bay.applyHidden(Visible)
	}
}

func handleBayStatus(r *Remote, f *frame) {
	dev := r.deviceFor(f)
	if dev == nil {
		return
	}
	port, ok := f.u8(1)
	if !ok {
		return
	}
	bay := dev.getByPortnumLocked(int(port))
	if bay == nil {
		return
	}
	if feat, ok := f.u32(24); ok {
		bay.features = BayFeaturesMask(feat)
	}
	if st, ok := f.u32(20); ok {
		status := BayStatusMask(st)
		bay.applyBayStatus(status)
		if !status.Has(BayStatusSignalDetected) || !dev.isV2IP() {
			desc, _ := f.str(2, 16)
			bay.applySignalStatus(status.Has(BayStatusSignalDetected), &desc)
		}
	}
}

// handleLinks merges one page of link descriptors into the link registry.
// Paged the same way as the bay config, so the same merge rule applies.
func handleLinks(r *Remote, f *frame) {
	dev := r.deviceFor(f)
	if dev == nil {
		return
	}
	p := f.payload()
	const recSize = 38
	for off := 0; off+recSize <= len(p); off += recSize {
		rec := p[off : off+recSize]
		port := int(rec[0])
		linkedSerial := cstr(rec[2:18])
		linkedBay := cstr(rec[18:34])
		features := uint32(rec[34]) | uint32(rec[35])<<8 | uint32(rec[36])<<16 | uint32(rec[37])<<24
		if bay := dev.getByPortnumLocked(port); bay != nil {
			r.links.update(bay, linkedSerial, linkedBay, features)
		}
	}
	dev.onLinkConfigReceived()
}

func handleRCKey(r *Remote, f *frame) {
	dev := r.deviceFor(f)
	if dev == nil {
		return
	}
	var port int
	if f.protocol() >= 6 {
		v, ok := f.u16(0)
		if !ok {
			return
		}
		port = int(v)
	} else {
		v, ok := f.u8(0)
		if !ok {
			return
		}
		port = int(v)
	}
	var key uint16
	var ok bool
	if dev.hello.supportedProtocol >= 6 {
		key, ok = f.u16(2)
	} else {
		key, ok = f.u16(1)
	}
	bay := dev.getByPortnumLocked(port)
	if bay == nil || !ok {
		return
	}
	bay.emitKey(RCKey(key))
}

func handleMirrorStatus(r *Remote, f *frame) {
	dev := r.deviceFor(f)
	if dev == nil {
		return
	}
	target := mustUUID(f, 0)
	if dev.uid != target {
		return
	}
	out := dev.firstOutputLocked()
	if out == nil {
		return
	}
	master := mustUUID(f, 16)
	var ms BayMirrorStatus
	if !master.Empty() && master != target {
		bu := BayUID{Device: master, Port: 0}
		ms = BayMirrorStatus{Target: &bu}
	}
	out.setMirroring(ms)
}

func handleMeshOperation(r *Remote, f *frame) {
	dev := r.deviceFor(f)
	if dev == nil {
		return
	}
	op, ok := f.u8(0)
	if !ok {
		return
	}
	const meshReportMembership = 0xFF
	if op == meshReportMembership {
		dev.setMeshMaster(mustUUID(f, 4))
	}
}

func handleV2IPBayMapping(r *Remote, f *frame) {
	dev := r.deviceFor(f)
	if dev == nil {
		return
	}
	val, ok := f.u16(0)
	if !ok {
		return
	}
	nb := int(val >> 1)
	mode := "Output"
	if val&1 == 1 {
		mode = "Input"
	}
	first, ok := f.u16(2)
	if !ok {
		return
	}
	for i := 0; i < nb; i++ {
		uid, ok := f.uuid(8 + 16*i)
		if !ok {
			break
		}
		if bay := dev.getByModeBayLocked(mode, int(first)+i); bay != nil {
			bay.v2ipUID = uid
		}
	}
}

func handleTopology(r *Remote, f *frame) {
	dev := r.deviceFor(f)
	if dev == nil {
		return
	}
	p := f.payload()
	var topo []TopologyEntry
	for off := 0; off+20 <= len(p); off += 20 {
		var uid DeviceUID
		copy(uid[:], p[off:off+16])
		mask := uint32(p[off+16]) | uint32(p[off+17])<<8 | uint32(p[off+18])<<16 | uint32(p[off+19])<<24
		topo = append(topo, TopologyEntry{UID: uid, Mask: mask})
	}
	dev.setTopology(topo)
}

func handleMultiviewer(r *Remote, f *frame) {
	dev := r.deviceFor(f)
	if dev == nil {
		return
	}
	sub, ok := f.u8(16)
	if !ok {
		return
	}
	if sub == mvOpStatus {
		dev.updateMultiviewer(parseMVConfig(f))
	}
	if r.callbacks.OnMultiviewerCommand == nil {
		return
	}
	target, _ := f.uuid(0)
	cmd := MultiviewerCommand{Target: target, Op: sub}
	if p := f.payload(); len(p) > 24 {
		cmd.Params = append([]byte(nil), p[24:]...)
	}
	cb := r.callbacks.OnMultiviewerCommand
	r.emit(func() { cb(dev, cmd) })
}

func handleAudio(r *Remote, f *frame) {
	dev := r.deviceFor(f)
	if dev == nil {
		return
	}
	op, ok := f.u16(0)
	if !ok {
		return
	}
	switch op {
	case audioOpFeatures:
		eps := parseAudioEndpoints(f)
		nb, _ := f.u16(28)
		dev.setAudioEndpoints(eps)
		if audioHasLinks(f, int(nb)) {
			dev.applyAudioLinks(parseAudioLinks(f, 36+int(nb)*16))
		}
	case audioOpLinks:
		dev.applyAudioLinks(parseAudioLinks(f, 0))
	case audioOpSelectInput:
		if ch, ok := parseAudioChangeSource(f); ok {
			dev.setAudioSelectInput(ch)
		}
	case audioOpMute, audioOpTrigger, audioOpVolume:
		endpoint, ok := f.u16(20)
		if !ok {
			return
		}
		value, ok := f.u32(24)
		if !ok {
			return
		}
		dev.emitAudioParam(op, int(endpoint), value)
	}
}

func handleFirmwareVersion(r *Remote, f *frame) {
	dev := r.deviceFor(f)
	if dev == nil {
		return
	}
	ftype, ok := f.u8(0)
	if !ok {
		return
	}
	// read at most the field's own width, but settle for what a peer sending a
	// short frame did give us rather than losing the whole report over a name
	nameLen := len(f.payload()) - 12
	if nameLen > fwVersionLen {
		nameLen = fwVersionLen
	}
	if nameLen <= 0 {
		return
	}
	version, ok := f.str(12, nameLen)
	if !ok {
		return
	}
	hash, _ := f.u32(4)
	ts, ok := f.u32(8)
	if !ok {
		return
	}
	dev.setFirmwareVersion(FirmwareVersion{
		Type:      FirmwareType(ftype),
		Timestamp: int64(ts),
		Version:   version,
		Hash:      hash,
	})
}

func handleSystemStatus(r *Remote, f *frame) {
	dev := r.deviceFor(f)
	if dev == nil {
		return
	}
	status, ok := f.u16(16)
	if !ok {
		return
	}
	msg, _ := f.str(18, -1)
	dev.setSystemStatus(int(status), msg)
}

func mustUUID(f *frame, idx int) DeviceUID {
	uid, _ := f.uuid(idx)
	return uid
}

// ---- bay/device mutators used by handlers ----

func (b *Bay) applyConnectStatus(c ConnectStatus) {
	if b.isInput() {
		b.setBoolStatus(&b.signalDetected, c == Connected, b.dev.remote.callbacks.OnStatusSignalDetectedChanged, false)
	} else {
		b.setBoolStatus(&b.hpdDetected, c == Connected, b.dev.remote.callbacks.OnStatusHpdDetectedChanged, false)
	}
}

func (b *Bay) applyHidden(h HiddenStatus) {
	if h != HiddenUnknown {
		b.setBoolStatus(&b.hidden, h == Hidden, b.dev.remote.callbacks.OnStatusHiddenChanged, true)
	}
}

func (b *Bay) applySelected(video, audio *Bay) {
	if video != nil {
		b.setVideoSource(video)
	}
	if audio != nil {
		b.setAudioSource(audio)
	}
}

func (b *Bay) applySignalStatus(detected bool, desc *string) {
	b.setBoolStatus(&b.signalDetected, detected, b.dev.remote.callbacks.OnStatusSignalDetectedChanged, false)
	if desc != nil {
		b.setSignalType(*desc)
	}
}

func (b *Bay) emitAction(a RCAction) {
	r := b.dev.remote
	if r.callbacks.OnActionReceived != nil {
		r.emit(func() { r.callbacks.OnActionReceived(b, a) })
	}
	b.emitSelf()
}

func (b *Bay) emitKey(k RCKey) {
	r := b.dev.remote
	if r.callbacks.OnKeyPressed != nil {
		r.emit(func() { r.callbacks.OnKeyPressed(b, k) })
	}
	b.emitSelf()
}

func (d *Device) setV2IPDetails(det DeviceV2IPDetails) {
	merged := det.merge(d.v2ipDetails)
	d.v2ipDetails = &merged
	d.emitSelf()
}

func (d *Device) setV2IPSink(s DeviceV2IPSink) {
	d.v2ipSink = &s
	d.emitSelf()
}
