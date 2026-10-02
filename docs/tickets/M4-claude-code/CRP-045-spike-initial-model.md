---
id: CRP-045
title: "Spike: status line bridge and the model at launch"
milestone: M4 Claude Code
type: spike
status: todo
priority: P2
blocked_by: [CRP-042]
blocks: [CRP-074]
model: claude-sonnet-5-5
effort: medium
size: M
---

# CRP-045: Spike: status line bridge and the model at launch

## Goal

Establish whether Claude Code's status line can feed our program the model, context, cost and usage limits, on which surfaces, and how to set it up without breaking a user's own status line. Settles [ADR-0015](../../architecture/adr/0015-status-line-bridge.md) and the missing-model gap recorded as R9.

## Context

Under the preferred wiring, the model name arrives only in `SessionStart`, which `mcp_tool` hooks do not receive at launch. So presence shows no model until the user switches model, clears, or the session compacts.

The status line is a documented source for the model from the first moment, and also for effort, context used, session cost, and the 5-hour and weekly usage percentages. It has constraints: a plugin cannot set it, there is one per user, it runs through a shell, and whether it runs outside the terminal is not documented.

If CRP-001 adopted the fallback wiring, command hooks receive `SessionStart` at launch and the model question is already answered. The rest of this spike still applies.

This is a spike. Prototype code is thrown away.

## Scope

With a throwaway command that logs what it receives:

| # | Question |
|---|---|
| L1 | Does the status line command run in the terminal, in the Claude Desktop Code tab, and in the VS Code and JetBrains extensions? When, and how often? |
| L2 | Which of the documented fields are actually present on each surface, and from what point in a session? In particular the model, the usage percentages, and the session name |
| L3 | How can the bundled binary be given a path that a status line command can name, and that survives plugin updates? Evaluate the adapter keeping a copy in the plugin's data directory |
| L4 | What command string works under Git Bash, and under PowerShell when Git Bash is absent, including with spaces in the path? |
| L5 | Can an existing status line be wrapped so its output is unchanged? Test with a script, an inline command, and a multi-line status line |
| L6 | What is the added delay, and what does the user see if our command is slow or fails? |
| L7 | Does a status line set in user settings conflict with one in project settings? Which wins? |
| L8 | With the bridge in place, is the model known before the first prompt? |

Also record, without building on it: what automatically generated session names look like, as examples with anything sensitive redacted, in case an opt-in fallback for the summary is wanted later.

If the bridge is unworkable, evaluate these for the model alone, in this order, and stop at the first that works within the project's rules:

| Option | Question |
|---|---|
| A newer documented source | Has Claude Code added the model to another hook event, to the MCP `initialize` request, or to the server's environment? |
| Do nothing | Is the current behaviour acceptable as a permanent limitation? |

## Out of scope

- Reading settings files, transcripts, session files or credentials to find any of these values.
- Calling any Anthropic endpoint.

## Acceptance criteria

- [ ] `docs/research/crp-045-status-line-bridge.md` answers L1 to L8 per surface and operating system, with what was run and what was observed.
- [ ] ADR-0015 is marked Accepted, with the stable-path mechanism and the command forms recorded, or Rejected with the evidence.
- [ ] CRP-074 is amended to match, or set to `not-needed`.
- [ ] R9 in [risks.md](../../architecture/risks.md) is closed or restated with what remains.
- [ ] No production code is merged by this ticket.

## Notes for the implementer

- Start by re-reading the status line reference. Field names and triggers change between releases.
- L1 decides most of the value. The owner works mainly in the Claude Desktop Code tab; if the status line does not run there, say so prominently.
- These steps need the owner's machine and settings. Back up the settings file before any experiment, and restore it afterwards.
- Time box: one working day.

## Why this model and effort

A bounded investigation with clear questions.

## References

- [ADR-0015](../../architecture/adr/0015-status-line-bridge.md)
- [Risk register](../../architecture/risks.md), R9
- [Viability: known gaps](../../architecture/viability.md#known-gaps-in-the-preferred-wiring)
- Status line reference: https://code.claude.com/docs/en/statusline
- Hooks reference: https://code.claude.com/docs/en/hooks
