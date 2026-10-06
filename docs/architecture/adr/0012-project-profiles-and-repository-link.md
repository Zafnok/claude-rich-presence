# ADR-0012: Per-project profiles and an opt-in repository link

## Status

**Proposed.** The profile mechanism needs nothing verified. The link is built as a button in [CRP-048](../../tickets/M4-claude-code/CRP-048-repository-link.md); how Discord displays it to the user and to other people has not yet been observed on a real Discord, and this ADR stays Proposed until it has. See [the findings](../../research/crp-048-repository-link.md). Extends [ADR-0008](0008-privacy-and-safety-by-default.md) and replaces the directory allowlist first sketched in [ADR-0011](0011-model-authored-activity-summary.md).

## Context

The owner wants presence to be a way of giving chosen projects visibility: for a project the user opts in, show a link to its public repository, alongside the summary. This is a per-project choice. Most of a user's work should stay private; a few projects are ones they want people to find.

Facts:

| Fact | Basis |
|---|---|
| A Discord activity may carry up to two buttons, each a label of 1 to 32 characters and a URL of up to 512 | Docs |
| Newer activity fields also let the text lines and images carry URLs | Docs |
| How buttons appear to other users, and whether the user sees their own, is described inconsistently by third parties | Unverified |
| A git remote URL can embed credentials, as in `https://user:token@host/...` | Known git behaviour |
| Whether a repository is public can only be learned by asking its host over the network | Design |
| Our binary makes no network requests | [ADR-0008](0008-privacy-and-safety-by-default.md) |
| Claude Code asks the user before editing a file outside the working directory in its manual and accept-edits modes. In auto mode, which is the starting mode of interactive sessions from Claude Code 2.1.283, a classifier reviews such an edit and the user is not asked. In the mode that bypasses permissions nothing asks | Docs, read 2026-10-05 for CRP-048. Not yet observed |
| A session in the Claude Desktop Code tab may run in a worktree under the project's own directory | Observed |

## Decision

### Project profiles

1. Configuration gains a list of **project profiles**. Each names a directory and may set, for sessions whose working directory is inside it:

   | Field | Meaning |
   |---|---|
   | `path` | The project's root directory |
   | `privacy` | A privacy level for this project, overriding the global one in either direction. This includes `off`, which keeps the project off Discord entirely |
   | `personality` | A personality for this project. See [ADR-0014](0014-personalities.md) |
   | `name` | A display name, such as "Visions of Shuyi", used in place of the directory name |
   | `areas` | An optional list of the project's main parts, such as "battle engine" or "story". Claude chooses from it when labelling a task, which is what lets several sessions roll up to a shared line. See [ADR-0011](0011-model-authored-activity-summary.md) |
   | `link` | A repository URL to publish |

2. Profiles live **only in the user's own configuration file**. A file inside a repository can never opt a user in, because then cloning a repository would change what the user publishes.
3. The most specific matching profile wins. A session that matches no profile uses the global settings.
4. Profiles are resolved in the adapter, before the control channel, like every other privacy decision.

This replaces the separate allowlist in ADR-0011: enabling the summary for chosen projects is now `privacy: summary` in their profiles.

### The link

1. A link is published **only when a profile sets one**. There is no global switch that turns links on for every project, and no automatic detection.
2. **The link never comes from the model.** ADR-0011 strips links from model-written text, and that stands. The link comes from the user's configuration and nowhere else.
3. **Validated strictly**, by a pure function, in the adapter and again in the host:
   - `https` only;
   - host on an allowlist, `github.com` by default, extendable in configuration;
   - no user name, password or token in the URL;
   - no query string and no fragment;
   - a path of the form owner and repository, in a conservative character set;
   - within Discord's length limit.
   
   A link that fails validation is dropped with a warning that does not echo it.
4. **Shown as a button** on the activity, labelled for the host, such as "View on GitHub". Shown for the focus session only, and at every privacy level: the link is its own per-project opt-in, and a user who sets one at `minimal` has asked for it to be shown. If CRP-048 finds that buttons are not visible where it matters, the text line's URL field is the alternative.
5. **The binary does not check that the repository is public.** That would need a network request. Instead:
   - opting in is an explicit, per-project act;
   - the plugin provides a skill, `share-project`, that performs the setup inside a Claude Code session: it reads the remote, checks visibility with the user's own GitHub CLI when available, shows exactly what would be published, asks the user to confirm that summary, and only then writes the profile by editing the configuration file. The skill's own question is the consent step, because Claude Code's permission prompt for the edit is not shown in every permission mode. Where the prompt is shown it is a second check, and the skill does nothing to avoid it;
   - the documentation says plainly that a link to a private repository reveals its owner and name and leads nowhere.
6. There is **no tool that lets the model change configuration**. A profile is written by the user, or by an edit the user approves.

### Hiding and pausing

1. **Hiding** is the privacy level `off`, set globally or in a profile. A hidden session is not sent to the presence host at all, so it is not counted and cannot be shown.
2. **Pausing** switches the whole presence off for a period or until resumed, without changing any setting. It is held by the presence host and survives a change of host.
3. Pausing is done through a tool the model calls when the user asks. That is acceptable where a tool that writes profiles is not, because a pause can only reduce what is published.

Ticketed as [CRP-073](../../tickets/M7-personalisation/CRP-073-hide-pause-preview.md).

### Not for Claude Desktop Chat

Chat has no project. Profiles and links apply to Claude Code sessions only.

## Consequences

- One mechanism now covers per-project privacy, display names, summaries and links.
- A user can run at `minimal` globally and opt a single project in to everything.
- The binary stays offline. The cost is that it cannot notice a repository being made private later; the link then goes dead until the profile is removed.
- Matching is by directory, so a clone or worktree of the same repository elsewhere is not covered unless it has its own profile. Worktrees under the project directory are covered.
- The control protocol gains a validated link field and a display-name field. Both are additive.
- In permission modes that do not ask the user about an edit outside the working directory, which now include the starting mode of interactive sessions, text in a repository could talk Claude into editing the configuration file to opt that project in. The result would be a link and a phrase about that project being published. This is recorded as a risk; it is bounded by validation, by the skill's rule that only the user's own request starts it, and in auto mode by the classifier that reviews the edit.

## Alternatives considered

| Alternative | Why not |
|---|---|
| Detect the remote from the project's git configuration and publish it automatically at some privacy level | A link is the most identifying thing we could publish. It should never appear without a per-project decision |
| Check visibility from the binary through the host's API | A network request from a program that promises none |
| A committed file in the repository that enables the link | Lets a repository opt its users in |
| Let Claude supply the link through the summary tool | Model-written links are an injection route. See ADR-0011 |
| A tool on our server that writes profiles | Would let the model change what is published without the user seeing an edit |
| Key profiles by remote URL instead of directory | Covers every clone, but requires the binary to read and parse git metadata, including linked worktrees. Worth revisiting if directory matching proves awkward |
