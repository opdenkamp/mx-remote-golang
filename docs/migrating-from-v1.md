# Migrating from v1

The import path gains the `/v2` suffix that Go modules require at major version
2 and above:

```go
import mxremote "github.com/opdenkamp/mx-remote-golang/v2"
```

Two fields changed type, both on the V2IP device configuration. Everything else
is additive, so a consumer that does not touch these needs only the import path
change.

- `DeviceV2IPDetails.TxRate` is now `*uint8` rather than `uint8`. It is nil when
  the sender offered no rate, which a device does on every write that is not a
  rate change. In v1 that arrived as a real-looking `0`.
- `V2IPScalingSettings.Mode` is now `MxrSignalType` rather than `uint16`, which
  decodes the SVD, colour space and bit depth packed into it. Comparisons with
  untyped constants still compile; assigning it to a `uint16` needs a
  conversion.
