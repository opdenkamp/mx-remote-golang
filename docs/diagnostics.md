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

Statistics are off until asked for, and the subscription lapses 60s after the
request that armed it — call `ReadStats(true)` again inside the minute to keep
reports coming, since the library does not re-arm on its own. Reports then
arrive at 1Hz.

```go
d.ReadStats(true)
if st := d.V2IPStats(); st != nil {
    st.Tx; st.TxPerMinute      // video, audio, ancillary, stream-down, overflow
    st.Rx; st.RxPerMinute

    st.DecoderReported         // false on firmware that does not carry the block
    if dec := st.Decoder; dec != nil {
        dec.Reason             // primary cause
        dec.Flags              // every cause that applies, dec.HasReason(r)
        dec.Width; dec.Height  // recovered from the codestream, pre-scaler
        dec.Format             // recovered colour space
        dec.Updates            // readings stored so far
        dec.Blocking; dec.BlockedCount
    }
}
```

Cumulative counters and per-minute counters are reported side by side, so a rate
does not have to be derived from two samples.

### Decoder detail

`st.Decoder` is what the sink's decoder recovered from the codestream it is
being given, as opposed to what came out of the scaler after it. Three states
are distinct and mean different things:

| `DecoderReported` | `Decoder` | |
|---|---|---|
| false | nil | the firmware does not report decoder detail |
| true | nil | it does, and the decoder has never answered |
| true | non-nil | a reading |

A configured sink reports whether or not it is enabled, and **nothing in this
block answers enablement in either direction.** Newer firmware names a disabled
sink `DecoderReasonIdle`; firmware predating that reports
`DecoderReasonNoPackets` for the same sink, indistinguishable from one whose
source is dead — so no `DecoderReasonIdle` is not evidence a sink is on. Ask
`V2IP_DEVICE_CFG` or the device HTTP status.

Three things routinely get read wrong here:

- **Geometry, not `Format`, says whether anything was recovered.** With no
  stream `Format` reads `DecoderFormatRGB`, which a real RGB stream is
  indistinguishable from, so treating format 0 as no-signal reports a dead sink
  on a live one. `Decoder.Recovered()` asks the question correctly — about the
  codestream, which is not the same question as whether the sink is enabled.
  Geometry is read before any reason is decided, so an idle sink reporting a
  picture is a normal steady state rather than a stale reading.
- **`DecoderFormatUnnamed` is 255, and is not the `0xF` that means unknown in
  `MxrSignalType.ColourSpace`.** The two do not convert into one another.
- **Colour depth is absent and stays absent**, because it is not recovered
  from the codestream. Assert bit depth at the encoder's input bay instead.

`Reason` and `Format` values this library has no name for are passed through as
they arrived, so check for the constants you handle rather than assuming the
set is closed.

**Classify on `Flags`, and use `Reason` only for display.** Every cause that
applies sets its bit, where `Reason` is whichever one won a fixed priority
order — deliberately not the numbering, so it is neither the lowest nor the
highest bit set and cannot be derived from `Flags`. The case that makes this
concrete is `DecoderReasonTxBridgeUnlocked`, which ranks below every
input-side cause: a sink restarting in a loop names an input cause in `Reason`
and carries bit 9 in `Flags` alone, permanently. Code keyed on `Reason` misses
a restart loop entirely.

```go
if dec.HasReason(DecoderReasonTxBridgeUnlocked) {
    // true whether or not it is what Reason names
}
```

**Test `DecoderReasonIdle` first and stop.** It outranks every cause below it,
and those causes stay set — they are what the decoder genuinely observed, and
`HasReason` reports the word as it arrived rather than suppressing them. A
plain mask of the causes you treat as faults therefore calls a deliberately
switched-off sink broken.

```go
switch {
case dec.HasReason(DecoderReasonIdle):
    // configured, not enabled — not a fault, whatever else is set
case dec.HasReason(DecoderReasonNoPackets):
    // ...
}
```

### Reading freshness

The decoder is read every 2s and the report goes out every 1s, latched between
reads, so roughly every other report repeats a reading already seen: a frame
arriving says nothing about freshness. `Updates` counts readings actually
stored — monotonic, never reset, wrapping at 65535 (about 36 hours) — so a
stalled decoder leaves it still rather than implying a refresh.

After changing what a sink is pointed at, wait for `Updates` to advance by
**two** before trusting the geometry. The counter ticks when a reply lands
rather than when a query is sent, so a single tick can carry an answer read
fractionally before the switch.

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
