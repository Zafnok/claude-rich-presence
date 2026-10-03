# Architecture decision records

An ADR records one decision, why it was made, and what it costs. Accepted ADRs are binding. To change one, add a new ADR that supersedes it and update the status of the old one; do not rewrite history.

| ADR | Decision | Status |
|---|---|---|
| [0001](0001-core-architecture.md) | One binary: a presence host with thin adapters, built as ports and adapters | Accepted |
| [0002](0002-implementation-language.md) | Go, standard library only at runtime | Accepted |
| [0003](0003-license-and-dependency-policy.md) | MIT license and a strict dependency policy | Accepted |
| [0004](0004-in-house-protocol-implementations.md) | Implement the Discord IPC client and the MCP stdio subset ourselves | Accepted |
| [0005](0005-presence-host-election.md) | The presence host is elected among adapter processes; no daemon | Accepted |
| [0006](0006-control-channel.md) | Unix domain sockets and newline-delimited JSON between adapters and host | Accepted |
| [0007](0007-integration-and-distribution.md) | One MCPB bundle for both surfaces; `mcp_tool` hooks in Claude Code | Proposed, pending CRP-001 and CRP-002 |
| [0008](0008-privacy-and-safety-by-default.md) | Documented interfaces only, data minimised at the edge, never block Claude | Accepted |
| [0009](0009-quality-gates.md) | 100% statement coverage and SonarQube Cloud, enforced in CI | Accepted |
| [0010](0010-naming-and-branding.md) | Neutral product names, no Anthropic or Discord marks | Proposed, pending CRP-003 |
| [0011](0011-model-authored-activity-summary.md) | An opt-in activity summary written by Claude, through a tool call | Proposed, pending CRP-046 |
| [0012](0012-project-profiles-and-repository-link.md) | Per-project profiles, and an opt-in repository link from the user's configuration only | Proposed, pending CRP-048 |
| [0013](0013-summary-first-card-layout.md) | The summary owns the text lines; every other fact lives in hover text, images or buttons | Proposed, pending CRP-070 |
| [0014](0014-personalities.md) | Personalities: a voice for Claude's summaries and a matching vocabulary for the fixed text | Proposed, pending CRP-046 and CRP-072 |
| [0015](0015-status-line-bridge.md) | An opt-in status line bridge for model, context, cost and usage limits; never the login token | Proposed, pending CRP-045 |
| [0016](0016-windows-file-locations.md) | On Windows, nothing of ours lives directly under `AppData` | Accepted |

## Format

Each ADR has these sections, in this order:

1. **Status**: Proposed, Accepted, or Superseded by ADR-NNNN. A Proposed ADR names the ticket that will settle it.
2. **Context**: the forces at play and the facts, with how each fact is known.
3. **Decision**: what we will do, stated so that a reviewer can tell whether code complies.
4. **Consequences**: what gets easier, what gets harder, what we accept.
5. **Alternatives considered**: each with the reason it lost.

Number ADRs sequentially. Name the file `NNNN-short-slug.md`. Add a row to the table above in the same change.
