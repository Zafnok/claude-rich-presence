---
id: CRP-045
title: "Spike: model name at session launch"
milestone: M4 Claude Code
type: spike
status: todo
priority: P2
blocked_by: [CRP-042]
blocks: []
model: claude-sonnet-5-5
effort: medium
size: S
---

# CRP-045: Spike: model name at session launch

## Goal

Find a documented way to know which model a Claude Code session is using from its first moment, or conclude that there is none and close the question.

## Context

Under the preferred wiring, the model name arrives only in `SessionStart`, and `mcp_tool` hooks are skipped for that event at launch (R9). So presence shows no model until the user switches model, clears, or the session compacts. That is cosmetic, and the first release ships with it.

If CRP-001 adopted the fallback wiring, command hooks receive `SessionStart` at launch and this ticket is `not-needed`.

## Scope

Evaluate, in this order, and stop at the first that works within the project's rules:

| Option | Question |
|---|---|
| A newer documented source | Has Claude Code added the model to another hook event, to the MCP `initialize` request, or to the server's environment since the assessment? |
| One command hook for `SessionStart` | Can a single exec-form command hook run the bundled binary? This needs a path to the binary that a hook can name, which the bundle cache may not provide |
| An `http` hook for `SessionStart` | Would require the host to listen on a loopback port. Weigh against [ADR-0006](../../architecture/adr/0006-control-channel.md)'s rejection of loopback TCP |
| Do nothing | Is the current behaviour acceptable as a permanent limitation? |

## Out of scope

- Reading settings files, transcripts or session files to find the model. [ADR-0008](../../architecture/adr/0008-privacy-and-safety-by-default.md) rules those out.

## Acceptance criteria

- [ ] `docs/research/crp-045-initial-model.md` records each option, what was tried, and the result.
- [ ] Either a follow-up ticket exists for the chosen option, written with the `write-ticket` skill, or the limitation is recorded as accepted in [risks.md](../../architecture/risks.md) and R9 is closed.
- [ ] No production code is merged by this ticket.

## Notes for the implementer

- Time box: half a day.
- Start by re-reading the hooks reference. This area changes often and the simplest outcome is that the answer now exists.

## Why this model and effort

A bounded investigation with a low cost of error.

## References

- [Risk register](../../architecture/risks.md), R9
- [Viability: known gaps](../../architecture/viability.md#known-gaps-in-the-preferred-wiring)
- Hooks reference: https://code.claude.com/docs/en/hooks
