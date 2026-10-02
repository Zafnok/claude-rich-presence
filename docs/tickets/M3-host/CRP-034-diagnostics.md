---
id: CRP-034
title: Logging and diagnostics
milestone: M3 Host
type: feature
status: todo
priority: P1
blocked_by: [CRP-012]
blocks: [CRP-033]
model: claude-sonnet-5-5
effort: medium
size: S
---

# CRP-034: Logging and diagnostics

## Goal

When presence does not appear, a user can find out why in one step, and nothing they are asked to paste into a bug report reveals what they were working on.

## Context

The program runs unseen and its standard output is a protocol stream, so it cannot print. [ADR-0008](../../architecture/adr/0008-privacy-and-safety-by-default.md) forbids logging event payloads.

## Scope

Package `internal/diag`:

- **Logging** with the standard library's structured logger, to a file in the log directory from CRP-012. One file, capped in size, with one rotated predecessor. Level from configuration, default warning.
- **A logging interface for the rest of the code** that accepts only a message and typed attributes from a fixed set: state names, error classes, counts, durations, versions. There is no way to log an arbitrary string field or an event.
- **Counters** for things worth knowing without logging each one: events received, events dropped, reconnects, failovers.
- **Doctor checks**, each returning pass, warn or fail with a one-line explanation and a suggested action:

  | Check | Passes when |
  |---|---|
  | Configuration | It loads without warnings |
  | Runtime directory | It exists, is owned by the user, and the socket path fits |
  | Discord endpoint | A candidate pipe or socket exists |
  | Discord handshake | A handshake with the configured application id gets a ready event |
  | Host | A host answers `status` |
  | Version | Host and this binary report the same version |

- **Report formatting** that replaces the user's home directory with `~` in every path.

## Out of scope

- The `doctor` command wiring, which is CRP-033.
- Remote log collection. There is none.

## Acceptance criteria

- [ ] The logging interface makes it a compile error to log a domain event or a free-form string attribute.
- [ ] A test seeds events with marker strings in every forbidden field and asserts the marker never appears in the log file.
- [ ] The log file never exceeds its cap plus one message, and rotation keeps exactly one predecessor.
- [ ] A log directory that cannot be created or written disables logging and does not stop the program.
- [ ] Each doctor check is tested for pass, warn and fail through injected ports.
- [ ] The doctor report contains no absolute path that includes a user name, and no project name.
- [ ] The Discord handshake check closes its connection without setting an activity.

## Notes for the implementer

- The handshake check opens a second Discord connection briefly. That is fine; it sets nothing.
- Writing log files goes through a file-system interface so failures can be driven in tests.

## Why this model and effort

Conventional, with one design constraint that needs care: an interface that cannot leak.

## References

- [ADR-0008](../../architecture/adr/0008-privacy-and-safety-by-default.md)
- [Architecture: failure behaviour](../../architecture/README.md#failure-behaviour)
