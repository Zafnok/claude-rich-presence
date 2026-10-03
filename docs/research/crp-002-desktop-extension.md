# CRP-002 findings: Claude Desktop extension lifecycle

Ticket: [CRP-002](../tickets/done/CRP-002-spike-desktop-extension.md). Prototype: branch [`spike/crp-002`](https://github.com/Zafnok/claude-rich-presence/tree/spike/crp-002/spike/crp-002), never merged.

**Coverage.** Windows is covered, over two runs on one machine. macOS is untested. The gaps are listed under [Not tested](#not-tested).

## Answers

| # | Question | Answer | Basis |
|---|---|---|---|
| B1 | When is the server started | **At app launch**, within seconds, with no chat opened and no tool used. Also when the extension is installed, though on one of two installs only one of the two copies started until the app was next launched | Observed |
| B2 | One server per app, window or conversation | **Two per app.** Claude Desktop runs two copies of the server, one for each of two internal clients. New chats, messages and a new Code-tab session started no others. The app offered no second window | Observed |
| B3 | When is it stopped; is it restarted | It lives until the app quits. Minimising and closing the window to the tray do nothing. On quit and on uninstall the app closes the server's input, and ends a server that is still running about two seconds later. A force-killed app takes its servers with it at once. **A server that exits by itself is not restarted**, with exit code 0 or 1, until the extension is switched off and on or the app is restarted. Saving the extension's settings restarts one of the two copies only | Observed |
| B4 | `clientInfo` | `claude-ai` version `0.1.0` for one copy, `local-agent-mode-` followed by the extension's display name, version `1.0.0`, for the other. Claude Code sends `claude-code`. All three are distinguishable | Observed |
| B5 | A server with one diagnostic tool, or none | Both install and run with no warning beyond the standard notice that an extension can access everything on the computer. A tool call from Chat was never observed: on the one attempt, the copy that serves Chat was not running | Observed, with a gap |
| B6 | How `user_config` reaches the server | Substituted into the arguments and environment named in the manifest, as strings. An optional value left empty arrives as the **literal text** `${user_config.KEY}`. The server is first started before the settings form is saved | Observed |
| B7 | Shared host | **Fails at `%LOCALAPPDATA%\rich-presence`.** Works under `%TEMP%` and under a folder in the home directory, in both start orders. Named pipes also cross the boundary | Observed |
| B8 | Job object | Yes. The server is in a job whose limits include kill-on-close, and when the app was force-killed both servers were gone within 70 ms | Observed |
| B9 | macOS Gatekeeper | Not tested. Open, flagged in the risk register as R5 | |
| B10 | Unsigned binary on Windows | No SmartScreen or antivirus prompt, with Defender's real-time protection on and the bundle marked as downloaded from the internet | Observed |
| B11 | Extension in the Code tab | Not attached. A new Code-tab session saw no extension tool, and Claude Code started no copy of the server. No doubled adapter | Observed |
| B12 | Discord pipe from inside | The server opened `\\.\pipe\discord-ipc-0` every time. A handshake sent from inside Claude Desktop's process tree got the expected reply. A handshake with a real application id was not sent | Observed, with a gap |
| B13 | Console window | None seen at install or at app launch. The process has a console window that is not visible | Observed |
| | Cowork | Not tested | |

## Environment

| Item | Value |
|---|---|
| Dates | 2026-10-02 and 2026-10-03. Times below are UTC |
| Operating system | Windows 11 Pro 25H2, build 26200.9457 |
| Claude Desktop | 2.9939.4.0, Store package family `Claude_pzs8sxrjxfjjc`, x64 |
| Claude Code, as bundled with Claude Desktop | 2.1.284 |
| Discord | Desktop client, running |
| Microsoft Defender | Real-time protection on |
| Go | 1.27.0, `windows/amd64`, `CGO_ENABLED=0` |
| macOS | Not tested |

## What was run

The prototype is one Go binary, standard library only. As an MCP server it answers `initialize`, `ping`, `tools/list` and `tools/call`, and appends what it observes to a log in the home directory: start, arguments, environment, parent process chain, package identity, job object, console window, the real location of each path it opens, a heartbeat every five seconds, input closing, and exit. For B7 it plays the election of [ADR-0005](../architecture/adr/0005-presence-host-election.md) in three candidate directories at once: take the lock, then listen on the socket or connect to it. The same binary runs as a `peer` from a terminal for comparison. It was packed as an MCPB bundle of server type `binary`, manifest version `0.3`, with one `user_config` option of each kind. The bundle file was given a Mark of the Web, as a downloaded release would have.

Reading the parent process chain and the job object is spike instrumentation. The product does neither ([ADR-0008](../architecture/adr/0008-privacy-and-safety-by-default.md)).

Three kinds of process were compared:

| Name here | What it is | Parent chain |
|---|---|---|
| Extension server | The bundle's binary as Claude Desktop starts it | `Claude.exe` (the Store package) → server |
| Code-tab process | Started from a shell in a Desktop Code-tab session | `Claude.exe` → `claude.exe` (Claude Code) → `cmd.exe` → `powershell.exe` → prototype |
| Outside process | No Claude ancestor | Windows Terminal → PowerShell → prototype, and in the pre-tests `WmiPrvSE.exe` → prototype |

Three runs:

1. **Pre-tests**, by the agent, from a Code-tab session, 2026-10-02: the redirection probes, B7 in both orders between a Code-tab process and an outside process, the named pipe test, the `%APPDATA%` test, Claude Code's `clientInfo`, and a Discord handshake.
2. **First runbook**, by the owner, from Windows Terminal, 2026-10-02, 06:14 to 06:31: `runbook.ps1` on the spike branch prompted each action in Claude Desktop and recorded it in the same log. The owner installed the bundle by the install dialog, kept the default settings, typed a sample secret, and left the Discord application id empty.
3. **Second runbook**, by the owner, 2026-10-03, 04:38 to 04:48: `runbook.ps1 -Later`, the steps that need the app to be quit. Install again, a tool request in Chat, close to the tray, quit, relaunch with a terminal copy already holding the locks, force-kill, relaunch, uninstall.

Corrections the owner made to the first runbook, applied here:

- The steps that said "close the window with X", "quit Claude Desktop" and "start Claude Desktop again" were **not performed** in that run. Their recorded results are discarded. The second runbook covers them.
- Several steps that asked for a Chat conversation were done in the **Code tab** instead.

## Observations

### Start, number of servers and clients (B1, B2, B4)

In the first runbook, at 06:15:59, on completing the install dialog and before any chat was opened, two servers started within 8 ms of each other. Both are direct children of the Claude Desktop main process. They differ only in the client that initialised them:

| | First copy | Second copy |
|---|---|---|
| `clientInfo.name` | `claude-ai` | `local-agent-mode-CRP-002 spike (throwaway)`, which is `local-agent-mode-` plus the manifest's `display_name` |
| `clientInfo.version` | `0.1.0` | `1.0.0` |
| `protocolVersion` | `2025-11-25` | `2025-11-25` |
| `capabilities` | `extensions` with `io.modelcontextprotocol/ui` | The same, plus `roots` with `listChanged` |
| After `initialize` | `notifications/initialized`, then `tools/list` | The same |
| Later messages | None | `notifications/roots/list_changed`, three times |

For comparison, Claude Code 2.1.284 in print mode sent `clientInfo.name` `claude-code`, title `Claude Code`, version `2.1.284`, protocol `2025-11-25`, capabilities `roots` and `elicitation`.

Between 06:17 and 06:22 the owner opened a new conversation, sent messages, opened a second conversation and started a new Code-tab session. No further server started. No `ping` and no `tools/call` reached either server at any point in the run.

The three `roots/list_changed` notifications arrived within seconds of the owner sending a message or starting a session. The prototype logged the method name only. This is a signal about activity, which the project does not use ([ADR-0008](../architecture/adr/0008-privacy-and-safety-by-default.md)).

**The second install behaved differently.** In the second runbook, completing the install dialog started only the `local-agent-mode` copy. It was closed nine seconds later and started again nine seconds after that. No `claude-ai` copy ran for the six minutes until the app was quit. In that time the owner saved the settings form, which restarted nothing, and asked Claude in a Chat conversation to call the tool; Claude answered that there was no such tool, although the extension was listed and enabled. Why the two installs differed is not known.

**App launch**, observed twice in the second runbook, with no chat opened:

| Time after the first server appears | What happened |
|---|---|
| 0 | The `local-agent-mode` copy starts and is initialised |
| About 10 ms | Another copy starts. Its input is closed at once, before any `initialize` |
| About 2 s | That copy is gone. It was set to stay alive after input closes, so the app ended it |
| About 2.1 s | The `claude-ai` copy starts and is initialised |

Both launches gave the same sequence, to within 50 ms. After a launch both copies had the saved settings, including the one changed while only the stale copy was running.

### Stop and restart (B3)

| Event | What Claude Desktop did |
|---|---|
| Window minimised for 15 s | Nothing. Both servers kept running |
| Window closed with X, app still in the tray, for 26 s | Nothing. The server kept running |
| App quit from the tray | The server's input was closed. This server was not set to stay alive, and exited by itself 56 ms later. No app process was left |
| App force-killed | Both servers were gone within 70 ms, with no input-closed event. They were set to stay alive after input closes, so they did not exit by themselves. No app process was left |
| Both servers exited by themselves with code 0 | Not restarted in the 116 s before the owner intervened. The app showed "MCP server disconnected". A tool call in that time failed with "Tool execution failed"; it did not restart the server |
| Extension switched off and on | Both copies started again |
| Both servers exited by themselves with code 1 | The same: "server disconnected", no restart in 54 s, both started again when switched off and on |
| Settings form saved, twice | The `claude-ai` copy had its input closed and a replacement started about 90 ms later with the new values. **The `local-agent-mode` copy was not restarted** and kept the old values |
| Extension uninstalled | Both copies had their input closed, in the first runbook within 20 ms of each other and in the second 2 s apart. A copy set to stay alive after input closes was ended by the app: the copy connected to it saw the connection drop 2.0 s after the input closed |

A server that does not exit when its input closes is ended about two seconds later. That was timed three times: twice at launch and once at uninstall. Whether the same happens on a normal quit was not seen, because the server running at that quit exited by itself.

### Tools (B5)

With one tool listed, the install dialog said only that the extension would have access to everything on the computer. With the `claude-ai` copy restarted to list no tools, the owner saw no warning.

No tool call reached a server in any run. In the second runbook the request was made from Chat, but the `claude-ai` copy was not running then, and Claude answered that there was no such tool. In the first runbook, the one request to call the tool from a Code-tab session was answered by Claude with "There is no spike_status tool in this session". A later request, made while both servers were down, was attempted by Claude and failed, which shows the tool was listed in that conversation; which mode that conversation was in is not recorded.

### Settings (B6)

| Manifest option | How it arrived |
|---|---|
| String with a default, `minimal` | `minimal`, in both the environment variable and the argument |
| Number with a default, `42`, later changed to `7` | `42`, then `7`, as text |
| Boolean | `true` or `false`, as text |
| Optional string, no default, left empty | The literal text `${user_config.discord_client_id}` |
| Sensitive string | In the two servers started at install, before the form was saved: the literal placeholder. After saving: the value, in the environment in plain text |
| `${__dirname}` | `C:\Users\<user>\AppData\Roaming\Claude\Claude Extensions\local.mcpb.nick-wentz.crp002-spike`, which is physically under the package's private copy of `Roaming` |

Other facts about how the server is started:

- The command `${__dirname}/server/crp002-spike.exe`, from the manifest's `win32` override, was run as written, with the forward slash.
- Working directory: `C:\Windows\system32`.
- Standard input and output are pipes. Standard error is a file.
- The environment is a plain Windows user environment of 38 variables, plus the manifest's. No Claude-specific variable is set. `TEMP`, `LOCALAPPDATA` and `APPDATA` have their normal values.
- The extracted binary carries no Mark of the Web, although the bundle did.

### Redirection of `AppData` under Claude Desktop

1. A file written to `%LOCALAPPDATA%\crp002-probe\` from a Code-tab process was physically created at `%LOCALAPPDATA%\Packages\Claude_pzs8sxrjxfjjc\LocalCache\Local\crp002-probe\`, as reported by `GetFinalPathNameByHandle`. From inside it is visible at both paths. From outside it is visible only at the second.
2. The extension server's lock file under `%LOCALAPPDATA%` landed in the same private location.
3. Neither kind of process has package identity: `GetCurrentPackageFullName` returns 15700. The redirection applies anyway.
4. That private directory already held `go-build`, `pip`, `NuGet`, `GitHub CLI` and others: caches written by tools that Code-tab sessions ran.
5. Claude Desktop's package manifest declares file-system write virtualisation with four excluded directories, all Claude's own.
6. `%APPDATA%` behaves the same way. Tested with a folder holding a `config.json`:

   | Who created the folder | Result |
   |---|---|
   | An outside process first | Fully shared. The inside process read the file, overwrote it and added a new file, all at the real path, and the outside process saw every change |
   | An inside process first | **Split for good.** The folder exists only in the private copy. The outside process did not see it and created its own. The inside process kept reading its private file |

Microsoft documents the rule for packaged apps on Windows 10 version 1903 and later: new files and folders created directly under `AppData\Local` and `AppData\Roaming` go to a private per-package location, and a file that exists only at the real location is opened there without redirection.

### Shared host (B7)

Each process tried, in each candidate directory, to take the lock, and then either listened on the socket or connected to it.

**Extension servers running, then an outside process from a terminal** (the runbook):

| Candidate | Extension servers | Outside process | Result |
|---|---|---|---|
| `%LOCALAPPDATA%\rich-presence-crp002` | One took the lock, in the private copy. `bind` failed: "An invalid argument was supplied". The other could not connect, same error | Took its own lock at the real path and listened | **Two hosts, and no working socket inside** |
| `%TEMP%\rich-presence-crp002` | One took the lock at the real path and listened. The other connected to it | Lock refused. Connected to the extension server | One host |
| `%USERPROFILE%\.rich-presence-crp002` | The same | The same | One host |

**Code-tab process first, then an outside process** (pre-test): the same three results.

**Outside process from a terminal first, then the app launched** (second runbook):

| Candidate | Outside process | Extension servers | Result |
|---|---|---|---|
| `%LOCALAPPDATA%\rich-presence-crp002` | Took the lock at the real path and listened | Lock refused, correctly. `connect` failed: "An invalid argument was supplied" | **One host, unreachable from inside** |
| `%TEMP%\rich-presence-crp002` | Took the lock and listened | Lock refused. Both copies connected to the terminal process | One host |
| `%USERPROFILE%\.rich-presence-crp002` | The same | The same | One host |

When the app was force-killed, the terminal process saw both connections close within 70 ms.

**Outside process first, private copy deleted, then a Code-tab process** (pre-test):

| Candidate | Outside process | Code-tab process | Result |
|---|---|---|---|
| `%LOCALAPPDATA%\rich-presence-crp002` | Took the lock at the real path and listened | Lock refused, correctly. `connect` failed: "An invalid argument was supplied" | **One host, unreachable from inside** |
| `%TEMP%\rich-presence-crp002` | Took the lock and listened | Lock refused. Connected | One host |
| `%USERPROFILE%\.rich-presence-crp002` | Took the lock and listened | Lock refused. Connected | One host |

Failover was also seen under `%TEMP%` and the home directory: when the copy holding the lock was replaced after a settings change, the surviving copy noticed the closed connection and connected to the new holder 300 ms later.

`%TEMP%` is `%LOCALAPPDATA%\Temp`. Files created in it are not redirected, which matches the documented rule: the `Temp` folder already exists at the real location.

**Named pipes.** A pipe created outside was opened from a Code-tab process, and one created inside was opened from outside. Data flowed both ways.

### Job object (B8)

| Process | In a job | Limit flags of the innermost job |
|---|---|---|
| Extension server | Yes | `0x3c00`: kill on job close, die on unhandled exception, breakaway allowed, silent breakaway allowed |
| Code-tab process | Yes | None |
| Outside process | No | |

### Unsigned binary and console window (B10, B13)

The owner saw no SmartScreen prompt, no antivirus prompt and no console window during the install, and no console window at app launch. The server reports that it has a console window and that the window is not visible.

### Code tab (B11)

The owner started a new Code-tab session and asked for tools with "spike" in the name. Claude reported none. In the whole run no server was started by a `claude-code` client.

### Discord (B12)

Every extension server and outside process, in both runbooks, opened `\\.\pipe\discord-ipc-0` at the first attempt. With no application id configured, no handshake was sent from an extension server. A handshake sent from a Code-tab process with the deliberately invalid id `1` was answered by a close frame, opcode 2, code 4000, "Invalid Client ID". That is the reply the `discord-ipc` skill describes for a bad application id.

## Inferred, not observed

- The redirection comes from the job or container Claude Desktop's children run in, not from package identity. It therefore reaches **every adapter started under Claude Desktop, including the Claude Code plugin's adapter in a Code-tab session**. The Code-tab results above are direct evidence for that case.
- The `local-agent-mode` client serves Cowork or agent sessions. Its name and its `roots` capability suggest it. Nothing observed says which.
- The servers died with the force-killed app because of the kill-on-close job. The timing fits; the cause was not isolated.
- The `claude-ai` copy is the one that serves Chat. On the one occasion it was not running, Chat had no tool.
- A handshake with a valid application id would succeed from an extension server. The pipe opens there, and the protocol works from a Code-tab process.

## Not tested

| What | Why | Where it stays open |
|---|---|---|
| A tool call from a Chat conversation | Requested once from Chat, when the copy that serves Chat was not running | CRP-052, step V10 |
| Why the second install started only one copy | Seen once. Not investigated | CRP-052, step V1 |
| Whether a server that ignores its input closing is ended on a normal quit | The server running at the one quit exited by itself. It is ended in two seconds at launch and at uninstall | Not needed: our adapter exits when its input closes |
| A second window | The app offered none | |
| A Discord handshake with a real application id, from an extension server | No application id exists yet (CRP-003) | CRP-052, step V2 |
| Cowork | Skipped | The first release does not cover Cowork |
| Everything on macOS, including Gatekeeper (B9) and the socket location | No Mac was used | Risk register, R5 |
| Other Windows versions, and other antivirus products | One machine | CRP-052 |
| Whether the specification's bare relative command, without `${__dirname}`, works | The prototype used `${__dirname}` | CRP-051 |

## Consequences applied in the same pull request

| Finding | Change |
|---|---|
| B7 fails at `%LOCALAPPDATA%` | [ADR-0006](../architecture/adr/0006-control-channel.md): the Windows runtime directory is under `%TEMP%`. CRP-031 amended to match |
| The same redirection would split the configuration file and the logs | New [ADR-0016](../architecture/adr/0016-windows-file-locations.md): on Windows nothing of ours lives directly under `AppData`. CRP-012 and CRP-034 amended |
| After an install the `claude-ai` copy may not run until the app is restarted; at launch a copy is started and closed before the real one | ADR-0007, CRP-050 and CRP-052 |
| Two servers per app, clients named as above, no restart after a self-exit | [ADR-0007](../architecture/adr/0007-integration-and-distribution.md), Desktop section revised. [ADR-0005](../architecture/adr/0005-presence-host-election.md) clarified. CRP-050 and CRP-052 amended |
| Empty optional settings arrive as a placeholder; first start precedes the settings form; one copy keeps old settings | CRP-012, CRP-050 and CRP-051 amended |
| No doubled adapter in the Code tab | Recorded in CRP-050 |
| The directory no longer accepts MCPB submissions | CRP-064 amended |
| R3, R4 and R5 | [Risk register](../architecture/risks.md) updated |
| Facts others will look up | `claude-surfaces` skill updated |

## Live documentation checked on 2026-10-02

| Source | What was confirmed |
|---|---|
| [MCPB manifest specification](https://github.com/modelcontextprotocol/mcpb/blob/main/MANIFEST.md) | Manifest version `0.3`. Server type `binary`. `platform_overrides` by `win32`, `darwin`, `linux`. `user_config` types string, number, boolean, directory, file. Substitution of `${user_config.KEY}` in arguments and environment. Nothing about server lifetime or signing. The specification's own binary example uses a path relative to the bundle without `${__dirname}` |
| [Claude's MCPB page](https://claude.com/docs/connectors/building/mcpb) | Install by double-click, by drag and drop, or from Settings > Extensions > Advanced settings > Install Extension. Nothing about when the server starts or stops. **The directory no longer accepts MCPB submissions**; a local server reaches the directory only inside a plugin |
| [Microsoft, "Understanding how packaged desktop apps run on Windows"](https://learn.microsoft.com/en-us/windows/msix/desktop/desktop-to-uwp-behind-the-scenes) | The `AppData` redirection rule quoted above |
