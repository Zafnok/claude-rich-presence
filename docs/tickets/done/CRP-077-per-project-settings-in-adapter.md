---
id: CRP-077
title: Per-project privacy level and display name in the Claude Code adapter
milestone: M7 Personalisation
type: feature
status: done
priority: P1
blocked_by: [CRP-014, CRP-041]
blocks: [CRP-047, CRP-048, CRP-073]
model: claude-sonnet-5-5
effort: high
size: M
---

# CRP-077: Per-project privacy level and display name in the Claude Code adapter

## Goal

A session in a profiled project is published at that project's privacy level and under its display name, so that the profiles a user writes in the configuration file take effect in Discord.

## Context

CRP-014 added project profiles to `internal/config` and a pure function, `Config.Effective`, that returns the settings for a working directory. Nothing calls it. The adapter from CRP-041 takes one privacy level when it is built and derives the project name from the directory, so a profile changes nothing a user can see.

[ADR-0012](../../architecture/adr/0012-project-profiles-and-repository-link.md) says profiles are resolved in the adapter, before the control channel. The summary, the repository link and hiding each need that resolution. This ticket builds it once, for the level and the name, and those tickets add their own fields to it.

This ticket is in M7 because the id range of M4, where the adapter lives, is full.

Three facts about the present code shape the work:

| Fact | Where |
|---|---|
| The adapter may import `internal/domain` and `internal/mcp` only, so it cannot call `internal/config` | `internal/adapter/code/doc.go` |
| The working directory is parsed only when the level is `full`, and dropped otherwise | `parseHook` in `internal/adapter/code/allowlist.go` |
| A refresh event with an empty project does not clear the session's project in the domain | `Session.apply` in `internal/domain/session.go` |

## Scope

In `internal/adapter/code`:

- `Options` gains an optional resolver: a function from a working directory to the settings for it, which are a privacy level and a display name. The type is declared in this package. When the resolver is nil the adapter behaves exactly as it does today.
- The existing `Privacy` option stays required. It is the level before any hook has supplied a working directory, and the level whenever the resolver is nil.
- On each accepted hook that carries a working directory, the adapter resolves the settings for it and uses them for that call and for later calls that carry none.
- The working directory is read at every privacy level when a resolver is set, because the level cannot be known without it. It is used to resolve the settings and, at `full`, to derive the directory name. It is never stored, logged or forwarded.
- A display name from the resolver replaces the directory name wherever the project name is published. It is published only when the effective level is `full`, like the directory name.
- When the effective level changes during a session, because the working directory moved into or out of a profiled project:
  - events from then on are restricted at the new level;
  - the host is told the new level;
  - after a change to a lower level, nothing published at the higher level remains in the session's state at the host. Ending the session and reopening it under the same id and start time, as a rebind does, is one way; choose in this ticket and say which in the pull request.
- A resolver that panics or returns an invalid level is treated as the most restrictive outcome: the call is handled at `minimal` with no name.
- The `presence_status` tool reports the effective level, not the global one.

In `internal/cli`, only if the composition root from CRP-033 already builds the adapter when this ticket starts: pass a resolver built from `Config.Effective` and the running operating system. If it does not yet, change nothing there; CRP-033 passes it.

In `docs/architecture/adr/0008-privacy-and-safety-by-default.md`: a dated clarification to section 3, item 4, which says the working directory is discarded on receipt at levels other than `full`. With profiles it is used once, in memory, to choose the level, and then discarded. The decision is unchanged; the owner confirms the wording in review. Use the `record-decision` skill.

## Out of scope

- The repository link, which is CRP-048. It adds the link to the resolved settings.
- The `summary` level and the areas list, which are CRP-047.
- The level `off`, which is CRP-073.
- Reading the working directory of the process, or anything inside a project directory.
- Profiles for Claude Desktop Chat, which has no project.
- Any change to `internal/config`.

## Acceptance criteria

- [ ] With a nil resolver, every existing test of the adapter passes unchanged.
- [ ] With the global level `minimal` and a resolver that returns `full` and a name for one directory, a session whose hooks carry that directory publishes the name as its project, and a session elsewhere publishes only what `minimal` allows.
- [ ] With the global level `full` and a resolver that returns `minimal` for one directory, a session there publishes no project, tool kind, status or model after its first hook.
- [ ] A resolved display name is published in place of the directory name at `full`, and is not published at `standard` or `minimal`.
- [ ] A session that moves from a `full` directory to a `standard` one leaves no project name in the session state rebuilt from the published events. Shown by applying the published events to a `domain` registry.
- [ ] A session that moves from a `minimal` directory to a `full` one publishes the project from the next hook on.
- [ ] A hook with no working directory keeps the settings last resolved.
- [ ] A resolver that panics, and one that returns an unknown level, each result in events restricted at `minimal`, and the tool call still returns its constant result.
- [ ] The test that seeds forbidden fields shows the working directory appears in no published event and in no status output, at every level, with a resolver set.
- [ ] `presence_status` prints the effective level for a profiled directory.

## Notes for the implementer

- The resolver runs on the path Claude waits on. It must not perform I/O. `Config.Effective` does none; say so in the option's comment, so that nobody passes one that does.
- The level a session opens with is the global one, because `Open` runs before any hook. If the global level is higher than the project's, the opening event carries no project, since no directory is known yet. Check that nothing else in that event exceeds what the project's level allows, and if something does, describe it in the pull request and stop rather than weaken the criterion.
- `restrict` is today the last thing done to a batch and takes one level. A batch that spans a change of level needs each event restricted at the level that applied to it.
- A dropped batch leaves the adapter's memory as it was, so the next call repeats the moves. The resolved settings are part of that memory.
- Load the `claude-surfaces` skill, and re-check which hook events carry `cwd` against the live documentation before relying on it.

## Why this model and effort

It changes where the privacy boundary is decided inside a state machine that already has rebinding and dropped-batch rules, and a mistake publishes a project name the user meant to keep private.

## References

- [ADR-0012](../../architecture/adr/0012-project-profiles-and-repository-link.md), [ADR-0008](../../architecture/adr/0008-privacy-and-safety-by-default.md)
- [Configuration reference](../../configuration.md), for how a profile is matched
- CRP-014, which supplies `Config.Effective`, and CRP-041, which built the adapter
