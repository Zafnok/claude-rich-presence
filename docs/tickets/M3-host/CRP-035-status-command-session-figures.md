---
id: CRP-035
title: Show the privacy level and event counts in the status command
milestone: M3 Host
type: feature
status: todo
priority: P3
blocked_by: [CRP-033]
blocks: []
model: claude-sonnet-5-5
effort: medium
size: S
---

# CRP-035: Show the privacy level and event counts in the status command

## Goal

`rich-presence status` prints the figures the `presence_status` tool prints that today only a session's own adapter knows: its privacy level and how many events it ignored and dropped. Then the command and the tool answer the same questions.

## Context

[CRP-033](../done/CRP-033-cli.md) made `status` print what the host's `status_result` carries: the Discord connection, the number of sessions, the host's version and its uptime, under the same words the tool uses. The tool also reports `Privacy`, `Events ignored` and `Events dropped`. They belong to one adapter's session, and the host does not hold them: a follower adapter's counters never leave its process, and a separate `status` process has none of its own.

Whether the host should hold them is a decision this ticket leaves open. The choices are to carry per-session figures in `sync` and `event` and report their total in `status_result`, to report only the host process's own adapter, or to decide that the command should not show them. Record the choice in an ADR if it changes the control protocol, with the `record-decision` skill.

## Scope

- Decide which figures the command can honestly show, and what a figure means when several sessions are open.
- If the control protocol changes, change [the protocol document](../../protocol/control.md) and its golden files with it, and raise the protocol version only if an older binary would misread the new message.
- Print them in `status`, in the words of the tool.

## Out of scope

- The tool's own output, which does not change.
- Anything that would carry prompt text, paths or project names to the host: the figures are numbers and one of three level words.

## Acceptance criteria

- [ ] `status` prints a privacy level and the two counts, or the ticket records why it does not, and the criterion of CRP-033 that asks for the same fields is amended to match.
- [ ] A host and a follower running different versions of this binary still answer each other's `status` without error.
- [ ] No figure is a string taken from an event.

## Why this model and effort

A small, well-bounded change once the open choice is made; the choice itself is recorded, not guessed.

## References

- [CRP-033](../done/CRP-033-cli.md), [the control protocol](../../protocol/control.md), [ADR-0006](../../architecture/adr/0006-control-channel.md), [ADR-0008](../../architecture/adr/0008-privacy-and-safety-by-default.md)
