# MX Remote — Pulse-Eight device interface

Go library for discovering and controlling [Pulse-Eight](https://www.pulse-eight.com/)
AV distribution hardware over a local network: video/audio matrices, HDMI-over-IP
encoders/decoders, multiviewers, and audio amplifiers. Supports device discovery,
video/audio routing, volume control, remote-control key passthrough, HDMI-over-IP
streaming, multiviewer control, and more.

If you are looking to integrate Pulse-Eight **neo**, **OneIP**, or **ProAmp8** devices into
your own software or home-automation system, this is the library for it.

## What is MX Remote?

MX Remote is the network protocol these Pulse-Eight devices use to discover and control one
another over UDP (multicast or broadcast). All of them run the shared **MatrixOS** firmware,
which speaks this protocol natively. This library is a client implementation of that
protocol — its purpose is to expose the devices to third-party software.

## Supported devices

All devices below run MatrixOS and are controlled through the same protocol:

- **[Pulse-Eight neo](https://www.pulse-eight.com/)** — HDBaseT video/audio matrices
  (neo:4, neo:8, neo:X, and splitters)
- **[Pulse-Eight OneIP](https://www.pulse-eight.com/p/248/oneip-tx)** — HDMI-over-IP
  units: Transmitter (TX), Receiver (RX), Transceiver (TZ), and Multiviewer
- **[Pulse-Eight ProAmp8](https://www.pulse-eight.com/p/219/proamp-8)** — 8-zone audio amplifier with
  Dolby support

## Requirements

- Go 1.19 or later (Linux, macOS, or Windows)
- Network access to one or more of the Pulse-Eight devices above (multicast or broadcast)

## Installation

```
import mxremote "github.com/opdenkamp/mx-remote-golang/v2"
```

### Upgrading from v1

Two fields changed type, both on the V2IP device configuration. Everything else
is additive, so a v1 consumer that does not touch these needs only the import
path change.

- `DeviceV2IPDetails.TxRate` is now `*uint8` rather than `uint8`. It is `nil`
  when the sender offered no rate, which a device does on every write that is
  not a rate change — previously that arrived as a real-looking `0`.
- `V2IPScalingSettings.Mode` is now `MxrSignalType` rather than `uint16`, which
  decodes the svd, colour space and bit depth packed into it. Comparisons with
  untyped constants still compile; assigning it to a `uint16` needs a
  conversion.

## Quick start

```go
ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
defer cancel()

mx := mxremote.New(mxremote.Config{
    Name: "my app",
    Callbacks: mxremote.Callbacks{
        OnDeviceConfigComplete: func(d *mxremote.Device) {
            fmt.Println("ready:", d.Serial(), d.ModelName(), d.Address())
        },
        OnVideoSourceChanged: func(b, src *mxremote.Bay) {
            fmt.Printf("%s -> %s\n", b.BayLabel(), src.BayLabel())
        },
    },
})
if err := mx.Start(ctx); err != nil {
    log.Fatal(err)
}
defer mx.Close()
```

`Start` opens the connection (multicast `224.8.8.8:8812` by default), announces
this client, and begins discovery in the background. Devices and their bays are
populated as frames arrive; use the callbacks or poll `mx.Devices()`.

See [`examples/discover`](examples/discover) for a runnable program:

```
go run ./examples/discover            # multicast
go run ./examples/discover -b         # broadcast
go run ./examples/discover -l 192.168.1.20
go run ./examples/discover -i br_v8   # join multicast on a named interface (no-IP VLAN)
```

## Configuration

```go
mxremote.Config{
    TargetIP:  "",    // multicast/broadcast IP; empty = default
    Port:      0,     // 0 = default for the mode (8812 mcast / 8811 bcast)
    LocalIP:   "",    // interface to bind by IP; empty = first non-loopback
    Interface: "",    // interface name for multicast (takes precedence over LocalIP)
    Broadcast: false, // broadcast instead of multicast
    Name:      "",    // advertised name
    Callbacks: mxremote.Callbacks{ /* optional handlers */ },
}
```

`Interface` selects the multicast interface by **name** (e.g. `"br_v8"`) rather
than by local IP. It joins the group and sends egress keyed on the interface
index, so it works even on an interface with **no IPv4 address** — a tagged VLAN
where the devices are reachable but the host has no address in their subnet. It
takes precedence over `LocalIP` when set. This index-based join is Linux only;
on macOS and Windows the named interface must have an IPv4 address.

## Devices and bays

```go
for _, d := range mx.Devices() {
    d.Serial(); d.Name(); d.ModelName(); d.Address(); d.Status()
    d.IsV2IP(); d.IsOneIPTx(); d.IsOneIPRx(); d.IsOneIPMultiviewer()
    d.V2IPSources(); d.V2IPDetails(); d.V2IPSink()
    for _, b := range d.Outputs() { /* ... */ }
    for _, b := range d.Inputs()  { /* ... */ }
}

b.BayLabel(); b.Mode(); b.SignalDetected(); b.PowerStatusValue()
b.VideoSource(); b.AudioSource(); b.V2IPSource()
```

All accessors are safe for concurrent use.

## V2IP routing control

On a V2IP **sink** bay:

```go
sink.SelectVideoSource(0)              // route video from source port 0
sink.SelectAudioSource(0)              // route audio from source port 0
sink.SelectAudioSourceAddr("239.1.2.4", 50022, nil) // raw multicast address
```

Other control: `bay.SetName`, `bay.SetHidden`, `bay.SelectEdidProfile`,
`bay.TxAction`, `bay.VolumeSet`, `device.Reboot`.

## Callbacks and the fan-in model

Like the reference library, every specific callback (e.g. `OnPowerChanged`)
also fires the generic `OnBayUpdate` (or `OnDeviceUpdate` for device-level
events). Consumers that just want "something changed, re-read state" can set
only `OnBayUpdate`/`OnDeviceUpdate`. Per-device and per-bay callbacks can also
be registered directly with `Device.RegisterCallback` / `Bay.RegisterCallback`.

`OnKeyPressed` and `OnActionReceived` do not fan into `OnBayUpdate` (matching
the reference).

## Links, mirroring and mesh

- Virtual links: `Bay.Link()` → `*BayLink` (`Serial`, `LinkedBayName`,
  `LinkedBay`, `IsAudio`, `IsVideo`, `Configured`), plus `Bay.LinkConfigured`,
  `Bay.LinkedBay`, and the `OnBayLinked`/`OnBayUnlinked` callbacks.
- Mirroring: `Bay.Mirroring()` → `BayMirrorStatus` and `OnMirrorStatusChanged`.
- Mesh: `Device.MeshMaster()`, `Device.IsMeshMaster()`, `Device.IsMeshMember()`.
- Cross-device bay identity: `Bay.BayUID()` (resolves V2IP sources to their
  originating device), `Remote.GetBayByPortnum`.

## Multiviewer

`Device.Multiviewer()` → `*Multiviewer`: full status readout (view mode, PIP,
audio, EDID, HDCP, output mode, per-screen sources, source mappings, firmware
versions) and control (`SetViewMode`, `SetVideoSource`, `SetAudioSource`,
`SetAudioVolume`, `SetPipSize/Position`, `SetScreenAspect`, `SetAutoSwitch`,
`SetOutputMode`, `SetOutputITCMode`, `SetHDCPMode`, `SetEdidTemplate`,
`SetRemoteControl`, `SetConnectedSource`, `AutoRoute`).

## V2IP audio

`Device.AudioEndpoints()`, `Device.AudioEndpointByID`, `Bay.AudioEndpoint()`:
audio endpoint tree with features, addresses, parent/child and input routing;
audio links; and control (`Device.AudioMute`, `AudioTrigger`, `AudioVolumeSet`,
`AudioSelectInput`).

## Amplifier (ProAmp8)

`Bay.AmpSettings()` (gain, volume range, delay, bass/treble, bridging, power
mode/level/timeout, 5-band EQ per channel) and `Bay.SetZoneSettings`;
`Device.DolbySettings()` with the `OnAmpDolbySettingsChanged` /
`OnAmpZoneSettingsChanged` callbacks.

## Diagnostics

`Device.NetworkStatus()` (per-port link speed/duplex, link errors, VCT, UTP
cable pairs, IP/MAC/IGMP querier; both pre-0x22 and 0x22+ wire formats),
`Device.MACAddress()`; V2IP statistics via `Device.ReadStats(bool)` +
`Device.V2IPStats()` (TX/RX cumulative and per-minute counters, decoder state);
firmware versions (`Device.FirmwareVersions()`); detailed signal status with SVD
resolution decoding (`Bay.SignalType` shows e.g. `1920x1080 / RGB / 8bpp /
60Hz`, `LookupSvd`); and system status (`Device.SystemStatus()`,
`Device.StatusMessage()`).

## License

BSD 3-Clause. See [LICENSE](LICENSE).
