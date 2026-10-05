---
id: CRP-024
title: "Discord session manager: a prompt first activity and the configured interval"
milestone: M2 Discord
type: feature
status: done
priority: P1
blocked_by: [CRP-013, CRP-023]
blocks: [CRP-043]
model: claude-sonnet-5-5
effort: high
size: S
---

# CRP-024: Discord session manager: a prompt first activity and the configured interval

## Goal

Presence appears as soon as Discord is ready after a host starts or takes over, and the time between updates is the one in the configuration. Today the first activity of every host waits out a full interval, and the `min_update_interval` setting is read by nothing.

## Context

Found while building the presence host ([CRP-032](../done/CRP-032-presence-host.md)). Both points are in `internal/discord/session`, which [CRP-023](../done/CRP-023-discord-session-manager.md) built, and its pull request names the first as a known limit.

1. **A dropped emission counts towards the rate limit.** The manager hands every `Set` to the update scheduler at once. What the scheduler emits while Discord is not connected is dropped, but the scheduler has recorded it as sent. When the connection becomes ready the manager resets the scheduler, which then waits out the rest of the interval before sending. A presence host calls `Set` within a millisecond of taking the lock, which is always before the handshake with Discord has finished. So after every host start, and after every failover, nothing is shown for one whole interval: 15 seconds by default. The [architecture](../../architecture/README.md#the-presence-host) promises a gap of "well under a second on a local socket plus Discord's handshake".
2. **The interval is a constant.** `session.New` builds its scheduler with `schedule.DiscordInterval`. The configuration has `min_update_interval`, with a floor of 4 seconds ([configuration](../../configuration.md)), and the end-to-end tests need to shorten the interval through it ([CRP-043](../M4-claude-code/CRP-043-end-to-end-tests.md)).

Discord's limit is on what is sent to it. Something that was never written to the connection cannot have used any of it.

## Scope

- Only an activity that was written to a ready connection counts towards the interval. An emission that is dropped because there is no ready connection leaves the scheduler as if it had not been made.
- When a connection becomes ready, the current activity is sent at once, unless an activity was written to Discord, on any earlier connection, less than one interval ago. In that case it is sent when that interval has passed, as now.
- `session.Config` gains a required `Interval`, used for the scheduler in place of the constant. A value that is not positive is a programming error: `New` treats it as `schedule.DiscordInterval`.
- The package documentation of `internal/discord/session`, which describes the present behaviour under "Shape", is brought up to date.
- The composition root passes `Config.MinUpdateInterval`. If the `mcp` command of CRP-033 exists when this ticket starts, change it there. If it does not, this ticket ends at `session.Config`, and CRP-033 passes the value because the field is required.

How the scheduler learns that an emission was dropped is the implementer's choice. Two that fit the present design: the scheduler's consumer reports whether it took the value, and a refused value is not recorded as shown or as sent; or the manager holds the desired activity itself and submits it to the scheduler only while the connection is ready.

## Out of scope

- The limit itself and how updates are coalesced, which stay as [CRP-013](../done/CRP-013-update-scheduler.md) left them.
- Anything in `internal/host`. It calls `Set` and `Clear` as before.

## Acceptance criteria

Against the fake server from CRP-022, with the fake clock:

- [ ] An activity set before the first connection is ready is written to Discord as soon as the connection is ready, with the clock not moved in between.
- [ ] An activity set while Discord is absent, then changed twice more before Discord appears, results in exactly one write when it appears: the latest.
- [ ] After an activity has been written, a disconnect and a reconnect inside the interval resend it no sooner than one interval after that write. `TestReconnectsAndResendsNoFasterThanTheSchedulerAllows` passes unchanged.
- [ ] A clear obeys the same rules as an activity.
- [ ] With `Interval` set to 4 seconds, two activities set one second apart on a ready connection are written 4 seconds apart.
- [ ] No two writes of an activity are ever closer than the interval, on one connection or across connections. Shown by a test that sets, disconnects and reconnects at random under the fake clock and checks the times the fake server recorded.
- [ ] The scheduler's own acceptance criteria from CRP-013 still hold, and its tests pass. If its interface changes, the tests that change say why.
- [ ] `Set` and `Clear` still return at once in every state.

## Notes for the implementer

- `TestShowsTheActivityOnceDiscordAppears` asserts the present behaviour in its middle part, with the comment "The dropped update counted towards the rate limit". That assertion is what this ticket reverses. Change it and say so in the pull request.
- The manager's `emit` runs on the scheduler's goroutine and must not block. Whatever tells the scheduler that a value was not taken must keep that true.
- Think through a `Set` that races with the connection becoming ready. The outcome must be one write of the latest activity, whichever side wins.
- The scheduler is generic and pure. Keep it so: it must not learn what a connection is.

## Why this model and effort

A small change to a rate limiter and to one method of the manager, with precise criteria, but in code where an off-by-one in ordering sends too often or not at all.

## References

- [Architecture: talking to Discord](../../architecture/README.md#talking-to-discord)
- [Architecture: the presence host](../../architecture/README.md#the-presence-host)
- [CRP-013](../done/CRP-013-update-scheduler.md), [CRP-023](../done/CRP-023-discord-session-manager.md)
