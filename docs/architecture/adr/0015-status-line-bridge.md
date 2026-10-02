# ADR-0015: An opt-in status line bridge for model, context, cost and usage limits

## Status

**Proposed.** Settled by [CRP-045](../../tickets/M4-claude-code/CRP-045-spike-initial-model.md), which now covers the whole bridge and not only the model name.

## Context

Other projects show the model, tokens, cost, context usage and the 5-hour and weekly usage limits. The owner wants these available, opt-in, provided they take no space from the summary ([ADR-0013](0013-summary-first-card-layout.md)).

Hook events do not carry most of them. The model arrives only at session start and on a switch, and nothing in a hook carries cost, context or limits.

Almost every other project gets the usage limits by reading Claude Code's stored login token and calling Anthropic's usage endpoint with it. Two read Claude Desktop's private cache files. We will not do either, and not only because of our own rules:

- Anthropic's terms for Claude Code say that "developers may not collect, store, or intermediate Claude.ai credentials or session tokens", and that Anthropic may enforce this without notice. A presence tool that reads the token and calls an endpoint with it puts its users' accounts at risk, whether or not they opted in.
- It would also need a network connection, which [ADR-0008](0008-privacy-and-safety-by-default.md) excludes.

There is a documented source for the same numbers. Claude Code's status line runs a command the user configures and passes it session data on standard input. Read on 2026-10-02, that data includes:

| Field | Notes |
|---|---|
| Model id and display name | Present from session start |
| Effort level, fast mode | |
| Context window used, as a percentage | May be empty early in a session |
| Session cost in dollars | An estimate at list price |
| 5-hour and 7-day usage, as percentages, with reset times | Pro and Max subscribers only, after the first response |
| Open pull request for the current branch | When one exists |
| Session name, including an automatically generated title | Not written for a public audience |

The same page states the limits of the mechanism:

| Fact | Basis |
|---|---|
| The status line is a user or project setting. A plugin cannot set it. A plugin can only supply a status line for subagent rows | Docs |
| There is one status line command per user | Docs |
| It runs through a shell: Git Bash on Windows when present, otherwise PowerShell | Docs |
| It runs at session start and after each assistant message, debounced, and a running command is cancelled when a new update arrives | Docs |
| Whether it runs at all in the Claude Desktop Code tab and the IDE extensions, which have no terminal status row | **Unverified** |

## Decision

1. **A second, optional adapter.** The binary gains a `statusline` command. It reads the status line data, keeps only an allowlist of fields, sends them to the presence host, and exits.
2. **Allowlist.** Model, effort, fast mode, context percentage, session cost, the two usage percentages with their reset times, and whether a pull request is open. Never the transcript path, the working directories, the repository identity, or the session name.
3. **It must not take the user's status line away.** If the user already has a status line command, ours runs it and passes its output through unchanged. If they have none, ours prints nothing.
4. **Opt-in, set up by a skill.** A plugin skill, `connect-statusline`, shows what will change, then edits the user's settings to point the status line at our command, wrapping any existing one, and can undo it. The edit goes through Claude Code's normal permission prompt.
5. **A stable path for the binary.** The status line needs a path that survives plugin updates. How to provide one is the main question for the spike: the plugin's data directory is the candidate.
6. **Where the facts go.** Hover text and, if the user chooses, a usage gauge as the small image. Never the text lines while a summary is shown. See ADR-0013.
7. **Each fact is its own opt-in.** Cost and usage limits are off by default even when the bridge is connected.
8. **No token, no endpoint.** The program never reads Claude's stored credentials and never contacts Anthropic. If the status line does not supply a number, the number is not shown.

## Consequences

- Usage limits, cost and context become available with no credentials and no network, on the surfaces where the status line runs.
- The model is known from the first moment of a session there, which closes the gap recorded as R9.
- Setup is a step the user takes. It cannot be automatic.
- If the status line does not run in the Claude Desktop Code tab, these facts are unavailable there, and the model keeps arriving late. That would be a permanent limit on that surface.
- A second short-lived process runs after each assistant message. It must be fast, and it must never break the user's own status line: on any internal error it still passes the original output through.
- Usage limits are published to Discord only for users who turn that on. They say something about how much someone uses Claude, which some will not want public.

## Alternatives considered

| Alternative | Why not |
|---|---|
| Read the login token and call the usage endpoint | Against Anthropic's terms, needs network access, and handles a credential |
| Read Claude Desktop's cached usage file | Undocumented, and has already moved once according to another project |
| Compute cost and tokens from the transcript | Undocumented format, and ruled out by ADR-0008 |
| Publish the automatically generated session title as the summary | It costs nothing, but it was not written for a public audience. CRP-045 records what the titles look like, in case an opt-in fallback is wanted later |
| Ask the user to write their own status line script that calls us | Works for some, but the setup skill is what makes it usable |
