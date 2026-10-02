# CRP-002 findings: Claude Desktop extension lifecycle

**Status: draft.** The parts that could be run without the Claude Desktop user interface are done and recorded below. The owner's run of the runbook is pending; every answer that depends on it says so.

Ticket: [CRP-002](../tickets/M0-foundation/CRP-002-spike-desktop-extension.md). Prototype: branch [`spike/crp-002`](https://github.com/Zafnok/claude-rich-presence/tree/spike/crp-002/spike/crp-002), never merged.

## Environment

| Item | Value |
|---|---|
| Date | 2026-10-02 |
| Operating system | Windows 11 Pro 25H2, build 26200.9457 |
| Claude Desktop | 2.9939.4.0, Store package `Claude_pzs8sxrjxfjjc`, x64 |
| Claude Code, as bundled with Claude Desktop | 2.1.284 |
| Go | 1.27.0 |
| macOS | Not tested yet |

## Method

The prototype is one Go binary. As an MCP server it answers `initialize`, `ping`, `tools/list` and `tools/call`, and appends everything it observes to a log in the user's home directory: start, arguments, environment, parent process chain, package identity, job object, console window, and where each path really resolves. For B7 it plays the election of ADR-0005 in three candidate directories at once. The same binary runs as a `peer` from a terminal for comparison. Its README on the spike branch describes it in full.

Two kinds of process were compared:

- **Inside**: a process whose ancestors include Claude Desktop. So far this is a shell in the Desktop Code tab: `Claude.exe` (the Store package) → `claude.exe` (Claude Code) → `cmd.exe` → `powershell.exe` → prototype.
- **Outside**: a process with no Claude ancestor. So far this was started through WMI, so its parent is `WmiPrvSE.exe`. The owner's run repeats it from an ordinary terminal.

## Observed so far

### Claude Desktop's children see a redirected `%LOCALAPPDATA%`

1. A file written to `%LOCALAPPDATA%\crp002-probe\` from the Code-tab shell was physically created at `%LOCALAPPDATA%\Packages\Claude_pzs8sxrjxfjjc\LocalCache\Local\crp002-probe\`, as reported by `GetFinalPathNameByHandle`. Inside, it is visible at both paths.
2. That private directory already held `go-build`, `pip`, `NuGet`, `GitHub CLI` and others: caches written by tools that Code-tab sessions ran.
3. The inside process has **no package identity**: `GetCurrentPackageFullName` returns 15700. The redirection applies anyway. It is in a job object; the innermost job has no limit flags.
4. Claude Desktop's package manifest declares file-system write virtualisation with four excluded directories, all Claude's own. Anything else new under `AppData` is redirected.
5. Claude Code reports its own path under `...\Packages\Claude_pzs8sxrjxfjjc\LocalCache\Roaming\Claude\claude-code\`, so `%APPDATA%` is redirected in the same way.

Microsoft documents this for packaged apps on Windows 10 version 1903 and later: new files and folders created directly under `AppData\Local` and `AppData\Roaming` go to a private per-package location; an existing real folder is opened in place.

### B7, first result: `%LOCALAPPDATA%\rich-presence` does not work

Each process tried, in each candidate directory, to take the lock, and then either listened on the socket or connected to it.

**Inside process first, then outside:**

| Candidate | Inside process | Outside process | Result |
|---|---|---|---|
| `%LOCALAPPDATA%\rich-presence-crp002` | Lock taken, in the private package copy. `bind` on the socket failed: "An invalid argument was supplied" | Lock taken, in the real directory. Listening | **Two hosts** |
| `%TEMP%\rich-presence-crp002` | Lock taken, real path. Listening | Lock refused. Connected to the inside process | One host |
| `%USERPROFILE%\.rich-presence-crp002` | Lock taken, real path. Listening | Lock refused. Connected to the inside process | One host |

**Outside process first, private copy deleted, then inside:**

| Candidate | Outside process | Inside process | Result |
|---|---|---|---|
| `%LOCALAPPDATA%\rich-presence-crp002` | Lock taken, real path. Listening | Lock refused, correctly. `connect` failed: "An invalid argument was supplied" | **One host, unreachable from inside** |
| `%TEMP%\rich-presence-crp002` | Lock taken. Listening | Lock refused. Connected | One host |
| `%USERPROFILE%\.rich-presence-crp002` | Lock taken. Listening | Lock refused. Connected | One host |

So under `%LOCALAPPDATA%` the design fails both ways. If the inside process creates the directory, there are two hosts. If the outside process creates it, the lock is shared but an inside process can neither bind nor connect a Unix socket there.

`%TEMP%` is `%LOCALAPPDATA%\Temp`. It is not redirected, because the folder already exists for real, and Unix sockets work in it from both sides.

### Named pipes cross the boundary

A named pipe created outside was opened from inside, and one created inside was opened from outside. Data flowed both ways. This used .NET's pipe classes from PowerShell, not the prototype.

### B4, first half: what Claude Code sends

Claude Code 2.1.284, started in print mode with the prototype as its only MCP server, sent:

| Field | Value |
|---|---|
| `protocolVersion` | `2025-11-25` |
| `clientInfo.name` | `claude-code` |
| `clientInfo.title` | `Claude Code` |
| `clientInfo.version` | `2.1.284` |
| `capabilities` | `roots`, `elicitation` |

It started the server with pipes for all three standard streams and no console window.

## Inferred, not yet observed

- The extension's server, a direct child of `Claude.exe`, sees the same redirection as the Code-tab shell. The owner's run checks it.
- The redirection affects **every adapter started under Claude Desktop**, including the Claude Code plugin's adapter in a Code-tab session. The problem is wider than the desktop extension.
- The configuration directory of CRP-012 and the log directory of CRP-034 are under `AppData` and would be affected in the same way.

## Answers

| # | Question | Answer |
|---|---|---|
| B1 | When is the server started | Pending owner run |
| B2 | One server per app, window or conversation | Pending owner run |
| B3 | When is it stopped; is it restarted | Pending owner run |
| B4 | `clientInfo` from Claude Desktop | Pending owner run. Claude Code's is above |
| B5 | A server with one diagnostic tool, or none | Pending owner run |
| B6 | How `user_config` reaches the server | Pending owner run |
| B7 | Shared host | **Fails at `%LOCALAPPDATA%`** for a Code-tab process. Works at `%TEMP%` and in a home-directory folder. Pending confirmation for the extension's own server |
| B8 | Job object | Code-tab shell: in a job, no limit flags. Pending for the extension's server |
| B9 | macOS Gatekeeper | Not tested |
| B10 | Windows SmartScreen and antivirus | Pending owner run |
| B11 | Extension in the Code tab | Pending owner run |
| B12 | Discord pipe from inside | Pending owner run. Named pipes in general cross the boundary |
| B13 | Console window | Pending owner run |

## Untested

Everything marked pending, and all of macOS.

## Live documentation checked on 2026-10-02

| Source | What was confirmed |
|---|---|
| MCPB manifest specification | Manifest version `0.3`. Server type `binary`. `platform_overrides` by `win32`, `darwin`, `linux`. `user_config` types string, number, boolean, directory, file. Substitution of `${user_config.KEY}` in arguments and environment. The specification's own binary example uses a path relative to the bundle with no `${__dirname}`; the prototype uses `${__dirname}` |
| Claude's MCPB page | Install by double-click, by drag and drop, or from Settings > Extensions > Advanced settings > Install Extension. Nothing about when the server starts or stops. **The directory no longer accepts MCPB submissions**; a local server reaches the directory only inside a plugin |
| Microsoft, "Understanding how packaged desktop apps run on Windows" | The `AppData` redirection rules quoted above |
