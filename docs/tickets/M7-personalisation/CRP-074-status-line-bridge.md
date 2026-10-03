---
id: CRP-074
title: Status line bridge
milestone: M7 Personalisation
type: feature
status: todo
priority: P2
blocked_by: [CRP-045, CRP-071]
blocks: []
model: claude-sonnet-5-5
effort: high
size: M
---

# CRP-074: Status line bridge

## Goal

A user who connects the bridge gets the model from the first moment of a session, and can opt in to showing context used, session cost and their usage limits, all without the program reading a credential or touching the network.

## Context

Designed in [ADR-0015](../../architecture/adr/0015-status-line-bridge.md). CRP-045 establishes whether the status line runs on each surface, how the binary gets a stable path, and how an existing status line is wrapped. Read its findings first. If it found the bridge unworkable, this ticket is `not-needed`. If it works only in the terminal, build it for the terminal and say so in the documentation.

This command runs after every assistant message, in the user's shell, in place of their own status line. It must be fast, and it must never be the reason their status line breaks.

## Scope

- **`statusline` command** (`internal/adapter/statusline`, wired in `internal/cli`):
  - read the status line data from standard input;
  - keep only the allowlist in ADR-0015; never read the transcript path, directories, repository identity or session name;
  - send one message to the presence host and do not wait for an answer;
  - if a wrapped command is configured, run it with the same input and pass its output and exit status through unchanged; otherwise print nothing;
  - on any internal error, still run the wrapped command and still exit as it did.
- **Domain and protocol**: optional fields on a session for effort, fast mode, context percentage, session cost, the two usage percentages with reset times, and whether a pull request is open. Additive. The model from this source fills the same field the hooks fill.
- **Rendering**, in the slots of [ADR-0013](../../architecture/adr/0013-summary-first-card-layout.md):
  - each new fact available to `hover_facts`, off by default;
  - an optional usage gauge as the small image, chosen by a setting, using a few fixed image keys by threshold;
  - nothing on the text lines.
- **Stable path**: whatever CRP-045 chose, implemented and kept current across plugin updates.
- **Plugin skills** `connect-statusline` and `disconnect-statusline`: show the current status line setting and what it will become, edit the user's settings on confirmation, wrap an existing command, and restore it exactly on disconnect.
- **Doctor**: a check that reports whether the bridge is connected and when it last delivered data.

## Out of scope

- Any reading of credentials or any network request. See ADR-0015.
- Publishing the session name.
- Rewriting or restyling the user's own status line.

## Acceptance criteria

- [ ] With a wrapped command configured, the bridge's standard output and exit status are byte-for-byte those of the wrapped command, including when the presence host is unreachable and when the input is malformed.
- [ ] With no wrapped command, the bridge prints nothing and exits 0.
- [ ] The command returns within a stated budget on each operating system, measured in an end-to-end test, without waiting on the host.
- [ ] A leak test seeds markers in every field outside the allowlist and finds them nowhere downstream.
- [ ] No fact from the bridge appears on a text line. Each appears in hover text only when enabled.
- [ ] Usage percentages are absent, not zero, when the status line does not supply them.
- [ ] The usage gauge picks the right image for values just below, at and just above each threshold.
- [ ] With the bridge connected, a new session shows the model before the first prompt.
- [ ] `connect-statusline` followed by `disconnect-statusline` leaves the user's settings identical to before. Checked on a machine with an existing status line and on one without.
- [ ] The binary still links no network client, and no code path opens Claude's credentials file. If the no-network test from CRP-062 exists, it is extended to cover this command. If not, this ticket adds that check for this command.
- [ ] The user documentation states which surfaces the bridge works on, what each fact reveals, and that usage limits are published only if enabled.

## Notes for the implementer

- Claude Code cancels a running status line command when a new update arrives. Expect to be killed mid-run and leave nothing half-written.
- The shell that runs the command differs on Windows depending on whether Git Bash is installed. The skill writes the command for the shell that machine uses, and CRP-045 records the forms that work.
- Cost is an estimate at list price and may differ from the user's bill. Say so wherever it is shown.
- Usage limits exist only for some subscription plans. Absence is normal.

## Why this model and effort

A command on the user's critical path with a strict pass-through contract, plus an allowlist boundary.

## References

- [ADR-0015](../../architecture/adr/0015-status-line-bridge.md), [ADR-0013](../../architecture/adr/0013-summary-first-card-layout.md), [ADR-0008](../../architecture/adr/0008-privacy-and-safety-by-default.md)
- Findings of CRP-045, in `docs/research/`
- Status line reference: https://code.claude.com/docs/en/statusline
