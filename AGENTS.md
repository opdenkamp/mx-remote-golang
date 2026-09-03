# AGENTS.md

Instructions for coding agents working in this repository.

## What this is

A Go client library for Pulse-Eight MatrixOS devices (neo matrices, OneIP/V2IP
units, ProAmp8 amplifiers) over UDP multicast/broadcast: discovery, A/V routing,
volume, remote-control passthrough, V2IP streaming, multiviewer, audio
endpoints, amplifier settings, and diagnostics.

- Module `github.com/opdenkamp/mx-remote-golang/v2`, package `mxremote`. The
  import path basename differs from the package name, so import it aliased:
  `import mxremote "github.com/opdenkamp/mx-remote-golang/v2"`. The `/v2` suffix
  is required by Go modules at major version 2 and above.
- Single flat package. The public API is exported; all wire and frame machinery
  is unexported.
- Sibling implementations decode the same protocol: the Python client at
  <https://github.com/opdenkamp/mx-remote> and a Rust port at
  <https://github.com/opdenkamp/mx-remote-rust>. Neither reads this repository,
  so a wire-level finding has to be sent to them deliberately.
- User-facing documentation is `README.md` and `docs/`. An API change means the
  matching `docs/` page changes with it.

## Commands

```bash
go build ./...                 # build
go test ./...                  # run tests
go vet ./...                   # vet
gofmt -l .                     # list unformatted files (should be empty)
go run ./examples/discover     # discover devices on the network (-b broadcast, -l <ip>)
```

Cross-platform (Linux, macOS, Windows). `conn.go` creates the UDP socket via
the `net` package and applies multicast, reuse and broadcast options through
`RawConn.Control`; the only per-OS code is the fd-type cast in `conn_unix.go`
and `conn_windows.go`. Check the other targets with `GOOS=windows go build
./...` and `GOOS=darwin go build ./...`.

## Architecture

One package, grouped by concern:

- **Wire layer** — `frame.go` (24-byte `P8` header pack/unpack plus payload
  accessors), `uid.go` (`DeviceUID`/`BayUID`), `constants.go` (opcodes, network
  defaults), `enums.go`, `types.go`, `bayconfig.go`.
- **Runtime** — `remote.go` (`Remote`: UDP lifecycle, registry, discovery,
  dispatch), `conn.go`, `device.go`, `bay.go`, `callbacks.go`.
- **RX** — `dispatch.go` holds `frameHandlers map[uint16]func(*Remote,*frame)`;
  each handler decodes a frame and mutates device or bay state. Subsystem
  parsing lives in `links.go`, `multiviewer.go`, `audio.go`, `amp.go`,
  `network.go`, `stats.go`, `svd.go`.
- **TX** — `control.go` (frame builders and public control methods).
  Multiviewer and audio also have builders in their own files.

### Wire frame format

`[0x50,0x38, protocol(u16 LE), uid(16), opcode(u16 LE), length(u16 LE), payload]`

`0x02` SYS_BAY_CONFIG, `0x03` SYS_LINKS and `0x23` SYS_BAY_CONFIG_SECONDARY are
paged across several frames whose record counts vary. Handlers merge records
into the cache; never replace a cached list from one frame.

`0x3C` V2IP_DEVICE_CFG carries every field behind its own validity marker, since
a sender zeroes the payload and fills in only what it is writing.
`DeviceV2IPDetails.merge` folds a frame onto the cached config field by field.

### Receive flow

`receiveLoop` → `Remote.processFrame` → `parseFrame` → look up the handler by
opcode → run under `Remote.mu`, mutating state and queueing callbacks → fire
callbacks after the lock is released. Frames whose sender UID equals our own are
skipped. Any frame from a known device refreshes its liveness (`Device.touch`).

`processFrame` decodes a frame whatever protocol version its header stamps.
Receive has **no version ceiling and must not grow one**: a ceiling silently
drops whole opcodes the moment a device ships a newer stamp, and takes the
fields that already worked with it. `TestReceiveHasNoProtocolCeiling` pins this.

### Transmit

`buildFrame` is the only frame constructor and every frame reaches the wire
through `Remote.transmit`, which takes the target as a parameter — so a send
that skips the protocol gate is a compile error rather than an omission.

Frames are stamped `protocolFor(opcode)`, the per-opcode minimum from
`opcodeProtocol`, never `ProtocolVersion`: a device silently drops a frame
stamped above its own version, with no NAK at any layer, so the call would
otherwise succeed while nothing happened. `Device.requireOpcodeLocked` refuses
an opcode whose floor exceeds what the target advertised; a device that has
reported no version is allowed through, since not knowing is not the same as
knowing it is too old.

Some `opcodeProtocol` entries deliberately sit below the floor the device
firmware lists for the same opcode. Read the comment beside an entry before
reconciling it with anything.

`ProtocolVersion` is what this library announces in its hello frame, and the
stamp for an opcode with no floor of its own. Raising it claims every layout up
to that version is understood, so raise it together with the decoding.

### Concurrency

A single `Remote.mu` guards the whole object graph. Mutations run under the lock
and queue callback closures via `Remote.emit`; `runLocked` fires them after
unlocking, so no callback ever runs under the lock. Every specific callback fans
into the generic `OnBayUpdate`/`OnDeviceUpdate` (see `Bay.notify` and
`Device.notify`). Public getters lock; `*Locked` helpers assume the lock is
held.

## Working on the protocol

The wire format must stay byte-for-byte compatible. Layouts are pinned by tests
rather than by this file: `wire_test.go` holds byte-exact TX vectors generated
from the Python client, and RX offsets are pinned end-to-end by feeding frames
through `processFrame` in `state_test.go` and `subsystems_test.go`.

- **Never guess a layout by summing field widths.** Settle it against a
  captured frame, a reference vector, or a sibling client.
- **Unknown values are unknown, never clamped.** An enum value with no name
  here is passed through as it arrived, and an unhandled opcode is ignored.
  Zero is usually a valid value, so a confidently wrong reading is worse than an
  unrecognised one. Keep the conversion late — hold the raw value and return the
  typed one from the accessor, so an unrecognised value survives to the caller.
- **A payload that grew tells its versions apart by length, not by the header
  stamp.** Parse the prefix you understand and ignore the tail. Length gates
  are minimums; where several forms share an opcode, test them longest first,
  or a grown short form is swallowed by the longer form's minimum. An exact
  length is right only where a field widening superseded a layout, shifting
  every following offset - and that is selected by the frame's protocol
  version, as `handleNetworkStatus` does. A block appended at the back needs
  the stamp too, as a floor read with the length rather than instead of it:
  the length says the bytes are there, the stamp says they are that block
  rather than a later growth, and `handleV2IPStats` reads both. A floor is
  never a ceiling on the frame - the prefix is read whatever the sender
  stamps.
- **No payload length may panic a handler.** The slicing that reads a fixed
  block is bounded by a gate written elsewhere in the handler, so a gate
  moved or narrowed indexes past the payload and takes the receive goroutine
  with it. `TestNoPayloadLengthPanicsAHandler` sweeps every handler for this.
  A trailing variable array is the one shape no gate protects: bytes appended
  past its last element read as another element, so growing one is a wire
  break its sender has to announce.
- **Build test payloads with `poisoned()`, not `make([]byte, n)`.** A
  zero-filled fixture cannot catch a field read at the right offset but the
  wrong width. Give every field a distinct value, and assert each against the
  value its own offset carries rather than against another field's read.
- **Do not name specific device firmware build numbers** in code, comments,
  tests or documentation.

Three decoders deliberately disagree with the Python client, which reads these
at offsets its own frame builder contradicts. Do not reconcile them:

- `0x09` MX_SET_ROUTE — bay port ids are `uint16`, so `no_power_on` is at 20.
- `0x0A` RC_IR — the tick timestamp aligns to 4, after the port's padding.
- `0x43` V2IP_AUDIO SELECT_INPUT — the sink is at 20 and the source at 36.

Adding an opcode:

1. Add the constant to `constants.go`.
2. Add an RX handler and register it in `dispatch.go`'s `frameHandlers`.
3. Add a TX builder and public method in `control.go` (or the subsystem file).
4. Add a byte-exact TX test and/or an RX integration test.

## Conventions

- Minimal comments: doc-comment the exported API; avoid line-by-line narration
  inside function bodies unless a byte offset or subtlety isn't self-evident.
- Every `.go` file starts with the two-line author/copyright header followed by
  a blank line, so it isn't taken as a package doc comment.

## Not ported (intentionally)

The HTTP device API (`get_api`/`get_log`, `send_key` for arbitrary CEC keys, and
non-V2IP matrix routing via `port/set`) — no protocol-native equivalent exists,
and a consumer can call the device HTTP API directly. Also the `mxr` CLI and PDU
control: the PDU *state* frame `0x16` is decoded, but nothing transmits PDU
commands.

Several opcodes are deliberately left undecoded because nothing transmits them.
`frameHandlers` is the current list of what is decoded.
