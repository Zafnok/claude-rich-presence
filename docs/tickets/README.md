# Tickets

The work plan. One file per ticket, grouped by milestone. Each ticket states what blocks it, what it blocks, its acceptance criteria, and which Claude model and effort level to run it with.

## How to use this directory

1. Pick a ticket whose `blocked_by` tickets are all `done`.
2. Follow the `work-ticket` skill.
3. In the pull request that completes it, set the `status` in the ticket's own file to `done` and move the file to [done/](done/).

Closed tickets, `done` or `not-needed`, live in [done/](done/). Every ticket still in a milestone directory is open, so the milestone directories are the list of what is left.

To see every ticket's status:

```bash
grep -rH "^status:" docs/tickets --include="CRP-*.md"
```

## Working in parallel

`blocked_by` is complete. If every ticket listed there is `done` on `main`, the ticket can be started and finished with nothing else. A ticket with an empty list can start now, alongside any other.

A ticket may mention a ticket that is not in its `blocked_by`. Such a mention is information only. It says what to do whether or not the other ticket has landed, and never requires the other ticket's output. If you find a ticket that needs something from a ticket it does not list, that is a defect in the ticket. Fix the ticket; do not wait.

| Topic | Rule |
|---|---|
| Claiming | Before starting, check that no open pull request, remote branch or local worktree exists for the ticket. Then open a draft pull request at once. The open pull request is the claim |
| Status | A ticket's status lives only in its own file. The index below has no status column, so parallel tickets do not collide on this page. Closing a ticket changes only the link in its own index row |
| Other tickets | Do not edit a ticket that is `in-progress` or `done`. If your findings affect one, write a follow-up ticket |
| Shared files | Tickets in the same wave are independent in what they need, but some add to the same files: the domain and protocol types, the renderer, the plugin's hook file. Rebase on `main` before merging and expect small conflicts there |
| Spikes | A spike's prototype is pushed to a branch named `spike/crp-NNN` and never merged. A later ticket that builds on it is blocked by the spike and reads that branch |
| The owner | CRP-001, CRP-002, CRP-003 and CRP-070 can all start at once, but each needs steps on the owner's machine or accounts. Agents can prepare them in parallel. The owner's time is the one thing that does not parallelise |

New tickets follow [TEMPLATE.md](TEMPLATE.md) and the `write-ticket` skill. Ids are never reused.

## Status values

| Status | Meaning |
|---|---|
| `todo` | Not started |
| `in-progress` | Someone has a branch |
| `blocked` | Started, then stopped on something outside the ticket. The ticket says what |
| `done` | Merged, definition of done met. The file is in `done/` |
| `conditional` | Built only if a named condition occurs. Otherwise closed as `not-needed` |
| `not-needed` | Closed without being built. The file is in `done/` |

## Milestones

| Milestone | Outcome |
|---|---|
| [M0 Foundation](M0-foundation/) | Open questions answered, repository and gates in place |
| [M1 Core](M1-core/) | Sessions, rendering, configuration and scheduling, all pure and fully tested |
| [M2 Discord](M2-discord/) | A working, resilient Discord connection |
| [M3 Host](M3-host/) | Election, failover, the control channel, the command line |
| [M4 Claude Code](M4-claude-code/) | The plugin works end to end in Claude Code |
| [M5 Claude Desktop](M5-claude-desktop/) | The extension works in Claude Desktop |
| [M6 Release](M6-release/) | A published, documented, reviewed first release |
| [M7 Personalisation](M7-personalisation/) | The card is laid out around the summary, speaks in a chosen personality, and carries optional extras |

## Index

Size is rough effort for the recommended model: S is under an hour of agent time, M is a few hours, L is a day or more including review.

| Id | Title | Blocked by | Model | Effort | Size |
|---|---|---|---|---|---|
| [CRP-001](M0-foundation/CRP-001-spike-claude-code-adapter.md) | Spike: Claude Code adapter wiring | none | Opus 5.5 | high | M |
| [CRP-002](done/CRP-002-spike-desktop-extension.md) | Spike: Claude Desktop extension lifecycle | none | Opus 5.5 | high | M |
| [CRP-003](M0-foundation/CRP-003-naming-branding-discord-app.md) | Owner: naming, branding, Discord application | none | Owner, with Haiku 4.5 | n/a | S |
| [CRP-004](M0-foundation/CRP-004-repo-scaffolding.md) | Repository scaffolding and toolchain | none | Sonnet 5.5 | low | S |
| [CRP-005](M0-foundation/CRP-005-ci-pipeline.md) | CI pipeline and the coverage gate | 004 | Sonnet 5.5 | medium | M |
| [CRP-006](M0-foundation/CRP-006-sonarqube-cloud.md) | SonarQube Cloud integration | 005 | Sonnet 5.5 | low | S |
| [CRP-007](M0-foundation/CRP-007-supply-chain-policy.md) | Supply-chain policy enforcement | 005 | Sonnet 5.5 | low | S |
| [CRP-010](M1-core/CRP-010-domain-model.md) | Domain model: events, sessions, registry | 004, 005 | Sonnet 5.5 | high | M |
| [CRP-011](M1-core/CRP-011-presence-renderer.md) | Presence renderer | 010 | Sonnet 5.5 | medium | M |
| [CRP-012](M1-core/CRP-012-configuration.md) | Configuration | 004, 005 | Sonnet 5.5 | medium | M |
| [CRP-013](M1-core/CRP-013-update-scheduler.md) | Update scheduler | 004, 005 | Sonnet 5.5 | high | S |
| [CRP-014](M1-core/CRP-014-project-profiles.md) | Project profiles in configuration | 012 | Sonnet 5.5 | medium | M |
| [CRP-020](M2-discord/CRP-020-discord-codec.md) | Discord IPC codec | 004, 005, 010 | Sonnet 5.5 | medium | S |
| [CRP-021](M2-discord/CRP-021-discord-transport.md) | Discord transport: pipe and socket dialers | 004, 005, 022 | Opus 5.5 | high | M |
| [CRP-022](M2-discord/CRP-022-fake-discord-server.md) | Fake Discord IPC server for tests | 004, 005 | Sonnet 5.5 | high | M |
| [CRP-023](M2-discord/CRP-023-discord-session-manager.md) | Discord session manager | 013, 020, 021, 022 | Opus 5.5 | high | M |
| [CRP-030](M3-host/CRP-030-control-protocol.md) | Control protocol | 010 | Sonnet 5.5 | medium | S |
| [CRP-031](M3-host/CRP-031-control-transport.md) | Control transport and the host lock | 004, 005 | Opus 5.5 | high | M |
| [CRP-032](M3-host/CRP-032-presence-host.md) | Presence host: election, serving, failover | 010, 011, 023, 030, 031 | Opus 5.5 | xhigh | L |
| [CRP-033](M3-host/CRP-033-cli.md) | Command line and composition root | 012, 032, 034, 041 | Sonnet 5.5 | medium | M |
| [CRP-034](M3-host/CRP-034-diagnostics.md) | Logging and diagnostics | 012 | Sonnet 5.5 | medium | S |
| [CRP-040](M4-claude-code/CRP-040-mcp-server.md) | Minimal MCP stdio server | 004, 005 | Sonnet 5.5 | high | M |
| [CRP-041](M4-claude-code/CRP-041-code-adapter.md) | Claude Code adapter | 001, 010, 012, 040 | Sonnet 5.5 | high | M |
| [CRP-042](M4-claude-code/CRP-042-plugin-packaging.md) | Plugin and marketplace packaging | 001, 033, 051 | Sonnet 5.5 | medium | S |
| [CRP-043](M4-claude-code/CRP-043-end-to-end-tests.md) | End-to-end tests | 022, 033 | Opus 5.5 | high | L |
| [CRP-044](M4-claude-code/CRP-044-fallback-command-hooks.md) | Fallback: command hooks and standalone host | 001, 002, 043 | Opus 5.5 | high | L |
| [CRP-045](M4-claude-code/CRP-045-spike-initial-model.md) | Spike: status line bridge and the model at launch | 042 | Sonnet 5.5 | medium | M |
| [CRP-046](M4-claude-code/CRP-046-spike-activity-summary.md) | Spike: activity summary written by Claude | 001 | Sonnet 5.5 | high | M |
| [CRP-047](M4-claude-code/CRP-047-activity-summary.md) | Activity summary in Claude Code | 014, 042, 046 | Sonnet 5.5 | high | M |
| [CRP-048](M4-claude-code/CRP-048-repository-link.md) | Repository link for opted-in projects | 014, 042 | Sonnet 5.5 | high | M |
| [CRP-049](M4-claude-code/CRP-049-shared-area-rollup.md) | Shared-area roll-up across sessions | 047 | Sonnet 5.5 | medium | S |
| [CRP-050](M5-claude-desktop/CRP-050-desktop-adapter.md) | Claude Desktop adapter | 002, 033, 043 | Sonnet 5.5 | medium | S |
| [CRP-051](M5-claude-desktop/CRP-051-mcpb-bundle.md) | MCPB bundle | 001, 033 | Sonnet 5.5 | medium | S |
| [CRP-052](M5-claude-desktop/CRP-052-desktop-validation.md) | Desktop validation on real machines | 002, 042, 050, 051 | Owner, with Sonnet 5.5 | low | S |
| [CRP-053](M5-claude-desktop/CRP-053-desktop-summary.md) | Activity summary in Claude Desktop Chat | 047, 050 | Sonnet 5.5 | medium | S |
| [CRP-060](M6-release/CRP-060-release-pipeline.md) | Release pipeline | 003, 005, 007, 042, 043, 051 | Sonnet 5.5 | high | M |
| [CRP-061](M6-release/CRP-061-user-documentation.md) | User documentation | 003, 042, 052 | Haiku 4.5 | n/a | S |
| [CRP-062](M6-release/CRP-062-security-review.md) | Security review and threat model | 043 | Opus 5.5 | high | S |
| [CRP-063](M6-release/CRP-063-code-signing.md) | Code signing and notarisation | 002, 052, 060 | Owner, with Sonnet 5.5 | low | S |
| [CRP-064](M6-release/CRP-064-directory-submission.md) | Directory submission | 003, 060, 061 | Owner, with Haiku 4.5 | n/a | S |
| [CRP-070](M7-personalisation/CRP-070-spike-discord-display.md) | Spike: how Discord displays an activity | none | Sonnet 5.5 | medium | S |
| [CRP-071](M7-personalisation/CRP-071-summary-first-layout.md) | Summary-first card layout | 047, 070 | Sonnet 5.5 | high | M |
| [CRP-072](M7-personalisation/CRP-072-personalities.md) | Personalities | 047 | Sonnet 5.5 | medium | M |
| [CRP-073](M7-personalisation/CRP-073-hide-pause-preview.md) | Hide, pause and preview | 014, 042 | Sonnet 5.5 | medium | S |
| [CRP-074](M7-personalisation/CRP-074-status-line-bridge.md) | Status line bridge | 045, 071 | Sonnet 5.5 | high | M |
| [CRP-075](M7-personalisation/CRP-075-moments.md) | Moments: just shipped | 042 | Sonnet 5.5 | medium | S |
| [CRP-076](M7-personalisation/CRP-076-spike-wsl.md) | Spike: Claude Code in WSL with Discord on Windows | 033 | Opus 5.5 | high | M |

## Dependency graph

An arrow means "must be done before".

```mermaid
flowchart TD
    C001["001 spike: Code wiring"]
    C002["002 spike: Desktop"]
    C003["003 owner: naming"]
    C004["004 scaffolding"]
    C005["005 CI"]
    C006["006 Sonar"]
    C007["007 supply chain"]
    C010["010 domain"]
    C011["011 renderer"]
    C012["012 config"]
    C013["013 scheduler"]
    C014["014 project profiles"]
    C020["020 codec"]
    C021["021 transport"]
    C022["022 fake Discord"]
    C023["023 Discord session"]
    C030["030 control protocol"]
    C031["031 control transport"]
    C032["032 presence host"]
    C033["033 CLI"]
    C034["034 diagnostics"]
    C040["040 MCP server"]
    C041["041 Code adapter"]
    C042["042 plugin"]
    C043["043 end to end"]
    C044["044 fallback, conditional"]
    C045["045 spike: status line"]
    C046["046 spike: summary"]
    C047["047 summary, Code"]
    C048["048 repository link"]
    C049["049 shared-area roll-up"]
    C050["050 Desktop adapter"]
    C051["051 MCPB bundle"]
    C052["052 Desktop validation"]
    C053["053 summary, Desktop"]
    C060["060 release"]
    C061["061 user docs"]
    C062["062 security review"]
    C063["063 signing"]
    C064["064 directory"]
    C070["070 spike: Discord display"]
    C071["071 summary-first layout"]
    C072["072 personalities"]
    C073["073 hide, pause, preview"]
    C074["074 status line bridge"]
    C075["075 moments"]
    C076["076 spike: WSL"]
    C004 --> C005
    C005 --> C006
    C005 --> C007
    C004 --> C010
    C005 --> C010
    C010 --> C011
    C004 --> C012
    C005 --> C012
    C004 --> C013
    C005 --> C013
    C012 --> C014
    C004 --> C020
    C005 --> C020
    C010 --> C020
    C004 --> C021
    C005 --> C021
    C022 --> C021
    C004 --> C022
    C005 --> C022
    C013 --> C023
    C020 --> C023
    C021 --> C023
    C022 --> C023
    C010 --> C030
    C004 --> C031
    C005 --> C031
    C010 --> C032
    C011 --> C032
    C023 --> C032
    C030 --> C032
    C031 --> C032
    C012 --> C033
    C032 --> C033
    C034 --> C033
    C041 --> C033
    C012 --> C034
    C004 --> C040
    C005 --> C040
    C001 --> C041
    C010 --> C041
    C012 --> C041
    C040 --> C041
    C001 --> C042
    C033 --> C042
    C051 --> C042
    C022 --> C043
    C033 --> C043
    C001 -.-> C044
    C002 -.-> C044
    C043 -.-> C044
    C042 --> C045
    C001 --> C046
    C014 --> C047
    C042 --> C047
    C046 --> C047
    C014 --> C048
    C042 --> C048
    C047 --> C049
    C002 --> C050
    C033 --> C050
    C043 --> C050
    C001 --> C051
    C033 --> C051
    C002 --> C052
    C042 --> C052
    C050 --> C052
    C051 --> C052
    C047 --> C053
    C050 --> C053
    C003 --> C060
    C005 --> C060
    C007 --> C060
    C042 --> C060
    C043 --> C060
    C051 --> C060
    C003 --> C061
    C042 --> C061
    C052 --> C061
    C043 --> C062
    C002 --> C063
    C052 --> C063
    C060 --> C063
    C003 --> C064
    C060 --> C064
    C061 --> C064
    C047 --> C071
    C070 --> C071
    C047 --> C072
    C014 --> C073
    C042 --> C073
    C045 --> C074
    C071 --> C074
    C042 --> C075
    C033 --> C076
```

## Suggested order

Tickets in the same wave have no dependencies on each other and can run in parallel.

| Wave | Tickets |
|---|---|
| 0 | 001, 002, 003, 004, 070 |
| 1 | 005, 046 |
| 2 | 006, 007, 010, 012, 013, 022, 031, 040 |
| 3 | 011, 014, 020, 021, 030, 034, 041 |
| 4 | 023 |
| 5 | 032 |
| 6 | 033 |
| 7 | 043, 051, 076 |
| 8 | 042, 044 only if CRP-001 failed, 050, 062 |
| 9 | 045, 047, 048, 052, 060, 073, 075 |
| 10 | 049, 053, 061, 063, 071, 072 |
| 11 | 064, 074 |

The critical path is 004, 005, 022, 021, 023, 032, 033, 051, 042, 060. CI (005) comes before every code ticket, because their definition of done needs it. The visibility features, which are project profiles, the activity summary and the repository link (014, 046, 047, 048, 049, 053), are deliberately off it: the first release does not wait for them. Nor does anything in M7. The spikes and the owner task in wave 0 have the longest lead time and involve people, so start them first.

## Choosing a model and effort

Prices per million tokens, input and output, as listed on 2026-09-25. Check current prices before budgeting.

| Model | Input | Output | Use it for |
|---|---|---|---|
| Haiku 4.5 | $1 | $5 | Prose and mechanical edits from a complete specification. It has no effort setting |
| Sonnet 5.5 | $2 | $10 | The default. Well-specified implementation with tests |
| Opus 5.5 | $4 | $20 | Concurrency, cross-platform I/O, failure handling, security, and anything where being wrong is found late |
| Fable 5.1 | $10 | $50 | Not needed for any ticket here |

Effort levels are `low`, `medium`, `high`, `xhigh` and `max`.

| Effort | Use it for |
|---|---|
| `low` | Configuration and boilerplate where the ticket leaves no decisions |
| `medium` | Ordinary implementation against clear acceptance criteria |
| `high` | State machines, protocol code, test harnesses, anything with subtle edge cases |
| `xhigh` | Only CRP-032, where election and failover races are the main risk |
| `max` | Not used |

Rules for keeping cost down without losing quality:

1. **Do not upgrade a model to compensate for a vague ticket.** Fix the ticket.
2. **Escalate on evidence.** If a ticket at its recommended setting fails review twice for the same kind of reason, rerun at the next effort level, then the next model.
3. **Keep the session focused on one ticket.** Load only the documents the ticket links.
4. **Review is cheaper than rework.** Run `/code-review` on every pull request. For the Opus tickets, review with Opus as well.
5. **Spikes are time-boxed.** A spike that has not answered its questions in its box stops and reports what it found.

By this plan thirty-two tickets run on Sonnet 5.5, ten on Opus 5.5 (one of them conditional), one on Haiku 4.5, and four are owner tasks with light model assistance.

## Owner actions

These need a person with the owner's accounts. Nothing else in the plan does.

| Action | Ticket |
|---|---|
| Decide names; create the Discord application and upload artwork | CRP-003 |
| Create the SonarQube Cloud organisation and project; add the `SONAR_TOKEN` secret | CRP-006 |
| Run the spike prototypes inside real Claude Code and Claude Desktop on Windows, and on a Mac if one is available | CRP-001, CRP-002 |
| Confirm presence in a real Discord client on real machines | CRP-052 |
| Look at test activities from a second Discord account and record what is visible | CRP-070 |
| Approve the built-in personalities and their wording | CRP-072 |
| Decide whether to pay for signing and notarisation | CRP-063 |
| Submit to directories | CRP-064 |
