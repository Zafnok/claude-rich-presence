---
id: CRP-073
title: Hide, pause and preview
milestone: M7 Personalisation
type: feature
status: done
priority: P1
blocked_by: [CRP-014, CRP-042, CRP-077]
blocks: [CRP-055, CRP-081]
model: claude-sonnet-5-5
effort: medium
size: S
---

# CRP-073: Hide, pause and preview

## Goal

The user can keep a project off Discord entirely, switch presence off for a while without touching settings, and see exactly what their card says right now.

## Context

Five other projects have ignore lists and three have a pause command. Our project profiles can lower a project's privacy level but cannot switch it off. And because much of our card lives in hover text ([ADR-0013](../../architecture/adr/0013-summary-first-card-layout.md)), the user has no easy way to see all of it.

The decisions are in [ADR-0012](../../architecture/adr/0012-project-profiles-and-repository-link.md), section "Hiding and pausing".

## Scope

- **Hide**: a privacy level `off`, valid globally and in a project profile. A session at `off` is never sent to the host, so it does not count toward session totals and cannot become the focus.
- **Pause**:
  - a tool, `presence_pause`, taking a duration or "until resumed", and a tool or argument to resume;
  - a pause applies to the whole presence, across all sessions, and is held by the host;
  - while paused, the activity is cleared and nothing is sent;
  - a pause with a duration ends by itself;
  - plugin skills `pause` and `resume` that have Claude call the tool, so the user types one command.
- **Preview**:
  - the status tool gains a preview: every slot of the card as currently sent, including hover text, the link, and whether presence is paused or hidden for this session;
  - a plugin skill `preview` that shows it.
- **Protocol**: pause and resume messages from a follower to the host, and the pause state in the host's answer to `status`.

## Out of scope

- Pattern-based ignore lists. A profile per project covers the need.
- Scheduled quiet hours.

## Acceptance criteria

- [x] A session in a project profiled `off` produces no control traffic beyond what is needed to stay connected, and no activity. A session elsewhere is unaffected.
- [x] With one session `off` and one visible, the count on the card is one.
- [x] `presence_pause` with a duration clears the activity at once and restores it when the duration ends. Tested with the fake clock.
- [x] A pause survives a host failover: the new host is still paused, for the remaining time.
- [x] Resume restores the current activity without waiting for a new event.
- [x] The pause tools can only pause and resume. No input to them can change what is shown, a privacy level, or a profile.
- [x] The preview lists every slot and matches what the fake Discord received.
- [x] The preview is returned only to the session's own user through the tool result. It is not logged.
- [x] An older host that does not know the pause messages ignores them, and the follower reports that pausing is unavailable.

## Notes for the implementer

- For a pause to survive failover, each follower must learn the pause state from the host and offer it back on reconnect, as it does with its own session. Take the latest end time when followers disagree.
- The preview contains the project name and summary, so it is unlike the diagnostic status output, which is safe to paste in public. Keep the two outputs separate and label the preview as private.
- The pause tools are called by the model on the user's instruction. They are safe to expose because they can only reduce what is published.

## Why this model and effort

Small features over existing machinery, with one subtle point in keeping a pause across failover.

## References

- [ADR-0012](../../architecture/adr/0012-project-profiles-and-repository-link.md), [ADR-0013](../../architecture/adr/0013-summary-first-card-layout.md), [ADR-0005](../../architecture/adr/0005-presence-host-election.md)
