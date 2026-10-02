---
id: CRP-043
title: End-to-end tests
milestone: M4 Claude Code
type: feature
status: todo
priority: P0
blocked_by: [CRP-022, CRP-033]
blocks: [CRP-060, CRP-062]
model: claude-opus-5-5
effort: high
size: L
---

# CRP-043: End-to-end tests

## Goal

Prove the built binary behaves as the architecture says, as real processes on a real operating system, with only Claude and Discord replaced by fakes.

## Context

Unit tests cover each package against fakes. What they cannot show is that real processes elect a host, survive each other's deaths, keep to the rate limit, and return to Claude quickly. These tests do, and they are also how `main` and the composition root reach 100% coverage.

## Scope

- `internal/testutil/mcpclient`: a scripted MCP client that starts the binary, performs `initialize` with a chosen client name, calls tools, and closes input.
- `test/e2e`: tests that build the binary with coverage instrumentation once, start one or more copies with an isolated runtime directory and a fake Discord server, and assert on what the fake server recorded.
- Coverage from these runs merged into the per-platform profile, using the mechanism from CRP-005.

Scenarios:

| # | Scenario | Asserts |
|---|---|---|
| E1 | One session: start, prompt, tool, stop, end | The expected sequence of activities, then a clear |
| E2 | Rapid events | No two updates closer than the minimum interval, and the last state is shown |
| E3 | Two sessions, one working and one idle | The working one is in focus and the count is shown |
| E4 | Host exits cleanly while a follower lives | Presence returns from the follower with the original start time |
| E5 | Host is killed | Same as E4 |
| E6 | Follower is killed | Its session disappears and the host's remains |
| E7 | Discord absent at start, appears later | Presence appears without restarting anything |
| E8 | Discord restarts | Presence is restored |
| E9 | Privacy | For each level, marker strings seeded in every forbidden field appear nowhere: not at the fake Discord, not on the control socket, not in the log |
| E10 | Latency | The `presence_event` call returns within the budget, including while Discord is unreachable |
| E11 | Disabled, and remote environment | MCP is served, and no lock, socket or Discord connection is created |
| E12 | Version skew | A newer binary takes over hosting from an older one |
| E13 | Claude Desktop client name | Once CRP-050 lands: the session shows as Desktop, and `presence_event` is not listed |
| E14 | Every command | `status`, `doctor`, `version`, usage, each run once against the built binary |

## Out of scope

- A real Claude or a real Discord. Those are the manual checks in CRP-042 and CRP-052.

## Acceptance criteria

- [ ] E1 to E12 and E14 pass on Linux, macOS and Windows in CI. E13 is added by CRP-050.
- [ ] Each test uses its own runtime directory and its own Discord endpoint name, so tests run in parallel and leave nothing behind.
- [ ] Time-dependent scenarios run with a shortened minimum interval set through configuration, so the suite finishes in under two minutes per operating system.
- [ ] E10 asserts a budget of 10 milliseconds at the 99th percentile on a developer machine and a relaxed bound in CI, both stated in the test, with the reason for the relaxation.
- [ ] E9 captures control-socket traffic by acting as the host in the test.
- [ ] With these runs merged, coverage of `cmd/rich-presence` and `internal/cli` is 100.0% on each operating system.
- [ ] A failed scenario prints the fake Discord server's record and the binary's log.
- [ ] No test relies on a fixed sleep to wait for a state. Each polls a condition with a deadline.

## Notes for the implementer

- E12 needs two builds that report different versions. Build the same source twice with different version settings.
- Killing a process differs by operating system. Wrap it once in the test helper.
- For E5 on Windows, confirm the lock is released when the process is terminated, not only when it exits normally.
- If a scenario reveals a design gap rather than a bug, stop and raise it. Do not weaken the assertion.

## Why this model and effort

Multi-process, cross-platform, timing-sensitive tests that must be reliable. A flaky suite here would be worse than none.

## References

- [Architecture: failure behaviour](../../architecture/README.md#failure-behaviour)
- [Quality strategy](../../architecture/quality-strategy.md)
- [ADR-0005](../../architecture/adr/0005-presence-host-election.md), [ADR-0008](../../architecture/adr/0008-privacy-and-safety-by-default.md)
- Go coverage for integration tests: https://go.dev/doc/build-cover
