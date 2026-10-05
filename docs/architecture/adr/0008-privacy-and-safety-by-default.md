# ADR-0008: Documented interfaces only, data minimised at the edge, never block Claude

## Status

Accepted.

## Context

This program sits beside a coding agent and publishes information about the user's work to a social platform. Three things could go wrong, in rising order of harm: it could break when a vendor changes something private, it could slow or break Claude, or it could publish something the user did not mean to share.

Hook events contain prompt text, tool inputs and outputs, assistant messages and file paths. The user's machine holds transcripts, session files and logs. Other projects read these. One reviewed project also ships default-on telemetry and reads an authentication token.

## Decision

### 1. Never impair Claude

1. Any code that Claude waits on returns without performing I/O. The `presence_event` tool validates its input, places it on a bounded in-memory queue, and returns. If the queue is full the event is dropped.
2. That tool always returns the same success result, which carries no information: the constant text `{}`. Presence failures are logged, never reported to Claude. (Clarified 2026-10-03. This read "an empty success result". [CRP-001](../../research/crp-001-claude-code-adapter.md) found that Claude Code adds a line to the model's context when the result is literally empty and adds nothing for `{}`. The decision is unchanged: the result never varies and never tells Claude anything.)
3. Every hook has an explicit short timeout.
4. A panic in any goroutine is recovered, logged, and does not take down the MCP server loop.
5. The end-to-end tests assert a latency budget on the tool call ([CRP-043](../../tickets/done/CRP-043-end-to-end-tests.md)).

### 2. Documented interfaces only

We use hook events, the plugin manifest, MCP, MCPB and Discord IPC as documented. We do not read transcripts, `~/.claude` internals, Claude Desktop's session or log files, process lists, or window contents, and we do not automate any user interface. If a feature can only be built on those, we do not build it.

### 3. Minimise data at the edge

1. Adapters accept an **allowlist** of fields per event. Anything else is not parsed.
2. Never read, held, logged or forwarded: prompt text, tool inputs, tool outputs, assistant messages, file paths, transcript paths, session titles.
3. Tool names are mapped to a small fixed vocabulary of kinds such as editing, running commands, searching. The raw name is not forwarded.
4. The working directory is reduced to its last path element inside the adapter, and only when the privacy level is `full`. At other levels it is discarded on receipt. (Clarified 2026-10-04. With project profiles ([ADR-0012](0012-project-profiles-and-repository-link.md)) the level cannot be known without the working directory, so when the adapter has been given a profile resolver, the directory is read at every level. It is used once, in memory, to choose the level and the display name for the call, and is then discarded: never stored, logged or forwarded. Without a resolver nothing changes. The decision is unchanged: at levels other than `full` the directory, and anything derived from it but the level, leaves no trace in what the adapter publishes.)
5. The privacy level is enforced in the adapter, before the control channel. The host cannot show what it was never sent.

### 4. Privacy levels

| Level | Published |
|---|---|
| `minimal` | Claude is in use; elapsed time |
| `standard`, the default | Plus status, model family, number of sessions |
| `full` | Plus project name |

The default never reveals what the user is working on.

[ADR-0011](0011-model-authored-activity-summary.md) proposes a fourth level, `summary`, which is opt-in and publishes one short phrase that Claude writes for a public audience. It does not change anything above for users who do not enable it, and it does not read prompts: the phrase is volunteered by the model through a tool call and sanitised in the adapter.

[ADR-0012](0012-project-profiles-and-repository-link.md) proposes per-project profiles, so a level can be set for one project without changing the default, and an opt-in repository link that comes only from the user's own configuration.

### 5. No network, no telemetry

The program opens exactly two kinds of connection: the local Discord pipe and the local control socket. It makes no network requests, checks for no updates, and collects no usage data.

### 6. Logs

Logs are local and size-capped, record only warnings and errors unless the user raises the level, and never contain event payloads. The `doctor` command prints nothing that would be unsafe to paste into a public issue.

## Consequences

- Claude Desktop Chat cannot show more than "open", permanently, unless Anthropic documents a signal.
- Presence text is generic by default. Users who want project names opt in.
- Some diagnostics are harder because payloads are not logged. `doctor` and counters compensate.
- These rules are testable: allowlist tests per event, a test that seeds forbidden fields and asserts they appear nowhere downstream, and a test that the binary imports no HTTP client.

## Alternatives considered

| Alternative | Why not |
|---|---|
| Show project names by default, as editor presence plugins do | A coding agent is often used on employer or client work. An unexpected leak is worse than a bland default |
| Enforce privacy in the renderer | Data would already have crossed a process boundary and could reach logs |
| Read private files for richer Desktop presence | Breaks on vendor updates, and reads data the user did not offer |
