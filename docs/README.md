# Documentation

Start with the [README](../README.md) for what this library is and how to get a
client running. These pages cover the API in more detail. The generated
reference is on
[pkg.go.dev](https://pkg.go.dev/github.com/opdenkamp/mx-remote-golang/v2).

- [Configuration](configuration.md): `Config`, multicast and broadcast, and
  choosing which interface to speak on.
- [Devices and bays](devices-and-bays.md): the device registry, bay state,
  links, mirroring and mesh.
- [Routing and control](routing.md): video and audio routing, EDID, power and
  volume.
- [Callbacks](callbacks.md): the event model, the fan-in rule and every handler.
- [Multiviewer](multiviewer.md): multiviewer status and control, tiling and
  video wall.
- [Audio and amplifier](audio.md): V2IP audio endpoints and ProAmp8 zones.
- [Diagnostics](diagnostics.md): network status, statistics, firmware versions
  and signal detail.
- [Migrating from v1](migrating-from-v1.md): the two field types that changed.

Contributors should read [AGENTS.md](../AGENTS.md), which covers the wire
protocol and how this package is tested.
