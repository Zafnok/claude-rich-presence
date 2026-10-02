---
id: CRP-071
title: Summary-first card layout
milestone: M7 Personalisation
type: feature
status: todo
priority: P1
blocked_by: [CRP-047, CRP-070]
blocks: [CRP-074]
model: claude-sonnet-5-5
effort: high
size: M
---

# CRP-071: Summary-first card layout

## Goal

The summary gets all the visible text space Discord offers, and every other fact on the card lives somewhere that costs the summary nothing.

## Context

Designed in [ADR-0013](../../architecture/adr/0013-summary-first-card-layout.md). CRP-070 supplies the measured widths and says which slots are really visible. Read its findings first; if a slot turned out not to be shown, follow the corrected ADR.

Until this ticket lands, the card uses the simple layout of CRP-011 and CRP-047: summary on line 1, project and status on line 2.

## Scope

- **Renderer** (`internal/presence`), implementing the slot table of ADR-0013:
  - a summary that fits the visible width of line 1 stays there, with project and status on line 2;
  - a longer one breaks at a word boundary and continues on line 2, and line 2's usual content is dropped from the text;
  - the summary cap becomes two visible lines;
  - the small image is the status, and its hover text is the status in words followed by the enabled facts;
  - the large image's hover text is the project name followed by the enabled facts assigned to it;
  - the member-list line is set to show line 1.
- **Secondary facts**:
  - a setting, `hover_facts`, listing which facts appear and in what order;
  - session and agent counts and the model on by default; effort level and plan mode available and off by default;
  - a fact that is not known is omitted without leaving a gap.
- **Adapter** (`internal/adapter/code`): add the effort level and whether the session is in plan mode to the allowlist, from the fields hook events already carry.
- **Codec** (`internal/discord/codec`): the display-type field.
- **Widths** are configuration with measured defaults, so a Discord redesign is a settings change.

## Out of scope

- Facts from the status line, which CRP-074 adds to the same slots.
- Moments, which are CRP-075.
- A usage gauge image, which is CRP-074.

## Acceptance criteria

- [ ] With a summary present, no fact other than the summary, the project name and the status ever appears on a text line. A test enables every fact and asserts the text lines are unchanged.
- [ ] A golden table covers: a summary shorter than line 1; exactly at the width; one word over; long enough to fill both lines; longer than both, which is truncated with an ellipsis; a single word longer than a line.
- [ ] Breaks fall on word boundaries and never inside a multi-byte character.
- [ ] When the summary overflows, the project name and status are still present in hover text.
- [ ] Each fact appears only when enabled and known, in the configured order, and hover text never exceeds the length CRP-070 measured.
- [ ] At privacy levels without a summary, the output is identical to before this ticket.
- [ ] The shared-area line from CRP-049, if built, is laid out by the same rules.
- [ ] The codec's output with the display-type field matches a fixture, and without it is unchanged.
- [ ] In a real session, a long summary is seen by a second account across both lines, and the member list shows its first line. Recorded with a screenshot.

## Notes for the implementer

- Visible width is not a character count if CRP-070 found that Discord truncates by rendered width. In that case use a conservative character budget and say so.
- Keep line breaking a pure function with its own table test.
- The effort level is absent on models that do not support it and on events outside a tool context. Treat absence as unknown, not as a value.

## Why this model and effort

Text layout with many edge cases, across renderer, adapter and codec.

## References

- [ADR-0013](../../architecture/adr/0013-summary-first-card-layout.md), [ADR-0011](../../architecture/adr/0011-model-authored-activity-summary.md)
- Findings of CRP-070, in `docs/research/`
