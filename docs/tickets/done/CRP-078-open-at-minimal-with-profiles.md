---
id: CRP-078
title: Open a profiled session at the lowest level until its first hook
milestone: M7 Personalisation
type: feature
status: done
priority: P2
blocked_by: [CRP-077]
blocks: []
model: claude-sonnet-5-5
effort: medium
size: S
---

# CRP-078: Open a profiled session at the lowest level until its first hook

## Goal

A session in a project whose profile is more private than the global level shows no more than that profile allows at any moment, including the time between the session opening and its first hook.

## Context

[CRP-077](../done/CRP-077-per-project-settings-in-adapter.md) made the adapter resolve the privacy level from the working directory of each hook. The directory is not known when the session opens, because `Open` runs before any hook, so the opening event carries the global level. The opening event holds only the session id, the start time and that level label: no project, model, tool or name.

The renderer ([`internal/presence/render.go`](../../../internal/presence/render.go)) decides what to show from the session's level label. A session labelled `standard` or `full` is shown with an "Idle" status line, a small status image and, when there are several, a session count. So a user with a global level of `standard` or `full` and a `minimal` profile for a project sees that card until the first hook arrives, which is the first prompt of the session and can be a long time. The card names nothing about the project, but it shows more than `minimal` allows.

This was found in CRP-077 and recorded in its pull request. CRP-077 left the opening level at the global one because its scope said so.

## Scope

In `internal/adapter/code`:

- When `Options.Resolve` is set, `Open` publishes the session at `minimal`, and the adapter's level before the first hook that carries a working directory is `minimal`. The first such hook resolves the level as it does today: a profile or the global level, by the existing raise and rebind rules.
- When `Options.Resolve` is nil, nothing changes: `Privacy` applies from `Open`.
- `Options.Privacy` stays required. With a resolver it becomes the level for a directory that matches no profile, and the doc comments on `Options`, `Adapter` and the package say so.
- `presence_status` reports the effective level, which with a resolver is `minimal` until the first hook with a working directory.

## Out of scope

- Any change to the renderer or the host.
- Passing the resolver from the command line, which is CRP-033.
- A level that depends on anything but the working directory.

## Acceptance criteria

- [x] With a resolver set and the global level `full`, the event `Open` publishes carries the level `minimal` and no project, model, tool or name.
- [x] With a nil resolver, every existing test of the adapter passes unchanged, including the one that shows `Open` publishing the global level.
- [x] With a resolver set and the global level `standard`, a session whose first hook is in a directory with no profile is published at `standard` from that hook on, shown by applying the published events to a `domain` registry.
- [x] With a resolver set and the global level `full`, a session whose first hook is in a `minimal` profile is at `minimal` in the registry before and after that hook, and no event published up to that hook carries a level above `minimal`.
- [x] With a resolver set, a first hook with no working directory leaves the session at `minimal`.
- [x] `presence_status` prints `minimal` before the first hook with a working directory and the resolved level after it.

## Notes for the implementer

- The cost of this choice is visible to every user with a resolver: before the first prompt, their card shows the `minimal` text, and it changes to the resolved level when the first hook arrives. That is the intended trade, a plainer card for a short time against showing more than a profile allows.
- Claude Code sends `cwd` with every hook (hooks reference, read 2026-10-04), so a session that stays at `minimal` for want of a directory is not expected. It is the safe outcome if it happens.
- `bound()` and `afterBind` in the adapter's test helpers assume the opening carries the global level. Add helpers for the resolver case rather than changing them.

## Why this model and effort

The change is a few lines in a state machine whose rules CRP-077 already set, and the criteria say exactly what to observe.

## References

- [CRP-077](../done/CRP-077-per-project-settings-in-adapter.md), pull request Zafnok/claude-rich-presence#23
- [ADR-0008](../../architecture/adr/0008-privacy-and-safety-by-default.md), [ADR-0012](../../architecture/adr/0012-project-profiles-and-repository-link.md)
