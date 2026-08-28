// Author: Lars Op den Kamp (lars@opdenkamp-it.nl)
// Copyright (c) 2026 Op den Kamp IT Solutions

package mxremote

// Callbacks holds optional event handlers invoked when device or bay state
// changes. Every field is optional; leave a field nil to ignore that event.
//
// Handlers are invoked sequentially from a single dispatcher goroutine, so they
// must not block for long. It is safe to call Remote/Device/Bay methods from a
// handler.
//
// A handler that panics takes the process with it. Handlers run on goroutines
// this library owns — the receive loop and the background probe — and an
// unrecovered panic in any goroutine ends the program, so there is nowhere for
// a caller to put a recover of its own. Not recovering on the caller's behalf
// is deliberate: this library has no logger, so a recover here would swallow
// the panic in silence and hide the bug rather than surface it. Recover inside
// the handler if a panic there should not be fatal.
type Callbacks struct {
	// Device-level events.
	OnDeviceConfigChanged       func(d *Device)
	OnDeviceConfigComplete      func(d *Device)
	OnDeviceOnlineStatusChanged func(d *Device, online bool)
	OnDeviceTemperatureChanged  func(d *Device)
	OnDeviceUpdate              func(d *Device)

	// Bay-level events.
	OnBayRegistered               func(b *Bay)
	OnBayUpdate                   func(b *Bay)
	OnVideoSourceChanged          func(b *Bay, videoSource *Bay)
	OnAudioSourceChanged          func(b *Bay, audioSource *Bay)
	OnVolumeChanged               func(b *Bay, volume *VolumeMuteStatus)
	OnPowerChanged                func(b *Bay, power PowerStatus)
	OnNameChanged                 func(b *Bay, name string)
	OnStatusSignalDetectedChanged func(b *Bay, detected bool)
	OnStatusFaultyChanged         func(b *Bay, faulty bool)
	OnStatusHiddenChanged         func(b *Bay, hidden bool)
	OnStatusPoePoweredChanged     func(b *Bay, powered bool)
	OnStatusHdbtConnectedChanged  func(b *Bay, connected bool)
	OnStatusSignalTypeChanged     func(b *Bay, signalType string)
	OnStatusHpdDetectedChanged    func(b *Bay, detected bool)
	OnStatusCecDetectedChanged    func(b *Bay, detected bool)
	OnStatusArcChanged            func(b *Bay, arc string)
	OnEdidProfileChanged          func(b *Bay, profile EdidProfile)
	OnRcTypeChanged               func(b *Bay, rcType RCType)
	OnKeyPressed                  func(b *Bay, key RCKey)
	OnActionReceived              func(b *Bay, action RCAction)
	OnMirrorStatusChanged         func(b *Bay, mirror BayMirrorStatus)
	OnBayLinked                   func(b *Bay, linkedSerial, linkedBay string, features LinkFeature)
	OnBayUnlinked                 func(b *Bay, linkedSerial, linkedBay string)
	OnAmpZoneSettingsChanged      func(b *Bay, settings AmpZoneSettings)
	OnAmpDolbySettingsChanged     func(d *Device, settings AmpDolbySettings)
	OnVolumeStep                  func(b *Bay, up bool)
	OnAudioClip                   func(b *Bay, clip AudioClip)
	OnIRCaptured                  func(b *Bay, capture IRCapture)
	OnFilteredDevicesChanged      func(b *Bay, filtered []DeviceUID)

	// Device state carried by the command opcodes.
	OnPDUStateChanged      func(d *Device, state PDUState)
	OnSetupStatusChanged   func(d *Device, completed bool)
	OnInstallerIDChanged   func(d *Device, installerID uint16)
	OnTilingChanged        func(d *Device, tiling V2IPTilingConfig)
	OnRCSettingsChanged    func(d *Device, settings RCSettings)
	OnV2IPLinkChanged      func(d *Device, target DeviceUID)
	OnMultiviewerCommand   func(d *Device, cmd MultiviewerCommand)
	OnAudioSelectInput     func(d *Device, change AudioChangeSource)
	OnAudioEndpointMute    func(d *Device, endpoint int, muted bool)
	OnAudioEndpointTrigger func(d *Device, endpoint int, active bool)
	OnAudioEndpointVolume  func(d *Device, endpoint int, volume uint32)

	// Requests addressed to a device. The sender is d; the request's own Target
	// names the unit it addresses, which need not be d or this client.
	OnDiscoverRequest            func(d *Device)
	OnSetRouteRequested          func(d *Device, req SetRouteRequest)
	OnEDIDRequested              func(d *Device, req EDIDRequest)
	OnEDIDReceived               func(d *Device, edid EDIDRecord)
	OnBayNameChangeRequested     func(d *Device, change BayNameChange)
	OnEDIDProfileChangeRequested func(d *Device, change EDIDProfileChange)
	OnRebootRequested            func(d *Device, req RebootRequest)
	OnFactoryResetRequested      func(d *Device, req FactoryResetRequest)
	OnMonitoringPulse            func(d *Device)
	OnUpgradeFPGARequested       func(d *Device)
	OnDetectBaysRequested        func(d *Device)
	OnPowerSaveRequested         func(d *Device, req V2IPPowerSaveRequest)
	OnKeyTransmitRequested       func(d *Device, req KeyTransmitRequest)
	OnActionTransmitRequested    func(d *Device, req ActionTransmitRequest)
	OnIRTransmitRequested        func(d *Device, req IRTransmitRequest)
	OnBlacklistChanged           func(d *Device, change V2IPBlacklistChange)
	OnVideoWallCommand           func(d *Device, cmd VideoWallCommand)
}
