---
id: CRP-015
title: Decide what the elapsed timer shows when several sessions are open
milestone: M1 Core
type: feature
status: in-progress
priority: P1
blocked_by: [CRP-011, CRP-042]
blocks: []
model: claude-sonnet-5-5
effort: medium
size: S
---

# CRP-015: Decide what the elapsed timer shows when several sessions are open

## Goal

With several Claude sessions open, the elapsed timer on the Discord card reads as one sensible clock to someone looking at it, by a rule that was chosen and not inherited. Today it looks stuck or broken, which is the first thing a viewer notices.

## Context

**What the owner saw.** On 2026-10-05, during the manual check of CRP-042, 13 sessions were open in the Code tab of Claude Desktop and one in a terminal. Within a few minutes the timer on the card read 1:11, then 0:16, 0:40, 5:01, 0:52. It looked as if it kept resetting. The owner linked it to switching between Claude Desktop and the terminal.

**Why it happens.** This is the behaviour CRP-011 designed, not a defect in the code.

- Discord shows one card. The renderer picks one session to describe, the focus session: a working session over a waiting one, over a compacting one, over an idle one; then Claude Code over Claude Desktop; then the one with the most recent activity (`internal/presence/focus.go`).
- The timer is that session's own start time (`internal/presence/render.go`). The comment on `Render` says the timer "jumps when focus moves to another session and at no other time".
- So each time a different session becomes the most recently active one, the card shows that session's clock. With 14 sessions that happens constantly.

The program does not look at windows, and switching windows changes nothing by itself. What moves the focus is activity in a session: a prompt, a tool running, a turn ending. Switching to a window and typing in it is what the owner saw.

With one session open, nothing is wrong: the timer starts when the session opens and runs until it closes. Every option below leaves that case exactly as it is.

The owner was not asked to choose. This ticket carries the decision, with a recommendation.

## Scope

Choose one, and build it. **Option B is recommended**: it gives one steady clock, it is a change to one pure function, and it keeps the timer from resetting when the host changes, because it is still made only of session start times.

- **Option A: keep one timer per session.** Change no code.
  - What a viewer sees: how long the session now on the card has been open. The clock jumps, forwards or backwards, whenever the card moves to another session.
  - Record the choice beside step 4 of "From sessions to one activity" in `docs/architecture/README.md`, and say in the user documentation, if `docs/` has any by then, that the timer belongs to the session shown.
- **Option B, recommended: one timer, from the earliest start among the open sessions.**
  - What a viewer sees: how long Claude has been open in any form. The clock never jumps while that oldest session stays open. When it closes, the clock moves forward once, to the next oldest session's start.
  - The cost: a session left open and forgotten, or Claude Desktop open for days, makes the clock read many hours while the user has only just begun work. It is never less than today's reading.
  - Make the change in `Render` in `internal/presence/render.go`. `focus` is not changed: it still chooses the text, the images and the privacy level.
- **Option C: one timer, from when the card last appeared.**
  - What a viewer sees: how long this stretch of work has been going. The clock starts when the card appears, after there were no sessions or after everything had been idle long enough to clear it, and runs until the card is cleared again.
  - This reads best, and costs most. The renderer is a pure function of the sessions and cannot know when the card appeared. The host would have to remember that moment and hand it to the host that takes over after a failover, which means a new field in the control protocol. If this option is chosen, do not build it here: write a ticket for the host and protocol work with the `write-ticket` skill, and close this one with option B built in the meantime or nothing built, stating which.

Whichever is chosen:

- Rewrite the comment on `Render` to say what the timer is.
- Update step 4 of "From sessions to one activity" in `docs/architecture/README.md`.
- Update the Timer row of the slot table in [ADR-0013](../../architecture/adr/0013-summary-first-card-layout.md), which says "Elapsed time of the focus session". The ADR is Proposed, so the row can be edited. If it has been Accepted by the time this is worked, change it with the `record-decision` skill instead.
- State the choice in the pull request in plain language.

## Out of scope

- Which session the card describes. The ranking in `focus` stays.
- The text lines, images and hover text, which CRP-071 lays out. That ticket does not touch the timer.
- Any reading of windows or of which application is in front. [ADR-0008](../../architecture/adr/0008-privacy-and-safety-by-default.md) rules it out, and it is not the cause.
- A setting to choose between timers. Add one only if the owner asks.

## Acceptance criteria

For every option:

- [ ] With one session, the activity's start is that session's start. A test in `internal/presence` asserts it.
- [ ] The comment on `Render`, step 4 in `docs/architecture/README.md` and the Timer row in ADR-0013 all describe the timer as built.
- [ ] A manual check in a real Discord client with at least two sessions open, each used in turn several times, is recorded in the pull request: what was done, and what the timer read after each switch. For options B and C the timer never went backwards or restarted. For option A the record shows the jumps and says they are intended.
- [ ] The same record says whether any session was replaced by a new one during the check (see the first note below), and how that was established.

For option A:

- [ ] A test in `internal/presence`, with a name that says so, shows the start following the focus session as focus moves between two sessions with different start times.

For option B:

- [ ] With several sessions, the start is the earliest of their start times, whichever is the focus session. A table test in `internal/presence` moves focus through every session by status, surface and last activity, and asserts the start is the same throughout.
- [ ] The same sessions in every order give the same start.
- [ ] Removing the session with the earliest start gives the next earliest. Removing any other session leaves the start unchanged.
- [ ] The text lines, images and privacy level of each golden case in `internal/presence/testdata` are unchanged.

For option C:

- [ ] A ticket for the host and control protocol work exists, is in the index, and names what this ticket left in place.

## Notes for the implementer

- **Check that focus is the whole explanation before choosing.** Four of the five readings were under 80 seconds. A session's start is the moment its adapter started, so those sessions had been open for under 80 seconds, with 14 sessions the owner had open for longer. That fits sessions being replaced: the adapters restarting, or the plugin being reinstalled during the check, which the [CRP-001 findings](../../research/crp-001-claude-code-adapter.md) saw start one adapter per open session within a second. It was not investigated. With the `log_level` at `debug`, watch whether sessions open and close while the timer jumps. If they do with nobody reinstalling anything, that is a separate fault: write a ticket for it. Option B hides it only until the oldest session is one of those replaced.
- A `/clear` gives a session a new id. `test/e2e/session_test.go` already asserts that the timer does not move across it; keep that passing.
- The tests that assert today's rule are in `internal/presence/render_test.go`, where the failure messages say "the focus session's start". Under option B they are rewritten, not deleted.
- The manual check needs the owner's machine and Discord. Give the steps one at a time in plain language and write the record from what the owner reports. Step M4 of CRP-079 checks the timer with one session, which no option changes, so neither ticket waits for the other.
- If Claude Desktop is being shown for as long as it is open (the question CRP-054 carries), option B's clock is at least as old as the Desktop app whenever it is running. Say so in the pull request if that rule is in force.
- CRP-071 edits the same file. Whichever lands second rebases; there is no dependency either way.
- CRP-070 looks at how Discord draws the timer for each activity type. Nothing here needs its findings: the timer is a start time, and Discord does the counting.

## Why this model and effort

A small change to one pure function with clear criteria, plus three documents; the judgement is in the decision, which the ticket has laid out.

## References

- "From sessions to one activity" in [the architecture overview](../../architecture/README.md)
- [ADR-0013](../../architecture/adr/0013-summary-first-card-layout.md), Timer row of the slot table
- [CRP-011](../done/CRP-011-presence-renderer.md), which chose the focus session's start time
- [CRP-042](../done/CRP-042-plugin-packaging.md), whose manual check is where this was seen
