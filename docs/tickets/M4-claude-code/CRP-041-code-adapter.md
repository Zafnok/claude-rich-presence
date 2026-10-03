---
id: CRP-041
title: Claude Code adapter
milestone: M4 Claude Code
type: feature
status: todo
priority: P0
blocked_by: [CRP-001, CRP-010, CRP-012, CRP-040]
blocks: [CRP-033]
model: claude-sonnet-5-5
effort: high
size: M
---

# CRP-041: Claude Code adapter

## Goal

The translation layer between Claude Code and the core: two MCP tools, one that turns hook events into presence events and one that reports status. This is the privacy boundary of the whole system.

## Context

Hooks of type `mcp_tool` call `presence_event` once per hook event ([ADR-0007](../../architecture/adr/0007-integration-and-distribution.md)). Claude waits for each call. Everything [ADR-0008](../../architecture/adr/0008-privacy-and-safety-by-default.md) says about never impairing Claude and minimising data applies here first.

[CRP-001](../../research/crp-001-claude-code-adapter.md) recorded the exact fields each event delivers and several behaviours this ticket must handle. Read its findings before starting. The fallback was not adopted.

## Scope

Package `internal/adapter/code`:

**`presence_event` tool**

- Input: an event name and the allowlisted fields for that event, as recorded by CRP-001. Nothing else is declared in the schema, and anything else present is ignored.
- Maps each hook event to a presence event using the [event table](../../architecture/README.md#where-the-events-come-from).
- Maps tool names to tool kinds with a fixed table. The raw tool name is discarded.

  | Tool names | Kind |
  |---|---|
  | Edit, Write, NotebookEdit and similar | Editing |
  | Bash, PowerShell | Running commands |
  | Read | Reading |
  | Grep, Glob | Searching |
  | WebFetch, WebSearch, browser tools | Browsing |
  | Agent, Task | Delegating |
  | Names beginning `mcp__` | Using tools |
  | Anything else | The generic kind |

- Maps a model id to a short family label, such as the family name and version. An unrecognised id gives no label.
- Applies the privacy level before publishing:
  - `minimal`: publish only that the session exists and its timestamps.
  - `standard`: add status, tool kind and model family.
  - `full`: add the project name, which is the last element of the working directory.
- Publishes through a small publish interface that this package defines. CRP-033 connects it to the host node. This ticket does not need the host, and tests against a fake.
- **Always returns the same success result: one text item whose text is `{}`**, for valid input, invalid input and internal errors alike. The value is a constant in the code. An empty result makes Claude Code add a line to the model's context on every prompt, and any other JSON can steer the session (ADR-0007, rule 4).
- Treats an empty string as an absent field. Claude Code substitutes an empty string for a field the event does not carry.
- Counts a `SubagentStop` only if it saw the matching `SubagentStart`. Claude Code emits stops with an empty `agent_type` and no start, after compaction and after ordinary turns. The count never goes below zero.
- Ignores a call that carries `_meta` with `claudecode/toolUseId`, which marks a call made by the model and not by a hook, and still returns the constant. This is defence in depth on undocumented behaviour, not a guarantee.

**Session identity**

- The session opens when MCP `initialize` completes, before any hook has fired, under a provisional id.
- The first event that carries a session id binds it. Later events with a different id, as after `/clear`, rebind. `CLAUDE_CODE_SESSION_ID` in the adapter's environment is not used: it is fixed at start, stale after `/clear`, and wrong under `--continue`.
- Events from inside a subagent count toward the same session.
- The session ends when the server's input closes, or on an interrupt or termination signal, which is what Linux sends at a normal exit. There is no `SessionEnd` event to rely on.

**`presence_status` tool**

- Returns a diagnostic summary as short text. Read-only. The summary arrives through a second small interface defined here, which CRP-033 connects to the host.

**Descriptions**

- Tool descriptions tell the model plainly that `presence_event` is called by hooks and should not be called otherwise.

## Out of scope

- The MCP server, the host node, and their wiring.
- The plugin's hook file, which is CRP-042.
- The Claude Desktop adapter, which is CRP-050.

## Acceptance criteria

- [ ] Every row of the event table has a test from tool input to the published presence event.
- [ ] The tool-kind table and the model-label mapping are covered row by row, including unknown inputs.
- [ ] **Leak test**: every field that could carry user content is seeded with a marker string. At every privacy level, the marker never appears in any published event. At `minimal` and `standard`, no part of the working directory appears either.
- [ ] At `full`, only the last path element of the working directory is published, for both slash styles and for a trailing separator.
- [ ] Unknown event names, missing fields, wrong types and oversized values all produce the constant success result and publish nothing.
- [ ] The result is byte-identical for every input, shown by a test over all the cases above.
- [ ] An empty string in any field is handled as the field being absent.
- [ ] A `SubagentStop` with no matching start leaves the count unchanged.
- [ ] With the publish interface stalled, the tool handler still returns immediately.
- [ ] The provisional id is replaced by the real one without creating a second session, and a changed id rebinds without leaving the old session behind.
- [ ] `presence_status` output contains no project name and no path.
- [ ] The handler performs no I/O. Shown by construction: it has only the publish interface and pure functions.

## Notes for the implementer

- Put the allowlist in one table keyed by event, and generate the tool's input schema from it, so the schema and the parser cannot drift.
- The same table is the source for the plugin's hook file in CRP-042. Consider a test in that ticket that checks the two agree.
- Keep the model-label mapping tolerant: model ids change often. Derive the label from the id's structure where possible and fall back to no label.

## Why this model and effort

Not algorithmically hard, but it is the privacy boundary and sits on Claude's critical path, so every edge matters.

## References

- [ADR-0007](../../architecture/adr/0007-integration-and-distribution.md), [ADR-0008](../../architecture/adr/0008-privacy-and-safety-by-default.md)
- [Architecture: where the events come from](../../architecture/README.md#where-the-events-come-from)
- Findings of CRP-001, in `docs/research/`
- Hooks reference: https://code.claude.com/docs/en/hooks
