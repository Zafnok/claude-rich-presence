---
id: CRP-021
title: "Discord transport: pipe and socket dialers"
milestone: M2 Discord
type: feature
status: todo
priority: P0
blocked_by: [CRP-004, CRP-022]
blocks: [CRP-023]
model: claude-opus-5-5
effort: high
size: M
---

# CRP-021: Discord transport: pipe and socket dialers

## Goal

Find and open the connection to the running Discord client on each operating system, returning a stream that supports deadlines and concurrent reading and writing.

## Context

On Windows the endpoint is a named pipe. Elsewhere it is a Unix socket whose directory depends on environment variables and on how Discord was installed. Discord may be listening on any of ten indices, and several Discord builds can run at once.

We want this on the standard library alone. Go 1.26 can open a named pipe for overlapped I/O, which is what makes deadlines and simultaneous read and write work on Windows. That claim comes from the release notes and has not been exercised by us (R12). This ticket exercises it.

## Scope

Package `internal/discord/transport`:

- A platform-neutral interface: given a context, return an open connection or an error that says Discord is not running.
- A pure function that lists candidate endpoints in order, given the operating system and environment:
  - Windows: `\\?\pipe\discord-ipc-N` for N from 0 to 9.
  - Linux and macOS: `discord-ipc-N` under the first set of `XDG_RUNTIME_DIR`, `TMPDIR`, `TMP`, `TEMP`, then `/tmp`; and on Linux also the Flatpak and Snap subdirectories for the stable and Canary builds.
- A Unix dialer and a Windows dialer, each in its own platform file, that try candidates in order and return the first that connects.
- The returned connection honours read and write deadlines and allows one goroutine to block in a read while another writes.
- The endpoint name prefix is injectable, so tests can use a unique name and never touch a real Discord.

## Out of scope

- The handshake, which is CRP-023.
- Choosing among several running Discord clients. First found wins.

## Acceptance criteria

- [ ] The candidate list is tested for all three operating systems on every operating system, including each environment variable fallback and the Flatpak and Snap paths.
- [ ] On each operating system, an integration test connects to the fake server from CRP-022 at a non-zero index, exchanges bytes both ways, and closes.
- [ ] With no server listening, the dialer returns the "not running" error within a bounded time.
- [ ] A read with a deadline returns a timeout error when nothing arrives. Proven on Windows as well as Unix.
- [ ] A write completes while another goroutine is blocked in a read on the same connection. Proven on Windows as well as Unix.
- [ ] Closing the connection unblocks a pending read.
- [ ] Cancelling the context aborts an in-progress dial.
- [ ] No dependency is added. If the standard library cannot satisfy the two Windows criteria above, stop, and record the evidence in a note superseding the relevant part of [ADR-0004](../../architecture/adr/0004-in-house-protocol-implementations.md) before adding the pre-cleared Microsoft package.
- [ ] Platform files contain only the system-specific open call and its error mapping. All selection logic is in the platform-neutral file.

## Notes for the implementer

- Read the Go 1.25 and 1.26 release notes on asynchronous file handles on Windows before writing the Windows dialer, and confirm the behaviour with a small test first.
- A pipe that exists but is busy returns a distinct Windows error. Treat it as "try the next index", and record the decision.
- Keep each platform file small enough that 100% coverage on its own operating system is natural.
- The Flatpak and Snap paths come from another project's source, not from Discord's documentation. Cite that in a comment.

## Why this model and effort

Cross-platform I/O with a specific unverified platform claim at its centre, and failure modes that only show under concurrency.

## References

- [ADR-0002](../../architecture/adr/0002-implementation-language.md), [ADR-0004](../../architecture/adr/0004-in-house-protocol-implementations.md)
- [Risk register](../../architecture/risks.md), R12
- Go release notes: https://go.dev/doc/go1.25 and https://go.dev/doc/go1.26
- Discord RPC documentation: https://docs.discord.com/developers/topics/rpc
