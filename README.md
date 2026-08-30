# MX Remote - Go Client for Pulse-Eight MatrixOS devices

[![Go Reference](https://pkg.go.dev/badge/github.com/opdenkamp/mx-remote-golang/v2.svg)](https://pkg.go.dev/github.com/opdenkamp/mx-remote-golang/v2)

A client for [Pulse-Eight](https://www.pulse-eight.com/) AV distribution
hardware: video and audio matrices, HDMI-over-IP encoders and decoders,
multiviewers and 8-zone amplifiers, all driven over the local network. It covers
discovery, video and audio routing, volume, remote-control key passthrough,
HDMI-over-IP streaming and multiviewer control.

If you want to drive Pulse-Eight **neo**, **OneIP** or **ProAmp8** hardware from
your own software or from a home automation system, this is the library for it.

```bash
go get github.com/opdenkamp/mx-remote-golang/v2
```

```go
import mxremote "github.com/opdenkamp/mx-remote-golang/v2"
```

The import path ends in `mx-remote-golang/v2` while the package is called
`mxremote`, so import it aliased. The API reference is on
[pkg.go.dev](https://pkg.go.dev/github.com/opdenkamp/mx-remote-golang/v2).

## What is MX Remote?

MX Remote is the protocol these devices use to discover and control one another
over UDP, by multicast or by broadcast. They all run the same **MatrixOS**
firmware, which speaks it natively. This library is a client implementation of
that protocol, written to open the devices up to third-party software.

Devices announce themselves and their bays, report signal, audio, streaming and
power state as it changes, and accept routing and configuration commands. The
library discovers them, keeps a snapshot of what they have reported, and sends
those commands.

## Supported devices

- **[neo](https://www.pulse-eight.com/)**: HDBaseT video and audio matrices. The
  neo:4, neo:8 and neo:X, and the splitters.
- **[OneIP](https://www.pulse-eight.com/p/248/oneip-tx)**: HDMI-over-IP units.
  Transmitter (TX), Receiver (RX), Transceiver (TZ) and Multiviewer.
- **[ProAmp8](https://www.pulse-eight.com/p/219/proamp-8)**: an 8-zone audio
  amplifier with Dolby decoding.

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

[`examples/discover`](examples/discover) is the same program, complete:

```bash
go run ./examples/discover            # multicast
go run ./examples/discover -b         # broadcast
go run ./examples/discover -l 192.168.1.20
go run ./examples/discover -i br_v8   # join multicast on a named interface (no-IP VLAN)
```

## Documentation

| Page | Covers |
| ---- | ------ |
| [Configuration](docs/configuration.md) | `Config`, multicast and broadcast, choosing an interface |
| [Devices and bays](docs/devices-and-bays.md) | the device registry, bay state, links, mirroring, mesh |
| [Routing and control](docs/routing.md) | video and audio routing, EDID, power, volume |
| [Callbacks](docs/callbacks.md) | the event model and every handler |
| [Multiviewer](docs/multiviewer.md) | multiviewer status and control, tiling, video wall |
| [Audio and amplifier](docs/audio.md) | V2IP audio endpoints and ProAmp8 zones |
| [Diagnostics](docs/diagnostics.md) | network status, statistics, firmware, signal detail |
| [Migrating from v1](docs/migrating-from-v1.md) | the two field types that changed |

## Other languages

Two other clients speak the same protocol, each in its own repository:

- **Python**: <https://github.com/opdenkamp/mx-remote>, the oldest of the three,
  and the one this client was ported from.
- **Rust**: <https://github.com/opdenkamp/mx-remote-rust>, which also ships a C
  ABI for C and C++ consumers.

The three are independent implementations rather than bindings over a shared
core.

## Requirements

- Go 1.19 or later.
- Linux, macOS or Windows. Selecting an interface that has no address of its
  own, such as a tagged VLAN, is Linux-only.
- Network access to one or more of the devices above, by multicast or broadcast.

## License

BSD 3-Clause. See [LICENSE](LICENSE).
