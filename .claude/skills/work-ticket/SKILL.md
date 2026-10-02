---
name: work-ticket
description: Start, carry out and finish a ticket from docs/tickets in this repository, including spikes and owner tasks. Use whenever asked to work on, pick up, implement, continue or close a CRP ticket, or asked what to work on next.
---

# Work a ticket

All work in this repository happens against a ticket in `docs/tickets/`. This is the procedure.

## 1. Choose and check

1. If no ticket was named, open `docs/tickets/README.md` and pick the lowest-numbered `todo` ticket in the earliest wave whose `blocked_by` tickets are all `done`.
2. Read the ticket in full. Read every document it links under References. Do not read the rest of `docs/` unless the ticket points there.
3. Confirm each `blocked_by` ticket has `status: done` in its own file on `main`. If one does not, stop and say which. If the list is empty or all are done, the ticket can start now, whatever else is in progress: `blocked_by` is complete. If the ticket turns out to need the output of a ticket it does not list, that is a defect in the ticket. Stop and report it. Do not wait for the other ticket and do not guess at its output.
4. If the ticket is `conditional`, check that its condition has occurred. If it has not, do not build it.
5. Check the ticket's `model` and `effort` against the session you are in. If you are running on a weaker setting than recommended, say so before starting. If the ticket is an owner task, your role is to prepare and to record, not to act on the owner's accounts.

## 2. Before writing code

1. Check nobody else has the ticket: no open pull request, no remote branch and no local worktree whose name contains its id.
2. Branch from `main`. Name the branch `crp-NNN-short-slug`, or keep the name the app gave the session if it contains the ticket id.
3. Set the ticket's `status` to `in-progress` in its own file, push, and open a draft pull request at once. The open pull request is the claim. Do not edit the index in `docs/tickets/README.md`; it has no status column.
4. If the ticket's notes say to verify something against live documentation, do that now. Vendor interfaces change. Record what you checked and the date in the pull request.
5. If anything in the ticket contradicts an accepted ADR, the ADR wins. Stop and raise it.
6. If the ticket leaves a real decision open, use the `record-decision` skill. Do not decide silently.

## 3. Implement

1. Follow the `tdd-full-coverage` skill: test first, 100% statement coverage, no exclusions.
2. Stay inside the ticket's scope. If you find work that belongs elsewhere, note it for a new ticket with the `write-ticket` skill instead of doing it. Do not edit another ticket that is `in-progress` or `done`; if your findings affect one, write a follow-up ticket.
3. Add no dependency without the `add-dependency` skill.
4. Touching Discord code: load the `discord-ipc` skill. Touching hooks, the plugin, MCP or the extension: load the `claude-surfaces` skill.

## 4. Spikes are different

A ticket with `type: spike` produces knowledge, not code.

1. Prototype on a branch named `spike/crp-NNN`. Push it, so that later tickets and reviewers can read it. It is never merged. Build your own prototype unless the ticket's `blocked_by` names the spike whose branch you are to extend.
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
5. Set the ticket's `status` to `done` in its own file, in the same pull request. Do not edit the index.
6. Open one pull request per ticket. Title: `CRP-NNN: ticket title`. Body: what changed, how each criterion was verified, what was not verified.

## If you get stuck

Set `status: blocked`, add a short "Blocked" section to the ticket saying on what, and report. Do not weaken an acceptance criterion to get past it.
