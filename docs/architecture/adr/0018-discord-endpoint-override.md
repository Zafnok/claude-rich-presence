# ADR-0018: An environment variable that names the Discord endpoint

## Status

Accepted.

## Context

The end-to-end tests ([CRP-043](../../tickets/done/CRP-043-end-to-end-tests.md)) run the built binary against a fake Discord. Each test must reach its own fake under its own name, and must never reach a Discord that is running on the machine.

| Fact | How it is known |
|---|---|
| On Linux and macOS the binary looks for Discord's socket in a directory taken from the environment, so a test can point it at a directory of its own | [candidates.go](../../../internal/discord/transport/candidates.go), from Discord's documentation ([CRP-021](../../tickets/done/CRP-021-discord-transport.md)) |
| On Windows the endpoint is the named pipe `\\?\pipe\discord-ipc-N`. No environment variable moves it, and all pipes on a machine share one namespace | The same file. Discord's documentation gives the name as fixed |
| So on Windows the built binary could reach only the real name. A fake listening there would collide with a real Discord on a developer's machine, and the binary would publish test presence to the real one whenever it won the name | Follows from the two facts above |
| Tests inside the process already avoid this by handing the dialer a prefix, through `transport.NewAt`. A separate process has no such seam | [dial.go](../../../internal/discord/transport/dial.go) |
| The control channel has the same need and meets it with `RICH_PRESENCE_RUNTIME_DIR`, "for tests and for unusual setups" | [ADR-0006](0006-control-channel.md), rule 4 |

## Decision

1. When the environment variable `RICH_PRESENCE_DISCORD_ENDPOINT` is set and not empty, its value is the only endpoint name the binary tries, with the indices 0 to 9 added. Nothing else is searched. It is a socket path on Linux and macOS and a pipe name on Windows, each up to and including `discord-ipc-`.
2. It is read in one place, the pure function that lists the candidate endpoints, so the `mcp` command and the `doctor` command agree on where Discord is.
3. It is not a setting. It has no entry in the configuration file and is not offered by the plugin or the extension.
4. Every end-to-end test sets it. A test of the built binary that does not is a defect.

## Consequences

- The end-to-end tests can run on Windows, in parallel, on a machine where Discord is running.
- The shipped binary has one more input. Whoever controls the process's environment can point presence at another local endpoint. That is no new power: the same environment already chooses the socket directory on Linux and macOS, and the configuration directory everywhere.
- A user who sets it by mistake gets no presence, and the `doctor` command reports that Discord was not found without saying why. If that ever happens in practice, the doctor should name the variable when it is set.
- The value is not validated. A value that names nothing behaves as Discord not running.

## Alternatives considered

| Alternative | Why it lost |
|---|---|
| Have the fake listen at the real pipe name on Windows | It collides with a real Discord, the tests could not run in parallel, and a test could publish to a real account |
| A build tag or a linker flag that changes the name in a test build | The tests would then run a binary that is not the one shipped, which is what they exist to avoid |
| A hidden command-line flag on the `mcp` command | The `doctor` and `status` commands would need it too, and Claude starts `mcp` with fixed arguments. An environment variable reaches every command the same way |
| Skip the scenarios that need Discord on Windows | Windows is where most users are, and where the lock and pipe behave least like the other two |
