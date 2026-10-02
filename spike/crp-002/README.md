# CRP-002 prototype

Throwaway code for [CRP-002](../../docs/tickets/M0-foundation/CRP-002-spike-desktop-extension.md). It lives on the branch `spike/crp-002` and is never merged. Findings are in `docs/research/crp-002-desktop-extension.md` on `main`.

It is not production code and does not follow the repository's rules for production code: no tests, no coverage, direct system calls.

## What it is

One Go binary, standard library only, with these roles:

| Command | What it does |
|---|---|
| `crp002-spike mcp` | The MCP server Claude Desktop starts from the bundle. Answers `initialize`, `ping`, `tools/list` and `tools/call`, and logs every method name it receives |
| `crp002-spike peer [--once]` | The same probes without MCP, for running from a terminal |
| `crp002-spike mark <text>` | Appends a marker line to the log |
| `crp002-spike pack <manifest> <out.mcpb> <binary>...` | Writes the bundle, so the Node.js `mcpb` tool is not needed |

Every process appends JSON lines to `~/crp-002-spike/spike.log`. On start it records its arguments, executable, working directory, environment variable names (values only for an allowlist, never anything that looks like a secret or an identifier), parent process chain, package identity, job object limits, console window state and where each path really resolves. It then:

- **Control channel (B7).** For each candidate directory (`%LOCALAPPDATA%`, `%TEMP%` and a home-directory folder on Windows), tries to take an exclusive lock on `host.lock`. The holder listens on `ctl.sock`; everyone else connects to it and exchanges process ids. This is ADR-0005 and ADR-0006 in miniature.
- **Discord (B12).** Opens `discord-ipc-0` to `9`. If an application id is configured, sends a handshake and logs only the reply's opcode, command, event and close code.
- **Lifetime (B1 to B3, B8).** Writes a heartbeat every five seconds, logs when its input closes and when it exits, and acts on trigger files in `~/crp-002-spike/triggers`: `exit-0`, `exit-1`, `discord`, `stop-peer`. With the `linger` option on, it stays alive for 90 seconds after its input closes, to see whether something else ends it.

The manifest's `user_config` has one option of each type, to see how each reaches the server (B6).

## Build

Needs Go. On Windows:

```powershell
powershell -ExecutionPolicy Bypass -File spike\crp-002\build.ps1
```

This writes `crp002-spike.exe`, `crp002-spike.mcpb` and `runbook.ps1` to `~/crp-002-spike`. The bundle carries a Windows binary and an Apple silicon binary, and is given a Mark of the Web, as a downloaded release would have.

## Run

From a PowerShell window opened from the Start menu, not from a terminal inside Claude:

```powershell
powershell -ExecutionPolicy Bypass -File "$env:USERPROFILE\crp-002-spike\runbook.ps1"
```

The runbook asks for each action in Claude Desktop, records it in the log, and shows which servers are alive after each step.

The first run, on 2026-10-02, left out every step that needs Claude Desktop to be quit. Those are a second, shorter list of about ten minutes:

```powershell
powershell -ExecutionPolicy Bypass -File "$env:USERPROFILE\crp-002-spike\runbook.ps1" -Later
```

The raw logs stay on the owner's machine. They are not committed, because they hold local paths.

## Clean up

Uninstall the extension in Claude Desktop, then delete `~/crp-002-spike`, `~/.rich-presence-crp002`, `%TEMP%\rich-presence-crp002`, `%LOCALAPPDATA%\rich-presence-crp002` and `%LOCALAPPDATA%\Packages\Claude_*\LocalCache\Local\rich-presence-crp002`.
