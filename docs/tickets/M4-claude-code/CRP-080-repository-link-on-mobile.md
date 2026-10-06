---
id: CRP-080
title: Check the repository link button on Discord's mobile app
milestone: M4 Claude Code
type: owner-task
status: in-progress
priority: P2
blocked_by: [CRP-048]
blocks: []
model: owner
effort: low
size: S
---

# CRP-080: Check the repository link button on Discord's mobile app

## Goal

A record of whether the repository link button is shown on Discord's mobile app, to the user and to another account, so that the documentation can say where the link is visible.

## Context

CRP-048 built the button and confirmed it on the desktop client: another account sees it and it opens the repository, and the user does not see their own. See [the findings](../../research/crp-048-repository-link.md). Mobile was left unchecked because no second person was available with a phone. [ADR-0012](../../architecture/adr/0012-project-profiles-and-repository-link.md) is accepted on the desktop result.

The check needs a phone with the Discord app and, for the second half, another account that can see the owner's profile. An agent's part is to prepare, and to record what the owner reports.

## Scope

- Show a presence with a button from the owner's desktop, using [the check script](../../research/crp-048-discord-check.ps1), which holds it for five minutes and does not touch the real configuration.
- While it is shown, look at the owner's profile in the Discord mobile app:
  - signed in as the owner;
  - signed in as, or by asking, another account.
- For each, record whether the activity is shown at all, whether the button is shown, and whether tapping it opens the repository. Record the phone's operating system and the Discord app's version.
- Fill in the two mobile rows of the table under "What Discord shows" in the findings, and remove mobile from its "Not tested" list.
- Add one sentence to "Sharing a project" in [the configuration reference](../../configuration.md) saying where the button is and is not visible, desktop and mobile, to the user and to others.

## Out of scope

- Any change to how the link is published. If the button is not shown on mobile to other people, record that and open a ticket that proposes the text line's link field as an addition, with the evidence. Do not build it here.
- What mobile shows of the text lines and images, which is CRP-070.
- The button at the `minimal` level and the other gaps listed in the findings.

## Acceptance criteria

- [ ] The findings record, for the mobile app, what the owner sees on their own profile: activity, button, and whether the button opens the link.
- [ ] The findings record the same for another account, or say plainly that no second account was available and leave that row open.
- [ ] The phone's operating system and the Discord app's version are recorded with the date.
- [ ] `docs/configuration.md` says where the button is visible, and says nothing that was not observed.
- [ ] If the button is missing on mobile for other people, a follow-up ticket exists and is linked from the findings.

## Notes for the implementer

- Run the script from a checkout of `main`, after building the binary as the findings describe. It needs the Discord application id, which is in the configuration defaults or in ADR-0010 once CRP-003 has recorded it; otherwise ask the owner.
- The owner's own half needs nobody else: a phone signed in to the same account while the desktop shows the presence. Do that half first, so the ticket is not held up by the second.
- The desktop client hides the user's own button. Expect the same on mobile, and record what is seen whatever it is.
- Ask the owner in plain words for what was seen. Do not infer a row from the other.

## Why this model and effort

The work is looking at a phone and writing down what it shows; the agent only prepares the command and records the answer.

## References

- [CRP-048 findings](../../research/crp-048-repository-link.md)
- [ADR-0012](../../architecture/adr/0012-project-profiles-and-repository-link.md)
- Discord's activity object: https://docs.discord.com/developers/events/gateway-events
