# ADR-0011: An opt-in activity summary written by Claude

## Status

**Proposed.** Settled by [CRP-046](../../tickets/M4-claude-code/CRP-046-spike-activity-summary.md). Extends [ADR-0008](0008-privacy-and-safety-by-default.md) with a fourth, opt-in privacy level. Makes one stated exception to the empty-result rule in [ADR-0007](0007-integration-and-distribution.md).

## Context

The owner wants presence to say what the user is working on, in a phrase: "Building the battle system for Visions of Shuyi", "Closing blind spots in the code equivalency checker".

Nothing Claude Code hands a hook contains such a phrase. What was checked on 2026-10-02:

| Source | Finding | Basis |
|---|---|---|
| Hook event fields | No event carries a task or conversation summary. Prompt text and the last assistant message are available, but are raw content, not a summary | Docs |
| Session title | Only a custom title, set by the user or a hook, is exposed, and only in `SessionStart`. Automatically generated titles are not | Docs |
| `prompt` and `agent` hooks | They run a model and return a decision. They are gates, not a way to get text out. Agent hooks are marked experimental. Each firing is an extra model call | Docs |
| Summarising prompt text ourselves | Needs a model call from our binary: a network connection, a key, and cost per prompt | Design |
| Reading the transcript | Undocumented format, and ruled out by ADR-0008 | Design |

One party already knows what the session is about and can say it in ten words at almost no cost: the Claude running the session. And the documented way for a model to tell a local program something is a tool call to an MCP server, which is what our adapter already is.

Facts this depends on:

| Fact | Basis |
|---|---|
| A model can call a tool on a plugin's MCP server. Its callable name includes the plugin and server names | Docs |
| Claude Code passes an MCP server's instructions to the model | Observed in a live session. Not in the MCP documentation page |
| Tool search is on by default, so MCP tool definitions are deferred and the model sees only their names until it searches. A server or tool can be marked to always load | Docs |
| Text returned by a `UserPromptSubmit` hook is added to Claude's context, up to 10,000 characters | Docs. Whether an `mcp_tool` hook's result is treated identically is stated for hooks in general and is to be confirmed |
| Whether a model-initiated call to a plugin's MCP tool raises a permission prompt the first time | Not documented |
| The same tool mechanism exists in Claude Desktop Chat | Docs |

## Decision

### Mechanism

1. The adapter exposes a tool, `presence_summary`, taking a short `activity` phrase and an optional human-readable `project` name.
2. The tool is listed **only when the summary level is enabled for the session**. A user who has not opted in pays nothing for it and cannot be affected by it.
3. Claude is told when and how to call it through the tool's description and the server's instructions:
   - call it when a task begins and again only when the task changes substantially;
   - call it alongside other tool calls, never as a step by itself;
   - write for a public audience: no names of people or customers, no file paths, no links, no secrets, nothing quoted from the user.
4. The tool is marked to always load, so it is not hidden behind tool search.
5. If the spike shows instructions alone are not reliable enough, the adapter adds a nudge: when a session at this level has no summary yet, or its summary is older than a set number of prompts, the result of the `UserPromptSubmit` hook call carries a one-line reminder. This is the single exception to ADR-0007's rule that `presence_event` returns nothing. At every other level, and whenever the summary is fresh, the result stays empty.
6. A user-supplied style hint, a short free-text setting, is appended to the instructions, so the phrasing can be made casual, terse, or anything else.

### Safety

The summary is model-written text published under the user's name. It is treated as untrusted.

1. **Opt-in only.** A fourth privacy level, `summary`, above `full`. Never the default.
2. **Per-project scope.** An optional allowlist of project directories. When set, the level applies only inside them. A user can enable summaries for personal projects and not for work.
3. **Sanitised in the adapter**, before the control channel, by a pure function: one line, a hard length cap, control characters removed, and links, mentions, invite codes and markup stripped. A phrase that is empty after cleaning is discarded.
4. **Never logged.** The summary is content. Logs record only that one was set and its length.
5. **Never impairs Claude.** The tool returns immediately with an empty result, like `presence_event`. If a permission prompt cannot be avoided for model-initiated calls, the feature must not ship in a form that interrupts the user more than once.
6. **Lifetime.** A summary lives until replaced, and is dropped when the session ends or is cleared.

### Rendering

The first line becomes the activity phrase. The second line carries the project name, status and model. If no summary has been set yet, rendering falls back to the `full` level.

### Claude Desktop Chat

The same tool gives Chat a real activity line, which removes the "only that the app is open" limit for users who opt in. Chat conversations are more personal than code, so this is a separate setting, off even when the Code summary is on. Ticketed as [CRP-053](../../tickets/M5-claude-desktop/CRP-053-desktop-summary.md).

### Acceptance criteria for this ADR

CRP-046 must establish, in real Claude Code sessions:

| # | Criterion |
|---|---|
| S1 | With instructions alone, Claude sets a sensible summary within the first two turns in at least nine of ten fresh sessions, on each current model tier including the smallest |
| S2 | If S1 fails, the nudge in point 5 brings it to that level |
| S3 | Claude updates the summary when the task changes, and does not update it on every turn |
| S4 | The call is made alongside other tool calls in the large majority of cases, so it adds no extra model round trip |
| S5 | The measured cost per session is small and stated: tokens for the tool definition and instructions, tokens per call, and any extra round trips |
| S6 | No permission prompt appears, or exactly one that the user can dismiss permanently, with the steps recorded |
| S7 | With adversarial text planted in the project, the sanitised output contains no link, mention or markup, and never exceeds the cap |
| S8 | The summaries do not include paths, secrets or quoted prompt text across the test sessions |

If S1 and S2 both fail, or S6 fails, the feature is not built in this form and the ADR is marked Rejected with the evidence.

## Consequences

- Presence can say what the user is doing, in Claude's words, at a cost of a few dozen output tokens per task.
- Claude Desktop Chat gains real detail for users who want it.
- The quality of the summary is the model's. It will sometimes be bland or slightly wrong. It cannot be made deterministic.
- An opted-in session carries one more tool definition and a few lines of instructions in context.
- Untrusted content the model reads, such as a web page or a file in a cloned repository, can influence the phrase. The sanitiser bounds the damage to a short line of plain text. It cannot stop the phrase being wrong or embarrassing. The documentation must say so.
- The allowlist depends on the working directory, which the adapter already receives.
- ADR-0008's guarantee changes shape for opted-in users: from "nothing about your work is published" to "one short phrase that Claude wrote for a public audience is published". For everyone else it is unchanged.

## Alternatives considered

| Alternative | Why not |
|---|---|
| Summarise the prompt in our binary with an API call | Network access, a key to manage, cost on every prompt, and it sends prompts somewhere new. Breaks ADR-0008 |
| Run `claude -p` from a hook to summarise | A second model session per prompt. Slow, costly, and needs command hooks |
| `prompt` or `agent` hooks | Built to return a decision, not text. An extra model call each time. Agent hooks are experimental |
| Session title | Only custom titles are exposed, and only at session start |
| Read the transcript | Undocumented and ruled out |
| Derive a phrase from the git branch or repository description | Free and passive, but branch names are terse and can be sensitive. Worth considering later as a fallback at the `full` level |
| Task list events | Claude's own task list holds short descriptions of current work, and hooks fire when tasks are created and completed. Whether those events carry the text is unconfirmed. CRP-046 checks it as a possible passive source |
