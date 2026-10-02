# ADR-0006: Unix domain sockets and newline-delimited JSON between adapters and host

## Status

Accepted. The socket location on Windows is confirmed or revised by [CRP-002](../../tickets/M0-foundation/CRP-002-spike-desktop-extension.md).

## Context

Followers need a local, per-user, bidirectional, connection-oriented channel to the host ([ADR-0005](0005-presence-host-election.md)). A dropped connection must be detectable, because it is the liveness signal. The channel must be available from the Go standard library on all three operating systems ([ADR-0002](0002-implementation-language.md)).

## Decision

### Transport

A Unix domain stream socket, on every operating system. Windows has supported them since Windows 10 version 1803, and the Go standard library exposes them there.

| Operating system | Directory, in order of preference |
|---|---|
| Linux | `$XDG_RUNTIME_DIR/rich-presence`, else a per-user directory under the temp directory |
| macOS | A per-user directory under `$TMPDIR` |
| Windows | `%LOCALAPPDATA%\rich-presence`, else a per-user directory under `%TEMP%` |

Rules:

1. The directory is created with owner-only permissions, and the host refuses to use a directory it does not own.
2. The socket path must fit the platform limit, which is about 104 bytes on macOS and 108 elsewhere. If the preferred path is too long, use a short hashed name under the temp directory. The resolver is one function with one test table.
3. The lock file sits beside the socket.
4. `RICH_PRESENCE_RUNTIME_DIR` overrides the directory, for tests and for unusual setups.

### Protocol

1. One JSON object per line, UTF-8, at most 64 KiB per line. A longer line closes the connection.
2. The first message from a follower is `hello`, carrying the protocol version and the binary version. The host answers `welcome` or `refuse`.
3. A `sync` message carries the follower's full current session state. It is sent after `welcome` and is idempotent.
4. An `event` message carries one presence event.
5. `stand_down` asks an older host to give up the lock.
6. `status` asks the host for a summary, for the `status` command.
7. Unknown message types and unknown fields are ignored, so newer followers can talk to older hosts. A change that old hosts cannot ignore increments the protocol version.

The message catalogue and field definitions are specified in [CRP-030](../../tickets/M3-host/CRP-030-control-protocol.md) and live in `docs/protocol/control.md` once written.

### Trust

The channel is reachable only by the same operating-system user, by directory permission. Messages from it are still treated as untrusted input: size-limited, validated, and never able to make the host do anything other than update presence.

## Consequences

- One transport implementation for all platforms.
- Line-delimited JSON is debuggable with ordinary tools and fuzzable with the standard library.
- A path-length ceiling we must handle explicitly.
- On Windows, a packaged app may see a redirected `%LOCALAPPDATA%`. If the spike shows that, the Windows location moves to a path that is not redirected, or we fall back to the alternative below.

## Alternatives considered

| Alternative | Why not |
|---|---|
| Windows named pipe for the control channel | No server-side support in the standard library; would need `golang.org/x/sys` calls or `go-winio`. Kept as the fallback if socket files prove unusable under packaged apps |
| Loopback TCP | Reachable by other local users, needs an authentication token and port discovery |
| A shared state file | No liveness signal, races between writers, and polling |
| A binary framing | No benefit at this message rate, and harder to inspect |
