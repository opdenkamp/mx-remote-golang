# Callbacks

`Config.Callbacks` holds the event handlers. Every field is optional; a nil
field means the event is ignored.

```go
mx := mxremote.New(mxremote.Config{
    Callbacks: mxremote.Callbacks{
        OnBayUpdate: func(b *mxremote.Bay) { redraw(b) },
    },
})
```

Handlers run sequentially on one dispatcher goroutine, so a handler that blocks
delays every later event. Calling `Remote`, `Device` and `Bay` methods from a
handler is safe: events are queued while the library holds its lock and fired
after it is released.

A handler that panics ends the process. Handlers run on goroutines this library
owns, where an unrecovered panic is fatal and a caller has nowhere to put a
`recover` of its own. This library recovers nothing on the caller's behalf,
because it has no logger and a silent recover would hide the bug rather than
surface it. Recover inside the handler if a panic there should not be fatal.

## The fan-in model

Every specific bay event also fires `OnBayUpdate`, and every specific device
event also fires `OnDeviceUpdate`. A consumer that only wants "something
changed, re-read the state" can set those two and nothing else.

`OnKeyPressed` and `OnActionReceived` are the exception: they carry an event
rather than a state change, so they do not fan in. This matches the reference
Python library.

Handlers can also be registered per device or per bay, with
`Device.RegisterCallback` and `Bay.RegisterCallback`, instead of globally on the
`Config`.

## Device events

| Callback | Fires when |
| -------- | ---------- |
| `OnDeviceConfigChanged` | the device reported new configuration |
| `OnDeviceConfigComplete` | its configuration is complete and its bays are populated |
| `OnDeviceOnlineStatusChanged` | it went online or stopped answering |
| `OnDeviceTemperatureChanged` | a temperature reading changed |
| `OnDeviceUpdate` | any of the above, or any bay event on it |

## Bay events

| Callback | Fires when |
| -------- | ---------- |
| `OnBayRegistered` | a bay appeared on a device |
| `OnBayUpdate` | any bay event below |
| `OnVideoSourceChanged`, `OnAudioSourceChanged` | the bay's route changed |
| `OnVolumeChanged`, `OnVolumeStep`, `OnAudioClip` | audio level events |
| `OnPowerChanged` | the attached display's power state changed |
| `OnNameChanged` | the bay was renamed |
| `OnStatusSignalDetectedChanged`, `OnStatusSignalTypeChanged` | signal came, went or changed format |
| `OnStatusHpdDetectedChanged`, `OnStatusCecDetectedChanged`, `OnStatusArcChanged` | HDMI-side state |
| `OnStatusHdbtConnectedChanged`, `OnStatusPoePoweredChanged` | HDBaseT link and PoE |
| `OnStatusFaultyChanged`, `OnStatusHiddenChanged` | the bay was flagged faulty or hidden |
| `OnEdidProfileChanged`, `OnRcTypeChanged` | bay configuration changed |
| `OnKeyPressed`, `OnActionReceived`, `OnIRCaptured` | remote-control input arrived |
| `OnBayLinked`, `OnBayUnlinked`, `OnMirrorStatusChanged` | links and mirroring changed |
| `OnAmpZoneSettingsChanged`, `OnAmpDolbySettingsChanged` | ProAmp8 settings changed |
| `OnFilteredDevicesChanged` | the bay's device filter list changed |

## Device state carried by command opcodes

`OnPDUStateChanged`, `OnSetupStatusChanged`, `OnInstallerIDChanged`,
`OnTilingChanged`, `OnRCSettingsChanged`, `OnV2IPLinkChanged`,
`OnMultiviewerCommand`, `OnAudioSelectInput`, `OnAudioEndpointMute`,
`OnAudioEndpointTrigger`, `OnAudioEndpointVolume`.

## Requests seen on the network

These fire when another controller on the network addresses a device. The
`*Device` is the sender; the request's own `Target` names the unit it addresses,
which need not be the sender and need not be this client.

`OnDiscoverRequest`, `OnSetRouteRequested`, `OnEDIDRequested`, `OnEDIDReceived`,
`OnBayNameChangeRequested`, `OnEDIDProfileChangeRequested`, `OnRebootRequested`,
`OnFactoryResetRequested`, `OnMonitoringPulse`, `OnUpgradeFPGARequested`,
`OnDetectBaysRequested`, `OnPowerSaveRequested`, `OnKeyTransmitRequested`,
`OnActionTransmitRequested`, `OnIRTransmitRequested`, `OnBlacklistChanged`,
`OnVideoWallCommand`.
