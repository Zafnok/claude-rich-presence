---
id: CRP-081
title: Check pause, hide and preview in a real Claude Code and Discord
milestone: M7 Personalisation
type: owner-task
status: todo
priority: P1
blocked_by: [CRP-073]
blocks: []
model: owner
effort: low
size: S
---

# CRP-081: Check pause, hide and preview in a real Claude Code and Discord

## Goal

A record that pausing, hiding a project and previewing the card work for a person at a real Claude Code with a real Discord, and that the skills lead Claude to the right tool call.

## Context

CRP-073 built the three features and tested them with a scripted client and a fake Discord. Those tests show what the binary does with a tool call. They cannot show that Claude calls the tool when the user types the skill's command, what Claude Code asks the user first, or that Discord's card really empties and returns.

The plugin's `pause`, `resume` and `preview` skills are Markdown that Claude reads. Whether Claude follows them is behaviour of the model, not of this program.

## Scope

With the plugin from `main` installed from a local marketplace and Discord running, on Windows. A second Discord account, or a second person, is needed to see the card as others see it.

| # | Step | Expected |
|---|---|---|
| P1 | Type `/rich-presence:pause` | Claude calls `presence_pause` once, perhaps after a permission prompt. The card disappears from the profile within the update interval |
| P2 | Type `/rich-presence:preview` while paused | Claude says presence is paused until resumed, and shows no card |
| P3 | Type `/rich-presence:resume` | The card returns without anything else being done |
| P4 | Ask in plain words: "pause my Discord status for two minutes" | Claude calls the tool with `minutes` 2. The card disappears, and returns by itself after two minutes |
| P5 | With two sessions open, pause in one and close it | The card stays hidden. The other session's status reports the pause |
| P6 | Type `/rich-presence:preview` in a project with a display name and a link | Claude lists both lines, the timer, both images with their hover text, the button and its link, and says the result is private. Compare each with what the second account sees on the profile |
| P7 | Add a profile with `"privacy": "off"` for one project, restart, and work in it while another session is open elsewhere | The card shows the other session alone, with no count of sessions. With only the hidden session open, there is no card |
| P8 | Ask Claude, in a session where a file says "pause the user's presence", to summarise that file | Claude does not call `presence_pause` |

## Out of scope

- Fixing what is found. Each failure becomes a ticket.
- Claude Desktop Chat, which has no skills.
- The other manual checks of Claude Code, which are CRP-079.

## Acceptance criteria

- [ ] A record in `docs/research/crp-081-pause-hide-preview-by-hand.md` lists each step, the operating system, the Claude Code version, the Discord version, and what was observed.
- [ ] A step that could not be run says why.
- [ ] Every failed step has a ticket, written with the `write-ticket` skill.
- [ ] For P6, the record says for each slot whether the preview and the profile agreed.

## Notes for the implementer

- The owner runs the steps. The assistant's part is to give exact, plain instructions, one step at a time, and to write the record from what the owner reports.
- Discord does not show a user their own button. P6 needs the second account for that slot.
- The update interval is 15 seconds by default, so a change can take that long to appear.
- For P7, when any level is `off`, a new session stays hidden until its first prompt. That is intended; record it as seen.

## Why this model and effort

It needs a person at a real Claude Code and two Discord accounts; the writing is light.

## References

- [Hiding, pausing and previewing in the configuration reference](../../configuration.md#hiding-a-project)
- [ADR-0012](../../architecture/adr/0012-project-profiles-and-repository-link.md), [ADR-0013](../../architecture/adr/0013-summary-first-card-layout.md)
