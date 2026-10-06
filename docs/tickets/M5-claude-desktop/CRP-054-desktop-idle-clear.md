---
id: CRP-054
title: Decide how long Claude Desktop is shown when nothing else is running
milestone: M5 Claude Desktop
type: feature
status: todo
priority: P1
blocked_by: [CRP-050]
blocks: []
model: claude-sonnet-5-5
effort: medium
size: S
---

# CRP-054: Decide how long Claude Desktop is shown when nothing else is running

## Goal

Claude Desktop is shown in Discord for as long as the owner wants it shown, by a rule that was chosen and not inherited.

## Context

The Claude Desktop adapter reports one fact: the app is open. Its session is `Idle` from the moment it opens, and it never sends another event, because Claude Desktop tells an extension nothing about conversations.

The renderer clears the activity once every session has been idle longer than the idle-clear period, which is 15 minutes by default. CRP-050 left that rule applying to the Desktop session, as the safe default. The effect, with only Claude Desktop running:

- Presence appears when the app starts.
- 15 minutes later it clears.
- It does not come back until Claude Desktop is restarted, or a Claude Code session does something. Using Claude Desktop does not bring it back, because the adapter cannot see that.

So for someone who uses only Claude Desktop Chat, presence is visible for the first 15 minutes of each launch. The owner was not asked during CRP-050; this ticket carries the decision.

## Scope

Choose one, and build it. **Option A is recommended**: without it the Desktop integration is close to invisible, and a user who does not want a card that stays up can switch presence off.

- **Option A, recommended: Claude Desktop is shown for as long as it is open.** The idle-clear rule ignores a session of surface `desktop`. Sessions of surface `code` are cleared as now. When only idle Code sessions and a Desktop session are open, the Desktop session is what is shown.
  - Make the change in `internal/presence`, in the function that decides whether everything has been idle long enough and in the one that says when that moment comes. Do not add a status to the domain for it: the surface already says what is needed.
  - Update step 5 of "From sessions to one activity" in `docs/architecture/README.md`, and the description of `idle_clear_after` wherever the configuration is documented.
- **Option B: keep the rule as it is.** Change no code. Record the choice in `docs/architecture/README.md` beside step 5, and add a sentence to the Claude Desktop section of ADR-0007's consequences saying that Desktop alone is shown for the idle-clear period after launch.

Whichever is chosen, state it in the pull request in plain language.

## Out of scope

- Detecting activity in Claude Desktop Chat. There is no documented signal, and [ADR-0008](../../architecture/adr/0008-privacy-and-safety-by-default.md) rules out the undocumented ones.
- A separate setting for the Desktop period. Add one only if the owner asks.

## Acceptance criteria

For option A:

- [ ] A Desktop session alone is still rendered after more than the idle-clear period has passed.
- [ ] Idle Code sessions alone are cleared after the period, as before.
- [ ] An idle Desktop session beside Code sessions that have all been idle past the period renders the Desktop session.
- [ ] The host arms no idle-clear timer while a Desktop session is open.
- [ ] `docs/architecture/README.md` describes the rule as built.

For option B:

- [ ] `docs/architecture/README.md` and ADR-0007 say that Claude Desktop alone is shown for the idle-clear period after launch and then cleared.

## Notes for the implementer

- The end-to-end scenario E13 in `test/e2e/desktop_test.go` and the tests in `internal/cli/desktop_test.go` run for seconds, far inside the period, so neither choice changes them.
- The manual check of real idle behaviour is step V12 of CRP-052. It checks whichever rule is in force when it is run, so this ticket does not need to land before it.
- The golden files of the renderer are in `internal/presence/testdata`.

## Why this model and effort

A small change to two pure functions with clear criteria.

## References

- [ADR-0007](../../architecture/adr/0007-integration-and-distribution.md), [ADR-0008](../../architecture/adr/0008-privacy-and-safety-by-default.md)
- [CRP-050](../done/CRP-050-desktop-adapter.md), which left the rule as it was
- "From sessions to one activity" in [the architecture overview](../../architecture/README.md)
