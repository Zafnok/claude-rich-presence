---
id: CRP-011
title: Presence renderer
milestone: M1 Core
type: feature
status: done
priority: P0
blocked_by: [CRP-010]
blocks: [CRP-015, CRP-032]
model: claude-sonnet-5-5
effort: medium
size: M
---

# CRP-011: Presence renderer

## Goal

A pure function from a registry snapshot, the current time and display settings to the one activity Discord should show, or to "show nothing".

## Context

Discord displays a single activity: two short text lines, a large and a small image, and an elapsed timer. Several sessions may be open. See [From sessions to one activity](../../architecture/README.md#from-sessions-to-one-activity).

## Scope

Package `internal/presence`:

- **Focus selection**, in this order: working over waiting over compacting over idle; `code` over `desktop`; most recent activity; then session id, so the result is deterministic.
- **Text**, by the focus session's privacy level:

  | Level | First line | Second line |
  |---|---|---|
  | `minimal` | A fixed phrase per surface | Empty |
  | `standard` | A fixed phrase per surface | Status, tool kind when working, model family when known, session count when more than one |
  | `full` | As standard, with the project name | As standard |

  Write the exact phrases as a table in the package, one row per surface, status and tool kind.
- **Limits**: each line is at most 128 characters after truncation with an ellipsis, and a line shorter than 2 characters is omitted.
- **Timer**: the focus session's start time.
- **Images**: the `logo` asset as the large image, and `working`, `waiting` or `idle` as the small image, with hover text.
- **Idle clearing**: if every session has been idle longer than the configured period, the result is "show nothing".
- **Empty registry**: "show nothing".
- The output is the **Activity** type that CRP-010 defines in the domain package, or "show nothing".

## Out of scope

- User-defined templates, buttons, links, external image URLs.
- Rate limiting, which is CRP-013.

## Acceptance criteria

- [x] A golden table test covers every combination of surface, status, tool kind, privacy level, and one versus several sessions.
- [x] Focus selection is covered for every tie-break, and the same input always produces the same output.
- [x] A project name appears only when the focus session's level is `full`.
- [x] Session count appears only when more than one session is open and the level is not `minimal`.
- [x] Lines over the limit are truncated on a character boundary, never in the middle of a multi-byte character.
- [x] The idle-clear rule is tested at, just before and just after the threshold.
- [x] The package has no I/O and takes the current time as a parameter.

## Notes for the implementer

- The elapsed timer should not jump when focus moves between sessions more than necessary. Document the choice: the focus session's own start time.
- Keep phrases short. Discord truncates visually well before 128 characters in most views.
- Confirm the current field limits in the `discord-ipc` skill's references before fixing the numbers.

## Why this model and effort

Straightforward rules with many combinations, well served by table tests.

## References

- [Architecture: from sessions to one activity](../../architecture/README.md#from-sessions-to-one-activity)
- [ADR-0008](../../architecture/adr/0008-privacy-and-safety-by-default.md)
