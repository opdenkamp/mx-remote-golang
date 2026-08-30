# Diagnostics

## Network status

```go
for port, s := range d.NetworkStatus() {
    s.Port; s.Name
    s.LinkSpeed; s.LinkFullDuplex
    s.IP; s.MACAddress; s.Querier      // the IGMP querier the port sees
    s.Errors                            // *UtpLinkErrors, nil when not reported
    s.VCTStatus; s.CableStatus          // cable test results, per pair
}

d.MACAddress()
```

`Errors`, `VCTStatus` and `CableStatus` are absent on ports and firmware that do
not report them, so check for nil before reading them. Devices report this in
two wire formats depending on their protocol version; both are decoded into the
same struct.

## V2IP statistics

Statistics are off until asked for:

```go
d.ReadStats(true)
if st := d.V2IPStats(); st != nil {
    st.Tx; st.TxPerMinute      // video, audio, ancillary, stream-down, overflow
    st.Rx; st.RxPerMinute
}
```

Cumulative counters and per-minute counters are reported side by side, so a rate
does not have to be derived from two samples.

## Firmware and system status

```go
d.FirmwareVersions()                      // map[FirmwareType]FirmwareVersion
status, message, ok := d.SystemStatus()
d.StatusMessage()
d.Temperatures()
```

## Signal detail

`Bay.SignalType()` renders the current mode, for example
`1920x1080 / RGB / 8bpp / 60Hz`. The parts behind it are available separately:

```go
b.SignalDetected(); b.SignalMode()
if sd := b.SignalDetails(); sd != nil {
    sd.FrameRate       // Hz, corrected for a 1000/1001 clock
    sd.TmdsClock       // Hz
    sd.ClockRate       // Hz
    sd.Scaling         // the signal type the bay is scaling to
    sd.Status
}

svd, ok := mxremote.LookupSvd(id)   // resolve a standard video descriptor
```

A signal type this library has no name for is passed through as it arrived
rather than folded to the nearest known one, so an unrecognised value reaches
the caller intact.
