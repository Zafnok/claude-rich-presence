---
id: CRP-050
title: Claude Desktop adapter
milestone: M5 Claude Desktop
type: feature
status: todo
priority: P1
blocked_by: [CRP-002, CRP-033]
blocks: [CRP-052, CRP-053]
model: claude-sonnet-5-5
effort: medium
size: S
---

# CRP-050: Claude Desktop adapter

## Goal

When the binary is started by Claude Desktop, it reports that Claude Desktop is open, and nothing more.

## Context

Claude Desktop Chat gives an extension no signal about conversations. The only reliable fact is that the server process is alive, which, if CRP-002 confirms it, means the app is open. See the [viability assessment](../../architecture/viability.md#what-claude-desktop-chat-offers-instead).

Read CRP-002's findings first. They fix the client name to recognise and how a doubled adapter in the Code tab is handled.

## Scope

Package `internal/adapter/desktop`, and its wiring in `internal/cli`:

- Recognise Claude Desktop from the client name in MCP `initialize`, using the value CRP-002 recorded. An unrecognised client continues to be treated as Claude Code.
- On `initialize`, publish *session opened* for a session of surface `desktop` with a constant id for this adapter instance and the current time as start.
- On input closing, publish *session ended*.
- Expose `presence_status` only. `presence_event` is not listed for this client.
- Honour `enabled` and the privacy level. At every level the Desktop session publishes the same thing, since there is nothing private to withhold; the level is still carried so rendering is consistent.
- Add scenario E13 to the end-to-end tests.

## Out of scope

- Any attempt to detect chat activity, model, or conversation.
- A separate Discord application for Desktop.

## Acceptance criteria

- [ ] With the Claude Desktop client name, `tools/list` returns only `presence_status`.
- [ ] With the Claude Code client name, behaviour is unchanged.
- [ ] A Desktop session and a working Code session together render the Code session in focus, with a count of two.
- [ ] A Desktop session alone renders the Desktop phrase with an elapsed timer.
- [ ] Input closing removes the session and, if it was the last, clears presence.
- [ ] If CRP-002 found that Desktop extensions also attach to Code-tab sessions, the behaviour it prescribed for the duplicate is implemented and tested.
- [ ] E13 passes on all three operating systems.

## Notes for the implementer

- Match the client name exactly as recorded, and keep the recognised names in one table so adding another host later is a one-line change.
- The idle-clear rule applies to a Desktop session too: it is `Idle` from the start, so an app left open all day stops showing after the configured period. Confirm with the owner that this is the wanted behaviour and record the answer in the pull request. If the owner wants Desktop to show for as long as it is open, give the Desktop session a status that the idle-clear rule ignores.

## Why this model and effort

Small and well bounded once the spike has answered its questions.

## References

- [ADR-0007](../../architecture/adr/0007-integration-and-distribution.md), [ADR-0008](../../architecture/adr/0008-privacy-and-safety-by-default.md)
- Findings of CRP-002, in `docs/research/`
