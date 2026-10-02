---
id: CRP-064
title: Directory submission
milestone: M6 Release
type: owner-task
status: todo
priority: P3
blocked_by: [CRP-003, CRP-060, CRP-061]
blocks: []
model: owner
effort: n/a
size: S
---

# CRP-064: Directory submission

## Goal

Decide whether to list the plugin and the extension in Anthropic's directories, and if so, submit them.

## Context

Until listed, users install by adding this repository as a marketplace, which works. A directory listing makes the project easier to find and subjects it to Anthropic's review and naming rules, which is why [ADR-0010](../../architecture/adr/0010-naming-and-branding.md) keeps vendor names out of the plugin name.

## Scope

- Read the current submission requirements for the Claude Code plugin directory and for desktop extensions.
- Fill in the directory listing fields in the plugin manifest: icon, documentation, support and privacy links.
- Write a privacy policy page if one is required. It can be short, because the program collects nothing: draw it from `docs/user/privacy.md`.
- Submit, and track the review.

## Out of scope

- Listing in third-party catalogues.

## Acceptance criteria

- [ ] The decision to submit or not is recorded, with the reason.
- [ ] If submitting: the listing fields are set, `claude plugin validate --strict` still passes, the submission is made by the owner, and its status is recorded in this ticket.
- [ ] Any change the reviewers require becomes a ticket.

## Notes for the implementer

- A model can prepare the listing text and the privacy page. Haiku 4.5 is enough. The submission itself is the owner's action.
- If the reviewers object to naming, return to ADR-0010 rather than working around the objection.

## Why this model and effort

An optional owner action with a little drafting.

## References

- [ADR-0010](../../architecture/adr/0010-naming-and-branding.md)
- Plugin manifest reference, directory listing fields: https://code.claude.com/docs/en/plugins-reference
