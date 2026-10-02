# ADR-0010: Neutral product names, no Anthropic or Discord marks

## Status

**Proposed.** The owner settles it in [CRP-003](../../tickets/M0-foundation/CRP-003-naming-branding-discord-app.md).

## Context

| Fact | Source |
|---|---|
| Anthropic permits saying in plain text that a product works with Claude Code. It does not permit the Claude Code or Anthropic names or logos in a product name or logo, or use implying endorsement, without written permission | Claude Code legal and compliance page; Anthropic trademark guidelines |
| `claude plugin validate` treats a plugin name beginning `claude-`, `anthropic-` or similar as an error, and a name containing `claude` as a whole word as a warning. Claude Code still loads such a plugin, but its own tooling refuses to create or tag one | Plugin manifest reference |
| Discord's Developer Portal is reported to reject application names containing "Claude". Existing projects use look-alike spellings and characters to get around it | Prior-art READMEs. The exact rule is unverified |
| Discord's developer policy forbids impersonating another application | Discord developer policy |
| The repository is already named `claude-rich-presence`, and is public | Observed |

The Discord application's name is the text Discord shows as the activity title, so it is the most visible name of all.

## Decision

1. **Binary and plugin name: `rich-presence`.** No vendor mark. It passes strict plugin validation and reads sensibly in a process list.
2. **Descriptive references are fine.** Documentation says "for Claude Code and Claude Desktop" in plain text.
3. **No vendor logos.** Discord art assets are original artwork. The Claude logo is not used unless Anthropic gives written permission.
4. **No look-alike characters** to evade the Discord name filter.
5. **Every user-facing surface carries an unaffiliated notice**: README, plugin description, extension description.
6. **All names are defined once**, in one place in the code and one in the release configuration, so a rename is a one-line change.
7. **The Discord application id is configurable**, so a user can substitute their own application and name.

Left to the owner in CRP-003:

| Question | Recommendation |
|---|---|
| Discord application name | A neutral name that the portal accepts and that still tells a viewer what it is. Decide by trying candidates in the portal |
| Repository name | Keep `claude-rich-presence` as a descriptive slug, or rename to match the product. Renaming is cheap now and expensive after release |
| Asking Anthropic for permission to use the name or logo | Optional. Worth doing before any submission to Anthropic's plugin directory |

## Consequences

- The plugin can be validated strictly in CI and is eligible for directory submission.
- Presence will not say "Claude Code" as its title unless the owner obtains permission or the portal accepts it. The detail lines can still describe the activity.
- Module path and repository slug may differ from the product name. That is common and harmless.

## Alternatives considered

| Alternative | Why not |
|---|---|
| Name everything `claude-rich-presence` | Fails plugin validation and conflicts with Anthropic's naming guidance |
| A look-alike Discord application name | Evading a filter invites removal of the application, which would break presence for every user at once |
| Use the Claude logo as the large image | Not permitted without permission |
