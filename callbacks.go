// Author: Lars Op den Kamp (lars@opdenkamp-it.nl)
// Copyright (c) 2026 Op den Kamp IT Solutions

package mxremote

// Callbacks holds optional event handlers invoked when device or bay state
// changes. Every field is optional; leave a field nil to ignore that event.
//
// Handlers are invoked sequentially from a single dispatcher goroutine, so they
// must not block for long. It is safe to call Remote/Device/Bay methods from a
// handler.
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
}
