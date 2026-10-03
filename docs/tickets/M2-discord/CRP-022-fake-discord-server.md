---
id: CRP-022
title: Fake Discord IPC server for tests
milestone: M2 Discord
type: feature
status: todo
priority: P0
blocked_by: [CRP-004, CRP-005]
blocks: [CRP-021, CRP-023, CRP-043]
model: claude-sonnet-5-5
effort: high
size: M
---

# CRP-022: Fake Discord IPC server for tests

## Goal

A scriptable stand-in for the Discord client's IPC endpoint that runs on all three operating systems, so transport, session and end-to-end tests never need a real Discord.

## Context

No automated test may talk to a real Discord client. Every behaviour we need from Discord, including its failures, must be reproducible on a CI runner.

This is also our second implementation of the framing. It must be written from Discord's documentation and must not import the codec from CRP-020, so that the two check each other.

## Scope

Package `internal/testutil/fakediscord`:

- Listens on a Unix socket on Linux and macOS, and on a named pipe on Windows, at a unique name chosen by the test.
- Accepts connections, reads frames, and by default behaves like Discord: answers a valid handshake with a ready event, and acknowledges each set-activity command with the same nonce.
- Records what it received, exposed to the test: handshakes, activities set, clears, in order, with timestamps from a time function the test passes in. It does not need the fake clock package from CRP-013.
- Scriptable deviations:
  - reject the handshake with a given error code;
  - close the connection after N frames, or immediately;
  - stop responding without closing;
  - delay responses;
  - send a ping;
  - send a close frame with a code;
  - send a malformed frame.
- A helper that returns the environment or name prefix a dialer needs to find it.
- Clean shutdown that fails the test if a goroutine is left behind.

## Out of scope

- Emulating anything beyond the handshake and set-activity.
- Being part of the production binary. This is test code.

## Acceptance criteria

- [ ] The server runs on Linux, macOS and Windows in CI.
- [ ] It does not import `internal/discord/codec`.
- [ ] Each scripted deviation has a self-test using a raw connection.
- [ ] Two servers can run in the same test process at different indices without interfering.
- [ ] Nothing it creates is left on disk or in the pipe namespace after shutdown.
- [ ] Any dependency it needs for the Windows pipe server is `golang.org/x/sys` only, used in test code only, and listed in the allowlist from CRP-007 if that ticket has landed.
- [ ] It is classified as test code for coverage and for SonarQube, as [ADR-0009](../../architecture/adr/0009-quality-gates.md) describes.

## Notes for the implementer

- The standard library has no named-pipe server on Windows. Creating one needs a direct system call, available through `golang.org/x/sys/windows`, which the dependency policy pre-approves. Keep that code confined to one file.
- Use message-independent byte-stream pipe mode so partial reads behave as they do against Discord.
- Named pipes share one namespace per machine. Include a random component in the name so parallel test processes do not collide.

## Why this model and effort

A test harness that other tickets trust completely, with a platform-specific server and many scripted failure modes.

## References

- [ADR-0004](../../architecture/adr/0004-in-house-protocol-implementations.md), on independent implementations
- [Quality strategy](../../architecture/quality-strategy.md)
- Open reimplementation of the Discord IPC server, useful for expected behaviour: https://github.com/OpenAsar/arrpc
