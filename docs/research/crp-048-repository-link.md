# CRP-048 findings: the repository link

Ticket: [CRP-048](../tickets/M4-claude-code/CRP-048-repository-link.md). Decision: [ADR-0012](../architecture/adr/0012-project-profiles-and-repository-link.md).

**Coverage.** The link is built and tested end to end against the fake Discord. What a real Discord shows, and how the `share-project` skill behaves in a real Claude Code session, has **not been observed**. Those checks need the owner's Discord, a second account and an interactive session, and are written out under [Checks for the owner](#checks-for-the-owner). Nothing below fills that gap with an assumption.

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
| A real Discord accepts an activity with a button over the local pipe | **Not observed** |
| The user sees the button on their own profile | **Not observed.** Third-party descriptions disagree |
| Another account sees the button, on desktop and on mobile | **Not observed** |
| The button shows for the "playing" activity type, which is the one sent | **Not observed.** One other project reports that buttons show for one type only. CRP-070 measures this and has not run |

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

None of this was observed in a session. The table is the documentation's account.

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
| The user | Desktop, own profile | | | |
| The user | Mobile, own profile | | | |
| Second account | Desktop, profile or member list popout | | | |
| Second account | Mobile, profile | | | |

Run it once more with `-Privacy minimal` and confirm the button is still there with only the first text line.

If the second account does not see the button, the ticket switches to the text line's URL field, as ADR-0012 allows.

### 2. The `share-project` skill

Needs an interactive Claude Code session with the plugin loaded from this branch, for example `claude --plugin-dir plugin`, started in each repository below. Ask it to share the project in Discord presence.

| Case | Expected | Observed |
|---|---|---|
| A public GitHub repository, GitHub CLI signed in | Reports it public, shows the summary, writes the profile after a yes | |
| A private GitHub repository | Stops with an explanation and writes nothing, even when asked to go on | |
| A repository whose `origin` is `https://user:token@github.com/owner/repo.git` | The link shown and written has no credential, and the token is not repeated | |
| GitHub CLI signed out | Says it could not check and asks whether the repository is public | |
| "Stop sharing this project" | Shows the profile, asks whether to remove the link or the whole entry, and edits accordingly | |

For each of the manual, accept-edits and auto modes, note whether Claude Code itself asked to approve the edit to the configuration file.

## Not tested

- Everything in [Checks for the owner](#checks-for-the-owner).
- A host other than `github.com`, on a real Discord.
- macOS and Linux, beyond the automated tests in CI.
