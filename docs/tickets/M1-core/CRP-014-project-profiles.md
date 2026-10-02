---
id: CRP-014
title: Project profiles in configuration
milestone: M1 Core
type: feature
status: todo
priority: P1
blocked_by: [CRP-012]
blocks: [CRP-047, CRP-048]
model: claude-sonnet-5-5
effort: medium
size: M
---

# CRP-014: Project profiles in configuration

## Goal

A user can give individual projects their own privacy level, display name and repository link, so that a few chosen projects are visible while everything else stays private.

## Context

Designed in [ADR-0012](../../architecture/adr/0012-project-profiles-and-repository-link.md). This ticket builds the configuration and matching. The features that consume it are the summary (CRP-047) and the link (CRP-048). On its own it already delivers per-project privacy levels and display names.

## Scope

In `internal/config`:

- A `projects` list in the configuration file. Each entry has `path`, and optionally `privacy`, `name` and `link`.
- A pure function that, given a working directory and the loaded configuration, returns the effective settings for that session: the matching profile's values over the global ones.
- Matching rules:
  - a session matches a profile when its working directory is the profile's path or inside it;
  - the longest matching path wins;
  - comparison is on cleaned absolute paths, case-insensitive on Windows and macOS, and tolerant of either separator style on Windows;
  - no matching profile means global settings.
- Validation per entry, with the fall-back-and-warn behaviour of CRP-012: a bad entry is skipped with one warning and never stops the program.
- A link validator as a pure function, implementing the rules in ADR-0012: `https` only, host allowlist with `github.com` as default and a `link_hosts` setting to extend it, no credentials, no query, no fragment, owner and repository path in a conservative character set, length limit. It accepts a trailing `.git` and removes it.
- A display-name validator: one line, a length cap, control characters and markup removed.
- Use of the profile's `name` in place of the directory name wherever the project name is shown.

Profiles are read from the user's configuration file only. Environment variables do not define profiles.

## Out of scope

- Publishing the link, which is CRP-048.
- The summary level's behaviour, which is CRP-047. This ticket only makes the level settable per project.
- Reading anything from inside a project directory, including git metadata.
- Any command or tool that writes profiles.

## Acceptance criteria

- [ ] Matching is covered by a table test: exact path, nested path, sibling with a common prefix that must not match, nested profiles where the longest wins, trailing separators, mixed separators and case differences on Windows, and no match.
- [ ] A session in a worktree directory under a profiled project matches that profile.
- [ ] A profile can lower the privacy level as well as raise it.
- [ ] The link validator has a table test with at least: a valid GitHub link, `http`, a credential in the URL, a query string, a fragment, an extra path segment, a host not on the allowlist, a look-alike host such as one that merely ends with or contains the allowed name, an over-long value, and a link with a trailing `.git`.
- [ ] A fuzz test shows the validator never accepts a URL containing `@`, `?` or `#`, and never panics.
- [ ] A rejected link produces one warning that names the profile and does not echo the value.
- [ ] An invalid or duplicate profile entry is skipped with one warning, and the rest load.
- [ ] No code path reads a file inside a project directory.
- [ ] The configuration reference lists the new settings with an example.

## Notes for the implementer

- Parse the URL with the standard library and compare the parsed host exactly. Do not match hosts with string prefixes or suffixes.
- Do not resolve symbolic links when matching. State that in the documentation, so the behaviour is predictable.
- Git remotes are often written in the SSH form. The validator does not convert them. The `share-project` skill in CRP-048 does that before writing the profile.
- Keep the effective-settings function free of I/O, so the adapter can call it for every session cheaply.

## Why this model and effort

Ordinary configuration work with a validator and path matching that have many small cases.

## References

- [ADR-0012](../../architecture/adr/0012-project-profiles-and-repository-link.md), [ADR-0008](../../architecture/adr/0008-privacy-and-safety-by-default.md)
- CRP-012, which this extends
