---
id: CRP-055
title: Pause and preview from Claude Desktop
milestone: M5 Claude Desktop
type: feature
status: todo
priority: P2
blocked_by: [CRP-073]
blocks: []
model: claude-sonnet-5-5
effort: medium
size: S
---

# CRP-055: Pause and preview from Claude Desktop

## Goal

A user of the Claude Desktop app can pause and resume their Discord presence and see what the card says, as a user of Claude Code can.

## Context

CRP-073 built pausing and the private preview. The host holds the pause for every session, whatever surface it came from, so a pause made in Claude Code already hides a Claude Desktop session. What is missing is a way in from Claude Desktop itself: its adapter offers the status tool only, with no `preview` argument and no pause tool.

The pieces exist and are not specific to Claude Code:

- `host.Node` has `Pause`, `Resume` and `Preview`, which return at once from memory.
- The Claude Code adapter's pause tool and preview text are in `internal/adapter/code/pause.go` and `internal/adapter/code/status.go`.

Claude Desktop runs two copies of the server ([CRP-002 findings](../../research/crp-002-desktop-extension.md)). The copy the app initializes is a node. The second copy is not: it never takes the lock and asks the host over a connection of its own.

The decisions are in [ADR-0012](../../architecture/adr/0012-project-profiles-and-repository-link.md), section "Hiding and pausing".

## Scope

- The pause tool, `presence_pause`, in the Claude Desktop adapter, with the same arguments, answers and limits as in the Claude Code adapter.
- The `preview` argument on the Claude Desktop adapter's status tool, with the same text and the same label as private.
- One implementation of each, shared by the two adapters. Move it to a package both may import; do not copy it.
- The diagnostic status of the Claude Desktop adapter gains the `Paused` line the Claude Code adapter has.
- The second copy of the server:
  - decide whether it offers the pause tool at all. If it does, it sends the pause over its own connection without becoming a node;
  - its preview asks the host as its status does.
- The extension's manifest and its documentation name the new tool.

## Out of scope

- Hiding the Claude Desktop session, which the global privacy level `off` already does.
- Any change to the control protocol. The messages CRP-073 added are enough.
- Skills. Claude Desktop Chat has none; the user asks in plain words.

## Acceptance criteria

- [ ] In a session initialized as Claude Desktop, `presence_pause` clears the activity for every session and `resume` restores it. Tested with the fake Discord.
- [ ] The same session's status tool with `preview` set lists every slot of the card and matches what the fake Discord received.
- [ ] The pause tool and the preview text exist once in the source, and both adapters use them.
- [ ] The second copy either does not list the pause tool, or pauses through the host without taking the lock. A test shows which, and that it holds no session either way.
- [ ] No tool call waits on I/O: a test with a host that never answers shows each tool returning.
- [ ] The preview is not logged.

## Notes for the implementer

- A tool call may not wait for the host ([ADR-0008](../../architecture/adr/0008-privacy-and-safety-by-default.md)). The second copy's status already answers from what the host said last and asks again on another goroutine. Do the same for its preview.
- The client names that tell the two copies apart were observed and are not documented. Re-check them as the `claude-surfaces` skill describes before relying on them.
- If the second copy is to pause, it must not offer a stale pause to a later host, because it is not a node and keeps no state between questions.

## Why this model and effort

The behaviour exists and is tested in one adapter; this moves it and adds a second caller, with one decision about the second copy.

## References

- [ADR-0012](../../architecture/adr/0012-project-profiles-and-repository-link.md), [ADR-0008](../../architecture/adr/0008-privacy-and-safety-by-default.md)
- [Pausing in the control protocol](../../protocol/control.md#pausing)
- [CRP-002 findings](../../research/crp-002-desktop-extension.md)
