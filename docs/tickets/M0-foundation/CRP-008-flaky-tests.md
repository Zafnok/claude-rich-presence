---
id: CRP-008
title: Fix two flaky tests in the scheduler and the session manager
milestone: M0 Foundation
type: chore
status: todo
priority: P1
blocked_by: [CRP-005, CRP-013, CRP-023, CRP-024]
blocks: []
model: claude-opus-5-5
effort: high
size: S
---

# CRP-008: Fix two flaky tests in the scheduler and the session manager

## Goal

`go test -race` passes every time, on all three operating systems, so that a red CI run always means a real fault and nobody learns to rerun it.

## Context

On the first two CI runs of the pull request for [CRP-051](done/CRP-051-mcpb-bundle.md), which changed no code in either package, two tests failed and then passed on a rerun:

| Test | Package | What happened |
|---|---|---|
| `TestRefusedUpdateIsDeliveredAgainWhenSubmittedAgain` | `internal/schedule` | Windows, race detector: the test ran for 30 seconds and failed |
| `TestStopWhileReadyClearsBeforeClosing` | `internal/discord/session` | Linux, race detector: failed after 0.00 seconds |

A third was seen on 2026-10-05, on the pull request for [CRP-062](../done/CRP-062-security-review.md), which changed no product code: `TestAnIntervalThatIsNotPositiveIsTheDiscordLimit/-1s` in `internal/discord/session`, on Linux with the race detector, failed after 10 seconds with "waited 10s for 2 set-activity events, have 1". It is in scope here as a neighbouring test of the same file.

A different test failed on each operating system, and the change under test was unrelated. That points to a timing assumption in the tests or a race in the code, not to a platform bug. Both packages use a fake clock and, for the manager, the fake Discord server.

## Scope

- Reproduce each failure, with `-race -count` high enough to see it, and with the CPU limited if that helps (`GOMAXPROCS=1` or `-cpu 1,2,4`).
- Find the cause of each. Say in the pull request whether it is in the test (a missing wait for a goroutine, a real sleep, an unsynchronised read) or in the code under test.
- Fix the cause. A fix in the code under test needs a test that fails without it.
- Check the neighbouring tests of the same two files for the same pattern, and fix what is found there.

## Out of scope

- Retrying failed tests in CI, or raising timeouts, which hides the fault.
- Changing the behaviour of the scheduler or the manager beyond what a real fault needs.

## Acceptance criteria

- [ ] `go test -race -count=200 ./internal/schedule ./internal/discord/session` passes on Linux, and on Windows if a fault was seen there.
- [ ] The pull request names the cause of each of the two failures, with the evidence that it was the cause.
- [ ] If the cause is in the code under test, a test fails without the fix.
- [ ] No test sleeps for a real duration to wait for another goroutine.
- [ ] Statement coverage stays at 100.0%.

## Notes for the implementer

- The failure logs of the two CI runs are gone after a while. Do not rely on them; reproduce.
- A 30 second failure is the test's own timeout waiting for something that never happened, which usually means a lost wake-up, not a slow machine.
- The `tdd-full-coverage` skill says how tests here handle time and goroutines.

## Why this model and effort

Finding a race from a failure that rarely reproduces is concurrency investigation, where a missed cause means the flake returns.

## References

- [CRP-013](done/CRP-013-update-scheduler.md), [CRP-023](done/CRP-023-discord-session-manager.md), [CRP-024](done/CRP-024-prompt-first-activity.md)
- [Quality strategy](../../architecture/quality-strategy.md)
