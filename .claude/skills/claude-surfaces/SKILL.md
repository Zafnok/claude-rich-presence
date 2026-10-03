---
name: claude-surfaces
description: Reference for how this project integrates with Claude Code and Claude Desktop - hook events and their fields, mcp_tool hooks, plugin and marketplace manifests, MCPB bundles, and the MCP stdio subset - with what is documented, what is unverified, and how to re-check. Use when touching internal/adapter, internal/mcp, plugin/, extension/, or the hook file, and when a Claude Code or Claude Desktop update changes behaviour.
---

# Claude integration surfaces

How Claude starts our binary and tells it things. Assembled on 2026-10-02 from the documentation and from Claude Code 2.1.284. **These interfaces change often. Re-check the live sources at the bottom before relying on a detail**, and correct this file when you find a difference. Once CRP-001 and CRP-002 have run, their findings in `docs/research/` take precedence over anything here.

## Rules that apply to every surface

From ADR-0008, and not negotiable:

1. Anything Claude waits on returns immediately and performs no I/O.
2. The event tool always returns an empty success result.
3. Read only allowlisted fields. Never prompt text, tool inputs or outputs, assistant messages, file paths, transcript paths or session titles.
4. Documented interfaces only. No transcripts, no `~/.claude` internals, no Claude Desktop logs or session files, no process or window inspection.

## Where each surface stands

| Surface | Runs our plugin | Notes |
|---|---|---|
| Claude Code in a terminal | Yes | |
| Claude Desktop, Code tab, local session | Yes | Runs the same Claude Code binary and reads the same user plugins. Observed. A desktop extension is not attached to these sessions (CRP-002) |
| VS Code and JetBrains extensions | Expected | To be confirmed by CRP-001 |
| Cloud sessions, Claude Code on the web | Not usefully | Hooks run remotely. `CLAUDE_CODE_REMOTE` is `true` there, and the adapter does nothing |
| Claude Desktop, Chat | No | Uses the desktop extension instead. No hooks exist |
| Cowork | Unknown | Not in the first release |

## Hooks

### Handler types

`command`, `http`, `mcp_tool`, `prompt`, `agent`. We use `mcp_tool` (ADR-0007).

### `mcp_tool` hooks

| Field | Meaning |
|---|---|
| `server` | For a plugin's own server, the scoped name `plugin:<plugin-name>:<server-name>` |
| `tool` | The tool to call |
| `input` | Arguments. String values may contain `${path}` references into the hook's event, such as `${tool_name}` |
| `timeout` | Seconds. The default is 600, so always set it. We use 2 |

Documented behaviour to design around:

- **Skipped for `SessionStart` at launch**, including with resume or continue, and for every `Setup` event, because the session's MCP servers are not connected yet. `SessionStart` does run these hooks when it fires again after a clear or a compaction.
- On events that can block, such as `PreToolUse` and `Stop`, Claude Code waits for a connecting server, within the hook's timeout. On observational events, such as `Notification` and `SessionEnd`, it does not wait.
- If the server is not connected, the hook is a non-blocking error.
- The tool's text result is read the way hook standard output is read. For some events that adds to Claude's context. Return nothing.
- If the tool returns an error, the hook is a non-blocking error.
- `async` is documented for command hooks only.

Not documented, to be settled by CRP-001: what happens when a `${path}` is absent from the event.

### The `if` filter

A hook handler may carry `if`, written in permission-rule syntax such as a pattern for a shell command. It is evaluated only on tool events: `PreToolUse`, `PostToolUse`, `PostToolUseFailure`, `PermissionRequest`, `PermissionDenied`. On other events a hook with `if` never runs. For shell commands, each sub-command of a compound command is checked, and when Claude Code cannot tell what will run, it runs the hook regardless. `PostToolUse` fires only after a tool call succeeds. This is how moments are detected without reading the command (CRP-075). Whether it works on `mcp_tool` hooks is confirmed by CRP-001.

### Events we use, and the fields we read

Every event carries `session_id`, `cwd`, `hook_event_name`, and inside a subagent `agent_id` and `agent_type`. We read `session_id` always, and `cwd` only to take its last element at the `full` privacy level.

| Event | Extra fields we read | Fields present that we must not read |
|---|---|---|
| `SessionStart` | `source`, `model` when present | `session_title`, `transcript_path` |
| `UserPromptSubmit` | none | `prompt_text` |
| `PreToolUse` | `tool_name` | `tool_input` |
| `PostToolUse`, `PostToolUseFailure` | `tool_name` | `tool_input`, `tool_output` |
| `Stop`, `StopFailure` | none | `last_assistant_message` |
| `Notification` | `notification_type` | `message` |
| `PreCompact`, `PostCompact` | none | |
| `PostModelSwitch` | `to_model` | |
| `SubagentStart`, `SubagentStop` | none | `last_assistant_message` |
| `SessionEnd` | `reason` | |

Notification types that mean the user is needed: `permission_prompt`, `agent_needs_input`, `elicitation_dialog`. The type `idle_prompt` means idle.

The model is available only in `SessionStart`, and not always, and in the model-switch events. There is no environment variable for it.

The allowlist is defined once, as a table in `internal/adapter/code`. The hook file must match it, and a test checks that (CRP-042).

### If command hooks are ever used

This is the fallback (CRP-044). Know these before designing it:

- Exec form, with `args` set, runs with no shell. On Windows it needs a real executable; script shims cannot be spawned.
- Shell form uses Bash by default, and PowerShell on Windows when Git Bash is not installed. One command string cannot serve both.
- `async` runs the hook in the background. The timeout is not enforced on asynchronous hooks.
- `SessionEnd` hooks share a budget of about a second and a half.
- A hook process inherits the environment, plus `CLAUDE_PROJECT_DIR`, `CLAUDE_PLUGIN_ROOT`, `CLAUDE_PLUGIN_DATA` and `CLAUDE_PLUGIN_OPTION_<KEY>` for each user option.

## Model-initiated tool calls

Used by the opt-in activity summary (ADR-0011). Everything here is to be confirmed by CRP-046.

| Fact | Basis |
|---|---|
| A plugin server's tool is callable by the model as `mcp__plugin_<plugin-name>_<server-name>__<tool-name>` | Docs |
| Tool search is on by default, so MCP tool definitions are deferred and the model sees only names until it searches. A server or tool can be marked to always load | Docs |
| Claude Code passes an MCP server's `instructions` to the model | Observed in a live session. Not in the MCP documentation page |
| Text returned by a `UserPromptSubmit` or `SessionStart` hook is added to Claude's context, capped at 10,000 characters. For most other events hook output goes only to the debug log | Docs |
| Whether a model-initiated call to a plugin's tool raises a permission prompt the first time | Not documented |
| `prompt` and `agent` hooks run a model and return a decision. They are not a way to extract text. Agent hooks are experimental | Docs |
| The session title is exposed only in `SessionStart`, and only when a custom one was set | Docs |

A summary is model-written and therefore untrusted. It is sanitised in the adapter and never logged. Links are stripped from it: a published link comes only from a project profile in the user's configuration (ADR-0012).

## Plugin and marketplace

| Fact | Consequence |
|---|---|
| A plugin name starting with `claude-` or `anthropic-` is a validation error, and `claude` as a whole word elsewhere is a warning | The plugin is named `rich-presence` (ADR-0010) |
| `mcpServers` accepts a path or an HTTPS URL ending in `.mcpb`, which Claude Code downloads and extracts into a cache under the plugin | This is how the binary is delivered |
| `userConfig` values reach MCP server configuration through `${user_config.KEY}` and reach hook processes as `CLAUDE_PLUGIN_OPTION_<KEY>` | How the privacy setting arrives |
| Fixed-choice options in `userConfig` need a recent Claude Code | Sets the minimum version, or use free text |
| Plugin MCP servers receive `CLAUDE_PLUGIN_ROOT` and `CLAUDE_PLUGIN_DATA` in their environment | |
| A top-level `bin/` directory makes claude.ai and Cowork refuse the plugin | Do not create one |
| A `CLAUDE.md` at the plugin root is not loaded | Put guidance in a plugin skill |
| `claude plugin validate --strict` turns warnings into failures | Run it in CI |
| A marketplace is a repository with `.claude-plugin/marketplace.json`; a plugin entry's `source` may be a relative path | This repository is its own marketplace, with the plugin in `plugin/` |
| The `version` in the plugin manifest pins users to it until it changes | The release pipeline bumps it |

## Status line

Used by the opt-in bridge (ADR-0015). To be confirmed by CRP-045.

| Fact | Basis |
|---|---|
| The status line is a `statusLine` setting in user or project settings, running one command that receives session data as JSON on standard input | Docs |
| The data includes model id and display name, effort level, fast mode, context window percentage, session cost, 5-hour and 7-day usage percentages with reset times, an open pull request for the branch, repository identity, and the session name including an automatically generated title | Docs |
| Usage percentages appear only for some subscription plans, and only after the first response | Docs |
| It runs at session start, after each assistant message and on a few other changes, debounced. A running command is cancelled when a new update arrives | Docs |
| It runs through Git Bash on Windows when present, otherwise PowerShell | Docs |
| A plugin cannot set the main status line. It can supply one for subagent rows only | Docs |
| Whether it runs in the Claude Desktop Code tab and the IDE extensions | Not documented |

We read an allowlist only, and never the transcript path, directories, repository identity or session name. We never read Claude's stored credentials and never call Anthropic's endpoints: the terms for Claude Code do not allow third-party tools to collect or use those credentials.

## MCPB bundle

A zip archive with a `manifest.json`.

| Fact | Consequence |
|---|---|
| Server types: `node`, `python`, `binary`, `uv` | We use `binary`. No runtime needed |
| `platform_overrides` inside `mcp_config` selects by `win32`, `darwin`, `linux` | Not by CPU architecture. Hence a universal Mac binary and `amd64` only for Linux |
| Hosts append `.exe` on Windows | |
| `user_config` types include string, number, boolean | Values are substituted into the server's environment |
| `${__dirname}` refers to the extracted bundle directory | |
| Declaring tools in the manifest is optional | |

Desktop extension listings in Anthropic's directory are deprecated and the directory no longer accepts MCPB submissions (Claude's MCPB page, 2026-10-02). Users install the bundle file themselves.

### How Claude Desktop runs a bundle

Observed by CRP-002 on Windows with Claude Desktop 2.9939.4. Not documented anywhere. The full record is `docs/research/crp-002-desktop-extension.md`.

| Fact | Consequence |
|---|---|
| Two copies of the server run from app launch to quit, including while the app sits in the tray. Their `clientInfo.name` values are `claude-ai` and `local-agent-mode-` plus the extension's display name. Claude Code sends `claude-code` | Only the `claude-ai` copy reports the app (ADR-0007) |
| Neither is per conversation. Right after an install the `claude-ai` copy may be missing until the app is restarted | Install instructions say to restart Claude Desktop |
| At launch a copy is started and has its input closed before `initialize`; the `claude-ai` copy proper follows two seconds later. A server still running two seconds after its input closes is ended | Do nothing visible before `initialize`. Exit when input closes |
| A server that exits by itself is not restarted until the extension is switched off and on | Never exit while input is open |
| Saving the settings restarts the `claude-ai` copy only | |
| An optional setting left empty arrives as the literal text `${user_config.KEY}`. The first start comes before the settings form is saved | Treat a placeholder as unset |
| Settings arrive as text in the arguments and environment the manifest names. A sensitive one is still a plain environment variable | |
| The bundle is extracted under `%APPDATA%\Claude\Claude Extensions\`. The working directory is `C:\Windows\system32`. Standard error goes to a file | |
| No console window is shown, and no SmartScreen or Defender prompt appeared for an unsigned binary | One machine |
| The server is in a job with kill-on-close | |

Unverified: a tool call from a Chat conversation; everything on macOS; Cowork; and whether Claude Code's bundle loader honours everything in the specification, which CRP-001 owns.

### Claude Desktop's packaging on Windows

Claude Desktop is a Store app. Every process it starts, at any depth, has **new folders directly under `%LOCALAPPDATA%` and `%APPDATA%` redirected** to `%LOCALAPPDATA%\Packages\Claude_<id>\LocalCache\`. That includes extension servers, Code-tab sessions and every tool those sessions run. A Unix socket cannot be bound or connected in a redirected folder. `%TEMP%` and the home directory are not redirected, and named pipes are shared.

So on Windows we keep nothing directly under `AppData` (ADR-0006, ADR-0016). When testing anything that involves "inside Claude Desktop" against "a terminal", the terminal must be opened outside Claude Desktop.

## MCP over standard streams

We implement the minimum (ADR-0004, CRP-040): `initialize`, the `initialized` notification, `ping`, `tools/list`, `tools/call`. JSON-RPC 2.0, one message per line.

- The client's name and version arrive in `initialize`. That is how the adapter tells Claude Code from Claude Desktop. The actual names are recorded by the spikes.
- Standard output is the protocol stream. Never print anything else to it.
- When standard input closes, the host is gone. Shut down.

## How to re-verify

1. Read the hooks reference for event names, fields, handler types and the `mcp_tool` section.
2. Read the plugin manifest reference and the marketplace reference for field names and path rules.
3. Read the MCPB manifest specification for the manifest version and fields.
4. Run `claude plugin validate --strict` on `plugin/` and on the repository root.
5. In a real session, with the owner's help: install from a local marketplace, turn on Claude Code's debug logging, and watch hooks fire.

## Sources

- https://code.claude.com/docs/en/hooks
- https://code.claude.com/docs/en/plugins-reference
- https://code.claude.com/docs/en/mcp
- https://code.claude.com/docs/en/statusline
- https://code.claude.com/docs/en/legal-and-compliance
- https://code.claude.com/docs/en/plugin-marketplaces
- https://code.claude.com/docs/llms.txt, the documentation index
- https://claude.com/docs/connectors/building/mcpb
- https://github.com/modelcontextprotocol/mcpb/blob/main/MANIFEST.md
- https://modelcontextprotocol.io/specification

## In this repository

- Decisions: `docs/architecture/adr/0007-integration-and-distribution.md`, `0008-privacy-and-safety-by-default.md`, `0010-naming-and-branding.md`, `0011-model-authored-activity-summary.md`, `0014-personalities.md`, `0015-status-line-bridge.md`
- Event table: `docs/architecture/README.md`, section "Where the events come from"
- Tickets: CRP-001, CRP-002, CRP-040, CRP-041, CRP-042, CRP-045, CRP-046, CRP-047, CRP-050, CRP-051, CRP-053, CRP-072, CRP-074, CRP-075
