# ADR-0014: Personalities

## Status

**Proposed.** The built-in set is the owner's to settle in [CRP-072](../../tickets/M7-personalisation/CRP-072-personalities.md). Whether a personality's instruction reliably changes Claude's phrasing without weakening the public-audience rules is measured in [CRP-046](../../tickets/M4-claude-code/CRP-046-spike-activity-summary.md).

## Context

Other projects offer themes and presets: neutral, professional, jokey. Theirs are tables of fixed strings. Ours can do more, because the main line of our card is written by Claude ([ADR-0011](0011-model-authored-activity-summary.md)), and Claude can write in a voice. The owner's own example, "vibecoding the battle system", is a voice, not a template.

Two things on the card are worded: the summary, which Claude writes, and the fixed vocabulary our renderer supplies, such as the status words and the lead-in of the shared-area line. A personality should cover both, or the card reads as two different authors.

## Decision

### What a personality is

A named bundle of:

| Part | Used for |
|---|---|
| A voice instruction, one or two sentences | Appended to the instructions Claude is given for `presence_summary`. Shapes how the summary is phrased |
| A vocabulary | The fixed strings the renderer uses: status words, fallback phrases when there is no summary, the roll-up lead-in, the wording of moments |
| Optionally, an artwork set | Image keys for the large and small images |

### Choosing one

1. A global setting names the personality. The default is a plain, neutral one.
2. A project profile may name a different one for that project.
3. A user may define their own in the configuration file, giving a voice instruction and any vocabulary entries they want to change. Unset entries fall back to the plain personality.
4. A small built-in set ships with the program. The proposed set is plain, professional, casual, dramatic and deadpan. The final names and wording are settled in CRP-072.

This replaces the free-text style hint and the roll-up lead-in setting sketched earlier.

### Limits on a personality

1. **Voice only.** A personality changes tone and wording. The rules about what a summary may contain, and the sanitiser, are unchanged. The safety rules are stated after the voice instruction and take precedence over it.
2. **Fixed text.** A built-in voice instruction is constant. A custom one is the user's own configuration, cleaned and length-capped. Nothing a model wrote ever becomes part of an instruction.
3. **The same lengths.** Vocabulary entries obey the same limits as any other text on the card.
4. **Vocabulary works at every privacy level.** The voice applies only where a summary is enabled.

### Related appearance settings

Two settings sit beside the personality because users expect them from any presence integration: the activity type, which is how Discord labels the activity, and overrides for the image keys.

## Consequences

- The card has one consistent voice across what Claude writes and what we supply.
- Personalities are data. Adding one needs no code.
- A playful voice produces playful summaries, which makes an occasional odd or inaccurate phrase more likely. The dwell time and the user's ability to tell Claude to change it are the remedies.
- A voice instruction is a few more tokens in each opted-in session's context.
- If CRP-046 shows voices are ignored by some models, the vocabulary half still works and the voice half is documented as best effort.

## Alternatives considered

| Alternative | Why not |
|---|---|
| A full template language with placeholders | Encourages filling the text lines with facts, which [ADR-0013](0013-summary-first-card-layout.md) rules out |
| Free-text style hint only | No consistency between the summary and the fixed strings, and nothing to choose from for a user who does not want to write one |
| Rotating pools of joke phrases, as two other projects do | Fixed jokes wear thin. A voice applied to real work stays fresh |
| Let Claude choose the personality | The user's public voice is the user's decision |
