# Configuration

`mxremote.New` takes a `Config` and returns a `Remote` that is not yet on the
network; `Remote.Start` opens the socket and begins discovery. Every field is
optional.

```go
mxremote.Config{
    TargetIP:  "",    // multicast/broadcast IP; empty = default
    Port:      0,     // 0 = default for the mode (8812 multicast, 8811 broadcast)
    LocalIP:   "",    // interface to bind by IP; empty = first non-loopback
    Interface: "",    // interface name for multicast (takes precedence over LocalIP)
    Broadcast: false, // broadcast instead of multicast
    Name:      "",    // advertised name; empty = "MXR Go"
    Callbacks: mxremote.Callbacks{ /* optional handlers */ },
}
```

## Multicast or broadcast

Multicast on `224.8.8.8:8812` is the default and is what MatrixOS devices use
among themselves. `Broadcast: true` switches to broadcast on port 8811, for
networks where multicast is filtered.

## Choosing the interface

On a host with one NIC neither `LocalIP` nor `Interface` is needed. On a
multi-homed host, set one of them: the default picks the first non-loopback IPv4
the host enumerates, which is arbitrary.

Picking the wrong NIC fails in one direction only. Devices broadcast
periodically, so those frames still arrive and discovery looks healthy, while
every request this library sends leaves by the wrong interface and is never
answered.

`LocalIP` selects by address. `Interface` selects by name and takes precedence
when both are set:

```go
mx := mxremote.New(mxremote.Config{Interface: "br_v8"})
```

Selecting by name joins the group and keys egress on the interface index, so it
works on an interface with **no IPv4 address of its own**, such as a tagged VLAN
where the devices are reachable but the host holds no address in their subnet.
That index-based join is Linux only; on macOS and Windows the named interface
must have an IPv4 address.

## Changing the interface later

`Remote.UpdateConfig(localIP string, broadcast bool)` reopens the socket with a
different local address or transport, keeping the registry and the callbacks. It
returns immediately when neither value changed.
