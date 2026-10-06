---
id: CRP-067
title: Clean every piece of text before it is published
milestone: M6 Release
type: feature
status: done
priority: P1
blocked_by: [CRP-062]
blocks: []
model: claude-sonnet-5-5
effort: medium
size: S
---

# CRP-067: Clean every piece of text before it is published

## Goal

Every string that can reach a Discord text line has been through the same cleaning, wherever it came from, and the documentation says plainly what the `full` level publishes.

## Context

Finding F3 of the [threat model](../../architecture/threat-model.md), severity low.

A display name from a project profile goes through `config.CleanName`: one line, no control or invisible characters, no characters Discord reads as formatting, a mention or a link, cut to length. Two other sources of text do not:

- `projectName` in `internal/adapter/code/vocab.go` reduces the working directory to its last element and refuses only control characters. A directory named with a right-to-left override, backticks or `@everyone` is published as it is at `full`.
- The host takes `project` and `model` from a follower's `sync` or `event` on length alone (`internal/control/protocol/payload.go`, `internal/domain/event.go`), and the renderer joins them into a line.

The model can create a directory and work in it, so at `full` the project name can be text the model chose, up to 128 bytes. That is inherent in publishing a directory name. It should be clean, and it should be documented.

## Scope

- The directory name is cleaned by the same rules as a display name before the adapter publishes it. A name with nothing left is no name.
- The cleaning function has one home that both `internal/config` and `internal/adapter/code` can import without a dependency between packages that the purity tests forbid. Deciding where is part of the ticket; `internal/domain` is the likely place.
- The host drops a `sync` or `event` whose project or model is not already clean, meaning that cleaning it would give a different string. It is counted like any other invalid message.
- `docs/configuration.md`, in its description of the `full` level: the project name is the name of the directory Claude is working in unless a profile gives one, and Claude can create and enter directories.
- The row for F3 in the threat model: move it from the findings to the tables of sections 2 and 7, with the tests.

## Out of scope

- The activity summary's sanitiser, which is CRP-047.
- Restricting which directories count as projects.

## Acceptance criteria

- [x] A table test gives `projectName` directory names holding formatting characters, a mention, link syntax, a zero-width character and a right-to-left override, and each comes out as `CleanName` would give it.
- [x] `TestNothingLeaks` and `TestPrivacy` still pass.
- [x] A host test sends an event and a sync whose project holds a control character, and one whose model holds markup, and shows each dropped and counted, with the session unchanged.
- [x] A property or fuzz test shows that any project the adapter publishes is accepted by the host.
- [x] The purity tests of every package touched still pass, unchanged or with a stated reason.
- [x] The configuration document says what `full` publishes, as above.
- [x] The threat model names these tests.

## Notes for the implementer

- The adapter and the host must agree exactly, or the host will drop what the adapter sends. Use one function for both, and make the host's check "cleaning changes nothing".
- An older follower talking to a newer host may send a name the newer host refuses. That session then shows no project, which is the safe direction. Say so in the protocol document.
- Model labels come from a closed table in the adapter today, so the host's check on them can be stricter than cleaning: letters, digits, spaces and dots.

## Why this model and effort

Clear rules with an existing function to reuse; the care needed is in keeping two packages in agreement.

## References

- [ADR-0008](../../architecture/adr/0008-privacy-and-safety-by-default.md), section 3
- [ADR-0012](../../architecture/adr/0012-project-profiles-and-repository-link.md)
- [Threat model](../../architecture/threat-model.md), sections 2 and 7 and finding F3
