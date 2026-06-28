# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

A Go client library for Pulse-Eight MatrixOS devices (neo matrices, OneIP/V2IP
units, ProAmp8 amplifiers) over UDP multicast/broadcast: discovery, A/V routing,
volume, remote-control passthrough, V2IP streaming, multiviewer, audio
endpoints, amplifier settings, and diagnostics.

- Module: `github.com/opdenkamp/mx-remote-golang`; package `mxremote`. The import
  path basename differs from the package name, so import it aliased:
  `import mxremote "github.com/opdenkamp/mx-remote-golang"`.
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
Built by `buildFrame`.

### Concurrency
A single `Remote.mu` guards the whole object graph. Mutations run under the lock
and queue callback closures via `Remote.emit`; `runLocked` fires them after
unlocking (no callbacks under the lock → no re-entrancy). Every specific callback
fans into the generic `OnBayUpdate`/`OnDeviceUpdate` (see `Bay.notify` /
`Device.notify`), mirroring the reference `MxrCallbacks` base methods. Public
getters lock; `*Locked` helpers assume the lock is held.

## Working on the protocol

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

## Not ported (intentionally)

The HTTP device API (`get_api`/`get_log`, `send_key` for arbitrary CEC keys, and
non-V2IP matrix routing via `port/set`) — no protocol-native equivalent exists; a
consumer can call the device HTTP API directly. Also the `mxr` CLI and PDU
control (deprecated). Everything else the Python factory dispatches is handled.
