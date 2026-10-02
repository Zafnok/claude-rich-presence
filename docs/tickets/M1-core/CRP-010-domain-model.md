---
id: CRP-010
title: "Domain model: events, sessions, registry"
milestone: M1 Core
type: feature
status: todo
priority: P0
blocked_by: [CRP-004]
blocks: [CRP-011, CRP-030, CRP-032, CRP-041]
model: claude-sonnet-5-5
effort: high
size: M
---

# CRP-010: Domain model: events, sessions, registry

## Goal

The pure heart of the system: the presence event type, the per-session state machine, and the registry that reduces events into the set of open sessions. Everything else is built around these types.

## Context

Specified in the architecture overview under [Session state](../../architecture/README.md#session-state). The registry is rebuilt from followers after a failover, so events must be upserts and their order after a reconnect is not guaranteed.

## Scope

Package `internal/domain`:

- **Event**: a session id, a surface, a timestamp, a kind, and the optional fields that kind carries. Kinds: session opened, session refreshed, turn started, tool started, tool finished, attention needed, idle, turn finished, compaction started, compaction finished, model changed, subagent started, subagent stopped, session ended.
- **Session**: id, surface, status, tool kind, model family if known, project name if present, privacy level, start time, last activity time, subagent count.
- **Status**: idle, working, waiting, compacting.
- **Tool kind**: a closed vocabulary. Editing, running commands, reading, searching, browsing, delegating, using tools, and a generic value.
- **Registry**: applies an event and returns the new state; removes a session; replaces a session wholesale from a sync; returns an immutable snapshot.
- Validation of each event: required fields per kind, length limits on every string.

## Out of scope

- Turning hook payloads into events, which is CRP-041.
- Choosing what to display, which is CRP-011.
- Any I/O, any clock, any logging. Time arrives in the event.

## Acceptance criteria

- [ ] Every transition in the state diagram is covered by a table-driven test, including each transition that must not change state.
- [ ] Any event for an unknown session id creates that session with sensible defaults, so the registry can be rebuilt from any event.
- [ ] Applying a sync for a session twice gives the same state as applying it once.
- [ ] Subagent count never goes below zero.
- [ ] A snapshot cannot be used to mutate the registry.
- [ ] An invalid event is rejected with an error and leaves the registry unchanged.
- [ ] The package imports only the standard library, and nothing from `os`, `net`, `time.Now` or `log`.
- [ ] A property test over random event sequences shows the registry never panics and every session stays in a valid status.

## Notes for the implementer

- Model the transitions as data, a table from status and event kind to status, rather than nested conditionals. It makes the tests and the diagram match one to one.
- The registry is used from a single goroutine in the host. Do not add locking; document the assumption.
- Session identity for Claude Desktop is a constant id per adapter instance.
- Keep the model family as a short label decided by the adapter. The domain does not interpret model ids.

## Why this model and effort

Small, but the types here shape every other package, and the upsert and ordering rules are easy to get subtly wrong.

## References

- [Architecture: session state](../../architecture/README.md#session-state)
- [ADR-0001](../../architecture/adr/0001-core-architecture.md), [ADR-0005](../../architecture/adr/0005-presence-host-election.md)
