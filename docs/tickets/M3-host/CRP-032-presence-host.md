---
id: CRP-032
title: "Presence host: election, serving, failover"
milestone: M3 Host
type: feature
status: todo
priority: P0
blocked_by: [CRP-010, CRP-011, CRP-023, CRP-030, CRP-031]
blocks: [CRP-033]
model: claude-opus-5-5
effort: xhigh
size: L
---

# CRP-032: Presence host: election, serving, failover

## Goal

The component every adapter embeds. It gives the adapter one non-blocking call, "publish this event", and behind that call it either is the presence host or follows one, and moves between the two roles as processes come and go.

## Context

Read [ADR-0005](../../architecture/adr/0005-presence-host-election.md) and the [presence host section](../../architecture/README.md#the-presence-host) first. This is the hardest part of the system (R10). Its correctness properties:

1. At most one host per user at a time, guaranteed by the lock, not by this code.
2. If any adapter is running, presence is eventually correct.
3. No adapter ever blocks its caller.
4. No session outlives its adapter.

## Scope

Package `internal/host`:

**Node**, the type adapters hold:

- `Publish(event)`: never blocks, never returns an error to the adapter. Updates the node's own session state, which it always keeps, and passes the event on.
- `Status()`: the diagnostic summary, answered locally if host, or by asking the host if follower.
- `Run(context)`: the role loop below. Returns when the context ends.

**Role loop**:

1. Try the lock. If acquired, become host. If not, become follower.
2. As follower: dial, send `hello`, on `welcome` send `sync`, then forward events. On `refuse`, on a failed dial, or on a dropped connection, wait a jittered backoff and go to 1.
3. As host: listen, start the Discord session manager, serve followers and the node's own session, until the context ends or a valid `stand_down` arrives. On `stand_down`, stop hosting, release the lock, and go to 1 after a delay long enough for the requester to win.

**Host behaviour**:

- Each follower connection owns the sessions it has synced or sent events for. When the connection closes, those sessions are removed.
- Every change to the registry triggers render, then schedule, then the Discord session manager.
- A timer re-renders when the idle-clear threshold would be crossed.
- A follower whose first message is not a valid `hello` is disconnected.
- A slow or stalled follower cannot delay other followers or the host's own session.
- `status` is answered without touching Discord.

**Follower behaviour**:

- A bounded outgoing queue. When full, older events are dropped and a `sync` is sent instead, since the node's own state is always current.
- After reconnecting to any host, send `sync` before anything else.
- A follower with a newer binary version than the host sends `stand_down` once, then behaves normally.

**Shutdown**: stop accepting, clear presence, close follower connections, close the listener, release the lock, in that order.

## Out of scope

- The standalone daemon mode and process-id liveness, which are CRP-044 and built only if needed.
- Any real socket, lock or Discord in unit tests. Those arrive through the ports.

## Acceptance criteria

Unit tests use in-memory implementations of the lock, listener, dialer and Discord session, and the fake clock.

- [ ] A single node becomes host and its published events reach the renderer.
- [ ] A second node becomes follower, and its session appears in the host's registry after `sync`.
- [ ] When a follower's connection closes, its sessions disappear and presence is re-rendered.
- [ ] **Failover**: with one host and two followers, when the host stops, exactly one follower becomes host, the other reconnects to it, and the new host's registry contains both remaining sessions with their original start times.
- [ ] **Host killed**: the same outcome when the host's connections are cut and its lock released without a clean shutdown.
- [ ] **Version skew**: a newer follower causes an older host to stand down, the newer node becomes host, and the older node follows it.
- [ ] A follower that cannot reach any host and cannot take the lock keeps retrying with backoff and never blocks `Publish`.
- [ ] `Publish` returns in constant time in every role and every transition, including while the outgoing queue is full. Shown by a test that calls it with all I/O stalled.
- [ ] A queue overflow results in a `sync`, and the host's view converges to the follower's state.
- [ ] An invalid first message, an oversized line, and a mid-message disconnect are each handled without affecting other followers.
- [ ] Shutdown happens in the specified order and leaves no goroutine, verified in a test.
- [ ] A randomised test runs many nodes that start, publish and stop in random order under the fake clock and asserts after each step: at most one host; when the system is quiet, the host's registry equals the union of live nodes' sessions.
- [ ] All tests pass under the race detector, repeatedly.

## Notes for the implementer

- Draw the role state machine first and put it in the package documentation. Every transition gets a test named after it.
- One goroutine owns the registry. Everything reaches it through one channel. No mutex on registry state.
- Give every goroutine an owner that waits for it. Shutdown bugs here become stuck presence or leaked processes.
- The node's own session goes through the same path as a follower's, so there is one code path to test.
- The stand-down delay must exceed the follower backoff's maximum jitter. Derive both from one constant.
- Run the randomised test with a fixed seed in CI and print the seed on failure.
- Ask for review with Opus 5.5 as well, focused on races and shutdown.

## Why this model and effort

Distributed-systems behaviour in miniature: election, failover, replay and version skew, with concurrency throughout. It is the one ticket where the highest setting in the plan is warranted, because errors here are found late and are expensive.

## References

- [ADR-0005](../../architecture/adr/0005-presence-host-election.md), [ADR-0006](../../architecture/adr/0006-control-channel.md)
- [Architecture: the presence host](../../architecture/README.md#the-presence-host)
- [Architecture: failure behaviour](../../architecture/README.md#failure-behaviour)
- [Risk register](../../architecture/risks.md), R10
