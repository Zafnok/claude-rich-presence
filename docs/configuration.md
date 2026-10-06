# Configuration reference

Every setting has a default, so no configuration is needed. This page lists what can be changed and how.

A setting that is wrong never stops the program. It is reported as a warning, and the setting keeps the value it would have had without it.

## Where settings come from

Highest first:

1. Environment variables. The name is `RICH_PRESENCE_` followed by the setting's name in upper case, such as `RICH_PRESENCE_PRIVACY`.
2. The configuration file, `config.json`.
3. Defaults.

The file is JSON. Its location:

| Operating system | File |
|---|---|
| Windows | `%USERPROFILE%\.rich-presence\config.json` |
| macOS | `~/Library/Application Support/rich-presence/config.json` |
| Linux | `$XDG_CONFIG_HOME/rich-presence/config.json`, or `~/.config/rich-presence/config.json` when that variable is not set |

## Settings

| Setting | Values | Default | Environment |
|---|---|---|---|
| `enabled` | `true`, `false` | `true` | Yes |
| `privacy` | `off`, `minimal`, `standard`, `full` | `standard` | Yes |
| `discord_application_id` | A numeric string | The project's application id | Yes |
| `idle_clear_after` | A duration such as `15m`. `0` means never | `15m` | Yes |
| `min_update_interval` | A duration such as `15s`, at least `4s` | `15s` | Yes |
| `log_level` | `error`, `warn`, `info`, `debug` | `warn` | Yes |
| `link_hosts` | A list of host names | None | No, file only |
| `projects` | A list of project profiles | None | No, file only |

Two more environment variables exist for tests and unusual setups. They are not settings and have no entry in the file. `RICH_PRESENCE_RUNTIME_DIR` moves the directory of the control socket ([ADR-0006](architecture/adr/0006-control-channel.md)). `RICH_PRESENCE_DISCORD_ENDPOINT` replaces the search for Discord with one endpoint name ([ADR-0018](architecture/adr/0018-discord-endpoint-override.md)).

What each privacy level publishes is in [ADR-0008](architecture/adr/0008-privacy-and-safety-by-default.md). `off` publishes nothing: see [Hiding a project](#hiding-a-project).

### What `full` publishes

`full` adds the project name to what `standard` publishes. The project name is the name of the directory Claude is working in, unless a [project profile](#project-profiles) gives that directory a display name.

Claude can create directories and work in them. So at `full` the project name can be text that Claude chose, and not only the names of directories you made. It is cleaned before it is published, exactly as a display name is: one line of at most 128 bytes, with no formatting characters, no mention or link syntax, and no control or invisible characters. Cleaning limits how the name is shown, not what it says. If that matters to you, keep the global level at `standard` and set `full` only in the profiles of the projects you want named.

## Project profiles

A profile gives one project its own settings, so that a few chosen projects can be visible while everything else stays private. Profiles are designed in [ADR-0012](architecture/adr/0012-project-profiles-and-repository-link.md).

Profiles are read from your own configuration file and from nowhere else. An environment variable cannot define one, and nothing inside a project directory is ever read, so cloning a repository cannot change what you publish.

Each entry of `projects` is an object:

| Field | Required | Meaning |
|---|---|---|
| `path` | Yes | The project's directory, as an absolute path |
| `privacy` | No | The privacy level for this project. It may be lower or higher than the global one, and `off` hides the project |
| `name` | No | A display name, shown in place of the directory name |
| `areas` | No | The project's main parts, such as "battle engine" or "story" |
| `link` | No | The repository's address, shown as a button. See [Sharing a project](#sharing-a-project) |

### Example

```json
{
  "privacy": "minimal",
  "link_hosts": ["gitlab.com"],
  "projects": [
    {
      "path": "C:\\Users\\me\\code\\visions",
      "privacy": "full",
      "name": "Visions of Shuyi",
      "areas": ["battle engine", "story"],
      "link": "https://github.com/me/visions"
    },
    {
      "path": "C:\\Users\\me\\code\\client-work",
      "privacy": "minimal"
    }
  ]
}
```

In JSON a backslash is written twice. On Windows you may write forward slashes in a path, such as `C:/Users/me/code/visions`.

### Which profile applies

- A session uses a profile when its working directory is the profile's `path` or anywhere inside it. A worktree under the project directory is therefore covered. A clone or worktree somewhere else is not, unless it has its own profile.
- When several profiles match, the one with the longest path applies. Its values are used over the global ones. Values are not combined across profiles.
- A session that matches no profile uses the global settings.
- Paths are compared as text after cleaning: `.` and `..` segments and repeated or trailing separators are removed. On Windows and macOS upper and lower case are the same, and on Windows either kind of separator is accepted.
- **Symbolic links are not resolved.** The program does not look at the file system to match a profile. If you reach a project through a link, write the path that sessions actually report as their working directory.
- A path must be absolute. `~` and relative paths are not expanded.

### Limits and cleaning

| Field | Rule |
|---|---|
| `projects` | At most 64 profiles. The rest are ignored with a warning |
| `name` | One line, at most 128 bytes. Line breaks become spaces. Control and invisible characters and the markup characters ``* ` ~ | < > [ ] \ @`` are removed. A longer name is cut |
| `areas` | At most 8. Each is cleaned like a name and cut to 32 bytes. Empty entries are dropped, and so are entries that repeat an earlier one apart from case and spacing |
| `link_hosts` | At most 16 host names, such as `gitlab.com`: no scheme, port or path |

### The link

A link is accepted only in this form:

```
https://github.com/owner/repository
```

- `https` only.
- The host must be exactly `github.com` or one listed in `link_hosts`. A host that merely contains or ends with an allowed name is rejected.
- No user name, password or token, no port, no query string, no fragment, and no escaped characters.
- Exactly two path segments, the owner and the repository, each made of letters, digits, `.`, `-` and `_`, and at most 100 characters.
- A trailing `.git` is accepted and removed.
- The SSH form, `git@github.com:owner/repository.git`, is not accepted and is not converted.

The program makes no network requests, so it cannot tell whether a repository is public. A link to a private repository reveals its owner and name and leads nowhere.

### Hiding a project

The privacy level `off` keeps a session off Discord entirely ([ADR-0012](architecture/adr/0012-project-profiles-and-repository-link.md), "Hiding and pausing"). A hidden session is never sent to the process that holds the Discord connection. So it is not counted among your sessions, it is never the one the card describes, and if it is your only session the card is not shown at all.

Set it in a project's profile to hide that project:

```json
{
  "projects": [
    { "path": "/home/me/work/client-project", "privacy": "off" }
  ]
}
```

Set it as the global `privacy` to hide everything except the projects whose profiles name another level.

Two things follow from where a session's level comes from, which is the directory its hooks report:

- When any level in your configuration is `off`, a new session stays hidden until its first hook says which directory it is in. In Claude Code that is your first prompt. Without an `off` anywhere, a new session is shown at once, at `minimal`.
- A session that moves into a hidden project disappears from Discord, and one that moves out appears.

The Claude Desktop app has no project, so it follows the global level: at `off` it is hidden.

## Pausing

A pause switches the whole presence off for a while without changing any setting. It covers every open session. In Claude Code, ask Claude to pause your Discord status, for a length of time or until you say, and to resume it: the plugin's `pause` and `resume` skills call the tool `presence_pause` for you.

- A pause with a length ends by itself. The longest is one week.
- A pause does not end when the session that asked for it closes. It lasts as long as any session is open, and for its remaining time whichever process holds the Discord connection.
- While presence is paused nothing is sent to Discord. Your sessions go on being tracked, so that a resume shows what they are doing now.
- The tool can pause and resume, and nothing else. It cannot change what is shown, a privacy level or a profile.
- If another open session runs a version from before pausing and holds the Discord connection, the tool says that pausing is unavailable. Restart that session.

## Previewing the card

Much of the card is hover text, and Discord does not show you your own button. To see every part of the card as it is shown now, ask Claude to preview your Discord card: the plugin's `preview` skill calls the status tool with `preview` set. It lists both text lines, the timer, both images and their hover text, the button and its link, and says whether presence is paused and whether this session is hidden.

The preview can name your project, so it is private. It is returned to your own session and is never written to the log. The plain status report, from the `status` skill or `rich-presence status`, names nothing and is safe to paste into a public issue.

## Sharing a project

A profile with a `link` puts one button on your Discord status, such as "View on GitHub", while a session inside that project is the one your status describes. With several sessions open, the button belongs to the session in focus, and it goes away when focus moves to a session whose project has no link.

- **The link is per project.** There is no setting that turns links on everywhere, and nothing is detected from a project's git configuration. A project has a link only if its own profile sets one.
- **The link does not depend on the privacy level.** It is shown for that project at `minimal` too, because setting it is its own decision. The owner and the repository name are part of the link, so a link names the project even where the project name is otherwise hidden.
- **The program cannot tell whether a repository is public.** It makes no network requests. A link to a private repository reveals its owner and name to everyone who can see your status, and leads nowhere. If you make a repository private later, remove the link.
- **Other people see the button; you may not.** On Discord's desktop app and on its mobile app, the button is shown to other people on your profile and is not shown to you on your own. To check it, ask someone else to look.
- **The label comes from the host**: GitHub, GitLab, Bitbucket and Codeberg are named, and any other host listed in `link_hosts` gets "View repository".
- **A change applies to new sessions.** A session that is already open keeps the settings it started with.

In Claude Code, the plugin's `share-project` skill does the setup for the project you are in: ask Claude to share this project in your Discord status. It reads the git remote, checks with your own GitHub CLI that the repository is public when it can, shows you exactly what will be published, and edits the configuration file once you confirm. It refuses a repository that the GitHub CLI reports as private.

**To stop sharing a project**, ask Claude to stop sharing it, which runs the same skill, or edit the file yourself: delete the `link` line from the project's entry to keep its other settings, or delete the whole entry. Then restart the sessions that are open.

### When an entry is wrong

Each problem gives one warning. A warning names the entry by its position, counting from zero, such as `projects[1].link`. It never repeats what you wrote.

| Problem | What happens |
|---|---|
| The entry is not an object, or has no absolute `path` | The profile is skipped. The others load |
| The `path` repeats an earlier profile's | The later profile is skipped |
| `privacy` is not a known level | The profile applies with the global level |
| `name` is not text, or nothing is left after cleaning | The profile applies and the directory name is used |
| `areas` is not a list of strings | The profile applies without areas |
| `link` is rejected | The profile applies and no link is published |
| A field that is not listed above | The profile applies and the field is ignored |
