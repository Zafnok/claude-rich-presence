---
id: CRP-052
title: Desktop validation on real machines
milestone: M5 Claude Desktop
type: owner-task
status: todo
priority: P1
blocked_by: [CRP-002, CRP-042, CRP-050, CRP-051]
blocks: [CRP-061, CRP-063]
model: owner
effort: low
size: S
---

# CRP-052: Desktop validation on real machines

## Goal

Confirm, with a real Claude Desktop and a real Discord client, that the extension installs and presence behaves as designed, and write down the install steps that worked.

## Context

Automated tests replace both Claude and Discord with fakes. This is the one place the real combination is checked for Desktop. It needs a person, the owner's machine, and a Discord account.

## Scope

On Windows, and on a Mac if one is available, with the bundle built from `main`:

| # | Step | Expected |
|---|---|---|
| V1 | Install the bundle as a desktop extension | Installs without errors. Note any security prompt |
| V2 | Open Claude Desktop with Discord running | Presence appears, with an elapsed timer |
| V3 | Quit Claude Desktop | Presence clears within a few seconds |
| V4 | Start Discord after Claude Desktop | Presence appears without restarting Claude |
| V5 | Restart Discord while Claude Desktop is open | Presence returns |
| V6 | With Desktop open, start a Claude Code session in a terminal with the plugin installed, and give it a task | Presence switches to the Code session, with a count of two |
| V7 | End the Code session | Presence returns to Desktop |
| V8 | Quit Claude Desktop while the Code session is running | Presence continues from the Code session after a brief gap |
| V9 | Use the Code tab inside Claude Desktop with both the extension and the plugin installed | One Code session is shown, not two |
| V10 | Ask Claude in Desktop Chat whether rich presence is working | Claude calls the status tool and reports correctly |
| V11 | Change the privacy setting in the extension's settings and restart | The change takes effect |
| V12 | Leave everything idle past the idle-clear period | Behaviour matches what CRP-050 decided |
| V13 | If an alternative Discord client such as Vesktop is available, repeat V2 with it | Presence appears. If not, record it as a known limitation. One other project's card disappeared there |
| V14 | With presence showing, start a game that Discord detects | Record which activity Discord shows first. One other project's users lost their game activity |

## Out of scope

- Fixing what is found. Each failure becomes a ticket.

## Acceptance criteria

- [ ] A validation record in `docs/research/crp-052-desktop-validation.md` lists each step, the operating system and app versions, and what was observed, with a screenshot of Discord for V2 and V6.
- [ ] Every failed step has a ticket, written with the `write-ticket` skill.
- [ ] The install steps that worked, including any security prompt and how it was answered, are written up for CRP-061.
- [ ] If no Mac was available, that is stated, and the Mac items in [risks.md](../../architecture/risks.md) stay open.

## Notes for the implementer

- A model can prepare the checklist, read the logs, and write up the record. Sonnet 5.5 at low effort is enough. The steps themselves are the owner's.
- Run `doctor` from a standalone copy of the binary if a step fails, and attach its output.

## Why this model and effort

It is observation on real machines.

## References

- [Architecture: failure behaviour](../../architecture/README.md#failure-behaviour)
- Findings of CRP-002, in `docs/research/`
