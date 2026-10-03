---
id: CRP-044
title: "Fallback: command hooks and standalone host"
milestone: M4 Claude Code
type: feature
status: not-needed
priority: P0
blocked_by: [CRP-001, CRP-002, CRP-043]
blocks: []
model: claude-opus-5-5
effort: high
size: L
---

# CRP-044: Fallback: command hooks and standalone host

## Closed as not needed

CRP-001 found that the wiring in ADR-0007 works on Windows and Linux, and the ADR was accepted on 2026-10-03. The condition below did not occur. See the [findings](../../research/crp-001-claude-code-adapter.md).

## Condition

Build this only if CRP-001 concludes that the wiring in [ADR-0007](../../architecture/adr/0007-integration-and-distribution.md) cannot be used. Otherwise set the status to `not-needed`.

If it is needed, first write the ADR that supersedes ADR-0007, then split this ticket into smaller ones with the `write-ticket` skill. What follows is the outline to split.

## Goal

Claude Code support using the pattern existing projects use: command hooks that run the binary for each event, feeding a standalone host process.

## Context

Without `mcp_tool` hooks there is no long-lived adapter in a Claude Code session, so a host must be started on demand and must work out for itself when sessions end. The core, the Discord packages and the host's serving logic are unchanged.

## Scope

1. **`hook` command**: read one hook event from standard input, apply the same allowlist, mapping and privacy rules as CRP-041, send one event to the host, exit 0 always. If no host is running, start one and retry briefly.
2. **`daemon` command**: run the host node standalone. Exit after a configurable period with no sessions.
3. **Session liveness by process id**: the `hook` command reports its parent process, which is the Claude Code process when hooks run in exec form. The host periodically checks that each session's process is alive and removes sessions whose process is gone.
4. **Detached start** of the daemon on each operating system: no console window, not tied to the hook's lifetime.
5. **Plugin changes**: command hooks in exec form with `async` set, for every event including `SessionStart`, which this wiring does receive at launch along with the model name.
6. **Delivery**: the plugin distributed as a release archive containing the binaries and a launcher that selects the right one, placed outside any top-level `bin/` directory.
7. **Tests**: end-to-end scenarios equivalent to CRP-043, driving the `hook` command.

## Out of scope

- Claude Desktop, which is unaffected.

## Acceptance criteria

To be written when the ticket is split. They must include at least:

- [ ] A session killed without an end event disappears from presence within the liveness interval.
- [ ] The daemon exits when no sessions remain, and is restarted by the next hook.
- [ ] Two hooks starting a daemon at the same moment result in one daemon.
- [ ] The `hook` command adds no perceptible delay to Claude, with hooks marked asynchronous.
- [ ] The plugin works on Windows with and without Git Bash installed.
- [ ] Any launcher script is as small as possible, is exercised in CI on its operating system, and its exclusion from the coverage measure is recorded as an amendment to [ADR-0009](../../architecture/adr/0009-quality-gates.md).

## Notes for the implementer

- Whether Claude Code resolves an extension-less command to an `.exe` on Windows in exec form is undocumented. Test it before designing the launcher.
- On Windows, a daemon started from inside Claude Desktop's process tree may be terminated with the app. CRP-002 records whether that happens. The design must tolerate it: the next hook restarts the daemon and sessions are rebuilt from subsequent events.
- Prior projects that use this pattern are listed in the [viability assessment](../../architecture/viability.md#prior-art). Their issue trackers show where it goes wrong.

## Why this model and effort

Process lifecycle, detachment and liveness across three operating systems.

## References

- [ADR-0005](../../architecture/adr/0005-presence-host-election.md), section "If the fallback is adopted"
- [ADR-0007](../../architecture/adr/0007-integration-and-distribution.md), section "Fallback"
- Hooks reference: https://code.claude.com/docs/en/hooks
