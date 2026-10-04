---
id: CRP-047
title: Activity summary in Claude Code
milestone: M4 Claude Code
type: feature
status: todo
priority: P1
blocked_by: [CRP-014, CRP-042, CRP-046, CRP-077]
blocks: [CRP-049, CRP-053, CRP-071, CRP-072]
model: claude-sonnet-5-5
effort: high
size: M
---

# CRP-047: Activity summary in Claude Code

## Goal

A user who opts in sees a short phrase in Discord describing what they are working on, written by Claude, for the projects they chose.

## Context

Designed in [ADR-0011](../../architecture/adr/0011-model-authored-activity-summary.md). CRP-046 fixes the wording, the nudge, and how any permission prompt is handled. Read its findings first. If it rejected the ADR, this ticket is `not-needed`.

This is the first feature that publishes anything about the content of the user's work. The sanitiser and the opt-in are the whole of the protection.

## Scope

- **Configuration** (`internal/config`):
  - privacy level `summary`, above `full`;
  - settable globally or per project, through the project profiles of CRP-014;
  - `summary_min_dwell`, a duration, five minutes by default, zero to disable.
- **Domain** (`internal/domain`): a session carries an optional activity phrase, an optional area and an optional project display name. A new event kind sets them. A display name from the project's profile takes precedence over one the model supplies. They are cleared when the session id rebinds.
- **Sanitiser**, a pure function in `internal/adapter/code`: one line; a hard length cap; control characters removed; links, mentions, invite codes and markup stripped; whitespace collapsed; an empty result is rejected.
- **Tool** `presence_summary`:
  - listed only when the level is `summary` for this session;
  - description and server instructions as recorded by CRP-046, with the profile's `areas` list when there is one. The voice of the phrase is added later by the personalities of CRP-072;
  - marked to always load;
  - returns immediately in every case, with an empty result or the fixed acknowledgement CRP-046 chose.
- **Stability**, in the adapter, with the injected clock:
  - the adapter holds the session's current phrase and area, and nothing recomputes them;
  - a call whose cleaned phrase and area equal the current ones changes nothing;
  - a different phrase replaces the shown one no sooner than the dwell time. The latest pending phrase is applied when the period ends.
- **Nudge**, only if CRP-046 found it necessary: the reminder on the `UserPromptSubmit` result while the session has no summary. Never per turn or on a schedule.
- **Renderer** (`internal/presence`): at `summary` with a phrase set, the first line is the phrase and the second carries project, status and model. Without a phrase, as `full`.
- **Plugin** (`plugin/`): the fourth privacy choice, and a `privacy` skill update that explains it plainly, including that the phrase is written by a model and can be wrong.
- **Protocol** (`internal/control/protocol`): the new fields, as an additive change.

## Out of scope

- Claude Desktop Chat, which is CRP-053.
- Any summarising by our own code.
- The roll-up across sessions, which is CRP-049. This ticket only carries the area.
- Writing phrases to disk.

## Acceptance criteria

- [ ] At every level other than `summary`, `tools/list` does not include `presence_summary`, and no instruction mentions it.
- [ ] With the level set only in a project profile, a session outside that project does not list the tool, and a session inside it does.
- [ ] The sanitiser has a table test covering: over-long input, multiple lines, control characters, links with and without a scheme, mentions, invite codes, markup characters, and input that is empty after cleaning.
- [ ] A fuzz test shows the sanitiser's output never exceeds the cap, never contains a newline, and never contains a link.
- [ ] The summary text appears in no log line. A leak test seeds a marker as the summary and searches the log.
- [ ] The tool handler returns immediately with the host stalled.
- [ ] A cleared session starts with no summary.
- [ ] A call repeating the current phrase and area causes no update at the fake Discord.
- [ ] Two different phrases within the dwell time result in the first being shown for the whole period and then the latest. Tested with the fake clock, including a dwell of zero.
- [ ] If the nudge is built, it is sent only while the session has no phrase and stops as soon as one is set.
- [ ] After a host failover, the session's phrase and area reappear without any call from Claude.
- [ ] An older host that does not know the new fields ignores them, and a newer host accepts a follower that never sends them.
- [ ] The golden table for the renderer covers the new level with and without a phrase.
- [ ] An end-to-end scenario sets a summary through the tool and sees it at the fake Discord, sanitised.
- [ ] In a real session on one of the owner's projects, with the level enabled, Discord shows a sensible phrase. What was run and what was shown is recorded.
- [ ] The user documentation states what is published at this level, that it is model-written, and how to limit it to chosen projects.

## Notes for the implementer

- The length cap comes from the display measurements of CRP-070 if they are available, otherwise from CRP-046's examples. CRP-071 later lets a long phrase continue onto the second line.
- Stripping links must also catch bare domains, or a phrase could still advertise one.
- Keep the instruction text in one place, so the plugin skill, the tool description and the tests cannot drift apart.

## Why this model and effort

Mostly ordinary changes across several packages, with a sanitiser and an opt-in boundary that must be exactly right.

## References

- [ADR-0011](../../architecture/adr/0011-model-authored-activity-summary.md), [ADR-0008](../../architecture/adr/0008-privacy-and-safety-by-default.md)
- Findings of CRP-046, in `docs/research/`
