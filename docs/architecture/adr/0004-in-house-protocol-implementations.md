# ADR-0004: Implement the Discord IPC client and the MCP stdio subset ourselves

## Status

Accepted. The consequence "No third-party code in the binary" is superseded for Windows by [ADR-0017](0017-go-winio-for-windows-pipes.md).

## Context

We speak two external protocols.

**Discord IPC.** A frame is two little-endian 32-bit integers, an opcode and a length, followed by JSON. There are five opcodes. We need the handshake, one command (`SET_ACTIVITY`), and parsing of the ready, error and close responses. Discord's own library documents the whole thing in one short page. Third-party libraries are all single-maintainer ([ADR-0003](0003-license-and-dependency-policy.md)).

**MCP over stdio.** JSON-RPC 2.0, one message per line. As a server exposing at most one tool we need `initialize`, the `initialized` notification, `ping`, `tools/list`, `tools/call`, and a clean exit when input closes. An official Go SDK exists and is organisation-maintained, so it would pass the dependency policy, but it is a general framework with its own dependency tree, for a need of a few hundred lines.

## Decision

1. Implement the Discord IPC codec, transport and session logic in this repository, against Discord's documentation.
2. Implement the minimal MCP stdio server subset in this repository, against the MCP specification.
3. Each protocol lives in its own package, with the wire format isolated from behaviour, a fuzz test on every decoder, and its supported subset stated in the package documentation.
4. Every decoder enforces a maximum message size and rejects, without crashing, anything it does not understand.
5. Revisit point 2 if we ever need MCP features beyond tools, such as resources, prompts, sampling or the HTTP transport. At that point adopting the official SDK is the right move and needs only a superseding ADR.

## Consequences

- No third-party code in the binary.
- We own protocol drift. The `discord-ipc` and `claude-surfaces` skills carry the references and the re-verification steps, and the end-to-end tests run against fakes that encode our understanding.
- The fake Discord server ([CRP-022](../../tickets/done/CRP-022-fake-discord-server.md)) is a second implementation of the framing, which guards against the codec agreeing only with itself. It must be written from the documentation, not by reusing the codec.
- Behaviour we rely on that the documentation does not promise is listed in [risks.md](../risks.md): setting an activity without OAuth, and the stricter of two published rate limits.

## Alternatives considered

| Alternative | Why not |
|---|---|
| A third-party Discord library | Single maintainers, stale transitive dependencies |
| Discord's Social SDK | Closed binary, restrictive redistribution terms, needs cgo |
| The official MCP Go SDK now | Passes policy, but adds a dependency tree to replace a few hundred lines we can cover completely. Kept as the upgrade path |
