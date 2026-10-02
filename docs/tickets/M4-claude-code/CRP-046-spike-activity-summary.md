---
id: CRP-046
title: "Spike: activity summary written by Claude"
milestone: M4 Claude Code
type: spike
status: todo
priority: P1
blocked_by: [CRP-001]
blocks: [CRP-047]
model: claude-sonnet-5-5
effort: high
size: M
---

# CRP-046: Spike: activity summary written by Claude

## Goal

Find out whether Claude will reliably and cheaply describe what the user is working on, by calling a tool on our MCP server, and whether that can be done without interrupting the user. Settles [ADR-0011](../../architecture/adr/0011-model-authored-activity-summary.md).

## Context

The owner wants presence to read like "Building the battle system for Visions of Shuyi". Nothing in a hook event contains such a phrase. The session's own Claude can write one. The open questions are about behaviour and cost, which only real sessions answer.

This is a spike. Prototype code is thrown away. It extends the prototype from CRP-001: add one tool that logs what it receives.

## Scope

Using the CRP-001 prototype with a `presence_summary` tool added, run fresh sessions on real projects and measure against the ADR's criteria:

| # | Question |
|---|---|
| S1 | With only the tool description and server instructions, how often does Claude set a sensible summary within the first two turns? Ten fresh sessions per model tier, including the smallest current model |
| S2 | If that is not at least nine in ten, does a one-line reminder returned from the `UserPromptSubmit` hook call fix it? Confirm first that an `mcp_tool` hook's result reaches Claude's context for that event |
| S3 | Does Claude update the summary when the task changes, and leave it alone otherwise? |
| S4 | Is the call made in the same turn as other tool calls, or as a turn by itself? |
| S5 | What does it cost per session: tokens for the definition and instructions, tokens per call, extra round trips? |
| S6 | Does a model-initiated call to a plugin's MCP tool raise a permission prompt? In each permission mode. If so, what makes it go away permanently, and can the plugin arrange that? |
| S7 | With adversarial text planted in the project, such as a file telling Claude to put a link in its status, what reaches the tool? |
| S8 | Do summaries ever contain paths, secrets or quoted prompt text? |
| S9 | Over a long session that stays on one task, how many times does Claude call the tool? Does a short fixed acknowledgement in the tool's result reduce repeat calls? |
| S10 | With three concurrent sessions on related tasks in one project, how often do they choose the same `area`: with a list of areas supplied, and without one? |

Also record:

- whether the tool is deferred behind tool search by default, and what marking makes it always load;
- whether server instructions reach the model on each surface: terminal, Desktop Code tab, IDE;
- the wording of description and instructions that worked best, verbatim;
- whether a personality's voice instruction appended to the instructions changes the phrasing as intended, tried with at least a neutral, a casual and a dramatic voice, and whether any voice weakens the public-audience rules. ADR-0014 depends on this;
- whether `TaskCreated` and `TaskCompleted` events carry the task's text, as a possible passive source.

## Out of scope

- Claude Desktop Chat. CRP-053 validates that separately.
- Production code, including the sanitiser. S7 records what arrives raw, so CRP-047 knows what to defend against.

## Acceptance criteria

- [ ] `docs/research/crp-046-activity-summary.md` answers S1 to S10 with the sessions run, the models, the projects used, and the raw counts.
- [ ] Example summaries are listed, good and bad. Anything sensitive is redacted before committing.
- [ ] ADR-0011 is marked Accepted, with the chosen nudge and wording recorded, or Rejected with the evidence, following the rule in the ADR.
- [ ] CRP-047, CRP-049 and CRP-053 are amended to match the findings, or set to `not-needed`.
- [ ] No prototype code is merged.

## Notes for the implementer

- Use the owner's own projects for realism, with the owner running the sessions. Ask which projects are fine to quote from in the findings.
- Measure S5 from Claude Code's own usage reporting, comparing sessions with and without the tool.
- Do not tune the wording against one model only. The summary must work on whatever model the user happens to run.
- Time box: one and a half working days. S10 needs three sessions open at once on the same project.

## Why this model and effort

Structured experiments with clear measurements. The judgment needed is in designing fair trials, not in deep reasoning.

## References

- [ADR-0011](../../architecture/adr/0011-model-authored-activity-summary.md)
- [ADR-0008](../../architecture/adr/0008-privacy-and-safety-by-default.md)
- Hooks reference: https://code.claude.com/docs/en/hooks
- MCP in Claude Code: https://code.claude.com/docs/en/mcp
