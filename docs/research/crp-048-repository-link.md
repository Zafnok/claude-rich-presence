# CRP-048 findings: the repository link

Ticket: [CRP-048](../tickets/M4-claude-code/CRP-048-repository-link.md). Decision: [ADR-0012](../architecture/adr/0012-project-profiles-and-repository-link.md).

**Coverage.** The link is built and tested end to end against the fake Discord. On a real Discord, on the Windows desktop client, the button is shown to another account and opens the link, and is not shown to the user on their own profile. Mobile was not checked. How the `share-project` skill behaves in a real Claude Code session has **not been observed**. What remains is written out under [Checks for the owner](#checks-for-the-owner). Nothing below fills that gap with an assumption.

## What was built

| Layer | What it does |
|---|---|
| Configuration | Unchanged. The profile's `link`, validated when the file is loaded (CRP-014) |
| Adapter | Takes the link from the settings resolved for the session's working directory and puts it on the session's opening event. No field of the event tool is read for it. A change of link ends the session and opens it again |
| Control channel | An optional `link` on the session and on the event. No new protocol version |
| Host | Validates every link again with the same function, against its own list of hosts. A link that fails is dropped alone, with a warning that does not repeat it |
| Renderer | One button for the focus session's link. The label comes from a table by host: GitHub, GitLab, Bitbucket, Codeberg, otherwise "View repository" |
| Discord codec | `buttons`, a list of one object with `label` and `url`. A button outside Discord's limits is left out |
| Plugin | The `share-project` skill, which also removes a project |

## Choices made here

| Choice | Reason |
|---|---|
| The link is shown at every privacy level, `minimal` included | ADR-0012 says a link is published when a profile sets one, and names no level. A profile that sets a link and no level takes the global level, so tying the link to a level would silently hide it for a user who is at `minimal` globally, which is the case the ADR describes. The documentation says that the link names the project. Recorded in ADR-0012 |
| A change of link reopens the session | The ticket has the link set when the session opens and by nothing after. Reopening is how the adapter already takes away what was published at a higher level |
| The control message is not refused over a bad link | The ticket requires the session to be accepted when its link is invalid, so the protocol layer does not judge the field and the host does |
| The skill asks for confirmation itself, and does not rely on Claude Code's permission prompt | See [Permission modes](#permission-modes) |
| One skill with a "remove" path, not two skills | The ticket allows either |

## Discord's fields

Read on 2026-10-05, in the activity object of Discord's gateway documentation and the set-activity example of its RPC documentation.

| Fact | Basis |
|---|---|
| `buttons` is sent as a list of up to two objects, each with a `label` of 1 to 32 characters and a `url` of 1 to 512 | Docs |
| An activity Discord sends back lists the buttons as labels only, and bots cannot read the URLs | Docs |
| `details_url` and `state_url` make a text line a link; `large_url` and `small_url`, in the assets, make an image one | Docs |
| The fake Discord accepts the button and records it as sent | Observed, `test/e2e: TestRepositoryLink` |
| A real Discord accepts an activity with a button over the local pipe | Observed 2026-10-05, Windows desktop client: the activity was shown with both text lines and the timer, and the log has no error. An activity is accepted or rejected whole |
| The user sees the button on their own profile | **No**, on the desktop client's own profile popout. Observed 2026-10-05, at `full`: the name, both lines and the timer were shown and no button |
| Another account sees the button on desktop | **Yes.** Observed 2026-10-05, at `full`: a "View on GitHub" button under the activity on the profile card, and it opened the repository |
| Another account sees the button on mobile | **Not observed** |
| The button shows for the "playing" activity type, which is the one sent | Yes, as above. Other types were not tried; CRP-070 measures them |

## Permission modes

ADR-0012 assumed that Claude Code asks the user before editing a file outside the working directory in its default permission modes, and that this prompt is the consent step. Read in Claude Code's documentation of permission modes on 2026-10-05, with Claude Code 2.1.290 installed:

| Mode | Is the user asked before the skill's edit to the configuration file | Basis |
|---|---|---|
| Manual (`default`) | Yes. Only reads run without asking | Docs |
| `acceptEdits` | Yes. Edits are accepted only inside the working directory and the directories added to it | Docs |
| `plan` | No edit is made while planning | Docs |
| `auto` | **No.** An edit outside the working directory goes to the classifier, not to the user. From Claude Code 2.1.283 this is the starting mode of interactive terminal and VS Code sessions | Docs |
| `dontAsk` | The edit is denied unless a rule allows it | Docs |
| `bypassPermissions` | No | Docs |

So the assumption does not hold in the mode most sessions now start in. As the ticket provides for that case, the skill has an explicit confirmation step of its own: it shows the link, the name, the level and the entry it will write, and writes only after a clear yes. ADR-0012 and risk R19 are corrected to say so.

Observed once, on 2026-10-05 with Claude Code 2.1.290 in auto mode: the skill's write of a new `config.json` under the user profile was "Allowed by auto mode classifier" and Claude Code asked the user nothing. The only question the user saw was the skill's own. That agrees with the documentation's row for `auto`. The other modes were not observed.

## Checks for the owner

### 1. What Discord shows

Needs Discord running on this machine, signed in, with activity sharing on, and a second account that can see the first one's profile.

```bash
go build -o bin/rich-presence.exe ./cmd/rich-presence
```

```bash
pwsh -File docs\research\crp-048-discord-check.ps1 -ApplicationId <the Discord application id>
```

The [script](crp-048-discord-check.ps1) shows a presence with a "View on GitHub" button for five minutes. It uses a home directory and a configuration file of its own under `%TEMP%\rp-link-check`, so it does not touch the real configuration. It was tried on 2026-10-05 against an endpoint that is not Discord: the binary started, took the hook and reported one session at `standard`.

Record, for each row, what is seen:

| Viewer | Where | Is the button shown | Does it open the link | Notes |
|---|---|---|---|---|
| The user | Desktop, own profile | No | Not applicable | 2026-10-05, at `full`. Everything else on the card was shown |
| The user | Mobile, own profile | | | |
| Second account | Desktop, profile card | Yes | Yes, to the right repository | 2026-10-05, at `full` |
| Second account | Mobile, profile | | | |

Run it once more with `-Privacy minimal` and confirm the button is still there with only the first text line.

The second account sees the button, so the button stands and the text line's URL field is not needed. The mobile rows and the run at `minimal` are still open.

### 2. The `share-project` skill

Needs an interactive Claude Code session with the plugin loaded from this branch, for example `claude --plugin-dir plugin`, started in each repository below. Ask it to share the project in Discord presence.

| Case | Expected | Observed |
|---|---|---|
| A public GitHub repository, GitHub CLI signed in | Reports it public, shows the summary, writes the profile after a yes | **As expected**, 2026-10-05, Claude Code 2.1.290 on Windows, auto mode, started in a worktree of this repository. It read the remote, ran `gh repo view` for the visibility, proposed eight areas, asked one question naming the link, and wrote the file only after "Yes, write it as shown". It chose the main checkout as the profile's path, not the worktree, which covers the worktrees inside it |
| A private GitHub repository | Stops with an explanation and writes nothing, even when asked to go on | **As expected**, 2026-10-05, same setup. `gh repo view` reported the repository private; the skill stopped, explained that the link would reveal the owner and name and lead to a not-found page, and left the configuration file as it was. It was not then asked to go on regardless |
| A repository whose `origin` is `https://user:token@github.com/owner/repo.git` | The link shown and written has no credential, and the token is not repeated | |
| GitHub CLI signed out | Says it could not check and asks whether the repository is public | |
| "Stop sharing this project" | Shows the profile, asks whether to remove the link or the whole entry, and edits accordingly | |

For each of the manual, accept-edits and auto modes, note whether Claude Code itself asked to approve the edit to the configuration file.

## Not tested

- Everything in [Checks for the owner](#checks-for-the-owner).
- A host other than `github.com`, on a real Discord.
- macOS and Linux, beyond the automated tests in CI.
