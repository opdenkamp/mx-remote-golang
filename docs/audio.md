# Audio and amplifier

Two separate things live here: the audio endpoint tree a V2IP device exposes,
and the zone settings of a ProAmp8.

## V2IP audio endpoints

A V2IP device reports its audio as a tree of endpoints: inputs, outputs and
processing nodes, each with an ID, a feature set and, for a streaming endpoint,
a multicast address.

```go
eps := d.AudioEndpoints()
for _, ep := range eps.List() {
    ep.ID; ep.Features; ep.Address
    ep.IsInput(); ep.IsOutput(); ep.IsV2IP()
    ep.Bay()               // the bay this endpoint belongs to, or nil
    ep.Input()             // the endpoint currently routed into this one
    ep.InputsAvailable()   // the endpoints that may be routed into it
}

ep := d.AudioEndpointByID(3)
ep := b.AudioEndpoint()    // the endpoint of a bay
```

### Control

```go
d.AudioMute(endpointID, true)
d.AudioTrigger(endpointID, true)
d.AudioVolumeSet(endpointID, 40)
d.AudioSelectInput(sinkEP, sourceDeviceUID, sourceEP)
```

`AudioSelectInput` routes across devices: the sink endpoint is on `d`, the
source endpoint is on the device named by the UID. `Device.AudioSourceSelection()`
reads back the last selection this device reported.

Endpoint events arrive as `OnAudioEndpointMute`, `OnAudioEndpointTrigger`,
`OnAudioEndpointVolume` and `OnAudioSelectInput`.

## ProAmp8

Each amplifier zone is a bay. Its settings are read and written whole:

```go
if s := b.AmpSettings(); s != nil {
    s.GainLeft; s.GainRight
    s.VolumeMin; s.VolumeMax
    s.DelayLeft; s.DelayRight
    s.Bass; s.Treble
    s.Bridged
    s.PowerMode; s.PowerLevel; s.PowerTimeout
    s.EQLeft; s.EQRight        // five bands per channel
}

b.SetZoneSettings(settings)
```

`SetZoneSettings` sends the complete structure, so read the current settings,
change what you need and send them back rather than sending a zero value with
one field set.

Dolby state is device-wide:

```go
d.DolbySettings()        // mode, PCM upmix, whether Dolby is detected
d.AmpDolbyChannels()
```

`OnAmpZoneSettingsChanged` and `OnAmpDolbySettingsChanged` report changes.

A ProAmp8 on firmware 4.1.1 advertises protocol `0x11`, which is below the floor
of several opcodes this library sends. Those calls are refused before they
transmit rather than being sent and silently dropped, so check the error.
