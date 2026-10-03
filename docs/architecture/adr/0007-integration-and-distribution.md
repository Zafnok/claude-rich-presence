# ADR-0007: One MCPB bundle for both surfaces, `mcp_tool` hooks in Claude Code

## Status

**Accepted** on 2026-10-03. The fallback is not adopted.

Both spikes have reported. [CRP-001](../../tickets/done/CRP-001-spike-claude-code-adapter.md) tested the Claude Code wiring on Windows and on Linux under WSL2: [findings](../../research/crp-001-claude-code-adapter.md). [CRP-002](../../tickets/done/CRP-002-spike-desktop-extension.md) tested the Claude Desktop wiring on Windows: [findings](../../research/crp-002-desktop-extension.md). The Claude Code and Claude Desktop sections below were revised on that date to match what was observed.

Three points of the Claude Code wiring as first proposed did not survive, and each has a tested replacement, recorded in the rules below: the tool's reply, the `SessionStart` hook at launch, and the `SessionEnd` hook.

Untested: macOS on both surfaces, Linux outside WSL2, the VS Code and JetBrains extensions, and a download from GitHub Releases itself.

## Context

The binary has to reach the user's machine, be started by Claude, and receive events, on three operating systems, without requiring a shell or a language runtime.

Facts from the current documentation:

| Fact | Source |
|---|---|
| A plugin's `mcpServers` may be a path or HTTPS URL to an `.mcpb` bundle, which Claude Code downloads and extracts | Plugin manifest reference |
| MCPB supports `server.type: binary`, appends `.exe` on Windows, and allows per-operating-system overrides for `win32`, `darwin` and `linux`. It cannot distinguish CPU architecture | MCPB manifest specification |
| Hooks may be of type `mcp_tool`, calling a named tool on a plugin's server, addressed as `plugin:<plugin>:<server>`, with arguments substituted from the event | Hooks reference |
| `mcp_tool` hooks are skipped for `SessionStart` at launch and for `Setup` | Hooks reference |
| `async` is documented for command hooks only | Hooks reference |
| A hook tool's text result is treated like hook standard output | Hooks reference |
| Command hooks default to Bash, or to PowerShell on Windows without Git Bash. Exec form needs a real executable on Windows | Hooks reference |
| There is no documented way to choose a hook command or plugin file by operating system or architecture | Absence in the plugin manifest reference |
| Claude Desktop installs `.mcpb` bundles as desktop extensions | MCPB documentation |
| claude.ai and Cowork refuse a plugin that has a top-level `bin/` directory | Plugin manifest reference |

The only documented mechanism that selects a native binary per operating system without a shell is the MCPB manifest.

## Decision

### Artifact

Each release publishes one bundle, `rich-presence.mcpb`, containing:

- `server/rich-presence.exe` for Windows, `amd64`;
- `server/rich-presence-darwin`, a universal binary for both Mac architectures;
- `server/rich-presence-linux` for Linux, `amd64`;
- a manifest of server type `binary` whose `platform_overrides` select among them and whose command runs `rich-presence mcp`;
- `user_config` entries for the privacy level and optional Discord application id, passed to the server as environment variables.

Standalone binaries for all built targets, checksums and a provenance attestation are published alongside.

### Claude Code

1. This repository is its own plugin marketplace. The plugin lives in `plugin/` and contains only JSON and Markdown.
2. The plugin manifest sets `mcpServers` to the HTTPS URL of the bundle in the matching GitHub Release. The release pipeline writes that URL and the version.
3. `hooks/hooks.json` declares one `mcp_tool` hook per event in the [event table](../README.md#where-the-events-come-from). Each hook:
   - calls the tool `presence_event` on `plugin:rich-presence:presence`;
   - passes a literal event name and only the allowlisted fields for that event;
   - sets an explicit `timeout` of two seconds.
4. The tool always returns exactly one text item whose text is `{}`, a constant, and never an error. Nothing else is ever returned from `presence_event`.
5. The plugin has no top-level `bin/` directory.
6. The `SessionStart` hook carries the matcher `clear|compact`, so it does not match at launch.
7. No `SessionEnd` hook is declared. A session ends when the server's input closes or it receives an interrupt or termination signal.
8. An event has at most one handler. No event is given a second `mcp_tool` handler without measuring what it adds to the model's context.
9. The last segment of the server address is the `name` in the bundle's manifest. A test checks that the hook file and the manifest agree.
10. A release changes the plugin's `version` together with the bundle URL. A changed URL alone is ignored by Claude Code.
11. `plugin/` ignores `.mcpb-cache/`, which Claude Code writes there during local development and which records absolute paths.

Observed in CRP-001, on Windows with Claude Code 2.1.284 and on Linux with 2.1.287, and the reason for rules 4 to 11:

| Fact | Consequence |
|---|---|
| With an empty result, Claude Code adds the line `UserPromptSubmit hook success: UserPromptSubmit completed` to the model's context, once per handler, on every prompt. With the text `{}` it adds nothing. Not documented | Rules 4 and 8 |
| Text that is JSON with hook-control fields is acted on: added context reached the model, and on `Stop` it made Claude continue | Rule 4: the reply is a constant |
| The launch-time skip of a `SessionStart` hook is shown in the terminal at every start as `SessionStart:startup hook error` | Rule 6 |
| At exit, Claude Code closes the server before it runs `SessionEnd` hooks. The hook fails and an error line is written to the terminal | Rule 7 |
| `SessionStart` still arrives after `/clear`, with the new session id, and after compaction, with the model | Nothing is lost by rules 6 and 7 |
| A reference to a field that is absent arrives as an empty string and the hook succeeds | The adapter treats an empty string as absent |
| The session id in hooks changes on `/clear` while the server process stays. `CLAUDE_CODE_SESSION_ID` in the server's environment is fixed at start and wrong under `--continue` | The process is the session. The id is a label taken from the latest hook |
| A subagent's events arrive on the session's server with `agent_id` set. Claude Code also emits `SubagentStop` with an empty `agent_type` and no start, after compaction and after ordinary turns | Count only stops whose start was seen |
| A call made by the model carries `_meta` with `claudecode/toolUseId`. A call made by a hook carries no `_meta`. Not documented | `presence_event` may refuse calls that carry it. It must not depend on this for safety |
| A call made by the model needs the user's permission the first time | Affects `presence_status` and the activity summary ([ADR-0011](0011-model-authored-activity-summary.md)) |
| The bundle is downloaded when the plugin is first loaded, not when it is installed. A failed download leaves the plugin enabled with no server and no message outside the debug log | `doctor` and the user documentation must cover it |
| Installing the plugin while Claude Desktop is open starts a server in every open Code-tab session within a second | Host election must be correct under a burst of simultaneous starts |
| On Windows a killed session takes the server with it, with no chance to clean up. On Linux the server sees its input close | Nothing may depend on a clean shutdown |

An illustration of one hook entry, not a specification:

```json
{
  "type": "mcp_tool",
  "server": "plugin:rich-presence:presence",
  "tool": "presence_event",
  "input": { "event": "PreToolUse", "session_id": "${session_id}", "tool_name": "${tool_name}" },
  "timeout": 2
}
```

### Claude Desktop

The user installs the same bundle as a desktop extension.

Observed in CRP-002, on Windows with Claude Desktop 2.9939.4:

| Fact | Consequence |
|---|---|
| Claude Desktop starts **two** copies of the server at app launch, with no chat opened, and keeps both until it quits. One is initialised by a client named `claude-ai`, the other by a client named `local-agent-mode-` followed by the extension's display name | Two adapters, one app. Only one may report |
| Closing the window to the tray stops nothing. Quitting closes the server's input. A force-killed app takes its servers with it at once | The server is alive exactly while the app is. This is the lifetime the design assumed |
| A server still running two seconds after its input closes is ended | Exit promptly when input closes |
| At launch, a copy is started and has its input closed before any `initialize`, and the `claude-ai` copy proper starts two seconds later | Do nothing that others can see before `initialize` |
| Right after an install, the `claude-ai` copy ran in one test and did not in another. It ran after the next launch both times | Presence may need a restart of the app after installing |
| New conversations and Code-tab sessions start no further copies | The server is a signal for the app, not for a conversation |
| A server that exits by itself is not restarted until the extension is switched off and on, or the app is restarted | The adapter must not exit while its input is open |
| Saving the extension's settings restarts the `claude-ai` copy only. The other keeps the old values until the app is restarted | Settings are trusted only in the `claude-ai` copy. This is why the other copy does not report: it could keep showing presence after the user turned it off |
| The server is first started before the settings form is saved, and an optional setting left empty arrives as the literal text `${user_config.KEY}` | A placeholder is treated as unset |
| The extension is not attached to Code-tab sessions | The plugin's adapter is the only one in a Code-tab session |

Rules:

1. The adapter recognises the client from the MCP `initialize` request. The names are kept in one table.
2. The copy whose client is `claude-ai` reports that the app is open, and nothing more.
3. A copy whose client name starts with `local-agent-mode-` reports no session and does not stand for host. It still answers `presence_status`.
4. An adapter never exits while its input is open. Standing down as host ([ADR-0005](0005-presence-host-election.md)) releases the lock and keeps the process.
5. A setting whose value still contains `${user_config.` is treated as not set.
6. Before `initialize` arrives, an adapter does not take the host lock, connect to a host or publish anything. When its input closes it exits at once.

Not observed: macOS, Cowork, and a tool call from a Chat conversation. CRP-052 checks the last on a real install.

### Tools

| Tool | Exposed to | Purpose |
|---|---|---|
| `presence_event` | Claude Code only | Receives hook events. Returns the constant text `{}` |
| `presence_status` | Both surfaces | Read-only diagnostics: whether Discord is connected, which process is host, how many sessions. Lets a user ask Claude whether presence is working, and backs the plugin's status skill |

### Acceptance criteria for this ADR

CRP-001 had to show all of the following on Windows and on at least one of macOS or Linux. The last column is what it found on Windows and on Linux under WSL2.

| # | Criterion | Result |
|---|---|---|
| A1 | Claude Code installs the plugin, downloads the bundle from a URL, and starts the correct binary with no shell and no runtime present | Met. The URL was a local HTTPS server with a redirect, not GitHub |
| A2 | `mcp_tool` hooks deliver every event in the event table except launch-time `SessionStart`, with the substituted fields intact | Met, except `SessionEnd` at exit, which never arrives and is not needed (rule 7). Of the notification types only `idle_prompt` was provoked, on Windows |
| A3 | A field that is absent from an event does not make the hook fail in a way the user sees | Met |
| A4 | The tool's result adds nothing to Claude's context for any event, including `UserPromptSubmit` | Not met with an empty result. Met with the reply in rule 4 |
| A5 | Added latency per hooked event is under 10 milliseconds at the 99th percentile, and a hung or dead server delays Claude by no more than the hook timeout | Met. 99th percentile 5 ms on Windows and 8 ms on Linux over 256 calls each; three Linux calls took 12 to 17 ms |
| A6 | The server's process ends when the session ends, including when the session is killed | Met |
| A7 | Updating the plugin to a new bundle URL replaces the binary | Met when the plugin version changes too (rule 10) |
| A8 | The tool's presence in the model's context is small and causes no unwanted calls | Met. 78 and 59 tokens, loaded on demand; no unprompted call observed |

The rule was: if A1, A2, A4, A5 or A6 fails and cannot be worked around, adopt the fallback. A2 and A4 failed as first written and were worked around, so the fallback is not adopted.

### Fallback

Command hooks in exec form run `rich-presence hook`, reading the event from standard input with `async` set, and a standalone `rich-presence daemon` is the host ([ADR-0005](0005-presence-host-election.md)). The plugin is distributed as a release archive containing the binaries and a launcher per platform. This is what existing projects do and it is known to work, with these costs: a shell script outside coverage, reliance on undocumented executable resolution on Windows, a detached process, and process-id liveness tracking. It was ticketed as [CRP-044](../../tickets/done/CRP-044-fallback-command-hooks.md), now closed as `not-needed`. It stays described here in case a later Claude Code release breaks one of the undocumented behaviours the rules above rely on.

## Consequences

- One artifact, one code path, both surfaces.
- No shell anywhere. Privacy filtering happens inside Claude Code, before our code runs, because each hook names the fields it sends.
- The model name at launch is unavailable, since it arrives only in the skipped `SessionStart`. The model appears after the first model switch or compaction; a clear does not carry it. [CRP-045](../../tickets/M4-claude-code/CRP-045-spike-initial-model.md) looks for a better answer.
- Rules 4, 6 and 7 rest on behaviour of Claude Code that is observed and not documented. A release that changes it would show the user an error line or add text to the model's context. The end-to-end tests must detect that.
- A hung adapter costs two seconds on every hooked event, so a turn with a few tool calls would lose many seconds. The handler must be unable to block.
- Hook calls are synchronous. The latency criterion A5 and the two-second timeout are what keep [ADR-0008](0008-privacy-and-safety-by-default.md)'s first rule.
- Two small tool definitions enter the model's context in each Claude Code session, and one in Claude Desktop.
- Linux on Arm is not served by the bundle. Discord publishes no desktop client for it, so the loss is theoretical.
- Nobody had shipped this wiring before. It has now run in a prototype only.

## Alternatives considered

| Alternative | Why not the first choice |
|---|---|
| Command hooks and a launcher script, the fallback above | Needs a shell and per-platform scripts; see fallback costs |
| Require the user to install the binary on `PATH` first | Two-step install, and a missing binary fails silently in every session |
| Commit binaries into the plugin directory in git | Repository bloat on every release, and still no per-platform selection for hooks |
| Distribute through npm with per-platform optional packages | Needs npm on the user's machine |
| Download the binary from a hook on first run | Needs a shell to do the download, and adds a network fetch outside Claude Code's own installer |
