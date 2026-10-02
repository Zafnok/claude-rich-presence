---
id: CRP-030
title: Control protocol
milestone: M3 Host
type: feature
status: todo
priority: P0
blocked_by: [CRP-010]
blocks: [CRP-032]
model: claude-sonnet-5-5
effort: medium
size: S
---

# CRP-030: Control protocol

## Goal

The message format adapters and the host use to talk to each other, specified in a document and implemented as a pure codec.

## Context

[ADR-0006](../../architecture/adr/0006-control-channel.md) fixes the shape: one JSON object per line, a version in the first message, unknown content ignored. Different versions of the binary will run side by side during an upgrade, so compatibility rules matter from the first release.

## Scope

- `docs/protocol/control.md`: every message, every field, who sends it, when, and the compatibility rules.
- Package `internal/control/protocol`:

  | Message | Direction | Carries |
  |---|---|---|
  | `hello` | Follower to host | Protocol version, binary version |
  | `welcome` | Host to follower | Protocol version, binary version |
  | `refuse` | Host to follower | A reason code |
  | `sync` | Follower to host | The follower's complete current session, or none |
  | `event` | Follower to host | One presence event |
  | `stand_down` | Follower to host | Nothing |
  | `status` | Either to host | Nothing; the host answers with `status_result` |
  | `status_result` | Host to requester | Discord connection state, session count, host binary version, uptime |

- An encoder and a line-oriented decoder with a 64 KiB limit per line.
- Version negotiation: the host accepts any follower whose protocol version it understands; a follower treats a `refuse` as "retry the election later".
- Mapping between protocol messages and the domain types from CRP-010.

## Out of scope

- Sockets, which are CRP-031.
- What the host does with messages, which is CRP-032.

## Acceptance criteria

- [ ] `docs/protocol/control.md` exists and matches the code, with an example of each message.
- [ ] Every message round-trips through encode and decode.
- [ ] An unknown message type decodes to an "unknown" value, not an error.
- [ ] Unknown fields are ignored.
- [ ] A line over the limit, invalid JSON, and a message missing a required field each return a distinct error and never panic.
- [ ] No message type can carry prompt text, tool input, a file path or any free-form field not in the domain event. Shown by the type definitions and a test.
- [ ] A fuzz test on the decoder runs clean.
- [ ] `status_result` contains nothing unsafe to paste in a public issue: no project names, no paths.

## Notes for the implementer

- Keep protocol structs separate from domain structs, with explicit conversion, so a domain refactor cannot silently change the wire format.
- Add golden files for each message so a wire change shows up in review.

## Why this model and effort

A small, clear specification, with compatibility rules that need care.

## References

- [ADR-0006](../../architecture/adr/0006-control-channel.md)
- [ADR-0005](../../architecture/adr/0005-presence-host-election.md)
