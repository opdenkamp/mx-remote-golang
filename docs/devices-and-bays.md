# Devices and bays

A `Device` is one physical unit on the network: a neo matrix, a OneIP encoder or
decoder, a ProAmp8. A `Bay` is one port on it, an input or an output. Both are
snapshots of what the device has reported, kept current as frames arrive.

All accessors are safe for concurrent use.

## Finding a device

```go
for _, d := range mx.Devices() { /* every device discovered so far */ }

d := mx.GetBySerial("AB1234")
d := mx.GetByUID(uid)
d := mx.GetByUIDString("00:11:22:33:44:55:66:77")
b := mx.GetBayByPortnum(uid, 3)
```

Discovery is continuous, so `Devices()` grows during the first seconds after
`Start`. `OnDeviceConfigComplete` fires when a device has reported enough for
its bays to be populated.

## Device state

```go
d.Serial(); d.Name(); d.ModelName(); d.Address(); d.UID()
d.Status(); d.Online(); d.Version(); d.Protocol(); d.Features()
d.Temperatures()
```

`Protocol()` is the protocol version the device advertised. It decides which
commands it will accept: a receiver silently drops any frame stamped above its
own version, so this library refuses to send an opcode the device is too old
for rather than letting the call succeed while nothing happens.

Model predicates say what a device is:

```go
d.IsV2IP(); d.IsOneIPTx(); d.IsOneIPRx(); d.IsOneIPTz(); d.IsOneIPMultiviewer()
d.IsVideoMatrix(); d.IsAudioMatrix(); d.IsAmp()
```

## Bays

```go
for _, b := range d.Outputs() { /* ... */ }
for _, b := range d.Inputs()  { /* ... */ }
d.Bays()             // every bay, keyed by port number
d.GetByPortnum(3)
d.GetByPortname("HDMI 1")
d.FirstInput(); d.FirstOutput()
```

Each bay carries its identity, its mode and its reported state:

```go
b.BayLabel(); b.BayName(); b.UserName(); b.BayNumber(); b.Port(); b.Device()
b.Mode(); b.IsInput(); b.IsOutput(); b.IsHDMI(); b.IsHDBaseT(); b.IsAudio()
b.IsV2IPSource(); b.IsV2IPSink(); b.IsV2IPRemote(); b.IsLocal()

b.SignalDetected(); b.SignalType(); b.SignalMode(); b.SignalDetails()
b.PowerStatusValue(); b.HPDDetected(); b.CECDetected(); b.HDBTConnected()
b.PoEPowered(); b.Faulty(); b.Hidden(); b.ARC()
b.VolumeStatus(); b.Volume(); b.Muted()
b.EdidProfileValue(); b.RCTypeValue(); b.Features()
```

Current routing is read from the sink bay:

```go
b.VideoSource(); b.AudioSource(); b.V2IPSource()
```

## Bay identity across devices

`Bay.BayUID()` returns a `BayUID`, the pair of device UID and port that names a
bay anywhere on the network. For a V2IP stream it resolves to the bay on the
device that originates the stream, not to the local proxy for it, which is what
makes a source comparable across devices.

## Links and mirroring

A virtual link ties one bay to a bay on another device, so that routing one
routes the other.

```go
if l := b.Link(); l != nil {
    l.Serial(); l.LinkedBayName(); l.LinkedBay(); l.IsAudio(); l.IsVideo(); l.Configured()
}
b.LinkConfigured(); b.LinkedBay()
b.Mirroring()   // BayMirrorStatus
```

`OnBayLinked`, `OnBayUnlinked` and `OnMirrorStatusChanged` report changes.

## Mesh

Units running as a mesh elect a master, which the registry tracks:

```go
d.IsMeshMaster(); d.IsMeshMember(); d.MeshMaster()
```

## V2IP streaming

On a HDMI-over-IP unit:

```go
d.V2IPSources()   // the streams this device offers
d.V2IPSink()      // what it is currently receiving
d.V2IPDetails()   // its stream configuration: addresses, rate, scaling, DSCP
```

`V2IPDetails` is merged field by field as the device reports it. A field the
sender did not write stays as it was, rather than being overwritten with a zero.
