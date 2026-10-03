# CRP-001 findings: Claude Code adapter wiring

Spike for [CRP-001](../tickets/M0-foundation/CRP-001-spike-claude-code-adapter.md), run 2026-10-02 and 2026-10-03. It tests the wiring proposed in [ADR-0007](../architecture/adr/0007-integration-and-distribution.md): a native binary delivered as an MCPB bundle referenced from a plugin, receiving events through hooks of type `mcp_tool`.

Headless results cover Windows and Linux. One interactive terminal session was run on Windows by the owner. What could not be tested is listed under [Not yet tested](#not-yet-tested).

## How to read this

- **Observed** means seen in a log written by the prototype or by Claude Code during this spike.
- **Docs** means read in the vendor documentation on 2026-10-02 and not contradicted by an observation.
- **Inferred** means a conclusion drawn from observations. Each one says what it rests on.
- **Untested** means exactly that. Nothing untested is assumed.

## What was run

| | Windows | Linux |
|---|---|---|
| Operating system | Windows 11 Pro 10.0.26200, x64 | Ubuntu 24.04.4 under WSL2, kernel 6.6.87.2, x86-64 |
| Claude Code | 2.1.284, the binary Claude Desktop installs under `%APPDATA%\Claude\claude-code` | 2.1.287, native installer |
| How sessions were started | `claude -p` (headless) from Git Bash, `claude mcp list`, and one interactive session run by the owner in Windows Terminal | `claude -p` and `claude mcp list` from Bash |
| Login | Owner's claude.ai login for model turns. Install, update and start-up tests ran with no login | The same |
| Go | 1.27.0, cross-compiled from Windows for all three targets | |

macOS was not tested. Linux was tested under WSL2 only, not on a bare-metal desktop.

### The prototype

All of it lived outside the repository and is thrown away.

- **Probe**: a Go program, standard library only. As `probe mcp` it answers `initialize`, `ping`, `tools/list` and `tools/call` over standard streams and appends everything it sees to a local file: arguments, working directory, parent processes, selected environment variables, every request, and a heartbeat every five seconds. A control file switches its behaviour per run: return an empty result, return text, return hook-control JSON, return an error, never answer, or exit.
- **Bundle**: `manifest.json` (manifest version 0.3, server type `binary`, `platform_overrides` for `win32`, `darwin`, `linux`, one `user_config` option) plus the three binaries, zipped with executable mode bits set.
- **Plugin** named `rich-presence`: a manifest whose `mcpServers` is the bundle (a relative path in one variant, an HTTPS URL in the other) and a `hooks/hooks.json` declaring one `mcp_tool` hook per event in the event table, each with `timeout: 2`, calling `presence_event` on `plugin:rich-presence:presence`.
- **Field recorder**: a second plugin with command hooks that run the probe as `probe dump`. It records the names and types of the fields in each hook input, and values only for a fixed list of non-content scalars. It was enabled only for runs that needed the field table.
- **Hosting**: a local HTTPS server with a throwaway certificate, trusted through `NODE_EXTRA_CA_CERTS`. One path answers with a 302 redirect to the file, as a GitHub release asset does. Nothing was published anywhere.
- **Marketplace**: a local git repository reached through a git `insteadOf` rewrite, so that Claude Code treats it as a remote git marketplace and copies the plugin into its cache as it does for real users.

Install and update tests used an isolated configuration directory (`CLAUDE_CONFIG_DIR`). Logged-in sessions used the owner's normal configuration with the plugin loaded per session through `--plugin-dir`.

## Summary

| # | Criterion | Windows | Linux | Notes |
|---|---|---|---|---|
| A1 | Installs, downloads the bundle, starts the right binary with no shell | **Pass** | **Pass** | Download happens on first load, not at install |
| A2 | Hooks deliver every event except launch-time `SessionStart` | **Pass** for all but `SessionEnd` at exit (fails, see below). Of the notification types only `idle_prompt` was seen | Same, without `Notification` | |
| A3 | Absent field does not break the hook | **Pass** | **Pass** | Absent fields arrive as empty strings |
| A4 | Tool result adds nothing to context | **Pass** with an empty or plain-text result. **A result that is hook-control JSON is acted on** | Same | Mitigation is already the design: return empty content |
| A5 | Latency under 10 ms at p99; hung or dead server bounded by the timeout | **Pass**: p99 5 ms, max 6 ms over 256 calls. Hung: 2.000 s per hooked event. Dead: about 5 ms, then restarted | **Pass**: p99 8 ms over 256 calls, with three calls above 10 ms (12, 15, 17). Hung and dead as on Windows | |
| A6 | Server ends with the session, including a kill | **Pass** | **Pass** | Windows kill gives the server no chance to clean up |
| A7 | Updating to a new bundle URL replaces the binary | **Pass**, when the plugin version changes | **Pass** | A changed URL with an unchanged version does nothing |
| A8 | Tool definitions are small and not called unprompted | **Pass**: 78 and 59 tokens, deferred; no unprompted call in 434 observed calls | **Pass**: same sizes, no unprompted call | |

No criterion that forces the fallback (A1, A2, A4, A5, A6) has failed. Two findings need a change to the hook file, and the change was tested: `SessionEnd` at exit fails with a visible error, and the launch-time `SessionStart` skip is shown to the user as a hook error. See [the tested hook file changes](#the-tested-hook-file-changes).

## A1. Install, download, start

**What was run.** `claude plugin validate --strict`; `claude plugin marketplace add`; `claude plugin install rich-presence@<marketplace>`; `claude mcp list`; `claude --plugin-dir <dir> mcp list`. Both bundle forms: `"mcpServers": "./rp-probe.mcpb"` and `"mcpServers": "https://localhost:8443/redir/rp-probe-v1.mcpb"`.

**Observed, both platforms.**

- The plugin installs and the server is listed as `plugin:rich-presence:presence … ✔ Connected`.
- The server's name is the `name` field of the **bundle's** manifest, not anything in the plugin manifest. The hooks address `plugin:<plugin name>:<bundle manifest name>`.
- Claude Code picked the binary through `platform_overrides`: `server/rp-probe.exe` on Windows, `server/rp-probe-linux` on Linux. `${__dirname}` was substituted with the extraction directory.
- The process's parent is `claude.exe` (Windows) or `claude` (Linux) directly. No shell is in the chain.
- On Linux the extracted binaries kept mode `0755`. The bundle was zipped on Windows with the mode bits set explicitly in the archive.
- A URL bundle is **not** downloaded by `plugin install`. It is downloaded the first time the plugin's servers are loaded, with user agent `axios/1.15.2`, following the 302 redirect, into `<plugin cache>/<version>/.mcpb-cache/`. The archive is kept beside a metadata file and extracted into a directory named after its content hash.
- When the download fails (here: an untrusted certificate), the plugin still shows as enabled, `claude mcp list` says `No MCP servers configured`, and the only trace is in the debug log: `Plugin MCP server error - mcpb-download-failed`. Nothing is shown to the user by these commands.
- A local-path bundle is extracted into `.mcpb-cache/` inside the plugin directory.

**Observed, Windows only.** No console window: `GetConsoleWindow` returned null in the server process in every run. These runs had no visible parent console to inherit, so this covers the headless case. The Desktop Code tab is [not yet tested](#not-yet-tested). In the interactive Windows Terminal session there was no console window either.

**Other things seen.**

- A marketplace added from a local directory loads its plugins in place, as documented, and so writes `.mcpb-cache/` into the plugin's source directory. The cache metadata stores an **absolute** `extractedPath`. A copied or committed `.mcpb-cache/` therefore points the server at a path on another machine. `plugin/` must ignore that directory.
- `claude plugin validate --strict` accepts a plugin named `rich-presence`. It rejects a marketplace manifest with no description (`metadata.description`).
- `claude plugin details` reports `MCP servers (0)` for a plugin whose server comes from a bundle. Cosmetic.

**Not covered.** A download from GitHub Releases itself. The mechanism, including the redirect, was exercised against a local server.

## A2. Events delivered

**What was run.** Headless sessions with prompts that force each event: a successful tool, a failing tool, a subagent, `/compact`, `/clear` and `/model` over stream-JSON input, `--resume`, `--continue`.

**Observed on Windows and on Linux, with identical results.** Every hook listed below reached `presence_event` with its literal event name and substituted fields.

| Event | Delivered | Fields that arrived as declared |
|---|---|---|
| `SessionStart` at launch (`startup`, `resume`) | **No**, skipped as documented | |
| `SessionStart` after `/clear` | Yes | `source: clear`. No `model` |
| `SessionStart` after compaction | Yes | `source: compact`, `model` |
| `UserPromptSubmit` | Yes | `session_id`, `cwd` |
| `PreToolUse` | Yes | `tool_name`; `agent_id` and `agent_type` inside a subagent |
| `PostToolUse` | Yes | `tool_name` |
| `PostToolUseFailure` | Yes | `tool_name` |
| `Stop` | Yes | `stop_hook_active` arrives as the string `"false"` |
| `StopFailure` | Yes | Seen on an authentication failure |
| `PreCompact`, `PostCompact` | Yes | `trigger: manual` |
| `PostModelSwitch` | Yes | `from_model`, `to_model` |
| `SubagentStart`, `SubagentStop` | Yes | `agent_id`, `agent_type` |
| `SessionEnd` on `/clear` | Yes | `reason: clear` |
| `SessionEnd` at exit | **No**, see below | |
| `Notification` | Yes, Windows interactive | `notification_type: idle_prompt`, about 60 s after the turn ended. The permission and input types were not provoked |

Substitution details, all observed:

- `${cwd}` arrives intact on both platforms, backslashes included on Windows.
- A reference inside a longer string works: `"tool=${tool_name};"` arrived as `tool=Bash;`.
- A nested path works when present: `${effort.level}`. It was empty on a model with no effort setting.
- Non-string values are stringified: a boolean arrives as `"false"`.

### Finding: `SessionEnd` at exit does not arrive, and prints an error

In every headless run on both platforms, Claude Code closed the server before running the `SessionEnd` hook. The hook then failed, and this line was printed to the session's standard error:

```
SessionEnd hook [plugin:rich-presence:presence/presence_event] failed: MCP server "plugin:rich-presence:presence" is not connected
```

The server had already seen its input close (Windows) or received an interrupt signal (Linux) a few milliseconds earlier. The design already treats that as the end of the session, so nothing is lost by removing the `SessionEnd` hook. `SessionEnd` with `reason: clear` does arrive, but `SessionStart` with `source: clear` follows it immediately and carries the same information.

In the interactive session the same failure was logged on `/exit` (`reason: prompt_input_exit`) as `SessionEnd hook [plugin:rich-presence:presence/presence_event] failed: Not connected`, written to standard error as the program exited. The owner did not report seeing it.

**Consequence.** Do not declare a `SessionEnd` hook.

### Finding: the launch-time skip is logged as a hook error

The documented skip appears in the debug log as a warning and an error, and in stream-JSON output as a `hook_response` with `outcome: error`, `exit_code: 1`:

```
mcp_tool hooks are not available for the 'SessionStart' hook event (no MCP client context)
```

It did not affect the session. **In an interactive terminal it is shown to the user** at every start, under the welcome banner:

```
⎿  SessionStart:startup hook error
⎿  MCP server 'plugin:rich-presence:presence' not connected
```

**Consequence.** The `SessionStart` hook must not match at launch. A matcher does that.

### The tested hook file changes

A second copy of the hook file was run on Windows with two changes: the `SessionStart` entry given `"matcher": "clear|compact"`, and the `SessionEnd` entry removed. In a session with a prompt, `/clear`, `/model`, `/compact` and an exit:

- no hook reported an error; the stream showed two `SessionStart` hook responses, both `success`;
- nothing was written to standard error at exit;
- `SessionStart` with `source: clear` and with `source: compact` still arrived, the first with the new session id and the second with the model;
- every other event arrived as before.

This run was headless. That the banner no longer shows the error in an interactive terminal follows from the hook not matching at launch, and is inferred, not observed.

### Other observations that affect the design

- **`UserPromptSubmit` is not only the user.** It also fired when a background subagent finished and Claude Code fed the result back in as a new turn. For presence this is still "turn started", so the mapping holds.
- **Compaction produces a `SubagentStop` with an empty `agent_type` and no matching `SubagentStart`.** A subagent counter must not go below zero and should count only stops whose start it saw.
- **`SessionStart` at launch carried no `model` field in headless runs**, logged in or not, including through a command hook. **In the interactive session it did**: a command hook saw `model: claude-opus-5-5` with `source: startup`. An `mcp_tool` hook cannot receive that event, so this helps only a command hook. It is passed to [CRP-045](../tickets/M4-claude-code/CRP-045-spike-initial-model.md).
- **A `SubagentStop` with an empty `agent_type` and no start also followed an ordinary turn** in the interactive session, about a second after `Stop`. It is not specific to compaction.
- **A session id changes on `/clear`** while the server process stays the same. See [the server's environment](#the-servers-environment).

## A3. Absent fields

**What was run.** A second handler on `UserPromptSubmit` whose input referenced `${agent_id}` outside a subagent, `${no_such_field}` and `${no_such.nested}`. `PreToolUse` referenced `${agent_id}`, `${agent_type}` and `${effort.level}` on every call.

**Observed, both platforms.** Each absent reference arrived as an empty string. The hook ran and succeeded. Nothing was logged as an error.

**Consequence.** The adapter must treat an empty string as "absent".

## A4. Does a tool result reach Claude's context

**What was run, both platforms.** The probe returned a distinctive string from every hook call. The session ran one tool, and was then asked whether that string appeared anywhere it could see. Models: Haiku 4.5 and Sonnet 5.5. A control run returned hook-control JSON instead, to prove the model would report injected text if there were any.

| Tool result | What happened |
|---|---|
| Empty content | Nothing reached the conversation |
| Plain text | Nothing reached the conversation. Both models answered `NOTFOUND`. The stream contained the string in no user or assistant message. This held for `UserPromptSubmit` as well |
| `isError: true` with text | Non-blocking error. Text went to the debug log only. Model answered `NOTFOUND` |
| Text that is JSON with `hookSpecificOutput.additionalContext` | **Injected.** The model quoted it from `UserPromptSubmit` and from `Stop`. On `Stop` it made Claude continue, and the session looped through several extra turns |

The four rows were the same on Windows and Linux.

**Consequence.** With an empty result the criterion holds. The control run shows why the rule in ADR-0008 must be absolute: text beginning with `{` is parsed as hook output and can steer the session. The tool must never return text.

A re-fired `SessionStart` (after `/clear` and after compaction) was delivered with an empty result and the session continued normally. A marker run specifically on a re-fired `SessionStart` was not done.

## A5. Latency and failure

**What was run, both platforms.** Four sessions of thirty separate tool calls each, plugin loaded, no field recorder. Round trip measured from Claude Code's debug log: the line `Hooks: mcp_tool calling …` to the line reporting the hook's output. The log has millisecond resolution.

| | n | p50 | p90 | p99 | max |
|---|---|---|---|---|---|
| Windows: round trip seen by Claude Code | 256 | 1 ms | 1 ms | 5 ms | 6 ms |
| Windows: handling time inside the probe | 252 | 0.4 ms | | 1.5 ms | 1.7 ms |
| Linux (WSL2): round trip seen by Claude Code | 256 | 1 ms | 2 ms | 8 ms | 17 ms |
| Linux (WSL2): handling time inside the probe | 252 | 0.2 ms | | 0.4 ms | 0.5 ms |

On Linux three of the 256 calls took longer than 10 ms: 12, 15 and 17 ms. The probe's own handling time stayed under half a millisecond in every call, so the extra time was spent in Claude Code or the operating system, not in the server. The criterion is stated at the 99th percentile and holds; the tail is recorded here because WSL2 is not a bare-metal machine and the sample is small.

The probe reads a small control file on every call, which the real adapter will not do, so its handling time is an upper bound.

**Hung server, both platforms.** The hook was cancelled at 2.000 s, the configured timeout, on `UserPromptSubmit`, `PreToolUse`, `PostToolUse` and `Stop` alike. Claude Code sent `notifications/cancelled` for the request and carried on. The turn completed with the correct answer. A turn with two tool calls took about 8 s longer than normal, because each hooked event waited the full two seconds.

**Dead server, both platforms.** A call to a server that exits fails in about 5 ms with `Connection closed`, as a non-blocking error in the debug log only. In a logged-in session, Claude Code **started the server again** on the next hook call, every time, taking about 100 ms to connect on Windows: five starts in a two-tool turn on both platforms. The turn completed normally.

**Consequence.** The per-event bound holds. A hung adapter would still cost two seconds on every event, so the handler must be incapable of blocking, as CRP-041 already requires. A shorter timeout is worth considering: nothing observed needs more than a few milliseconds.

## A6. Server lifetime

**Observed on Windows.**

| Situation | What the server saw |
|---|---|
| Session start | Started once, before the first prompt, in parallel with launch-time `SessionStart` |
| Normal end of a headless session | Standard input closed. The probe exited within the same millisecond |
| `/clear` | Same process continues. Hook `session_id` changes |
| `/compact`, `/model` | Same process continues |
| `--resume <id>` | New process. Environment session id is the resumed id |
| `--continue` | New process. Environment session id differs from the id in hooks, as documented |
| Subagent | No new process. The subagent's tool events arrive on the session's server with `agent_id` set |
| `claude.exe` killed with `taskkill /F` | The server process was gone within a second, with **no** input-closed event and no further heartbeat. **Inferred**: the operating system terminated it together with its parent, most likely through a job object |

**Observed on Linux.** The table holds with two differences.

- At the normal end of a headless session the server received `SIGINT` rather than only seeing its input close.
- After `kill -9` of the `claude` process, the server saw its input close 9 ms later and exited by itself. It was not killed by the operating system.

**Consequences.**

- On Windows a killed session gives the adapter no chance to clear presence. That is acceptable: Discord clears the activity when the connection closes, and a follower takes over if the dead process was the host.
- On Linux the adapter must treat `SIGINT` and `SIGTERM` as a normal shutdown.

## A7. Updates

**What was run, both platforms.** Install at plugin version 0.0.1 pointing at bundle 1; change the marketplace; `claude plugin marketplace update`; `claude plugin update`; load.

| Change in the marketplace | Result |
|---|---|
| New bundle URL, same plugin `version` (Windows) | `already at the latest version`. The old binary keeps running |
| New bundle URL and new plugin `version` | New cache directory for the version, new bundle downloaded on first load, new binary runs |

Old version directories stayed in the cache after the update. Each holds its own copy of the bundle, about 4 MB for the probe.

**Consequence.** The release pipeline must change the plugin `version` together with the URL, which ADR-0007 already says. Bumping only the URL silently does nothing.

## A8. Context cost and unwanted calls

**Observed on both platforms, identical.**

- `/context` lists `presence_event` at 78 tokens and `presence_status` at 59 tokens, under deferred MCP tools. Only the names are in context until the model searches for them.
- When asked "is my Discord presence working", the model found `presence_status` through tool search and called it.
- **A model-initiated call needs permission.** In a headless session without an allow rule it was denied: `Claude requested permissions to use mcp__plugin_rich-presence_presence__presence_status, but you haven't granted it yet`.
- Across the Windows runs the probe received 434 tool calls. One was model-initiated, the `presence_status` call that was asked for. A Sonnet session doing unrelated work with both tools pre-approved called neither.
- **Hook calls and model calls are distinguishable.** A model-initiated call carries `_meta` with `claudecode/toolUseId` and a `progressToken`. A hook-initiated call carries no `_meta`.

**Consequences.**

- `presence_event` can refuse calls that carry `claudecode/toolUseId`, so the model cannot forge events. This is an observation of current behaviour, not a documented contract.
- The permission prompt on a model-initiated call matters for the activity summary. It is passed to [CRP-046](../tickets/M4-claude-code/CRP-046-spike-activity-summary.md).

## Per-event fields

Field names present in the hook input for each event, recorded on Windows with Claude Code 2.1.284 through command hooks. Only names and types were recorded. Fields under `tool_input` and `tool_response` vary by tool and are omitted. **Bold** fields are the ones the adapter may read.

Every event carries `session_id`, `cwd`, `hook_event_name` and `transcript_path`. `prompt_id` is present once a prompt has been submitted. The interactive session also carried `scratchpad_dir` on every event, and `effort.level` on tool events and `Stop` (`medium` arrived through `${effort.level}`).

| Event | Additional fields | Values seen |
|---|---|---|
| `SessionStart` | **`source`**; **`model`** on `compact` only; on `resume`: `context_tokens`, `estimated_cache_write_usd`, `prompt_cache_likely_expired`, `seconds_since_last_response` | `source`: `startup`, `resume`, `clear`, `compact` |
| `UserPromptSubmit` | `permission_mode`, `prompt` | |
| `PreToolUse` | **`tool_name`**, `tool_use_id`, `tool_input`, `permission_mode`; `agent_id`, `agent_type` inside a subagent | |
| `PostToolUse` | **`tool_name`**, `tool_use_id`, `tool_input`, `tool_response`, `duration_ms`, `permission_mode`; `agent_id`, `agent_type` inside a subagent | |
| `PostToolUseFailure` | **`tool_name`**, `tool_use_id`, `tool_input`, `error`, `is_interrupt`, `duration_ms`, `permission_mode` | |
| `Stop` | `stop_hook_active`, `last_assistant_message`, `background_tasks`, `session_crons`, `permission_mode`; `effort.level` on models with effort | |
| `StopFailure` | `error`, `last_assistant_message` | |
| `PreCompact` | **`trigger`**, `custom_instructions` | `manual` |
| `PostCompact` | **`trigger`**, `compact_summary` | `manual` |
| `PostModelSwitch` | **`to_model`**, `from_model`, `requested_model`, `source`, `cache_ttl`, `context_tokens`, `estimated_cache_write_usd`, `pricing`, `prompt_cache_warm` | `source`: `command` |
| `SubagentStart` | `agent_id`, `agent_type` | `general-purpose` |
| `SubagentStop` | `agent_id`, `agent_type`, `agent_transcript_path`, `stop_hook_active`, `last_assistant_message`, `background_tasks`, `session_crons`, `permission_mode` | `agent_type` empty for the compaction's own agent |
| `SessionEnd` | **`reason`** | `clear`, `other` (headless exit), `prompt_input_exit` (`/exit`) |
| `Notification` | **`notification_type`**, `message` | `idle_prompt` |

Fields that carry user content and must stay unread: `prompt`, `tool_input`, `tool_response`, `error`, `message`, `last_assistant_message`, `compact_summary`, `custom_instructions`, `transcript_path`, `agent_transcript_path`, `scratchpad_dir`.

## Other facts later tickets need

### `clientInfo`

Sent in `initialize` by Claude Code, identical on both platforms apart from the version:

```json
{ "name": "claude-code", "title": "Claude Code", "version": "2.1.284",
  "description": "Anthropic's agentic coding tool", "websiteUrl": "https://claude.com/claude-code" }
```

`protocolVersion` was `2025-11-25`. Capabilities offered: `roots` with `listChanged`, and `elicitation`. The interactive terminal session on Windows sent the same `clientInfo`. Other surfaces are listed under [Not yet tested](#not-yet-tested).

### The server's environment

| Item | Observed |
|---|---|
| Working directory | The session's working directory |
| Parent process | `claude.exe` or `claude`, directly. In the interactive session the chain above it was `cmd.exe`, `powershell.exe`, `WindowsTerminal.exe`, and no console window was created for the server |
| Standard input | A pipe on Windows, a socket on Linux |
| `CLAUDE_PLUGIN_ROOT` | The plugin's cache directory for the installed version, or the source directory for an in-place plugin |
| `CLAUDE_PLUGIN_DATA` | `<config>/plugins/data/rich-presence-<marketplace>`, or `…/rich-presence-inline` under `--plugin-dir` |
| `CLAUDE_PROJECT_DIR` | The session's project directory |
| `CLAUDE_CODE_SESSION_ID` | The session id at spawn. Equal to the hooks' `session_id` at first. Not updated on `/clear`. Wrong under `--continue`. Documented |
| `CLAUDE_CODE_ENTRYPOINT` | `sdk-cli` for `claude -p`, `cli` for the interactive terminal. A shell started by the Desktop Code tab had `claude-desktop` in its own environment; a server started from the Code tab was not observed |
| `CLAUDECODE` | `1` |
| `CLAUDE_CODE_REMOTE` | Not set in any local run |
| Also present, by name | `CLAUDE_CODE_MESSAGING_SOCKET`, `CLAUDE_CODE_MESSAGING_TOKEN`. The adapter has no use for them and must not read them |
| Inherited | The rest of Claude Code's own environment |

**Consequence for session identity.** The adapter is one process per session, and the process is the identity. The session id is a label that the latest hook supplies. The environment variable can seed it before the first hook but must not be trusted afterwards. This matches the "Session identity" section of CRP-041.

### Claude Desktop from the Microsoft Store redirects `AppData\Roaming`

**Observed on Windows.** Claude Desktop on the test machine is the Microsoft Store package. Processes it starts, including the Claude Code binary and everything that binary spawns, see `%APPDATA%\Claude\…`. The same files do not exist at that path for a process started from an ordinary terminal. Their real location is `%LOCALAPPDATA%\Packages\Claude_pzs8sxrjxfjjc\LocalCache\Roaming\Claude\…`. A helper script written against the first path worked from a Desktop-started shell and failed from the owner's PowerShell with "The system cannot find the path specified".

`%TEMP%` and the user profile (`~/.claude`) were the same from both sides in this spike.

**Consequence.** An adapter started by the Desktop Code tab and one started by a terminal session may not agree on anything under `AppData\Roaming`, and possibly under `AppData\Local`, which was not checked. The lock file and control socket ([ADR-0005](../architecture/adr/0005-presence-host-election.md), [ADR-0006](../architecture/adr/0006-control-channel.md)) and the configuration file must live somewhere both sides resolve identically, and that has to be verified with one adapter on each side before the location is fixed.

### How options arrive

- A `user_config` entry in the **bundle's** manifest, referenced as `${user_config.key}` in `mcp_config.env`, arrived in the server's environment with its default value. There was no prompt.
- How a user changes that value in Claude Code, and whether plugin-level `userConfig` reaches a bundled server, were not tested.

### The `if` filter on an `mcp_tool` hook

A `PostToolUse` handler with `"if": "Bash(git push *)"` was run against eight commands, on Windows and again on Linux with the same result.

| Command | Tool outcome | Filtered hook fired |
|---|---|---|
| `git status` | Success | No |
| `git push origin main` | Success | **Yes** |
| `echo git push origin main` | Success | No |
| `git log -1 --oneline` | Success | No |
| `git status && git push origin main` | Success | **Yes** |
| `echo $(git rev-parse HEAD)` | Blocked by permissions before running | Not exercised |
| `git pushx origin main` | Failed | No. `PostToolUseFailure` fired instead |
| `git fetch origin` | Success | No |

Two of two expected firings, none unexpected in five other successful commands. The documented case where the filter runs "regardless" on command substitution was not reached, because the command was blocked. [CRP-075](../tickets/M7-personalisation/CRP-075-moments.md) can rely on the filter working on `mcp_tool` hooks; the false-positive rate on substitution remains unmeasured.

## Not yet tested

| Item | Why | Owner |
|---|---|---|
| Interactive terminal on Linux | Driving the terminal interface through a pseudo-terminal stopped at first-run onboarding, which asked for a second sign-in | Untested |
| `Notification` types `permission_prompt`, `agent_needs_input`, `elicitation_dialog` | The interactive session ran in auto mode and showed no permission prompt | CRP-043 |
| `Notification` on Linux | No interactive session there | Untested |
| The corrected hook file in an interactive terminal | The corrected file was run headless only | CRP-042 |
| Claude Desktop Code tab: events, `clientInfo`, entrypoint, console window | Needs the plugin installed in the owner's configuration. Not run | CRP-043 |
| VS Code extension | The extension is not installed on the test machine | Untested |
| JetBrains extension | Not available | Untested |
| macOS | No machine | Untested |
| Linux outside WSL2 | No machine | Untested |
| A download from GitHub Releases | Would need a public upload. Tested against a local HTTPS server with a redirect instead | Exercised for real by CRP-042 |
| Marker test on a re-fired `SessionStart` | Not run | |
| Plugin-level `userConfig` reaching a bundled server; changing a bundle option | Not run | CRP-042 |
| `if` filter behaviour on command substitution | Command was blocked by permissions | CRP-075 |
| Automatic compaction (`trigger: auto`) | Not provoked | |
