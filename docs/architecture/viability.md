# Viability assessment

Assessed 2026-10-02. Each claim below is tagged with how it was established:

- **Docs**: read in the vendor's current documentation on that date. Sources are listed at the end.
- **Observed**: seen on the development machine (Windows 11, Claude Desktop from the Microsoft Store, Claude Code 2.1.284).
- **Unverified**: plausible but not confirmed. Each one is owned by a spike ticket.

## Verdict

| Route | Verdict |
|---|---|
| 1. A Claude plugin on its own | **Not viable as the whole solution.** It can cover Claude Code, and only if it ships a native executable. It cannot cover Claude Desktop Chat at all. |
| 2. A command line utility that interfaces with Claude | **Viable. This is the route.** One native binary is the product. A Claude Code plugin and a Claude Desktop extension are thin delivery wrappers around it. |

The two routes are not really alternatives. A plugin is the best way to *deliver and trigger* the utility inside Claude Code, and the utility is what makes the plugin work. The plan is route 2, packaged through route 1's mechanism where that mechanism exists.

Two limits are permanent under the constraint of using documented interfaces only, and the owner should accept them before work starts:

1. **Claude Desktop Chat shows only "Claude is open", unless the model itself tells us more.** Chat gives a local extension no signal about conversations, messages or models. The one exception is a tool call made by the model, which [ADR-0011](adr/0011-model-authored-activity-summary.md) proposes to use for an opt-in summary.
2. **Cloud and web sessions are out of reach.** Discord Rich Presence is a local connection to the Discord client on the same machine.

## What Discord Rich Presence requires

These constraints shape everything else.

| # | Constraint | Basis |
|---|---|---|
| D1 | Presence is set over a local IPC connection to the running Discord client: a named pipe `\\?\pipe\discord-ipc-{0..9}` on Windows, a Unix socket `discord-ipc-{0..9}` under the runtime or temp directory elsewhere. There is no web API for it. | Docs |
| D2 | **The connection must stay open.** When it closes, Discord clears the activity. So some process has to live for as long as presence should show. | Docs (Discord staff statement) and the open-source server reimplementation |
| D3 | Updates are rate limited. Two official figures exist: five per 20 seconds, and one per 15 seconds. We design for the stricter one. | Docs |
| D4 | A Discord application id is required. Its name is what Discord displays, and its art assets are uploaded in the Developer Portal. | Docs |
| D5 | Discord's own SDKs are unsuitable: the old library is deprecated, the Game SDK is archived, and the Social SDK is a closed binary under terms that restrict redistribution. Speaking the small IPC protocol directly is what every maintained project does. | Docs |
| D6 | Setting an activity after only the handshake, without OAuth, is how Discord's own library behaves, although the documentation text says commands need authentication. We depend on behaviour, not on the text. | Docs plus official source code |

D2 is the important one. A Claude Code hook is a process that runs for milliseconds, so a hook alone cannot hold presence. Something long-lived must.

## Route 1: plugin only

### What a Claude Code plugin can do

| Capability | Finding | Basis |
|---|---|---|
| Session lifecycle and activity events | Hooks cover session start and end, prompt submitted, before and after each tool, stop, notifications such as permission prompts, compaction, model switches, subagents | Docs |
| A long-lived process per session | A plugin can declare a stdio MCP server, started with the session and stopped at its end | Docs |
| Hooks that call that server directly | Hook type `mcp_tool` calls a tool on a plugin's MCP server, with arguments filled from the event. No shell, no process spawn | Docs |
| Shipping a native executable | `mcpServers` accepts an MCPB bundle by path or HTTPS URL. Claude Code downloads and extracts it. MCPB supports a `binary` server type with per-operating-system overrides | Docs |
| Runs on the user's machine | Yes for the terminal, the Desktop Code tab and the IDE extensions | Docs. Desktop Code tab also **Observed**: it runs the same Claude Code binary and reads the same user plugins |
| Runs in cloud sessions | Hooks there execute in the remote environment, which has no Discord | Docs |

### Why a plugin alone is not enough

1. **A plugin with no native executable is not robust.** Hook commands and MCP servers written in JavaScript or Python need that runtime on `PATH`. Claude Code ships as a native binary and does not provide one. **Observed**: the development machine has no `node` at all, and an installed third-party plugin whose hook runs `node -e ...` cannot work there. Shell scripts are no better: on Windows the hook shell is Git Bash when present and PowerShell otherwise (**Docs**), so one command string cannot serve both, and neither shell can frame binary messages over a named pipe portably.
2. **So the plugin must carry a compiled program.** At that point the program is the product and the plugin is packaging. That is route 2.
3. **Claude Desktop Chat does not load Claude Code plugins and has no hooks.** (**Docs**: none documented.) A plugin cannot reach it.

### What Claude Desktop Chat offers instead

| Capability | Finding | Basis |
|---|---|---|
| Local extensions | Desktop Extensions (MCPB bundles) run a local MCP server. Server type `binary` needs no runtime | Docs |
| Signal about conversations | **None.** A server is told only when the model calls one of its tools | Docs |
| Server lifetime | Started with the app and kept alive while it is open | **Unverified**, owned by [CRP-002](../tickets/M0-foundation/CRP-002-spike-desktop-extension.md) |

A server that lives exactly as long as the app is still useful: it is a reliable "Claude Desktop is open" signal with no polling and nothing installed at login.

Other projects go further by reading Claude Desktop's log files, watching its private session files, or inspecting window labels through UI automation. Those are undocumented, and at least one of them broke when an update moved the log directory. We do not do this. See [ADR-0008](adr/0008-privacy-and-safety-by-default.md).

## Route 2: a utility, wrapped for each surface

One Go binary, `rich-presence`, with these roles:

| Role | What it does |
|---|---|
| Adapter | Runs as a stdio MCP server inside each Claude Code session and inside Claude Desktop. Turns the host's signals into presence events |
| Presence host | Exactly one process per user owns the Discord connection and the list of open sessions. It is whichever adapter process started first. If it exits, another takes over |
| Tooling | `status`, `doctor`, `version` for diagnosis |

Why this is viable:

| Requirement | How it is met | Basis |
|---|---|---|
| Something must hold the Discord connection (D2) | The adapter processes already live as long as the sessions do. One of them is the host | Design |
| No runtime prerequisites | A static Go binary. No Node, Python, or shell | Design |
| Windows named pipes without third-party code | Go 1.26 opens pipes with overlapped I/O, so deadlines work | Docs (Go release notes), to be exercised in [CRP-021](../tickets/M2-discord/CRP-021-discord-transport.md) |
| Delivery into Claude Code without a shell | Plugin references a released MCPB bundle by URL | Docs, to be exercised in [CRP-001](../tickets/M0-foundation/CRP-001-spike-claude-code-adapter.md) |
| Delivery into Claude Desktop | The same MCPB bundle, installed as a desktop extension | Docs, to be exercised in [CRP-002](../tickets/M0-foundation/CRP-002-spike-desktop-extension.md) |
| 100% coverage and SonarQube | Go has built-in coverage including for compiled binaries. SonarQube Cloud analyses Go and is free for public repositories | Docs |
| Permissive licensing throughout | Standard library only (BSD-3-Clause). No copyleft anywhere | Design |

### Known gaps in the preferred wiring

The preferred wiring, hooks of type `mcp_tool` calling the adapter, has not been used by any project we found. It rests on documented features, but these points need the spike before we commit:

| Gap | Consequence if it holds | Owner |
|---|---|---|
| `mcp_tool` hooks are skipped for `SessionStart` at launch, because MCP servers are not connected yet (**Docs**) | The adapter learns a session exists from its own start-up, which is fine. But the initial model arrives only in that event, so it is unknown until the model is switched or the session is cleared or compacted | [CRP-001](../tickets/M0-foundation/CRP-001-spike-claude-code-adapter.md), then [CRP-045](../tickets/M4-claude-code/CRP-045-spike-initial-model.md) |
| `mcp_tool` hooks are not documented as supporting `async` | Each call is on Claude's path. The tool must return in microseconds and every hook needs a short timeout | [CRP-001](../tickets/M0-foundation/CRP-001-spike-claude-code-adapter.md) |
| A hook tool's text output is read like hook stdout, which for some events is added to Claude's context | The tool must return nothing that changes Claude's behaviour | [CRP-001](../tickets/M0-foundation/CRP-001-spike-claude-code-adapter.md) |
| The adapter's tool is visible to the model | A small fixed context cost per session | [CRP-001](../tickets/M0-foundation/CRP-001-spike-claude-code-adapter.md) |

If the spike fails, the fallback is the pattern every existing project uses: command hooks that run the binary, feeding a detached background process. It works, at the price of a launcher script per platform. It is specified in [ADR-0007](adr/0007-integration-and-distribution.md) and ticketed as [CRP-044](../tickets/M4-claude-code/CRP-044-fallback-command-hooks.md), to be built only if needed. The core of the system is the same either way.

## What each surface can show

| Surface | Supported | Mechanism | Detail available |
|---|---|---|---|
| Claude Code, terminal | Yes | Plugin | Session open, working, running tools, waiting for input, compacting, idle, elapsed time, subagent count. Model once known. Project name if enabled |
| Claude Desktop, Code tab, local session | Yes | Same plugin | Same |
| VS Code and JetBrains extensions | Expected | Same plugin | Same. **Unverified**, owned by CRP-001 |
| Claude Desktop, Chat | Partial | Desktop extension | App open, elapsed time. A summary phrase if the user opts in and ADR-0011 is accepted |
| Claude Desktop, Cowork | Not in the first release | | **Unverified** whether its extensions run on the host or in a sandbox |
| Claude Code on the web, cloud sessions | No | | Hooks run remotely |
| SSH, containers, WSL, where Discord runs on a different operating system instance | No | | The Discord pipe is not reachable |
| claude.ai in a browser, mobile apps | No | | No local extension point |

## Can presence say what the user is working on?

Yes, with one mechanism, and it is opt-in. Assessed 2026-10-02.

| Approach | Verdict | Basis |
|---|---|---|
| Read a summary from a hook event | No. No event carries one | Docs |
| Use the session title | No. Only a custom title is exposed, and only at session start | Docs |
| Use a `prompt` or `agent` hook to produce one | Poor fit. They return decisions, cost a model call each time, and agent hooks are experimental | Docs |
| Summarise the prompt in our binary | No. It would need a network call, a key and per-prompt cost | Design |
| **Have the session's Claude call a tool with a short phrase** | **Yes.** Costs a few dozen output tokens per task. Works in Claude Code and in Claude Desktop Chat | Docs for the tool mechanism. Reliability is **Unverified**, owned by [CRP-046](../tickets/M4-claude-code/CRP-046-spike-activity-summary.md) |

A link to the project's repository can be shown as well, as a button, for projects the user opts in one by one. It comes from the user's configuration, not from the model, and the binary cannot check that the repository is public without a network request, so a setup skill does that check inside a Claude Code session. See [ADR-0012](adr/0012-project-profiles-and-repository-link.md).

The design, its safeguards and its acceptance criteria are in [ADR-0011](adr/0011-model-authored-activity-summary.md). The open questions are behavioural: how reliably each model makes the call, whether a permission prompt appears, and what untrusted text in a project can make the phrase say.

## Prior art

About thirty small projects exist. The six most-starred were reviewed.

| Project | Stack | Approach | Desktop Chat |
|---|---|---|---|
| Younesfdj/vibecoder-discord-presence | TypeScript | Hooks write a marker file, a background process holds the connection | No |
| BrunoJurkovic/claude-code-discord-status | TypeScript | Bash hooks (needing `jq`) post to a local HTTP daemon | No |
| rar-file/claude-rpc | JavaScript | Hook writes a state file, daemon scans transcript files | Requested, open |
| tsanva/cc-discord-presence | Go | Plugin whose scripts start and stop a daemon, which tails transcript files | No |
| inerthel-agi/claude-rpc | Rust, Windows only | Tray app polling processes and window labels | Yes, by UI automation |
| dayfinggg/claude-code-discord-presence | TypeScript | Hooks to a loopback HTTP daemon | Yes, by watching private session files |

What we take from them:

- **The shape is settled**: hook events feed one long-lived process per user that owns the Discord connection. We keep that shape.
- **The common failure is stuck presence** after a session dies without running its end hook. We tie session liveness to an open connection, not to a counter.
- **Runtime prerequisites are the common installation problem** (Node, `jq`, Bash). We ship one static binary.
- **Nobody has solved Desktop Chat on documented interfaces.** We say so plainly and offer the honest partial.
- **Naming matters**: Discord's Developer Portal rejects application names containing "Claude", and projects have worked around it with look-alike characters. We choose a neutral name instead. See [ADR-0010](adr/0010-naming-and-branding.md).

We build rather than adopt because every reviewed project conflicts with at least one of our rules: a runtime prerequisite, reading undocumented files, a single-maintainer dependency, or no tests. All are MIT licensed, so their ideas are freely usable, and they are worth reading for behaviour on real machines.

## Sources

Claude Code and Claude Desktop:

- Hooks reference: https://code.claude.com/docs/en/hooks
- Plugin manifest reference: https://code.claude.com/docs/en/plugins-reference
- Plugin marketplaces: https://code.claude.com/docs/en/plugin-marketplaces
- MCPB bundles: https://claude.com/docs/connectors/building/mcpb
- MCPB manifest specification: https://github.com/modelcontextprotocol/mcpb/blob/main/MANIFEST.md
- Legal and compliance (naming): https://code.claude.com/docs/en/legal-and-compliance
- Trademark guidelines: https://www.anthropic.com/legal/trademark-guidelines

Discord:

- RPC and IPC: https://docs.discord.com/developers/topics/rpc
- Protocol notes in the official library: https://github.com/discord/discord-rpc/blob/master/documentation/hard-mode.md
- Rate limit discussion: https://github.com/discord/discord-api-docs/issues/668
- Game SDK page (rate limit, archived status): https://docs.discord.com/developers/developer-tools/game-sdk
- Social SDK terms: https://support-dev.discord.com/hc/en-us/articles/30225844245271-Discord-Social-SDK-Terms
- Developer policy: https://support-dev.discord.com/hc/en-us/articles/8563934450327-Discord-Developer-Policy
- Open reimplementation of the IPC server: https://github.com/OpenAsar/arrpc

Go and SonarQube:

- Go 1.25 and 1.26 release notes (overlapped I/O on Windows): https://go.dev/doc/go1.25 and https://go.dev/doc/go1.26
- SonarQube Cloud plans: https://www.sonarsource.com/plans-and-pricing/sonarcloud/
- SonarQube Cloud coverage parameters: https://docs.sonarsource.com/sonarqube-cloud/analyzing-source-code/test-coverage/test-coverage-parameters
- SonarQube Cloud automatic analysis limits: https://docs.sonarsource.com/sonarqube-cloud/analyzing-source-code/automatic-analysis
