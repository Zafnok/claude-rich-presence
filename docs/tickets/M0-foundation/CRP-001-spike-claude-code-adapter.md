---
id: CRP-001
title: "Spike: Claude Code adapter wiring"
milestone: M0 Foundation
type: spike
status: todo
priority: P0
blocked_by: []
blocks: [CRP-041, CRP-042, CRP-044, CRP-046, CRP-051]
model: claude-opus-5-5
effort: high
size: M
---

# CRP-001: Spike: Claude Code adapter wiring

## Goal

Decide, with evidence from real Claude Code sessions, whether [ADR-0007](../../architecture/adr/0007-integration-and-distribution.md) is accepted as written or the fallback is adopted. This is the largest open risk in the plan (R1, R2).

## Context

The preferred wiring delivers a native binary to Claude Code as an MCPB bundle referenced from a plugin manifest, and delivers events through hooks of type `mcp_tool`. Each piece is documented. Nobody we found has shipped the combination.

This is a spike. Prototype code is thrown away and never merged. The deliverable is a findings document and a decision.

## Scope

Build a throwaway prototype: a Go program that speaks just enough MCP over standard streams to answer `initialize`, `tools/list` and `tools/call`, and that appends everything it observes to a local file. Package it as an MCPB bundle of server type `binary`. Write a throwaway plugin that references the bundle and declares `mcp_tool` hooks for every event in the [event table](../../architecture/README.md#where-the-events-come-from).

Then answer the ADR's criteria:

| # | Question |
|---|---|
| A1 | Does Claude Code install the plugin, fetch the bundle from an HTTPS URL, and start the right binary with no shell and no runtime? Does a local bundle path work for development? |
| A2 | Do `mcp_tool` hooks fire for every listed event except launch-time `SessionStart`? Do the `${...}` substitutions arrive intact? Record the exact fields available for each event |
| A3 | What happens when a substituted field is absent, for example `${agent_id}` outside a subagent? |
| A4 | Does a tool result, empty or not, ever reach Claude's context? Test `UserPromptSubmit` and a re-fired `SessionStart` specifically |
| A5 | What latency does a hooked event add? What happens to Claude when the server hangs, and when it has exited? |
| A6 | When is the server process started and stopped? Test normal exit, `/clear`, `--resume`, and killing Claude Code |
| A7 | After changing the bundle URL and version in the plugin, does an update replace the binary? |
| A8 | How large is the tool definition in context, and does the model ever call the tool unprompted? |

Also record, because later tickets need them:

- the `clientInfo` name and version Claude Code sends in `initialize`, per surface;
- the server's working directory, parent process, and environment, in particular `CLAUDE_PLUGIN_ROOT`, `CLAUDE_PLUGIN_DATA`, `CLAUDE_CODE_REMOTE`, and how `userConfig` values arrive;
- whether subagents share the session's server or start their own;
- whether the same results hold in the terminal, the Claude Desktop Code tab, and the VS Code extension;
- whether `claude plugin validate --strict` accepts the plugin as named.

## Out of scope

- Any Discord connection. The prototype only logs.
- Claude Desktop Chat, which is CRP-002.
- Production code of any kind.

## Acceptance criteria

- [ ] `docs/research/crp-001-claude-code-adapter.md` exists and answers A1 to A8, each with what was run, on which operating system and Claude Code version, and what was observed.
- [ ] Results cover Windows and at least one of macOS or Linux. Any surface or platform that could not be tested is listed as untested, not assumed.
- [ ] The per-event field table is recorded, and the event table in the architecture overview is corrected if it was wrong.
- [ ] ADR-0007's status is changed to Accepted, or to Superseded with a new ADR adopting the fallback. The decision follows the rule in the ADR: A1, A2, A4, A5 or A6 failing without a workaround means fallback.
- [ ] CRP-044 is set to `not-needed` or to `todo` accordingly, and CRP-041, CRP-042 and CRP-051 are amended if the findings change their scope.
- [ ] No prototype code is merged.

## Notes for the implementer

- Go must be installed first. If CRP-004 has not landed, install it for the spike only.
- A bundle can be hosted for the test as an asset on a pre-release in a personal fork.
- To measure A5, timestamp inside the prototype and compare against a session with the plugin disabled. Use Claude Code's debug log for hook timings.
- To test A4, have the tool return a distinctive string and then ask Claude whether it has seen it.
- Running inside the owner's real Claude Code is required. Ask the owner to perform the steps that need their machine or accounts, and record exactly what they ran.
- Time box: one working day. If the answers are not in by then, stop and report what is known.

## Why this model and effort

The work is investigation and judgment across undocumented behaviour, and a wrong conclusion misdirects a whole milestone.

## References

- [ADR-0007](../../architecture/adr/0007-integration-and-distribution.md)
- [Viability: known gaps](../../architecture/viability.md#known-gaps-in-the-preferred-wiring)
- Hooks reference: https://code.claude.com/docs/en/hooks
- Plugin manifest reference: https://code.claude.com/docs/en/plugins-reference
- MCPB manifest specification: https://github.com/modelcontextprotocol/mcpb/blob/main/MANIFEST.md
