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
| `privacy` | `minimal`, `standard`, `full` | `standard` | Yes |
| `discord_application_id` | A numeric string | The project's application id | Yes |
| `idle_clear_after` | A duration such as `15m`. `0` means never | `15m` | Yes |
| `min_update_interval` | A duration such as `15s`, at least `4s` | `15s` | Yes |
| `log_level` | `error`, `warn`, `info`, `debug` | `warn` | Yes |
| `link_hosts` | A list of host names | None | No, file only |
| `projects` | A list of project profiles | None | No, file only |

Two more environment variables exist for tests and unusual setups. They are not settings and have no entry in the file. `RICH_PRESENCE_RUNTIME_DIR` moves the directory of the control socket ([ADR-0006](architecture/adr/0006-control-channel.md)). `RICH_PRESENCE_DISCORD_ENDPOINT` replaces the search for Discord with one endpoint name ([ADR-0018](architecture/adr/0018-discord-endpoint-override.md)).

What each privacy level publishes is in [ADR-0008](architecture/adr/0008-privacy-and-safety-by-default.md).

## Project profiles

A profile gives one project its own settings, so that a few chosen projects can be visible while everything else stays private. Profiles are designed in [ADR-0012](architecture/adr/0012-project-profiles-and-repository-link.md).

Profiles are read from your own configuration file and from nowhere else. An environment variable cannot define one, and nothing inside a project directory is ever read, so cloning a repository cannot change what you publish.

Each entry of `projects` is an object:

| Field | Required | Meaning |
|---|---|---|
| `path` | Yes | The project's directory, as an absolute path |
| `privacy` | No | The privacy level for this project. It may be lower or higher than the global one |
| `name` | No | A display name, shown in place of the directory name |
| `areas` | No | The project's main parts, such as "battle engine" or "story" |
| `link` | No | The repository's address |

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
