---
id: CRP-075
title: "Moments: just shipped"
milestone: M7 Personalisation
type: feature
status: todo
priority: P2
blocked_by: [CRP-042]
blocks: []
model: claude-sonnet-5-5
effort: medium
size: S
---

# CRP-075: Moments: just shipped

## Goal

When Claude pushes code, the card says so for a few minutes, without our program ever seeing the command.

## Context

One other project shows a "just shipped" state by reading the text of shell commands. We do not read tool inputs ([ADR-0008](../../architecture/adr/0008-privacy-and-safety-by-default.md)). Claude Code can do the matching for us: a hook may carry an `if` filter in permission-rule syntax, such as a pattern for a git push, and the hook then runs only for matching tool calls. Our tool receives a fixed event name and nothing about the command.

Two facts from the hooks reference matter. `PostToolUse` fires only when the tool call succeeded. And when Claude Code cannot tell which commands a call will run, it runs the hook regardless, so a moment can occasionally fire when nothing was pushed.

CRP-001 records whether the `if` filter works on hooks of type `mcp_tool`. If it does not, this ticket is `not-needed` under the preferred wiring.

## Scope

- **Plugin** (`plugin/hooks/hooks.json`): `PostToolUse` hooks on the shell tools, each with an `if` filter and a literal moment name:
  - a push: `shipped`;
  - optionally, creating a pull request with the GitHub command line tool: `opened-pr`.
- **Adapter**: the moment names join the allowlist as event names with no fields.
- **Domain**: a session carries an optional current moment with the time it began.
- **Renderer**, in the slots of [ADR-0013](../../architecture/adr/0013-summary-first-card-layout.md): for a configurable period, three minutes by default, the small image and the status word show the moment. Line 1 is untouched. When the period ends the card returns to normal.
- **Configuration**: `moments`, a list of enabled moments, and `moment_duration`. Zero or an empty list disables the feature.
- Wording comes from the personality's vocabulary if CRP-072 has landed, otherwise from fixed strings.

## Out of scope

- Reading command text, exit output, or anything else about the tool call.
- Detecting commits. Committing is frequent and not an event worth announcing.
- Moments in Claude Desktop Chat.

## Acceptance criteria

- [ ] The tool input for a moment contains only the moment name and the session id. Shown by the hook file and the adapter's allowlist test.
- [ ] A moment changes the small image and status word, leaves line 1 identical, and expires after the configured period. Tested with the fake clock.
- [ ] A second moment during the first restarts the period.
- [ ] A session ending during a moment clears it.
- [ ] With moments disabled in configuration, the event is ignored.
- [ ] The test that compares the hook file with the adapter's allowlist covers the new hooks.
- [ ] In a real session, a push by Claude produces the moment in Discord, and an unrelated shell command does not. Recorded.
- [ ] The documentation says that a moment can occasionally appear without a push, and why.

## Notes for the implementer

- Write the filters to match the push as one of several commands joined together, which the hooks reference says the filter handles, and record the exact patterns tested.
- The [CRP-001 findings](../../research/crp-001-claude-code-adapter.md) confirm that `if` works on an `mcp_tool` hook, on Windows and Linux: `Bash(git push *)` fired for `git push origin main` and for `git status && git push origin main`, and for none of five other successful commands, including `echo git push origin main`. A failed push fires `PostToolUseFailure`, not `PostToolUse`. The case where the filter runs regardless, on command substitution, was not reached and still needs measuring. A second handler on `PostToolUse` added nothing to the model's context there; ADR-0007's rule 8 asks for that to be checked again for whatever this ticket adds.
- A new image key, `shipped`, is needed. Uploading it to the Discord application is an owner action within this ticket: ask the owner, and store the source image under `assets/`.
- Moments pass through the update scheduler like everything else, so one may appear up to the minimum interval late. A three-minute display makes that unimportant.

## Why this model and effort

Small and well bounded: a few hooks, one field, one render rule.

## References

- [ADR-0013](../../architecture/adr/0013-summary-first-card-layout.md), [ADR-0008](../../architecture/adr/0008-privacy-and-safety-by-default.md)
- Hooks reference, the `if` field: https://code.claude.com/docs/en/hooks
- Findings of CRP-001, in `docs/research/`
