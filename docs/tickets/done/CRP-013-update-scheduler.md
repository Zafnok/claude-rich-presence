---
id: CRP-013
title: Update scheduler
milestone: M1 Core
type: feature
status: done
priority: P0
blocked_by: [CRP-004, CRP-005]
blocks: [CRP-023]
model: claude-sonnet-5-5
effort: high
size: S
---

# CRP-013: Update scheduler

## Goal

A component that takes a stream of desired activities and emits them to Discord no faster than the rate limit allows, always ending on the latest one.

## Context

Discord publishes two rate limits for activity updates: five per 20 seconds, and one per 15 seconds. We design for the stricter. Session state changes many times a minute, so most intermediate states will never be shown (R13). What matters is that the first change is immediate and the last change always arrives.

## Scope

Package `internal/schedule`, plus a fake clock in `internal/testutil/fakeclock`:

- Accept a desired activity, or "show nothing", at any time, without blocking.
- If no update was sent within the minimum interval, emit immediately.
- Otherwise remember only the most recent desired value and emit it when the interval has passed.
- Never emit a value equal to the last one emitted.
- "Show nothing" is an update like any other and obeys the same rule.
- A reset operation, used after a Discord reconnect, that forgets the last emitted value so the current one is sent again, while still respecting the interval.
- The clock and timers come from an injected interface.
- The scheduler is generic over any comparable value. It does not import the domain package, so it does not depend on CRP-010.

## Out of scope

- Sending to Discord, which is CRP-023.
- Deciding what the activity is, which is CRP-011.

## Acceptance criteria

- [x] The first submission after a quiet period is emitted with no delay.
- [x] A burst of submissions within the interval results in exactly two emissions: the first immediately and the last at the interval boundary.
- [x] Submitting the value already shown emits nothing.
- [x] Submitting A, then B, then A again within the interval emits nothing at the boundary, because the pending value equals the shown one.
- [x] After a reset, the current value is emitted again, no earlier than the interval allows.
- [x] Stopping the scheduler releases its timer and leaves no goroutine running, verified in a test.
- [x] All tests use the fake clock. None sleeps.
- [x] The tests pass under the race detector with submissions from several goroutines.

## Notes for the implementer

- Decide and document whether emissions are delivered by callback or channel. Whichever is chosen, a slow consumer must not block a submitter.
- The fake clock belongs in `internal/testutil` because the Discord session manager and the host need it too.

## Why this model and effort

Little code, but timing logic with a concurrency surface, where off-by-one behaviour at the boundary is the whole point.

## References

- [Viability: Discord constraints](../../architecture/viability.md#what-discord-rich-presence-requires), D3
- [Risk register](../../architecture/risks.md), R13
- Rate limit discussion: https://github.com/discord/discord-api-docs/issues/668
