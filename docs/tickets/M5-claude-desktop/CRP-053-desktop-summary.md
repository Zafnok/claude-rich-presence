---
id: CRP-053
title: Activity summary in Claude Desktop Chat
milestone: M5 Claude Desktop
type: feature
status: todo
priority: P2
blocked_by: [CRP-047, CRP-050]
blocks: []
model: claude-sonnet-5-5
effort: medium
size: S
---

# CRP-053: Activity summary in Claude Desktop Chat

## Goal

A user who opts in sees what they are discussing with Claude in Desktop Chat, as a short phrase, instead of only "Claude is open".

## Context

Chat gives an extension no signal about conversations, which is why Desktop support is otherwise limited to showing that the app is open. The one documented way for Chat to tell a local program anything is for the model to call a tool. [ADR-0011](../../architecture/adr/0011-model-authored-activity-summary.md) uses that.

Chat has no hooks, so there is no nudge: reliability rests on the tool description alone. Conversations in Chat are also more personal than code. So this is a separate setting, and it must be validated on a real machine before it is called done.

## Scope

- A setting `desktop_summary`, off by default, independent of the Code privacy level, exposed in the extension's configuration.
- When it is on, the Desktop adapter lists `presence_summary`, with a description written for Chat: set a phrase when a conversation's topic is clear, update it when the topic changes, keep it general, and leave it unset for anything personal, medical, financial or otherwise sensitive.
- The same sanitiser, storage and rendering as CRP-047.
- The phrase is dropped when the server's input closes. There is one Desktop session per app, so a new conversation replaces the phrase when Claude next sets one.
- Validation on a real Claude Desktop, recorded.

## Out of scope

- Detecting which conversation is in front, or when a conversation ends. There is no signal for either.
- Cowork.

## Acceptance criteria

- [ ] With the setting off, the Desktop adapter lists only `presence_status`.
- [ ] With it on, `presence_summary` is listed and a call sets the phrase on the Desktop session.
- [ ] The Code privacy level does not turn this on, and this setting does not affect Code sessions.
- [ ] The sanitiser and leak tests from CRP-047 pass for the Desktop path.
- [ ] Validation record in `docs/research/crp-053-desktop-summary.md`: in ten fresh conversations on a real Claude Desktop, how often Claude set a phrase unprompted, what the phrases were with anything sensitive redacted, what permission prompts appeared and how they were answered.
- [ ] The record states plainly whether the feature is reliable enough to recommend. If it is not, the documentation describes it as experimental, or the setting is removed and this ticket is closed `not-needed` with the reason.
- [ ] The user documentation explains what is published, that a stale phrase can remain after the conversation moves on, and how to turn it off.

## Notes for the implementer

- Claude Desktop asks the user to approve a tool the first time it is used. Record what that looks like and whether "always allow" persists.
- A phrase can outlive its conversation, because nothing tells us the conversation ended. Consider expiring it after a period without a new call, and record the choice.
- The validation steps need the owner's machine. Ask the owner to run them.

## Why this model and effort

A small extension of CRP-047, with the real uncertainty in validation, not in code.

## References

- [ADR-0011](../../architecture/adr/0011-model-authored-activity-summary.md)
- [Viability: what Claude Desktop Chat offers instead](../../architecture/viability.md#what-claude-desktop-chat-offers-instead)
