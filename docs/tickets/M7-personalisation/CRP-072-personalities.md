---
id: CRP-072
title: Personalities
milestone: M7 Personalisation
type: feature
status: todo
priority: P1
blocked_by: [CRP-047]
blocks: []
model: claude-sonnet-5-5
effort: medium
size: M
---

# CRP-072: Personalities

## Goal

The user picks a personality, and the whole card speaks in it: the summary Claude writes, and the fixed words around it.

## Context

Designed in [ADR-0014](../../architecture/adr/0014-personalities.md). A personality is a voice instruction for Claude plus a vocabulary for the renderer. CRP-046 records which voice instructions actually changed Claude's phrasing and whether any weakened the public-audience rules. Use the wording it found to work.

## Scope

- **Definitions**, as data in one place: for each built-in personality, a name, a voice instruction, and the vocabulary: status words, fallback phrases, the roll-up lead-in, and moment wording.
- **The built-in set.** Proposed: plain as the default, professional, casual, dramatic, deadpan. Draft the wording, then get the owner's approval of the names and of every string before merging.
- **Configuration**:
  - `personality`, global, naming a built-in or custom one;
  - `personality` in a project profile;
  - `personalities`, user-defined entries with a voice instruction and any vocabulary overrides. Missing entries fall back to plain;
  - `activity_type`, one of the types the local interface accepts;
  - overrides for the large and small image keys.
- **Adapter**: the voice instruction is appended to the summary tool's instructions, before the safety rules, which are stated last.
- **Renderer**: every fixed string comes from the active personality's vocabulary. The focus session's personality decides.
- **Plugin**: the personality offered as a choice in the plugin's settings, and a skill that lists the personalities with an example of each.
- Remove the earlier free-text style hint and roll-up lead-in settings if they were built.

## Out of scope

- A template language.
- Personalities for Claude Desktop Chat beyond the vocabulary, unless CRP-053 has landed, in which case the voice applies there too.
- New artwork. A personality may name image keys; drawing them is the owner's.

## Acceptance criteria

- [ ] Every fixed string the renderer can emit comes from a vocabulary. A test renders every state with a personality whose strings are all markers and finds no unmarked text.
- [ ] A custom personality that sets only a voice instruction renders with the plain vocabulary.
- [ ] A project profile's personality overrides the global one for that project only.
- [ ] Custom voice instructions and vocabulary entries are cleaned and length-capped. An over-long or empty one is rejected with one warning and falls back.
- [ ] The instructions sent to Claude contain the voice instruction followed by the safety rules, in that order, for every personality.
- [ ] The sanitiser's behaviour is identical under every personality.
- [ ] Golden tables cover every built-in personality across all statuses.
- [ ] The owner has approved the built-in names and strings. Recorded in the pull request.
- [ ] In real sessions on one of the owner's projects, each built-in personality produces summaries in a recognisably different voice. Three examples per personality are recorded, with anything sensitive redacted.

## Notes for the implementer

- Keep the vocabulary keys few. Every key is a string that five personalities must supply.
- Humour dates quickly. Prefer voices that are a manner of speaking over ones built on a specific joke.
- A voice must not ask for emoji or symbols the sanitiser removes, or the result will look broken.
- A model can draft the strings. The owner decides what represents them in public.

## Why this model and effort

Mostly data and wiring, with care needed where the voice meets the safety rules.

## References

- [ADR-0014](../../architecture/adr/0014-personalities.md), [ADR-0011](../../architecture/adr/0011-model-authored-activity-summary.md)
- Findings of CRP-046, in `docs/research/`
