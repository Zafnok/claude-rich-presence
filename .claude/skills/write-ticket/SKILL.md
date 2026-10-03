---
name: write-ticket
description: Create, split or amend a ticket in docs/tickets for this repository. Use when new work is discovered, when a ticket is too large, when a spike's findings change later tickets, or when asked to add something to the plan or backlog.
---

# Write a ticket

Tickets are the unit of work and the unit of cost. A good ticket lets a cheaper model succeed. A vague one makes an expensive model guess.

## Procedure

1. **Check it is not already covered.** Search `docs/tickets/` for the topic, including the closed tickets in `docs/tickets/done/`.
2. **Take the next free id.** Ids are `CRP-NNN`, grouped by milestone in tens: M0 is 001 to 009, M1 is 010 to 019, and so on. Use the next unused number in the milestone's range. Never reuse an id. Ids of closed tickets in `docs/tickets/done/` count as used.
3. **Copy `docs/tickets/TEMPLATE.md`** into the milestone directory as `CRP-NNN-short-slug.md`.
4. **Fill every section.** See the rules below.
5. **Set dependencies both ways.** Add the new id to `blocks` in each ticket it depends on, and to `blocked_by` in each ticket that depends on it. `blocked_by` must be complete: list every ticket whose merged output this one needs in order to be started and finished, including the test helpers, tools and CI it relies on. People run tickets in parallel on the strength of this field.
6. **Update `docs/tickets/README.md`:** the index row, the dependency graph, and the wave table. The index has no status column; status lives in the ticket's own file.
7. **Check the graph has no cycle.** Walk `blocked_by` from the new ticket; you must not arrive back at it.
8. **Check for hidden dependencies.** Find every other ticket id the ticket mentions. Each must be upstream of it through `blocked_by`, or downstream through `blocks`, or a mention that says what to do whether or not that ticket has landed. "Reuse the prototype from CRP-NNN", "use the helper from CRP-NNN" and "update CRP-NNN" are dependencies in disguise: add the blocker, or rewrite the sentence so the ticket stands alone.
9. **Check for shared definitions.** If two tickets that can run at the same time both need a type, a helper or a directory, name the one ticket that creates it and make the other depend on that ticket.

## Rules for each section

| Section | Rule |
|---|---|
| Goal | What exists afterwards and why it matters, in one or two sentences. Not a list of tasks |
| Context | Only what the linked documents do not say. Link ADRs, do not restate them |
| Scope | Concrete items. If an item needs a decision, either make it here or say who makes it |
| Out of scope | Name the neighbouring work and the ticket that owns it |
| Acceptance criteria | Each one observable and independent: a test passes, a command prints something, a file exists. No "works correctly", no "is robust" |
| Notes | Pitfalls, and what to verify against live documentation first |
| Why this model and effort | One sentence that would let someone disagree |

## Sizing

One ticket is one pull request. Split when any of these is true:

- more than about ten acceptance criteria;
- more than one package's worth of new behaviour;
- part of it could be done by a cheaper model than the rest;
- part of it needs the owner and part does not.

## Choosing model and effort

Use the table in `docs/tickets/README.md`. In short:

| Work | Model | Effort |
|---|---|---|
| Prose or mechanical edits from complete material | Haiku 4.5 | n/a |
| Configuration and boilerplate with no open decisions | Sonnet 5.5 | low |
| Ordinary implementation with clear criteria | Sonnet 5.5 | medium |
| State machines, protocol code, test harnesses | Sonnet 5.5 | high |
| Concurrency, cross-platform I/O, security, investigation | Opus 5.5 | high |

Default to the cheaper option and make the ticket precise enough for it. Reserve `xhigh` for work where a subtle error would be found late and cost a milestone.

## Amending an existing ticket

- Changing scope or criteria of a ticket that is `todo`: edit it, and say why in the pull request.
- A ticket that is `in-progress` or `done`: do not rewrite it. Add a follow-up ticket.
- A ticket that is no longer needed: set `status: not-needed` with a one-line reason and move it to `docs/tickets/done/`, as "Closing a ticket" in the `work-ticket` skill describes. Do not delete the file.
