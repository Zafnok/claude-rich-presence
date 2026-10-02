---
id: CRP-023
title: Discord session manager
milestone: M2 Discord
type: feature
status: todo
priority: P0
blocked_by: [CRP-013, CRP-020, CRP-021, CRP-022]
blocks: [CRP-032]
model: claude-opus-5-5
effort: high
size: M
---

# CRP-023: Discord session manager

## Goal

A component the host can hand a desired activity to at any time, which keeps Discord showing it for as long as Discord is reachable, and recovers by itself when Discord is not.

## Context

Discord may not be running when Claude starts, may be started later, may restart, and may reject us. The user should never have to do anything, and the host should never block on any of it.

## Scope

Package `internal/discord/session`:

- **States**: disconnected, connecting, ready, stopped.
- **Connecting**: dial through the transport, send the handshake, wait for the ready event with a deadline.
- **Ready**: send whatever the scheduler emits; answer ping with pong; match acknowledgements by nonce and log error responses.
- **Reconnecting**: on any read or write error, on end of stream, or on a close frame, drop the connection and retry with exponential backoff and jitter, from 1 second to a cap of 60 seconds. On becoming ready again, reset the scheduler so the current activity is resent.
- **Permanent errors**: an invalid application id is not retried at speed. Back off to the cap and log one actionable message.
- **Stopping**: on context cancellation, send a clear if ready, with a short deadline, then close.
- **Interface to the host**: a non-blocking "set desired activity" call, and a read-only status for diagnostics: state, last error class, time of last successful update.
- The process id sent with each activity is this process's own.
- Clock, jitter source and transport are injected.

## Out of scope

- Deciding what to show or how often, which are CRP-011 and CRP-013.
- Supporting more than one Discord application at a time.

## Acceptance criteria

Each proven against the fake server from CRP-022, with the fake clock:

- [ ] With Discord absent at start, the manager connects and shows the current activity once the server appears, with no caller involvement.
- [ ] When the server closes the connection, the manager reconnects and resends the current activity.
- [ ] When the server stops responding during the handshake, the attempt times out and is retried.
- [ ] Backoff delays grow to the cap and reset after a successful ready.
- [ ] A handshake rejected for an invalid application id results in retries at the cap and exactly one log line at warning level until the state changes.
- [ ] A ping is answered with a pong.
- [ ] "Set desired activity" returns immediately in every state, including while a write is blocked.
- [ ] On stop while ready, the server records a clear before the connection closes. On stop while disconnected, stop returns promptly.
- [ ] After stop, no goroutines remain, verified in a test.
- [ ] No activity is sent faster than the scheduler allows, including across a reconnect.
- [ ] All tests pass under the race detector.

## Notes for the implementer

- One goroutine owns the connection. A reader goroutine feeds it frames. Nothing else touches the stream.
- Think through shutdown while a dial is in progress, while the handshake is pending, and while a write is blocked. Each needs a test.
- Do not log activity contents. Log state changes and error classes.
- Whether Discord clears presence when the process named in the activity exits, while the socket stays open, is unverified. We sidestep it by always naming our own process.

## Why this model and effort

Connection lifecycle with reconnection, timeouts and shutdown interleavings. This is where presence tools usually hang or leak.

## References

- [Architecture: talking to Discord](../../architecture/README.md#talking-to-discord)
- [Architecture: failure behaviour](../../architecture/README.md#failure-behaviour)
- [ADR-0008](../../architecture/adr/0008-privacy-and-safety-by-default.md)
