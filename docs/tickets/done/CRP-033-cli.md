---
id: CRP-033
title: Command line and composition root
milestone: M3 Host
type: feature
status: done
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
- Version skew is detected by comparing binary versions, so the version must be meaningful in development builds too. How two versions are ordered is in [the control protocol](../../protocol/control.md#binary-version). A version that does not read as one, such as `(devel)` or `unknown`, never takes over from another and is never taken over from.
- The host node of CRP-032 is built with `host.New` from a `host.Config`, whose fields are ports. This ticket supplies the real ones:
  - `Acquire`: `transport.Acquire`, returning `host.ErrLocked` where it returns `transport.ErrLocked`. The lock it returns needs a small wrapper, because `host.Lock.Listen` returns a `host.Listener` whose `Accept` returns an `io.ReadWriteCloser`.
  - `Dial`: `transport.Dial` with the socket path and a timeout of a second or two.
  - `Discord`: a function that makes a new `session.Manager` each time it is called, wrapped so that `State` maps the manager's state to the protocol's: ready is `connected`, connecting is `connecting`, disconnected is `disconnected`, anything else is `unknown`. It passes `Config.MinUpdateInterval` as `session.Config.Interval` ([CRP-024](../done/CRP-024-prompt-first-activity.md)). The node calls it once per term as host, and calls `Run` on each result once.
  - `Render`: `presence.Render`. `Settings`: the idle period from the configuration.
  - `Clock`: `time.Now` and `time.AfterFunc`. `Jitter`: `rand.Float64` from `math/rand/v2`, which several goroutines may call at once; a source of your own would need a lock. `Counters` and `Logger` from `internal/diag`.
- `Node.Publish` has the signature the Claude Code adapter's `Publisher` wants. `Node.Status` returns at once from memory, as the adapter's `StatusSource` requires; map its role and Discord state to the adapter's words. For a follower it is what the host last said, so it can be a moment old.
- The `status` command is not a node. It dials, says `hello`, sends `status`, reads one `status_result` and closes, with the codec in `internal/control/protocol`.

## Why this model and effort

Wiring and conventional command handling over components that already exist.

## References

- [ADR-0001](../../architecture/adr/0001-core-architecture.md), [ADR-0007](../../architecture/adr/0007-integration-and-distribution.md), [ADR-0008](../../architecture/adr/0008-privacy-and-safety-by-default.md)
- [Quality strategy: reaching 100% in Go](../../architecture/quality-strategy.md#reaching-100-in-go)
