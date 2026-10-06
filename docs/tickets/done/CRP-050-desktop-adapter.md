---
id: CRP-050
title: Claude Desktop adapter
milestone: M5 Claude Desktop
type: feature
status: done
priority: P1
blocked_by: [CRP-002, CRP-033, CRP-043]
blocks: [CRP-052, CRP-053, CRP-054]
model: claude-sonnet-5-5
effort: medium
size: S
---

# CRP-050: Claude Desktop adapter

## Goal

When the binary is started by Claude Desktop, it reports that Claude Desktop is open, and nothing more.

## Context

Claude Desktop Chat gives an extension no signal about conversations. The only reliable fact is that the server process is alive, which means the app is open: CRP-002 saw the server start at app launch, survive the window being closed to the tray, and stop at quit. See the [viability assessment](../../architecture/viability.md#what-claude-desktop-chat-offers-instead).

Read [CRP-002's findings](../../research/crp-002-desktop-extension.md) and the Claude Desktop section of [ADR-0007](../../architecture/adr/0007-integration-and-distribution.md) first. In short:

- Claude Desktop runs **two** copies of the server from app launch to quit. One is initialised by a client named `claude-ai`. The other is initialised by a client whose name is `local-agent-mode-` followed by the extension's display name. Only the first reports.
- A server that exits by itself is not restarted until the user switches the extension off and on, or restarts the app.
- A settings change restarts only the `claude-ai` copy.
- At launch the app starts a copy, closes its input before sending `initialize`, and starts the `claude-ai` copy proper two seconds later.
- The extension is not attached to Code-tab sessions, so there is no doubled adapter to handle there.

## Scope

Package `internal/adapter/desktop`, and its wiring in `internal/cli`:

- Recognise the client from the name in MCP `initialize`:

  | Client name | Treated as |
  |---|---|
  | `claude-ai`, exactly | Claude Desktop. Reports the app |
  | Starts with `local-agent-mode-` | Claude Desktop's second copy. Passive |
  | `claude-code`, and anything unrecognised | Claude Code |

- For `claude-ai`: on `initialize`, publish *session opened* for a session of surface `desktop` with a constant id for this adapter instance and the current time as start. On input closing, publish *session ended*.
- For the passive copy: publish no session, do not take the host lock and do not connect as a follower. Answer `presence_status` by asking the host, as the `status` command does.
- Neither copy exits while its input is open, whatever happens to the host or to Discord.
- Before `initialize` arrives, the process takes no lock, opens no connection and publishes nothing. If its input closes first, it exits at once.
- Expose `presence_status` only. `presence_event` is not listed for this client.
- Honour `enabled` and the privacy level. At every level the Desktop session publishes the same thing, since there is nothing private to withhold; the level is still carried so rendering is consistent.
- Add scenario E13 to the end-to-end tests.

## Out of scope

- Any attempt to detect chat activity, model, or conversation.
- A separate Discord application for Desktop.

## Acceptance criteria

- [x] With the Claude Desktop client name, `tools/list` returns only `presence_status`.
- [x] With the Claude Code client name, behaviour is unchanged.
- [x] A Desktop session and a working Code session together render the Code session in focus, with a count of two.
- [x] A Desktop session alone renders the Desktop phrase with an elapsed timer.
- [x] Input closing removes the session and, if it was the last, clears presence.
- [x] With a client name starting with `local-agent-mode-`, no session is published, the host lock is never requested, and `tools/list` returns only `presence_status`.
- [x] Two adapters started as Claude Desktop starts them, one of each client name, result in exactly one `desktop` session on the host.
- [x] Losing the host, losing Discord and being asked to stand down each leave the process running with its input open.
- [x] A process whose input closes before `initialize` exits without having requested the host lock or dialled the host.
- [x] E13 passes on all three operating systems.

## Notes for the implementer

- Match the client name exactly as recorded, and keep the recognised names in one table so adding another host later is a one-line change.
- The names were recorded on Windows with Claude Desktop 2.9939.4. They are not documented and may change. If a later version sends neither name, the adapter falls back to the Claude Code behaviour, which shows nothing without hook events. CRP-052 re-checks the names on a real install.
- Right after an install, the `claude-ai` copy ran in one of CRP-002's tests and not in the other. It ran after the next app launch both times. So presence may not appear until Claude Desktop is restarted, and the install instructions should say so. Do not make the passive copy report to cover this: it keeps old settings, and would go on showing presence after the user turned it off.
- Do not use the `roots/list_changed` notifications the passive copy receives. They follow the user's activity, and [ADR-0008](../../architecture/adr/0008-privacy-and-safety-by-default.md) rules that out.
- The idle-clear rule applies to a Desktop session too: it is `Idle` from the start, so an app left open all day stops showing after the configured period. Confirm with the owner that this is the wanted behaviour and record the answer in the pull request. If the owner wants Desktop to show for as long as it is open, give the Desktop session a status that the idle-clear rule ignores.

## Why this model and effort

Small and well bounded once the spike has answered its questions.

## References

- [ADR-0007](../../architecture/adr/0007-integration-and-distribution.md), [ADR-0008](../../architecture/adr/0008-privacy-and-safety-by-default.md)
- Findings of CRP-002, in `docs/research/`
