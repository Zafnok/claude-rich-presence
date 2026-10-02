---
name: work-ticket
description: Start, carry out and finish a ticket from docs/tickets in this repository, including spikes and owner tasks. Use whenever asked to work on, pick up, implement, continue or close a CRP ticket, or asked what to work on next.
---

# Work a ticket

All work in this repository happens against a ticket in `docs/tickets/`. This is the procedure.

## 1. Choose and check

1. If no ticket was named, open `docs/tickets/README.md` and pick the lowest-numbered `todo` ticket in the earliest wave whose `blocked_by` tickets are all `done`.
2. Read the ticket in full. Read every document it links under References. Do not read the rest of `docs/` unless the ticket points there.
3. Confirm each `blocked_by` ticket has `status: done` in its own file. If one does not, stop and say which.
4. If the ticket is `conditional`, check that its condition has occurred. If it has not, do not build it.
5. Check the ticket's `model` and `effort` against the session you are in. If you are running on a weaker setting than recommended, say so before starting. If the ticket is an owner task, your role is to prepare and to record, not to act on the owner's accounts.

## 2. Before writing code

1. Branch from `main` as `crp-NNN-short-slug`.
2. Set the ticket's `status` to `in-progress` and update the index row.
3. If the ticket's notes say to verify something against live documentation, do that now. Vendor interfaces change. Record what you checked and the date in the pull request.
4. If anything in the ticket contradicts an accepted ADR, the ADR wins. Stop and raise it.
5. If the ticket leaves a real decision open, use the `record-decision` skill. Do not decide silently.

## 3. Implement

1. Follow the `tdd-full-coverage` skill: test first, 100% statement coverage, no exclusions.
2. Stay inside the ticket's scope. If you find work that belongs elsewhere, note it for a new ticket with the `write-ticket` skill instead of doing it.
3. Add no dependency without the `add-dependency` skill.
4. Touching Discord code: load the `discord-ipc` skill. Touching hooks, the plugin, MCP or the extension: load the `claude-surfaces` skill.

## 4. Spikes are different

A ticket with `type: spike` produces knowledge, not code.

1. Prototype in a scratch location outside the repository, or on a branch that will never be merged.
2. Write findings to `docs/research/crp-NNN-short-slug.md`: what was run, on which operating system and versions, what was observed. Separate what you saw from what you infer.
3. Anything that could not be tested is listed as untested. Never fill a gap with an assumption.
4. Apply the consequences the ticket names: update ADR status, amend dependent tickets, open follow-ups.
5. Respect the time box. When it is spent, stop and report.
6. Steps that need the owner's machine or accounts: write the exact steps, ask the owner to run them, and record what they report.

## 5. Finish

1. Run the `quality-gate` skill.
2. Walk the acceptance criteria one by one. For each, name the test or observation that shows it. If one is not met, the ticket is not done; say so plainly.
3. If the ticket required a manual check in a real Claude or Discord, record what was run and what was seen. If you could not run it, say that and ask the owner.
4. Update documentation the change made stale.
5. Set the ticket's `status` to `done`, update the index row in `docs/tickets/README.md`, and include both in the same pull request.
6. Open one pull request per ticket. Title: `CRP-NNN: ticket title`. Body: what changed, how each criterion was verified, what was not verified.

## If you get stuck

Set `status: blocked`, add a short "Blocked" section to the ticket saying on what, and report. Do not weaken an acceptance criterion to get past it.
