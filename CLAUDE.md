# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

A Go client library for Pulse-Eight MatrixOS devices (neo matrices, OneIP/V2IP
units, ProAmp8 amplifiers) over UDP multicast/broadcast: discovery, A/V routing,
volume, remote-control passthrough, V2IP streaming, multiviewer, audio
endpoints, amplifier settings, and diagnostics.

- Module: `github.com/opdenkamp/mx-remote-golang/v2`; package `mxremote`. The
  import path basename differs from the package name, so import it aliased:
  `import mxremote "github.com/opdenkamp/mx-remote-golang/v2"`. The `/v2` suffix
  is required by Go modules at major version 2 and above; without it a `go get
  -u` from a v1 consumer would silently pull breaking changes.
- Single flat package (no subpackages). Public API is exported; all wire/frame
  machinery is unexported.
- The reference implementation is the Python library at `../mx-remote`. The Go
  API is idiomatic and does NOT mirror the Python class layout — but the **wire
  protocol must stay byte-for-byte compatible** (it talks to embedded devices).

## Commands

```bash
go build ./...                 # build
go test ./...                  # run tests
go vet ./...                   # vet
gofmt -l .                     # list unformatted files (should be empty)
go run ./examples/discover     # discover devices on the network (-b broadcast, -l <ip>)
```

Cross-platform (Linux, macOS, Windows). `conn.go` creates the UDP socket via the
`net` package and applies multicast/reuse/broadcast options through
`RawConn.Control`; the only per-OS code is the fd-type cast in `conn_unix.go`
(`!windows`) and `conn_windows.go`. Check other targets with
`GOOS=windows go build ./...` and `GOOS=darwin go build ./...`.

## Architecture

One package, grouped by concern:

- **Wire layer** — `frame.go` (24-byte `P8` header pack/unpack + payload
  accessors), `uid.go` (`DeviceUID`/`BayUID`), `constants.go` (opcodes, network
  defaults), `enums.go`, `types.go` (data structs), `bayconfig.go`.
- **Runtime** — `remote.go` (`Remote`: UDP lifecycle, registry, discovery,
  dispatch), `conn.go` (sockets), `device.go`, `bay.go`, `callbacks.go`.
- **RX** — `dispatch.go` holds `frameHandlers map[uint16]func(*Remote,*frame)`;
  each handler decodes a frame and mutates device/bay state. Subsystem RX +
  parsing live in `links.go`, `multiviewer.go`, `audio.go`, `amp.go`,
  `network.go`, `stats.go`, `svd.go`.
- **TX** — `control.go` (frame builders + public control methods). Multiviewer
  and audio also have TX builders in their own files.

### Receive flow
`receiveLoop` → `Remote.processFrame` → `parseFrame` → look up handler by opcode
→ run under `Remote.mu`, mutating state and queueing callbacks → fire callbacks
after the lock is released. Frames whose sender UID equals our own are skipped.
Any frame from a known device refreshes its liveness (`Device.touch`).

### Wire frame format
`[0x50,0x38, protocol(u16 LE), uid(16), opcode(u16 LE), length(u16 LE), payload]`.
Built by `buildFrame`, which is handed `protocolFor(opcode)` — the per-opcode
minimum from `opcodeProtocol`, not `ProtocolVersion`. A receiver drops any frame
stamped above its own protocol version, so stamping the library's own version
would make every device with a lower cap ignore us.

`0x02` SYS_BAY_CONFIG, `0x03` SYS_LINKS and `0x23` SYS_BAY_CONFIG_SECONDARY are
paged across several frames whose record counts vary. Handlers merge records
into the cache; never replace a cached list from one frame.

`0x3C` V2IP_DEVICE_CFG carries every field behind its own validity marker, since
a sender zeroes the payload and fills in only what it is writing.
`DeviceV2IPDetails.merge` folds a frame onto the cached config field by field.

### Concurrency
A single `Remote.mu` guards the whole object graph. Mutations run under the lock
and queue callback closures via `Remote.emit`; `runLocked` fires them after
unlocking (no callbacks under the lock → no re-entrancy). Every specific callback
fans into the generic `OnBayUpdate`/`OnDeviceUpdate` (see `Bay.notify` /
`Device.notify`), mirroring the reference `MxrCallbacks` base methods. Public
getters lock; `*Locked` helpers assume the lock is held.

## Transmitting

Check the target's reported protocol version before sending, not just the
stamp. `Device.requireOpcodeLocked` refuses an opcode whose floor exceeds what
the device advertised: a receiver drops such a frame silently, with no NAK at
any layer, so the call would otherwise succeed while nothing happened. A
ProAmp8 on 4.1.1 reports `0x11`, below the floor of several opcodes here, so
this is not hypothetical. A device that has reported no version is allowed
through — not knowing is not the same as knowing it is too old.

`0x08` MX_ROUTE is decoded but no MatrixOS build transmits it:
`mxr_bay_broadcast_routes()` has a declaration and a definition and no callers.
The decoder still has to be right for third-party controllers, but do not
expect one on a live mesh, and do not treat its absence as a bug.

## Working on the protocol

**`PACKED` is the exception in this protocol, not the rule.**
`mx_remote_proto.h` mixes `struct PACKED`, `struct ALIGN(8)` and plain `struct`
freely, and only the `PACKED` ones can be decoded by summing field widths. On
the others the compiler inserts padding, so derive offsets from the declaration
and check `sizeof` — three separate decode bugs here came from summing widths
across a plain struct. Two recurring traps:

- `TMTicks` is `uint_fast32_t`, so it aligns to 4 and pads whatever precedes it.
- Where the firmware appends a variable-length tail at `sizeof(struct)`, the
  tail starts after the struct's *own* trailing padding, not at the end of its
  last field (`0x48` RC_IR_TX: timings at 36, not 34).

**Ask where a wire value first becomes a typed thing, before asking where it
is decoded.** Those are rarely the same file, and the narrowing boundary is
where unknown values get lost — so auditing decoders alone systematically
misses it. Here the raw value is kept and the conversion happens late (a bay
holds its EDID profile as an `int` and returns `EdidProfile` only from the
accessor), which is what makes an unrecognised value survive to the caller.
Keep it that way round.

**Every layer that asserts something can encode the same mistake — including
the one added to catch it.** This has bitten in the decoder, in a fixture
written to pin the decoder, and in a hand-check written to validate the fixture.
Treat a red test as evidence that the test and the code disagree, not that the
code is wrong, and check which one moved.

**A symptom too cheap for the defect means something is compensating.** When a
decoder disagrees with its struct, that tells you a field is misnamed — not yet
that behaviour is broken. Check the consumers before moving offsets: two errors
in the same direction cancel, and there the fix is to move the decoder and every
consumer together, since correcting the offsets alone introduces the bug. A
four-field reversal that shows up only as a backwards log line is the tell.

**Never widen a field to swallow its padding, and never assume a padding or
reserved byte is zero.** Cortex-M builds with `-fshort-enums`, so a plain enum
on the wire is often one byte followed by padding — and firmware `memcpy`s
uncleared stack locals over payloads, so that padding carries live junk that
differs frame to frame. The offsets still line up either way, which is why this
survives a cross-check between two implementations: only the field itself is
wrong, and it reads as a value that changes while the setting does not. Mask to
the field's real width rather than asserting the neighbouring bytes are zero
(`rc_target_t` at `0x45`+16 is one byte, not four).

**Ask what an instrument cannot say before believing what it does.** Three
checks here find three different things, and each is silent about the others:

| check | finds | blind to |
|---|---|---|
| offset-mutation sweep | fields no test exercises | wrong width, wrong branch |
| `poisoned()` fixtures | fields read at the wrong width | fields no test reaches |
| `go test -cover` on handlers | opcodes nothing runs at all | anything a test runs but never asserts |

A fixture hides a shift whenever the neighbouring bytes carry the same value:
adjacent fields set to the same number, a short NUL-terminated string in a wider
field, or a port whose high byte is zero. Give every field in a fixture a
distinct value, and assert that the bays a frame did *not* address were left
alone - two decode bugs here (`0x39`'s port and `0x08`'s source bays) survived
because the assertion passed on state an earlier frame had set.

A handler with no test reports clean from the first two for the same reason an
empty file does. Every productive finding here came from asking what a tool was
silent about, not from reading its output more carefully.

**Build test payloads with `poisoned()`, not `make([]byte, n)`.** A zero-filled
fixture cannot catch a field read at the right offset but the wrong width: the
padding beside it is zero, so the widened read returns the same answer and every
assertion still passes. Poisoning fills the payload with a non-zero pattern
first, so any read straying past a field's real width produces a wrong value.

That is the class an offset-mutation sweep structurally misses — mutating slice
bounds tests whether a *shifted* read is caught, and a wrong-width read is not
shifted. Mutation finds fields no test exercises; poison finds fields exercised
wrongly, and only that second kind survives a green suite indefinitely. A real
captured frame does both and additionally fixes the *expected* value from
outside the code under test, which poison cannot.

Poison is the default for padding, not a blanket substitution. Bytes the sender
leaves undefined and bytes it defines as zero look identical in a fixture and
mean opposite things: the video wall revert test feeds a genuinely zero window
on purpose, and poisoning it would test something else.

**Unknown values are unknown, never clamped.** The protocol is meant to stay
backwards compatible in both directions, so a driver must not break over a
firmware update. An enum value this library has no name for is passed through
as it arrived and an unhandled opcode is ignored — never folded to the nearest
known value, because zero is usually a valid one and a confidently wrong
reading is worse than an unrecognised one.

Auditing for this by searching for masks does not work: extracting a bit field
and folding a range look identical (`UtpLinkSpeed(d[3] & 0x7)` is a genuine
3-bit field; the duplex flag is bit 3). The mask is almost always right — look
at what the extracted value is then converted into.

**Do not mirror a firmware receiver without asking whether its handling is
defensible.** Firmware predating the fix builds `0x3C` from an uninitialised
`mxr_scaling_config` and ORs flags onto stack garbage, so on a receiver-capable
unit bits 2..6 of the scaling flags are noise and `MODE_VALID` can be set
spuriously (leaving `mode` and `refresh` behind it uninitialised). The
firmware's own receiver copies the whole top nibble; this library carries bit 7
alone, because caching noise as though it meant something is worse than
matching the reference.

Byte layouts must match the firmware/Python exactly. To verify a TX frame,
generate a reference vector from the Python library (stub an `mxr` with
`uid_raw`/`name`, call the `Frame*.construct`, print `.frame.hex()`) and assert
it in `wire_test.go`. RX offsets are validated end-to-end in `state_test.go` and
`subsystems_test.go` by feeding synthetic frames through `processFrame`.

Adding a new opcode:
1. Add the constant to `constants.go`.
2. Add an RX handler and register it in `dispatch.go`'s `frameHandlers`.
3. Add a TX builder + public method in `control.go` (or the subsystem file).
4. Add a byte-exact TX test and/or an RX integration test.

## Conventions

- Minimal comments: doc-comment the exported API; avoid line-by-line narration
  inside function bodies unless a byte offset or subtlety isn't self-evident.
- Every `.go` file starts with the two-line author/copyright header followed by a
  blank line (so it isn't taken as a package doc comment).

## Coverage

The dispatch table decodes every opcode the firmware still uses. To re-check
after a firmware bump, diff `frameHandlers` against `MXR_OPCODE()` in
`mx_opcodes.h`, and establish "still in use" from the firmware source: an
opcode is live if it has an `mxr_register_opcode()` call or a transmit site.
There are **two** transmit paths — `mxr_pbuf_alloc()` and `mxr_tx_bytes()` —
and grepping only the first wrongly reports the bare command opcodes as dead.

Deliberately left undecoded, because nothing in the firmware references them
outside the opcode table itself: `0x06` DEV_SIGNAL_OLD, `0x2D`
VIDEO_CLOCK_RATE_OLD, `0x36` SET_MASTER, `0x47` DEBUG, and `0x17`–`0x1E`, the
eight CEC opcodes that were specified and never implemented. The reference
Python library still decodes some of these for older firmware.

`0x2E`/`0x2F` V2IP_BLIST_* are decoded but `V2IP_SUPPORT_BLACKLIST` is `0` in
shipping builds, so current firmware emits neither.

Opcode-level coverage does not settle the **sub-opcodes**: `0x43` V2IP_AUDIO
multiplexes six commands on a u16 at payload offset 0, and `0x42`
V2IP_MULTIVIEWER sixteen on the byte at offset 16. Check those switches
separately — a dispatch entry says nothing about which sub-commands under it
are decoded. A cheap way to find one: scan for exported struct fields never
populated anywhere in the package. That is what surfaced the audio gap.

`0x42`'s parameters are deliberately exposed as raw bytes past its envelope
(target uid, sub-opcode at 16, seven pad, params from 24). The multiviewer
module owns the opcode, so there is no firmware source here to pin
per-sub-command field semantics against.

Three decoders deliberately disagree with the reference Python library, which
reads these at offsets its own C struct or frame builder contradicts:

- `0x09` MX_SET_ROUTE — `mbay_port_id` is a `uint16`, so the bays are two bytes
  each and `no_power_on` is at 20. Python reads bytes at 16/17, which puts the
  sink bay's high byte in the source bay.
- `0x0A` RC_IR — `mxr_ir_data` is not packed, so its `TMTicks` timestamp aligns
  to 4 and the port's two bytes are followed by padding. Python reads the
  timestamp at 2.
- `0x43` V2IP_AUDIO SELECT_INPUT — the sink is named twice (the command
  header's target, then again at 20) and the source is at 36. Python's decoder
  reads 20 as the source and 36 as the target, the reverse of what its own
  frame builder writes and of what `Device.AudioSelectInput` sends here.

## Not ported (intentionally)

The HTTP device API (`get_api`/`get_log`, `send_key` for arbitrary CEC keys, and
non-V2IP matrix routing via `port/set`) — no protocol-native equivalent exists; a
consumer can call the device HTTP API directly. Also the `mxr` CLI and PDU
control (deprecated) — the PDU *state* frame `0x16` is decoded, but nothing
transmits PDU commands.

`0x49` V2IP_VIDEOWALL is owned by the loadable v2ipwall module rather than
MatrixOS, so its layout comes from `vw_mesh_frame` in that module rather than
from this wire definition. Unlike `0x3C` it **replaces** rather than merges: no
field carries a validity marker, and a zero width or height means "clear the
wall", not "unset". A revert carries no window at all — see
`VideoWallCommand.HasWindow`. `0x40` V2IP_TILING is not a substitute: on a sink
running v2ipwall a `0x40` write is transient, because the module's reconciler
pushes its own window back within about a second.
