# ADR-0007: One MCPB bundle for both surfaces, `mcp_tool` hooks in Claude Code

## Status

**Proposed.** Accepted or replaced by the outcome of [CRP-001](../../tickets/M0-foundation/CRP-001-spike-claude-code-adapter.md) for Claude Code and [CRP-002](../../tickets/M0-foundation/CRP-002-spike-desktop-extension.md) for Claude Desktop. Until then, only the tickets that name this ADR depend on it. The core does not.

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
4. The tool returns an empty result and never an error, so it cannot add to Claude's context or influence a decision.
5. The plugin has no top-level `bin/` directory.

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

The user installs the same bundle as a desktop extension. The adapter recognises the client from the MCP `initialize` request and reports only that the app is open.

### Tools

| Tool | Exposed to | Purpose |
|---|---|---|
| `presence_event` | Claude Code only | Receives hook events. Returns an empty result |
| `presence_status` | Both surfaces | Read-only diagnostics: whether Discord is connected, which process is host, how many sessions. Lets a user ask Claude whether presence is working, and backs the plugin's status skill |

### Acceptance criteria for this ADR

CRP-001 must show all of the following on Windows and on at least one of macOS or Linux:

| # | Criterion |
|---|---|
| A1 | Claude Code installs the plugin, downloads the bundle from a URL, and starts the correct binary with no shell and no runtime present |
| A2 | `mcp_tool` hooks deliver every event in the event table except launch-time `SessionStart`, with the substituted fields intact |
| A3 | A field that is absent from an event does not make the hook fail in a way the user sees |
| A4 | The tool's result adds nothing to Claude's context for any event, including `UserPromptSubmit` |
| A5 | Added latency per hooked event is under 10 milliseconds at the 99th percentile, and a hung or dead server delays Claude by no more than the hook timeout |
| A6 | The server's process ends when the session ends, including when the session is killed |
| A7 | Updating the plugin to a new bundle URL replaces the binary |
| A8 | The tool's presence in the model's context is small and causes no unwanted calls |

If A1, A2, A4, A5 or A6 fails and cannot be worked around, adopt the fallback. A3, A7 and A8 failing leads to mitigation, not fallback.

### Fallback

Command hooks in exec form run `rich-presence hook`, reading the event from standard input with `async` set, and a standalone `rich-presence daemon` is the host ([ADR-0005](0005-presence-host-election.md)). The plugin is distributed as a release archive containing the binaries and a launcher per platform. This is what existing projects do and it is known to work, with these costs: a shell script outside coverage, reliance on undocumented executable resolution on Windows, a detached process, and process-id liveness tracking. Ticketed as [CRP-044](../../tickets/M4-claude-code/CRP-044-fallback-command-hooks.md), status `conditional`.

## Consequences

- One artifact, one code path, both surfaces.
- No shell anywhere. Privacy filtering happens inside Claude Code, before our code runs, because each hook names the fields it sends.
- The model name at launch is unavailable, since it arrives only in the skipped `SessionStart`. The model appears after the first switch, clear or compaction. [CRP-045](../../tickets/M4-claude-code/CRP-045-spike-initial-model.md) looks for a better answer.
- Hook calls are synchronous. The latency criterion A5 and the two-second timeout are what keep [ADR-0008](0008-privacy-and-safety-by-default.md)'s first rule.
- Two small tool definitions enter the model's context in each Claude Code session, and one in Claude Desktop.
- Linux on Arm is not served by the bundle. Discord publishes no desktop client for it, so the loss is theoretical.
- Nobody has shipped this wiring before. The spike is first in the plan for that reason.

## Alternatives considered

| Alternative | Why not the first choice |
|---|---|
| Command hooks and a launcher script, the fallback above | Needs a shell and per-platform scripts; see fallback costs |
| Require the user to install the binary on `PATH` first | Two-step install, and a missing binary fails silently in every session |
| Commit binaries into the plugin directory in git | Repository bloat on every release, and still no per-platform selection for hooks |
| Distribute through npm with per-platform optional packages | Needs npm on the user's machine |
| Download the binary from a hook on first run | Needs a shell to do the download, and adds a network fetch outside Claude Code's own installer |
