---
id: CRP-040
title: Minimal MCP stdio server
milestone: M4 Claude Code
type: feature
status: todo
priority: P0
blocked_by: [CRP-004]
blocks: [CRP-041]
model: claude-sonnet-5-5
effort: high
size: M
---

# CRP-040: Minimal MCP stdio server

## Goal

A small, strict implementation of the part of the Model Context Protocol we need: a server over standard streams that exposes a fixed set of tools.

## Context

We implement this subset ourselves ([ADR-0004](../../architecture/adr/0004-in-house-protocol-implementations.md)). It is the process Claude starts and talks to, so its robustness is our robustness: a malformed message must not kill it, and a slow handler must not stall it.

## Scope

Package `internal/mcp`:

- JSON-RPC 2.0 over two byte streams, one message per line.
- Requests handled: `initialize`, `ping`, `tools/list`, `tools/call`.
- Notifications accepted: `notifications/initialized`, `notifications/cancelled`. Other notifications are ignored.
- `initialize` negotiates the protocol version: answer with the client's requested version if we support it, otherwise with the latest we support. It records the client's name and version and passes them to a callback.
- Server capabilities advertise tools only.
- Tools are registered with a name, a description, an input schema and a handler. The set may depend on the client, decided after `initialize`.
- `tools/call` validates the tool name, runs the handler, and returns its result. A handler panic is recovered and returned as a tool error; the server keeps running.
- Errors: parse error for invalid JSON, invalid request for a bad envelope, method not found, invalid params. Standard codes.
- A maximum line length. A longer line is answered with an error and skipped.
- When input ends, call a shutdown callback and return.

## Out of scope

- Resources, prompts, sampling, roots, logging, progress, and the HTTP transport.
- The tools themselves, which are CRP-041 and CRP-050.

## Acceptance criteria

- [ ] A scripted session of `initialize`, `initialized`, `tools/list`, `tools/call` and end of input produces the expected responses, compared against golden files.
- [ ] Requests before `initialize`, other than `ping`, are rejected with the proper error.
- [ ] Each error case returns the right JSON-RPC code and the server continues to serve.
- [ ] A notification never produces a response.
- [ ] Request ids are echoed exactly, whether numbers or strings.
- [ ] A handler panic produces a tool error result and does not end the server.
- [ ] Responses are written whole and never interleaved, with concurrent handlers. Tested under the race detector.
- [ ] Nothing but protocol messages is written to the output stream.
- [ ] End of input triggers the shutdown callback exactly once.
- [ ] A fuzz test on the message decoder runs clean.
- [ ] The package documentation states the supported protocol versions and the supported subset.

## Notes for the implementer

- Check the current MCP specification for the protocol version strings, the `initialize` shapes and the tool result format before writing golden files. Record which revision was used.
- Decide whether handlers run inline or concurrently, and document it. Our handlers return immediately, so inline is acceptable and simpler.
- Before starting, check the official MCP Go SDK's license and dependency list and note them in the pull request. If the subset here grows, that SDK is the upgrade path named in ADR-0004.

## Why this model and effort

Protocol code with many edge cases, each simple, all of which must be right.

## References

- [ADR-0004](../../architecture/adr/0004-in-house-protocol-implementations.md)
- MCP specification: https://modelcontextprotocol.io/specification
