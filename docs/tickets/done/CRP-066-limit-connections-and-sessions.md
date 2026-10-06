---
id: CRP-066
title: Limit the connections and sessions a host holds
milestone: M6 Release
type: feature
status: done
priority: P2
blocked_by: [CRP-062]
blocks: []
model: claude-sonnet-5-5
effort: high
size: S
---

# CRP-066: Limit the connections and sessions a host holds

## Goal

A peer on the control socket cannot make the host hold an unbounded number of connections or sessions. Today nothing counts either.

## Context

Finding F2 of the [threat model](../../architecture/threat-model.md), severity low. Only a process of the same user can reach the socket, so this is about a buggy or confused peer more than an attacker.

In `internal/host/term.go`, `accept` starts a goroutine for every connection with no ceiling. In `internal/host/registry.go`, an `event` whose session id is new creates a session owned by that connection, so one connection can create as many as it sends ids. The number of sessions is shown on Discord at the `standard` level.

A real follower holds one connection and one session: `host.Node` replaces its session when the id changes.

## Scope

- A constant ceiling on open follower connections. A connection over it is closed at once, before its hello is read, and counted.
- A connection holds one session. An event or a sync for a second session id replaces the first, as a sync already does, so a connection can never own more than one.
- Both are constants in `internal/host`, with the reasoning for the numbers in a comment.
- The row for F2 in the threat model: move it from the findings to the table of section 2, with the tests.

## Out of scope

- A time limit on a welcomed connection that is silent. A follower is silent for as long as its session is.
- Rate limiting events. The scheduler already bounds what reaches Discord.

## Acceptance criteria

- [x] A test opens connections up to the ceiling, each welcomed, and shows the next one closed without a welcome while the others keep working.
- [x] A test shows that closing one connection lets a new one in.
- [x] A test sends events for two session ids on one connection and shows the host holding one session, the later.
- [x] `TestRandomStartsPublishesAndStops` and `TestABurstOfNodesElectsOneHost` still pass with the ceiling in place.
- [x] The threat model names these tests against the threat "a hostile follower using unlimited connections or sessions".

## Notes for the implementer

- Choose the ceiling well above any real number of sessions; several hundred costs nothing. The end-to-end tests start a handful.
- A session moving between ids on one connection must not leave the old id in `owners`.
- The protocol document, `docs/protocol/control.md`, says what a host does with each message. Update it for the one-session rule.

## Why this model and effort

Small, but in the host's concurrent connection handling, where the tests are the specification.

## References

- [ADR-0006](../../architecture/adr/0006-control-channel.md), "Trust"
- [Threat model](../../architecture/threat-model.md), section 2 and finding F2
