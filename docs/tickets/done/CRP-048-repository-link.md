---
id: CRP-048
title: Repository link for opted-in projects
milestone: M4 Claude Code
type: feature
status: done
priority: P1
blocked_by: [CRP-014, CRP-042, CRP-077]
blocks: [CRP-080]
model: claude-sonnet-5-5
effort: high
size: M
---

# CRP-048: Repository link for opted-in projects

## Goal

For a project the user has opted in, presence carries a link to its repository, and setting that up takes one command inside Claude Code.

## Context

Designed in [ADR-0012](../../architecture/adr/0012-project-profiles-and-repository-link.md). CRP-014 supplies the profile and the validated link. This ticket carries it to Discord and provides the setup skill.

Two things are not yet known and are settled here on a real Discord: how a button on an activity appears to other people, and whether the user sees it on their own profile. Third-party descriptions disagree.

## Scope

- **Domain** (`internal/domain`): a session carries an optional link. It is set when the session opens, from the effective settings, and never from an event field.
- **Adapter** (`internal/adapter/code`): add the link to the settings the adapter resolves for the session's working directory, which CRP-077 built, and attach it. The link never comes from tool input.
- **Protocol** (`internal/control/protocol`): an additive link field on the session. The host validates it again with the same function and drops it if invalid.
- **Renderer** (`internal/presence`): when the focus session has a link, add one button. The label names the host, for example "View on GitHub", from a fixed table keyed by host with a generic fallback.
- **Codec** (`internal/discord/codec`): encode buttons on the activity, at most two, within Discord's limits.
- **Plugin skill** `share-project`, Markdown only, which has Claude:
  1. find the project root and read its remote with git;
  2. convert an SSH-style remote to its `https` form and remove any embedded credentials;
  3. if the GitHub CLI is available and signed in, check that the repository is public, and stop with an explanation if it is not;
  4. if visibility cannot be checked, say so and ask the user to confirm it is public;
  5. optionally propose a short list of the project's main areas, for the user to edit and approve;
  6. show exactly what will be published: the link, the display name, and the privacy level;
  7. on confirmation, add or update the profile in the configuration file;
  8. tell the user the change applies to new sessions.
  
  And a matching `unshare-project` skill, or an argument to the same skill, that removes the profile.
- **Validation on a real Discord**, recorded: what the user sees, and what a second account sees, on desktop and on mobile.

## Out of scope

- Any network request from the binary.
- Links in Claude Desktop Chat.
- Hosts other than those on the allowlist from CRP-014.
- A second button.

## Acceptance criteria

- [x] A session in a profiled project with a link produces an activity with one button carrying that link. A session elsewhere produces none.
- [x] With two sessions, the button shown is the focus session's, and it changes when focus changes.
- [x] A `presence_event` or `presence_summary` call cannot set or change the link. A test passes a link in every input field and asserts no button results.
- [x] A follower that sends an invalid link over the control channel has it dropped by the host, and the session is otherwise accepted.
- [x] An older host ignores the new field, and a newer host accepts a follower that does not send it.
- [x] The codec's output for an activity with a button matches a fixture, and an activity without one is unchanged from before.
- [x] An end-to-end scenario shows the button at the fake Discord for a profiled directory, and its absence otherwise.
- [x] The `share-project` skill refuses to write a profile for a repository the GitHub CLI reports as private, and never writes a link containing credentials. Checked by running it on a public repository, a private one, and one with a token in its remote URL, with the results recorded.
- [x] The same record states, for each Claude Code permission mode, whether the user was asked to approve the skill's edit to the configuration file. ADR-0012 assumes they are asked in the default modes; if not, the skill gains an explicit confirmation step of its own and the ADR is corrected.
- [x] `docs/research/crp-048-repository-link.md` records how the button appears to the user and to a second account. If it is not visible to others, the ticket switches to the text line's URL field as ADR-0012 allows, and records that.
- [x] ADR-0012 is marked Accepted with the display mechanism that was confirmed.
- [x] The user documentation explains that the link is per project, that the program cannot tell whether a repository is public, and how to remove a profile.

## Outcome

Done on 2026-10-05. The link is shown as a button. On a real Discord desktop client another account sees it and it opens the repository; the user does not see their own. The `share-project` skill was run on a public repository, a private one and one with a token in its remote, and behaved as designed. In auto mode Claude Code did not ask the user to approve the skill's edit, so the skill's own confirmation is the consent step and ADR-0012 was corrected. What was not observed, mobile above all, is listed in [the findings](../../research/crp-048-repository-link.md#not-tested).

## Notes for the implementer

- CRP-070 measures how buttons display for each activity type and to whom. If it has run, use its findings for the validation step here. One other project reports that buttons show only for one activity type.
- Check the current Discord documentation for the button and URL fields before writing the codec change. The `discord-ipc` skill lists the sources.
- The validation needs the owner's Discord and a second account to view the profile. Ask the owner to run it.
- A skill is instructions for Claude, not code, so it is not measured for coverage. Keep it short and explicit, and keep every destructive or publishing step behind a confirmation.
- The skill edits a file outside the project. In Claude Code's default permission modes the user is asked to approve that edit, which is the consent step. Do not add a way around it.

## Why this model and effort

Small changes across several packages, with a publishing boundary where a mistake would put the wrong link on someone's profile.

## References

- [ADR-0012](../../architecture/adr/0012-project-profiles-and-repository-link.md), [ADR-0011](../../architecture/adr/0011-model-authored-activity-summary.md), [ADR-0008](../../architecture/adr/0008-privacy-and-safety-by-default.md)
- Discord RPC documentation: https://docs.discord.com/developers/topics/rpc
