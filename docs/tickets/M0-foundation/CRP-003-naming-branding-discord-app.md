---
id: CRP-003
title: "Owner: naming, branding, Discord application"
milestone: M0 Foundation
type: owner-task
status: todo
priority: P0
blocked_by: []
blocks: [CRP-060, CRP-061, CRP-064]
model: owner
effort: n/a
size: S
---

# CRP-003: Owner: naming, branding, Discord application

## Goal

Settle the names, create the Discord application that presence will appear under, and supply its artwork. Turns [ADR-0010](../../architecture/adr/0010-naming-and-branding.md) from Proposed to Accepted.

## Context

Discord shows the application's name as the title of the activity, so this name is the most visible one in the project. Discord's Developer Portal is reported to reject names containing "Claude", and Anthropic's guidance does not allow its names or logos in a product name or logo without permission.

Only the owner can do this. It needs the owner's Discord account and the owner's judgment on naming. Implementation does not wait for it: the code reads the application id from configuration, and tests use a placeholder.

## Scope

1. **Decide the Discord application name.** Try candidates in the Developer Portal. It should be neutral, accepted by the portal without look-alike characters, and still tell a viewer what it is.
2. **Create the application** and record its application id. The id is public and is committed as the default.
3. **Produce and upload artwork.** Original artwork only, 1024 by 1024, with these asset keys:

   | Key | Used for |
   |---|---|
   | `logo` | Large image, always |
   | `working` | Small image while Claude is working |
   | `waiting` | Small image while Claude is waiting for the user |
   | `idle` | Small image while idle |

4. **Decide the repository name**: keep `claude-rich-presence` as a descriptive slug, or rename. Renaming is cheap before the first release.
5. **Approve the unaffiliated notice** used in the README, plugin description and extension description.
6. **Decide whether to ask Anthropic** for permission to use the Claude name or logo. Optional.

## Out of scope

- A separate Discord application per surface. Not in the first release.

## Acceptance criteria

- [ ] ADR-0010 is marked Accepted and records the chosen application name, the repository-name decision, and the decision on asking Anthropic.
- [ ] The application id is recorded where CRP-012 defines the default, replacing the placeholder.
- [ ] The four assets are uploaded under the keys above, and the source artwork with its license or authorship is stored in the repository under `extension/` or a documented location.
- [ ] The extension icon is supplied for CRP-051.
- [ ] The notice text is final in the README.

## Notes for the implementer

- A model can help by drafting name candidates, the notice text, and a checklist for the portal. Haiku 4.5 is enough for that.
- Do not ask a model to create the Discord application or accounts. That is the owner's action.
- If the portal accepts no acceptable neutral name, record what was tried and choose the least bad option. Do not use look-alike characters.

## Why this model and effort

It is a decision and an account action, not an engineering task.

## References

- [ADR-0010](../../architecture/adr/0010-naming-and-branding.md)
- Anthropic trademark guidelines: https://www.anthropic.com/legal/trademark-guidelines
- Claude Code legal and compliance: https://code.claude.com/docs/en/legal-and-compliance
- Discord developer policy: https://support-dev.discord.com/hc/en-us/articles/8563934450327-Discord-Developer-Policy
- Discord Developer Portal: https://discord.com/developers/applications
