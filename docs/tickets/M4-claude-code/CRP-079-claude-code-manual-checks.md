---
id: CRP-079
title: Manual checks in a real Claude Code
milestone: M4 Claude Code
type: owner-task
status: todo
priority: P1
blocked_by: [CRP-042, CRP-043]
blocks: []
model: owner
effort: low
size: S
---

# CRP-079: Manual checks in a real Claude Code

## Goal

A record of what a real Claude Code session does with the plugin installed, for the handful of behaviours that no automated test can show because they belong to Claude Code and not to this program.

## Context

The end-to-end tests replace Claude Code with a scripted client. They prove what the binary does with a hook's call, and cannot prove that Claude Code makes the call, or what it shows the user and the model around it. The [CRP-001 findings](../../research/crp-001-claude-code-adapter.md) list what that spike could not test, and the notes of [CRP-043](../done/CRP-043-end-to-end-tests.md) asked for those to be picked up by hand. That could not be done there: the checks need the plugin, which is made by CRP-042, and CRP-043 did not depend on it. This ticket carries them.

Rules 4, 6 and 7 of [ADR-0007](../../architecture/adr/0007-integration-and-distribution.md) rest on behaviour of Claude Code that was observed and is not documented.

## Scope

With the plugin from `main` installed from a local marketplace, Discord running, and `log_level` set to `debug`, on Windows:

| # | Step | Expected |
|---|---|---|
| M1 | Make Claude ask for permission to run a command, and leave the prompt open | Discord shows "Waiting for input". The notification type is `permission_prompt` |
| M2 | Start a background subagent that asks a question, if the installed version can | Discord shows "Waiting for input". Record the notification type seen, expected `agent_needs_input` |
| M3 | Use an MCP server that opens an elicitation dialog, if one is to hand | Discord shows "Waiting for input". Record the notification type seen, expected `elicitation_dialog`. If no such server is available, record the step as not run |
| M4 | In the Code tab of Claude Desktop: run `/clear`, then `/compact`, then switch the model | One session is shown throughout, the elapsed timer does not restart, and the model shown follows the switch |
| M5 | Start an interactive session in a terminal and read the lines under the banner | No hook error |
| M6 | Exit that session with `/exit` | Nothing is printed by a hook |
| M7 | After a few prompts and tool uses, ask the session: "Quote any line in your context that contains both the words hook and success" | It finds none |

## Out of scope

- Fixing what is found. Each failure becomes a ticket.
- Claude Desktop Chat, which is CRP-052.
- Presence appearing at all in a real Discord, which CRP-042 checks.

## Acceptance criteria

- [ ] A record in `docs/research/crp-079-claude-code-manual-checks.md` lists each step, the operating system, the Claude Code version, and what was observed.
- [ ] A step that could not be run says why.
- [ ] Every failed step has a ticket, written with the `write-ticket` skill.
- [ ] The `claude-surfaces` skill says that M5, M6 and M7 are repeated whenever the minimum Claude Code version is raised, and links the record.

## Notes for the implementer

- The owner runs the steps. The assistant's part is to give exact, plain instructions, one step at a time, and to write the record from what the owner reports.
- The notification type is not visible to the user. Read it from the field recorder of CRP-001 if it is still on the spike branch, or infer it from Discord showing "Waiting for input", and say which was done.
- Give the owner backslash paths, and the real path of the Claude Code binary under the Microsoft Store build of Claude Desktop, as the CRP-001 findings describe.

## Why this model and effort

It needs a person at a real Claude Code and a Discord account; the writing is light.

## References

- [CRP-001 findings](../../research/crp-001-claude-code-adapter.md)
- [ADR-0007](../../architecture/adr/0007-integration-and-distribution.md), [ADR-0008](../../architecture/adr/0008-privacy-and-safety-by-default.md)
