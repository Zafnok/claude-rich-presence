---
id: CRP-047
title: Activity summary in Claude Code
milestone: M4 Claude Code
type: feature
status: todo
priority: P1
blocked_by: [CRP-042, CRP-046]
blocks: [CRP-053]
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
  - `summary_projects`, an optional list of directories. When set, the level applies only to sessions whose working directory is inside one of them; elsewhere the session behaves as `full`;
  - `summary_style`, an optional short free-text hint.
- **Domain** (`internal/domain`): a session carries an optional activity phrase and an optional project display name. A new event kind sets them. They are cleared when the session id rebinds.
- **Sanitiser**, a pure function in `internal/adapter/code`: one line; a hard length cap; control characters removed; links, mentions, invite codes and markup stripped; whitespace collapsed; an empty result is rejected.
- **Tool** `presence_summary`:
  - listed only when the level is `summary` for this session;
  - description and server instructions as recorded by CRP-046, with the style hint appended;
  - marked to always load;
  - returns an empty success result in every case.
- **Nudge**, only if CRP-046 found it necessary: the reminder on the `UserPromptSubmit` result when the summary is missing or stale.
- **Renderer** (`internal/presence`): at `summary` with a phrase set, the first line is the phrase and the second carries project, status and model. Without a phrase, as `full`.
- **Plugin** (`plugin/`): the fourth privacy choice, and a `privacy` skill update that explains it plainly, including that the phrase is written by a model and can be wrong.
- **Protocol** (`internal/control/protocol`): the two new fields, as an additive change.

## Out of scope

- Claude Desktop Chat, which is CRP-053.
- Any summarising by our own code.

## Acceptance criteria

- [ ] At every level other than `summary`, `tools/list` does not include `presence_summary`, and no instruction mentions it.
- [ ] With `summary_projects` set, a session outside those directories does not list the tool. Tested for nested directories, both separator styles, and paths differing only in case on Windows.
- [ ] The sanitiser has a table test covering: over-long input, multiple lines, control characters, links with and without a scheme, mentions, invite codes, markup characters, and input that is empty after cleaning.
- [ ] A fuzz test shows the sanitiser's output never exceeds the cap, never contains a newline, and never contains a link.
- [ ] The summary text appears in no log line. A leak test seeds a marker as the summary and searches the log.
- [ ] The tool handler returns immediately with the host stalled.
- [ ] A cleared session starts with no summary.
- [ ] An older host that does not know the new fields ignores them, and a newer host accepts a follower that never sends them.
- [ ] The golden table for the renderer covers the new level with and without a phrase.
- [ ] An end-to-end scenario sets a summary through the tool and sees it at the fake Discord, sanitised.
- [ ] In a real session on one of the owner's projects, with the level enabled, Discord shows a sensible phrase. What was run and what was shown is recorded.
- [ ] The user documentation states what is published at this level, that it is model-written, and how to limit it to chosen projects.

## Notes for the implementer

- Compare directories after cleaning and resolving them the same way on both sides. Do not follow symbolic links to decide membership without stating so.
- The length cap should suit what Discord displays in the member list, which is much shorter than its protocol limit. Take the number from CRP-046's examples.
- Stripping links must also catch bare domains, or a phrase could still advertise one.
- Keep the instruction text in one place, so the plugin skill, the tool description and the tests cannot drift apart.

## Why this model and effort

Mostly ordinary changes across several packages, with a sanitiser and an opt-in boundary that must be exactly right.

## References

- [ADR-0011](../../architecture/adr/0011-model-authored-activity-summary.md), [ADR-0008](../../architecture/adr/0008-privacy-and-safety-by-default.md)
- Findings of CRP-046, in `docs/research/`
