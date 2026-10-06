# ADR-0013: The summary comes first on the card

## Status

**Proposed.** How Discord actually displays each field is measured in [CRP-070](../../tickets/M7-personalisation/CRP-070-spike-discord-display.md). The numbers and any field that turns out not to be visible are filled in from its findings.

## Context

The owner's rule for every extra the card could carry, whether session counts, effort level, the model, cost or usage limits: it is welcome only if it takes no space from the summary.

A Discord activity has more places to put things than its two text lines:

| Slot | What it is | Basis |
|---|---|---|
| Line 1 and line 2 | Two text fields, up to 128 characters each in the protocol. What is visible is much less and varies by view | Docs for the limit. Visible width is **Unverified** |
| Large and small image | One of each | Docs |
| Hover text on each image | Text shown when the pointer is over the image | Docs. Whether and where it shows on mobile is **Unverified** |
| Timer | Elapsed time from a start timestamp | Docs |
| Buttons | Up to two | Docs. Visibility is **Unverified**; one project reports they show only for one activity type |
| Display-type field | Chooses which of the name or the two lines appears in the member list | Docs, a newer field |

Other projects fill the two lines with templates of model, tokens and cost. That is exactly the trade the owner does not want.

## Decision

### The rule

When a summary exists, **the text lines belong to it**. Everything else has a fixed home elsewhere and never moves onto a text line that the summary could use.

### Slots

| Slot | With a summary | Without one |
|---|---|---|
| Line 1 | The summary, or the shared-area line from the roll-up | The fixed phrase for the surface and privacy level |
| Line 2 | The rest of the summary if it does not fit on line 1. Otherwise project name and status | Status, and project name where the level allows |
| Small image | Status: working, waiting, idle. Briefly, a moment such as "just shipped". Optionally a usage gauge instead, if the user chooses | Status |
| Small image hover text | Status in words, then the secondary facts the user has enabled | The same |
| Large image | The project's or personality's artwork | Logo |
| Large image hover text | Project name, then further facts the user has enabled | The same, where the level allows |
| Timer | Elapsed time since the earliest start among the open sessions | The same |
| Button | The repository link, for an opted-in project ([ADR-0012](0012-project-profiles-and-repository-link.md)) | The same |
| Member-list line | Set to show line 1, so the summary is what people see next to the user's name | Default |

### Overflow

1. The renderer knows the visible width of a line, a number measured by CRP-070, not the protocol limit.
2. A summary that fits line 1 stays there, and line 2 shows project and status.
3. A longer one breaks at a word boundary and continues on line 2. Line 2's usual content yields; project and status are still available in the hover text and the small image.
4. The summary's length cap in [ADR-0011](0011-model-authored-activity-summary.md) becomes two visible lines' worth.

### Secondary facts

A setting lists which facts appear in hover text, in order. Each is shown only when it is known.

| Fact | Source | On by default |
|---|---|---|
| Status in words | Session state | Yes |
| Session and agent counts | Session registry | Yes |
| Model | Hook events, or the status line bridge | Yes |
| Effort level, plan mode | Hook events | No |
| Context used, session cost | Status line bridge ([ADR-0015](0015-status-line-bridge.md)) | No |
| Usage limits | Status line bridge | No |

None of these is ever placed on a text line while a summary is shown.

### Moments

A moment is a short-lived event worth showing, such as a successful push. For a set period it swaps the small image and the status word. It never touches line 1. Ticketed as [CRP-075](../../tickets/M7-personalisation/CRP-075-moments.md).

### Preview

Because hover text is invisible until someone hovers, the user needs a way to see the whole card. The status tool gains a preview that lists every slot as it is currently sent. Ticketed with hide and pause in [CRP-073](../../tickets/done/CRP-073-hide-pause-preview.md).

## Consequences

- The summary gets the most visible space Discord offers, and extras cost it nothing.
- Secondary facts are only seen by someone who hovers, on clients that show hover text. That is the accepted price.
- The renderer needs a measured width per view, so the layout cannot be finished before CRP-070. Until then the first release keeps the simple layout of CRP-011.
- If CRP-070 finds a slot is not shown in some views, the facts assigned to it are simply not seen there. Nothing falls back onto the text lines.
- A usage gauge as the small image replaces the status image. It is a choice between the two, not both.

## Alternatives considered

| Alternative | Why not |
|---|---|
| Templates with placeholders on both lines, as most other projects do | Puts model, tokens and cost in the summary's space |
| Rotate the second line through facts on a timer | Spends updates against Discord's rate limit and makes the card restless |
| Use the party-size field to show a session count | It appends to line 2, so it takes summary space when the summary overflows. CRP-070 records how it looks in case it is wanted later |
| Images generated per update to carry numbers | Needs hosting and network access |
