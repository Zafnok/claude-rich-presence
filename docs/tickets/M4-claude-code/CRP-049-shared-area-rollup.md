---
id: CRP-049
title: Shared-area roll-up across sessions
milestone: M4 Claude Code
type: feature
status: todo
priority: P2
blocked_by: [CRP-047]
blocks: []
model: claude-sonnet-5-5
effort: medium
size: S
---

# CRP-049: Shared-area roll-up across sessions

## Goal

When several sessions are working on related tasks in one project, presence shows what they have in common: three sessions on battler AI, the prep screen and sprites read as "Working on the battle engine".

## Context

Designed in [ADR-0011](../../architecture/adr/0011-model-authored-activity-summary.md), section "Several sessions in one project". CRP-047 already carries an `area` with each summary and, when the project's profile lists `areas`, tells Claude to pick from that list. This ticket uses the areas.

The roll-up is a fixed rule in the renderer. No model is involved, and no session is shown another session's text. Read CRP-046's findings on how often sessions agree on an area. If agreement was poor even with a list, this ticket is `not-needed`.

## Scope

- **Adapter and protocol**: each session at the `summary` level carries a project key, a stable hash of its profile's path, so the host can tell which sessions belong together without receiving the path. An additive field.
- **Renderer** (`internal/presence`), for the focus session's project, counting sessions that are open in it and have a phrase:

  | Situation | First line |
  |---|---|
  | One such session | Its own phrase |
  | Two or more, and the focus session shares its area with at least one other | A line built from that area, with the session count |
  | Two or more, with no area shared by the focus session | The focus session's phrase, with the session count |

- Areas are compared after normalising case and spacing.
- The roll-up line is a lead-in plus the area. The lead-in is a setting, `summary_rollup_lead`, "Working on" by default, so the phrasing can match the user's style hint.
- Sessions in different projects, or with no profile, are never grouped.
- User documentation: how to list a project's areas in its profile, and why that makes the roll-up dependable.

## Out of scope

- Any model call to compute a theme.
- Passing one session's phrase or area to another session's Claude. ADR-0011 rules it out unless a later decision changes that.
- Learning areas automatically or remembering them between sessions.

## Acceptance criteria

- [ ] The rule is covered by a golden table: one session; two with the same area; three where the focus shares with one; three where the other two share and the focus does not; areas differing only in case or spacing; sessions in two projects; a session with a phrase but no area.
- [ ] The result is deterministic for a given set of sessions, whatever order they arrived in.
- [ ] When a session leaves and one remains, the line returns to that session's own phrase.
- [ ] The project key is not reversible to a path, and the path itself never crosses the control channel. Shown by the type definitions and a leak test.
- [ ] No control message from host to follower carries another session's phrase or area. Shown by the message types.
- [ ] An older host ignores the new field and behaves as before.
- [ ] An end-to-end scenario with three sessions in one profiled directory, given areas through the tool, shows the shared line at the fake Discord.
- [ ] In real sessions on one of the owner's projects with `areas` listed in its profile, three concurrent sessions on related tasks produce the shared line. What was run and what Discord showed is recorded.

## Notes for the implementer

- Keep the lead-in and the area as separate values until the last step, so truncation never cuts the lead-in and leaves no area.
- "Shares its area with at least one other" is deliberately centred on the focus session: the line should describe what the most active session is part of, not a majority it does not belong to.
- A changing roll-up line passes through the same update scheduler as everything else, so it cannot flicker faster than Discord's rate limit allows.

## Why this model and effort

A small, fully specified rule with a table of cases.

## References

- [ADR-0011](../../architecture/adr/0011-model-authored-activity-summary.md), [ADR-0012](../../architecture/adr/0012-project-profiles-and-repository-link.md)
- [Architecture: from sessions to one activity](../../architecture/README.md#from-sessions-to-one-activity)
- Findings of CRP-046, in `docs/research/`
