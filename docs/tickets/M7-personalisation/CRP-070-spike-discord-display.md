---
id: CRP-070
title: "Spike: how Discord displays an activity"
milestone: M7 Personalisation
type: spike
status: todo
priority: P1
blocked_by: []
blocks: [CRP-071]
model: claude-sonnet-5-5
effort: medium
size: S
---

# CRP-070: Spike: how Discord displays an activity

## Goal

Measure, on real Discord clients, what a viewer actually sees of each field of an activity, so the card can be laid out around the summary with real numbers instead of protocol limits. Settles [ADR-0013](../../architecture/adr/0013-summary-first-card-layout.md).

## Context

The protocol allows 128 characters per text line, but Discord shows far fewer and the amount differs between the member list, the profile popout, the full profile and mobile. Hover text, buttons and the newer display-type field are described inconsistently by other projects. The layout, the summary's length cap and the link button all depend on these facts.

This is a spike. A throwaway script sets activities; a person looks at them.

## Scope

With a throwaway program that connects to the local Discord client and sets a given activity, and with a second Discord account to view the first, record for each view below what is shown:

| View | Client |
|---|---|
| Member list entry | Desktop, mobile |
| Profile popout | Desktop |
| Full profile, activity section | Desktop, mobile |
| The user's own view of their profile | Desktop |

Questions:

| # | Question |
|---|---|
| D1 | How many characters of line 1 and of line 2 are visible before truncation, in each view? Is truncation by character count or by width? |
| D2 | Where does the hover text of the large and small image appear? Is it reachable on mobile at all? How long can it be before it is cut? |
| D3 | What does the display-type field change in the member list? Can line 1 be made the text shown next to the user's name? |
| D4 | For each activity type that the local interface accepts, how do the title, the two lines and the timer look? |
| D5 | Are buttons visible to the second account, and to the user themselves, for each activity type? What happens when one is clicked? |
| D6 | Do the URL fields for the text lines make them clickable, and for whom? |
| D7 | How does the party-size field render, and on which line? |
| D8 | What does Discord show when an image key does not exist? |
| D9 | When the user is also running a game, which activity does Discord show first? |
| D10 | How quickly does a change become visible to the second account? |

## Out of scope

- Any production code.
- Alternative Discord clients, which CRP-052 touches.

## Acceptance criteria

- [ ] `docs/research/crp-070-discord-display.md` answers D1 to D10 with screenshots, the Discord client versions, and the exact activity payload used for each.
- [ ] The visible width of each line in each view is given as a number the renderer can use.
- [ ] ADR-0013's slot table is corrected against the findings, and the ADR is marked Accepted or revised.
- [ ] The answer to D5 is recorded in the findings and in [ADR-0012](../../architecture/adr/0012-project-profiles-and-repository-link.md). CRP-048 reads it from there. This ticket does not edit CRP-048.
- [ ] The `discord-ipc` skill is updated with whatever was learned about fields.
- [ ] The throwaway script is pushed to a branch named `spike/crp-070`, linked from the findings, and never merged.

## Notes for the implementer

- Go must be installed first. If CRP-004 has not landed, install it for the spike only.
- A model can write the throwaway script and prepare the list of payloads. The looking has to be done by the owner, with a second account or a friend's.
- Use a test Discord application, not the project's final one, so experiments do not appear under its name.
- Test strings should make counting easy: numbered blocks of ten characters, and a second set with wide and narrow letters to tell character limits from width limits.
- Time box: half a day.

## Why this model and effort

Preparing payloads and writing up observations. The observing is the owner's.

## References

- [ADR-0013](../../architecture/adr/0013-summary-first-card-layout.md)
- Discord RPC documentation: https://docs.discord.com/developers/topics/rpc
