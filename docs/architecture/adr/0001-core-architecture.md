# ADR-0001: One binary, a presence host with thin adapters

## Status

Accepted.

## Context

- Discord presence needs one long-lived connection per user ([viability](../viability.md), D2).
- Claude signals arrive from several places with different shapes: hook events in Claude Code, process lifetime in Claude Desktop.
- A user may have many sessions open at once. The development machine had more than thirty Claude Code processes running during this assessment.
- Vendor interfaces on both sides change often. Hooks, plugin manifests and MCPB have all gained and changed fields within the past year.
- The owner requires 100% test coverage.

## Decision

Build a single executable with a ports-and-adapters structure.

1. **A pure core.** The session state machine, the presence renderer, the update scheduler and both wire codecs are functions of their inputs. They import no operating-system packages and are tested without one.
2. **Ports for everything outside.** The clock, the Discord pipe, the control socket, the host lock, the file system and the standard streams are each a small interface defined by the code that uses it. Each has one real implementation per platform and a fake.
3. **Inbound adapters translate, and do nothing else.** An adapter turns one surface's signals into the core's event type. It holds no policy. Adding a surface means adding an adapter.
4. **One composition root.** Only the CLI package constructs real implementations and wires them together.
5. **One process per user owns Discord.** That is the presence host ([ADR-0005](0005-presence-host-election.md)). All sessions feed it.

Dependencies point inward: adapters and transports depend on the core; the core depends on nothing of ours.

## Consequences

- Vendor changes are contained. A new hook field touches one adapter. A Discord protocol change touches one package.
- Every branch is reachable from a test, because every external effect can be faked, including its failures.
- The fallback wiring in [ADR-0007](0007-integration-and-distribution.md) can be adopted without touching the core.
- More small interfaces and more wiring than a direct implementation would need. We accept this as the price of testability.
- One binary means one thing to build, sign, version and ship, and no version skew between parts except across concurrent processes, which the control protocol handles ([ADR-0006](0006-control-channel.md)).

## Alternatives considered

| Alternative | Why not |
|---|---|
| Each session opens its own Discord connection | Discord shows one activity per application. Several connections with the same application id compete, and rate limits apply to each. No way to summarise sessions |
| A separate daemon binary plus a client binary | Two artifacts to ship and keep in step, for no gain over one binary with two roles |
| A script-based plugin with no compiled core | Needs a runtime or shell the user may not have. See [viability](../viability.md), route 1 |
| A direct implementation without ports | Simpler to write, but failure paths in pipe, socket and lock handling could not be covered |
