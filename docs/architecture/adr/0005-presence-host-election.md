# ADR-0005: The presence host is elected among adapter processes

## Status

Accepted. The decision assumes the wiring of [ADR-0007](0007-integration-and-distribution.md), in which every session has a long-lived adapter process. If CRP-001 forces the fallback wiring, this ADR is amended as described under "If the fallback is adopted".

## Context

One process per user must own the Discord connection ([ADR-0001](0001-core-architecture.md)). The usual answer, used by every prior project, is a detached background daemon started by the first hook and stopped by a timer or a reference count.

That answer has known problems:

- **Stuck presence.** Reference counts drift when a session is killed without its end hook. One prior project has this as an open bug on Windows.
- **Per-platform detachment.** Starting a process that survives its parent differs on each operating system and is awkward to test.
- **Job objects.** Claude Desktop on Windows runs an extension's server in a job whose limits include kill-on-close ([observed in CRP-002](../../research/crp-002-desktop-extension.md)). A daemon started from inside it may be killed when the app closes, even while terminal sessions still need it. The design below does not depend on this.
- **A lingering process.** A background program that the user did not start and cannot see draws suspicion from users and from security software.

In the chosen wiring every Claude Code session already runs one adapter process for exactly as long as it lives. Claude Desktop runs two for as long as the extension is enabled, of which one reports ([ADR-0007](0007-integration-and-distribution.md)).

## Decision

There is no daemon. One of the adapter processes acts as host.

1. **Election by lock.** On start, an adapter tries to take an exclusive lock on a per-user lock file, using a lock that the operating system releases when the process dies: `flock` on Unix, an exclusive-share open on Windows. Both are in the standard library.
2. **The holder is the host.** It removes any stale socket file, listens on the control socket ([ADR-0006](0006-control-channel.md)), connects to Discord, and serves its own session through the same in-process interface followers use over the socket.
3. **Everyone else is a follower.** It connects to the control socket and forwards events.
4. **Liveness is the connection.** A session exists on the host exactly while its follower's connection is open. There are no reference counts and no process-id polling.
5. **Followers hold their own truth.** Each keeps the current state of its session. On losing the host it retries the lock with jittered backoff. The winner becomes host; the others reconnect and resend state.
6. **The host's state is disposable.** It can always be rebuilt from followers, so the host never persists anything.
7. **Version skew.** A follower newer than the host asks it to stand down. The host releases the lock and continues as a follower, and the newer binary wins the election.

## Consequences

- When the last Claude session closes, nothing is left running.
- A crashed or killed session disappears from presence as soon as its socket closes.
- No process spawning, no detachment code, no idle timers.
- Failover produces a short gap in presence while the new host handshakes with Discord. The elapsed timer is preserved because the start time travels with session state.
- Each session costs one idle process of a few megabytes. With dozens of sessions that is tens of megabytes in total, small beside the sessions themselves.
- Election and failover are the hardest code in the project to get right. They get the strongest model and effort in the plan ([CRP-032](../../tickets/done/CRP-032-presence-host.md)) and dedicated end-to-end tests ([CRP-043](../../tickets/M4-claude-code/CRP-043-end-to-end-tests.md)).
- If the lock file and socket are not visible to all adapters, there will be two hosts and two Discord connections. This degrades to a duplicated or flickering activity, never to a blocked session. [CRP-002](../../research/crp-002-desktop-extension.md) found exactly this at the first Windows location, under Claude Desktop's packaging, and [ADR-0006](0006-control-channel.md) moved the location.

## If the fallback is adopted

With command hooks there is no long-lived adapter in Claude Code, so a standalone host is needed. The same host code runs as a `daemon` subcommand, started on demand by the hook command and exiting after a period with no sessions. Sessions are then tracked by parent process id with periodic liveness checks, because there is no connection to watch. That work is [CRP-044](../../tickets/done/CRP-044-fallback-command-hooks.md) and is not built unless needed.

## Alternatives considered

| Alternative | Why not |
|---|---|
| Detached daemon | The problems listed in context |
| A service installed at login | Changes system configuration, needs an installer and an uninstaller, and runs when Claude is not in use |
| Each adapter connects to Discord itself | Competing activities, no summary, rate limits per connection ([ADR-0001](0001-core-architecture.md)) |
| Election by binding the socket alone | Removing a stale socket file races between two starters: the second can delete the first's fresh socket. A lock makes cleanup safe |
