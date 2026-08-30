# Routing and control

Every control method returns an `error`. A method refuses before it transmits
when the bay is the wrong kind for the command, and when the target device
advertised a protocol version older than the command requires. That second check
matters because a device that receives a frame stamped above its own version
drops it silently, without a reply at any layer: without the check the call
would succeed while nothing happened.

## Video and audio routing

Routing is issued on the **sink** bay and names the source:

```go
sink.SelectVideoSource(0)                  // route video from source port 0
sink.SelectAudioSource(0)                  // route audio from source port 0
sink.SelectVideoSourceByUserName("Apple TV")
sink.SelectAudioSourceByName("Apple TV", nil)
```

Both `ByName` forms look the source up by its user-assigned name on the same
device and then route by port.

For a source that is not a bay on the mesh, address the stream directly:

```go
sink.SelectAudioSourceAddr("239.1.2.4", 50022, nil)
```

A zero port means the standard V2IP audio port. The video and ancillary streams
are left as they are. Passing a `*V2IPAudioFormat` overrides the receiver's
sample rate and channel count; `nil` keeps what the stream declares.

These four are V2IP sinks only. The current route is read back with
`Bay.VideoSource()`, `Bay.AudioSource()` and `Bay.V2IPSource()`.

## Volume and power

```go
b.VolumeSet(30, nil)     // 0-100; pass a *bool to set mute at the same time
b.VolumeUp(); b.VolumeDown()
b.MuteSet(true)
b.PowerOn(); b.PowerOff()
```

`VolumeUp`, `VolumeDown` and `MuteSet` are built on the current volume, so they
fail when the device has not reported one yet.

Power is a remote-control action sent to the display attached to the bay, not a
device power state: `PowerOn` and `PowerOff` are `TxAction(ActionPowerOn)` and
`TxAction(ActionPowerOff)`.

## Naming, hiding and EDID

```go
b.SetName("Kitchen")           // at most 16 characters
b.SetHidden(true)              // hide the bay in device UIs
b.SelectEdidProfile(profile)   // input bays
```

## Remote-control passthrough

```go
b.TxAction(mxremote.ActionPowerOn)
```

Keys and actions arriving from a device reach `OnKeyPressed` and
`OnActionReceived`, and captured IR reaches `OnIRCaptured`.

## Device-level commands

```go
d.Reboot()
d.ReadStats(true)          // enable periodic statistics reporting
mx.SendMonitoringPulse()
```
