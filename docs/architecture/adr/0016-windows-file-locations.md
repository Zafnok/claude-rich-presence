# ADR-0016: On Windows, nothing of ours lives directly under `AppData`

## Status

Accepted.

## Context

The binary keeps three kinds of per-user file: the runtime directory with the lock and control socket ([ADR-0006](0006-control-channel.md)), an optional configuration file, and a log file. The usual Windows homes for these are `%LOCALAPPDATA%` and `%APPDATA%`, which is what the Go standard library's user-directory functions return.

Every adapter must see the same files, wherever it was started: a terminal session, the Claude Desktop Code tab, or Claude Desktop itself.

| Fact | How it is known |
|---|---|
| Claude Desktop on Windows is a packaged Store app with file-system write virtualisation on, excluding only four directories of its own | Read in its package manifest, 2026-10-02 |
| For a packaged app on Windows 10 version 1903 and later, a new file or folder created directly under `AppData\Local` or `AppData\Roaming` goes to a private per-package location. A file that exists only at the real location is opened there | [Microsoft documentation](https://learn.microsoft.com/en-us/windows/msix/desktop/desktop-to-uwp-behind-the-scenes), read 2026-10-02 |
| The redirection reaches every process Claude Desktop starts, at any depth: an extension's server, and a shell inside a Code-tab session. These processes have no package identity | Observed, [CRP-002](../../research/crp-002-desktop-extension.md) |
| A folder under `%APPDATA%` that an outside process creates first is fully shared. One that an inside process creates first exists only in the private copy, and the two sides stay split | Observed, CRP-002 |
| Under `%LOCALAPPDATA%`, the lock file is split the same way, and a Unix socket can be neither bound nor connected from inside | Observed, CRP-002 |
| Folders under `%TEMP%` and under the home directory are shared in both start orders | Observed, CRP-002 |
| Other users have reported the same redirection to Anthropic as a defect | The titles of issues [93152](https://github.com/anthropics/claude-code/issues/93152) and [94254](https://github.com/anthropics/claude-code/issues/94254) in Claude Code's tracker, seen in a web search on 2026-10-02. The issues themselves were not read. Whether the behaviour will change is unknown |

For most of our users the first process to run will be one started by Claude Desktop. So with the usual locations, the split is the normal case, not a corner.

## Decision

On Windows:

1. Configuration and logs live in `%USERPROFILE%\.rich-presence`: the file `config.json` and the folder `logs`.
2. The runtime directory is `%TEMP%\rich-presence`, as [ADR-0006](0006-control-channel.md) states.
3. No code path creates a file or folder directly under `%APPDATA%` or `%LOCALAPPDATA%`. The directory resolvers do not call the standard library's user configuration or user cache directory functions on Windows.
4. The resolvers are pure functions of the operating system and the environment, tested with a Windows environment on every operating system.

macOS and Linux keep the operating system's user configuration directory and the locations in ADR-0006.

## Consequences

- An adapter started by Claude Desktop and one started from a terminal read the same configuration, write the same log and meet at the same socket.
- A dot-folder in the home directory is unusual on Windows, although common among developer tools. Users who look in `%APPDATA%` will not find the configuration; the user documentation and the `doctor` report must say where it is.
- If Anthropic stops redirecting, nothing here breaks.
- A home directory on a network share makes the configuration and logs slower to reach. The socket is not affected, because it is under `%TEMP%`.
- macOS is untested. If Claude Desktop's sandboxing there turns out to redirect anything, this ADR is extended.

## Alternatives considered

| Alternative | Why not |
|---|---|
| Keep `%APPDATA%` and `%LOCALAPPDATA%` | Works only when a process outside Claude Desktop creates the folder first. We cannot arrange that: there is no installer |
| Keep them, and have adapters under Claude Desktop only read, never create | Logs must be created by whoever runs first, and a user asking Claude in a Code-tab session to write the configuration would create the private copy |
| Detect the redirection and write to the real path | Creating a new folder at the real path from inside is exactly what the system redirects |
| `%PROGRAMDATA%` | Shared by all users of the machine, so it needs access control we would have to set and test |
| The Documents folder | Often synchronised to cloud storage or redirected |
| Put the runtime directory in the home folder too, for one location | [ADR-0006](0006-control-channel.md) explains why it stays under `%TEMP%` |
