# claude-rich-presence

Discord Rich Presence for Claude Code and Claude Desktop. Go, MIT, one static binary named `rich-presence`.

The Go module and toolchain are in place ([CRP-004](docs/tickets/done/CRP-004-repo-scaffolding.md)): a package directory per component, and the commands `mcp`, `status`, `doctor` and `version` wired together in `internal/cli` ([CRP-033](docs/tickets/done/CRP-033-cli.md)). CI runs on Linux, macOS and Windows and fails below 100.0% statement coverage ([CRP-005](docs/tickets/done/CRP-005-ci-pipeline.md)). Build, test and coverage commands are in the Development section of [CONTRIBUTING.md](CONTRIBUTING.md).

## Source of truth

- Architecture: [docs/architecture/README.md](docs/architecture/README.md)
- Decisions: [docs/architecture/adr/](docs/architecture/adr/README.md). An accepted ADR is binding. To change one, write a new ADR that supersedes it.
- Work plan: [docs/tickets/README.md](docs/tickets/README.md). All work happens against a ticket.

## Rules that are easy to break

1. **Presence must never impair Claude.** Anything Claude waits on (an MCP tool call made by a hook) returns immediately and never performs I/O on the calling path. See [ADR-0008](docs/architecture/adr/0008-privacy-and-safety-by-default.md).
2. **Data is minimised at the edge.** Adapters read only an allowlist of hook fields. Prompt text, tool inputs, tool outputs, assistant messages, file paths and transcript paths are never parsed, stored, logged or forwarded.
3. **The activity summary is the one piece of work content that may be published**, and only as [ADR-0011](docs/architecture/adr/0011-model-authored-activity-summary.md) allows: opt-in, written by the model through a tool call, sanitised in the adapter, never logged. A repository link is published only from a project profile in the user's own configuration ([ADR-0012](docs/architecture/adr/0012-project-profiles-and-repository-link.md)): never from the model, never detected automatically, and no tool may write profiles.
4. **Documented interfaces only.** Do not read `~/.claude/sessions`, transcripts, Claude Desktop logs or window titles, even though they exist and other projects use them. Never read Claude's stored credentials or call Anthropic's endpoints; Anthropic's terms do not allow it ([ADR-0015](docs/architecture/adr/0015-status-line-bridge.md)).
5. **The summary owns the text lines.** No other fact is placed on a text line while a summary is shown. Extras go to hover text, images or buttons ([ADR-0013](docs/architecture/adr/0013-summary-first-card-layout.md)).
6. **No new runtime dependency without an ADR.** Pre-approved: the Go standard library and `golang.org/x/*`. The one exception is Microsoft's go-winio, for the Discord pipe on Windows and nowhere else ([ADR-0017](docs/architecture/adr/0017-go-winio-for-windows-pipes.md)). See [ADR-0003](docs/architecture/adr/0003-license-and-dependency-policy.md) and the `add-dependency` skill.
7. **100% statement coverage, no exclusions.** If a line cannot be tested, redesign it so it can. See [quality-strategy.md](docs/architecture/quality-strategy.md) and the `tdd-full-coverage` skill.
8. **Do not use the names "claude", "anthropic" or their logos in the plugin name, binary name or Discord assets.** See [ADR-0010](docs/architecture/adr/0010-naming-and-branding.md).
9. **Platform facts go stale.** Claude Code hooks, plugin manifests, MCPB and Discord IPC all change. Before relying on a detail, re-check it against the live docs listed in the `claude-surfaces` and `discord-ipc` skills, and record what you verified.

## Working on a ticket

Use the `work-ticket` skill. In short: confirm every ticket in `blocked_by` is `done`, branch as `crp-NNN-short-slug`, write tests first, run the `quality-gate` skill before opening a pull request, and update the ticket's `status` in its own file in the same pull request. A ticket that becomes `done` moves to `docs/tickets/done/` in that pull request, so the milestone directories hold only open tickets. A ticket whose `blocked_by` tickets are all done can be worked in parallel with any other.

## Skills in this repository

| Skill | Use it when |
|---|---|
| `work-ticket` | Starting, executing or finishing any ticket, including spikes |
| `write-ticket` | Adding or splitting a ticket |
| `record-decision` | A choice needs an ADR |
| `tdd-full-coverage` | Writing or changing Go code |
| `quality-gate` | Before every pull request |
| `add-dependency` | Anything new would enter `go.mod`, CI, or the release pipeline |
| `discord-ipc` | Touching the Discord connection, frames, activity payloads or rate limits |
| `claude-surfaces` | Touching hooks, the plugin, the MCP server or the desktop extension |
