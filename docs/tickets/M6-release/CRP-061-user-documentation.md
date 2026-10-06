---
id: CRP-061
title: User documentation
milestone: M6 Release
type: docs
status: todo
priority: P1
blocked_by: [CRP-003, CRP-042, CRP-052]
blocks: [CRP-064]
model: claude-haiku-4-5
effort: n/a
size: S
---

# CRP-061: User documentation

## Goal

Someone who has never seen the project can install it, understand what it will and will not show, change the privacy setting, and fix the common problems, from the README and four short pages.

## Context

The README today describes a project in its architecture phase. By this ticket the software exists and has been validated, and the install steps that worked are recorded by CRP-042 and CRP-052.

## Scope

- **README**, rewritten for users: what it is, the unaffiliated notice, a screenshot, install in two commands for Claude Code, install for Claude Desktop, what is shown on each surface, the privacy default, links onward. Keep the architecture links in a section for contributors.
- `docs/user/install.md`: Claude Code and Claude Desktop, per operating system, with any security prompt and how to answer it, and how to update and uninstall.
- `docs/user/configuration.md`: every setting from CRP-012, where to set it on each surface, and the configuration file.
- `docs/user/privacy.md`: exactly what is and is not read, sent to Discord, and logged, at each level. Written from [ADR-0008](../../architecture/adr/0008-privacy-and-safety-by-default.md) and checked against the code's allowlist.
- `docs/user/troubleshooting.md`: presence does not appear, presence is stuck, two activities show, the model is not shown, Discord installed through Flatpak or Snap, and how to run `doctor` and what to include in an issue.
- **Limitations**, stated plainly in the README and in troubleshooting: Claude Desktop Chat shows only that the app is open; cloud and web sessions, remote and WSL setups are not supported.

## Out of scope

- Contributor documentation, which already exists.
- Translations.

## Acceptance criteria

- [ ] Every install step was performed as written on Windows, and on a Mac if CRP-052 covered one. Steps that could not be verified are marked as unverified.
- [ ] Every setting in CRP-012 appears in `configuration.md` with its default, and nothing appears that the code does not implement.
- [ ] `privacy.md` lists the allowlisted fields per event, generated from or checked against the table in the code.
- [ ] The unaffiliated notice approved in CRP-003 appears in the README.
- [ ] Each troubleshooting entry names a symptom, a likely cause, and an action.
- [ ] All relative links resolve.
- [ ] The minimum Claude Code version from CRP-042 is stated.

## Notes for the implementer

- Write for someone who uses Discord and Claude but does not know what MCP or a hook is. Define a term or avoid it.
- Do not claim anything the validation records do not support.
- Turning presence off: disabling the plugin does not stop it in sessions that are already open. They keep showing presence until each is restarted, or the plugin is uninstalled, which stops it at once. Observed in [CRP-042](../done/CRP-042-plugin-packaging.md). Say this in the install and troubleshooting pages.
- A changed privacy level applies to sessions started afterwards. The card shows one session at a time at that session's level, so with older sessions still open it can show more than the new level allows. Same record.
- A screenshot needs the owner. Ask for one from CRP-052's record.

## Why this model and effort

Prose from complete source material. The cheapest model is sufficient, with a review pass by the owner.

## References

- [ADR-0008](../../architecture/adr/0008-privacy-and-safety-by-default.md), [ADR-0010](../../architecture/adr/0010-naming-and-branding.md)
- [Viability: what each surface can show](../../architecture/viability.md#what-each-surface-can-show)
- Validation records in `docs/research/`
