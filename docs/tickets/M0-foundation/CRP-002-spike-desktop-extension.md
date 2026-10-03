---
id: CRP-002
title: "Spike: Claude Desktop extension lifecycle"
milestone: M0 Foundation
type: spike
status: done
priority: P0
blocked_by: []
blocks: [CRP-044, CRP-050, CRP-052, CRP-063]
model: claude-opus-5-5
effort: high
size: M
---

# CRP-002: Spike: Claude Desktop extension lifecycle

## Goal

Establish how Claude Desktop runs a desktop extension's server, so we know whether "Claude Desktop is open" can be shown reliably, and whether an adapter started by Claude Desktop can share a presence host with adapters started from a terminal. Retires R3, R4 and part of R5.

## Context

The Desktop design in [ADR-0007](../../architecture/adr/0007-integration-and-distribution.md) assumes the extension's server process lives exactly as long as the app. The documentation implies this but does not state it. On Windows, Claude Desktop is a packaged Store app, which may change what its child processes see of the file system.

This is a spike, and it is self-contained: it builds its own small prototype and needs nothing from any other ticket. Prototype code is never merged.

## Scope

Build a throwaway prototype: a Go program that speaks just enough MCP over standard streams to answer `initialize`, `tools/list` and `tools/call`, and that appends everything it observes to a local file, including when it started and stopped, its environment, its working directory, its parent process and every path it resolves. Package it as an MCPB bundle of server type `binary`, install it as a desktop extension, and answer:

| # | Question |
|---|---|
| B1 | When is the server started: at app launch, at first conversation, or at first tool use? |
| B2 | Is there one server per app, per window, or per conversation? |
| B3 | When is it stopped: on quit, on window close, when minimised to the tray? What happens if it exits by itself; is it restarted? |
| B4 | What `clientInfo` does Claude Desktop send in `initialize`? Is it distinguishable from Claude Code? |
| B5 | Does a server that lists only a diagnostic tool, or none, install and run without warnings? |
| B6 | How do `user_config` values reach the server? |
| B7 | **Shared host.** Can a process started by Claude Desktop and a process started from a terminal lock the same file and connect to the same Unix socket, at the default location in [ADR-0006](../../architecture/adr/0006-control-channel.md)? On Windows, check specifically whether the packaged app's children see a redirected local application data directory |
| B8 | On Windows, are Claude Desktop's children placed in a job that kills them when the app exits? Record it; the design does not depend on the answer |
| B9 | On macOS, does an unsigned binary inside an installed bundle run, or does Gatekeeper block it? Does it differ between a bundle downloaded in a browser and one fetched by Claude Code? |
| B10 | On Windows, does an unsigned binary inside the bundle run without a SmartScreen or antivirus prompt? |
| B11 | Is a desktop extension also attached to sessions in the Desktop Code tab? If the plugin is installed too, would one Code-tab session have two adapters? |
| B12 | Can the server connect to the real Discord pipe from inside Claude Desktop's process tree? |
| B13 | On Windows, does a console window appear or flash when Claude Desktop starts the binary? |

Optional, if time allows: does Cowork run an extension's server on the host or in a sandbox?

## Out of scope

- Any attempt to detect conversation activity. See [ADR-0008](../../architecture/adr/0008-privacy-and-safety-by-default.md).
- Production code.

## Acceptance criteria

- [x] `docs/research/crp-002-desktop-extension.md` exists and answers B1 to B13 with what was run, on which operating system and app version, and what was observed.
- [x] Windows is covered. macOS is covered, or listed as untested with B9 left open and flagged in [risks.md](../../architecture/risks.md).
- [x] If B7 fails on any platform, [ADR-0006](../../architecture/adr/0006-control-channel.md) is amended with the corrected location or transport. If CRP-031 is still `todo`, it is amended to match. If it has started or finished, a follow-up ticket is written instead.
- [x] If B1 to B3 show the lifetime assumption is false, the Desktop section of ADR-0007 is revised and CRP-050 and CRP-052 are amended, or set to `not-needed` with the reason.
- [x] If B11 shows doubled adapters, CRP-050 states how the duplicate is handled.
- [x] The prototype is pushed to a branch named `spike/crp-002`, linked from the findings, and never merged.

## Outcome

Done on 2026-10-03, over two runs on Windows. The [findings](../../research/crp-002-desktop-extension.md) answer B1 to B13. macOS was not tested: no Mac was used, B9 stays open, and it is flagged in the risk register under R5. A tool call from a Chat conversation was never seen to reach the server; CRP-052 checks it.

## Notes for the implementer

- Go must be installed first. If CRP-004 has not landed, install it for the spike only.
- CRP-001 builds a similar logger for Claude Code. The two are separate on purpose, so the spikes can run at the same time. Do not wait for it and do not share code with it. The program is about a hundred lines.
- For B7, have the prototype take the lock, create the socket, and log the absolute paths it resolved. Run a second copy from a terminal and compare.
- For B12, a raw open of the pipe and a handshake is enough. No activity needs to be set. A Discord application id is needed for the handshake; use any test application of the owner's.
- These steps need the owner's machine and Claude Desktop install. Ask the owner to perform them and record exactly what was run.
- Time box: one working day.

## Why this model and effort

Operating-system packaging behaviour is subtle, and the conclusions decide whether a milestone exists.

## References

- [ADR-0005](../../architecture/adr/0005-presence-host-election.md), [ADR-0006](../../architecture/adr/0006-control-channel.md), [ADR-0007](../../architecture/adr/0007-integration-and-distribution.md)
- [Risk register](../../architecture/risks.md): R3, R4, R5
- MCPB documentation: https://claude.com/docs/connectors/building/mcpb
