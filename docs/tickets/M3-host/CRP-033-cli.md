---
id: CRP-033
title: Command line and composition root
milestone: M3 Host
type: feature
status: todo
priority: P0
blocked_by: [CRP-012, CRP-032, CRP-034, CRP-041]
blocks: [CRP-042, CRP-043, CRP-050, CRP-051, CRP-076]
model: claude-sonnet-5-5
effort: medium
size: M
---

# CRP-033: Command line and composition root

## Goal

The `rich-presence` executable does something: it wires the real implementations together and exposes the commands Claude and users run.

## Context

`internal/cli` is the only package allowed to construct real operating-system implementations ([ADR-0001](../../architecture/adr/0001-core-architecture.md)). Everything below it is already tested against fakes. This ticket is mostly wiring, and the wiring is covered by running the built binary.

## Scope

Commands:

| Command | Behaviour |
|---|---|
| `mcp` | Run as a stdio MCP server: load configuration, start a host node, attach the adapter for the detected surface, serve until input closes |
| `status` | Ask the host for its summary and print it. Exit 0 if a host answered, 3 if none is running |
| `doctor` | Run the diagnostic checks from CRP-034 and print a report. Exit 0 if all pass, 1 otherwise |
| `version` | Print version and build information |
| `help`, no command, or an unknown command | Print usage. Unknown command exits 2 |

- `Run(args, stdin, stdout, stderr, env) int` is the entry point. `main` only calls it.
- In `mcp` mode, standard output carries protocol messages only. Everything else goes to the log.
- When `enabled` is false, or `CLAUDE_CODE_REMOTE` is `true`, `mcp` still serves MCP correctly but starts no host node and publishes nothing.
- On input closing or a termination signal, shut down in order and exit 0.
- If the Claude Code adapter's options accept a resolver of per-directory settings when this ticket starts, which CRP-077 adds, pass one built from `Config.Effective` and the running operating system. If they do not, pass nothing; CRP-077 adds it here.
- The surface is chosen from the client name in the MCP `initialize` request. Until CRP-050 lands, an unrecognised client is treated as Claude Code.

## Out of scope

- The adapters and the node themselves.
- The Claude Desktop adapter, which CRP-050 adds to this wiring.
- `hook` and `daemon` commands, which belong to CRP-044.

## Acceptance criteria

- [ ] Each command's output and exit code is tested through `Run` with in-memory streams.
- [ ] `mcp` with `enabled` false, and with the remote environment variable set, answers `initialize` and `tools/list` and creates neither a lock file nor a socket.
- [ ] `mcp` exits 0 within one second of its input closing, with presence cleared.
- [ ] Nothing other than protocol messages is ever written to standard output in `mcp` mode, including on panics and startup errors. Shown by a test that provokes each.
- [ ] `status` prints the same fields the `presence_status` tool returns.
- [ ] The only place real clocks, sockets, locks, files and dialers are constructed is this package.
- [ ] The built binary is exercised for every command by at least one end-to-end run, so `main` and the wiring are covered.

## Notes for the implementer

- Keep argument parsing to the standard library's flag handling. There are only a handful of commands.
- Version skew is detected by comparing binary versions, so the version must be meaningful in development builds too.

## Why this model and effort

Wiring and conventional command handling over components that already exist.

## References

- [ADR-0001](../../architecture/adr/0001-core-architecture.md), [ADR-0007](../../architecture/adr/0007-integration-and-distribution.md), [ADR-0008](../../architecture/adr/0008-privacy-and-safety-by-default.md)
- [Quality strategy: reaching 100% in Go](../../architecture/quality-strategy.md#reaching-100-in-go)
