package transport

import (
	"path"
	"strconv"
)

// endpointName is what every Discord IPC endpoint is called, before its index.
const endpointName = "discord-ipc-"

// indices is how many endpoints one location can hold: 0 to 9.
const indices = 10

// windowsPrefix is the named pipe, as Discord's documentation spells it.
const windowsPrefix = `\\?\pipe\` + endpointName

// runtimeDirVariables are the environment variables that can name the
// directory of the socket, in the order Discord's documentation gives.
var runtimeDirVariables = []string{"XDG_RUNTIME_DIR", "TMPDIR", "TMP", "TEMP"}

// fallbackDir is used when none of runtimeDirVariables is set.
const fallbackDir = "/tmp"

// sandboxDirs are where a sandboxed Discord on Linux puts its socket, below
// the runtime directory. They are not in Discord's documentation. They come
// from the source of pypresence, pypresence/utils.py, read on 2026-10-03:
// the Snap package, then the Flatpak stable and Canary builds.
var sandboxDirs = []string{
	"snap.discord",
	"app/com.discordapp.Discord",
	"app/com.discordapp.DiscordCanary",
}

// EnvEndpoint names the environment variable that replaces the whole search
// with one endpoint name, given without its index. It is for tests of the
// built binary, which must reach a fake Discord under a name of their own and
// never a real one, and for unusual setups. Windows has no other way to do
// that: the pipe name is fixed and no variable moves it.
const EnvEndpoint = "RICH_PRESENCE_DISCORD_ENDPOINT"

// Prefixes lists, in the order to try them, the endpoint names without their
// index, for an operating system named as runtime.GOOS names it and an
// environment. An environment variable that is empty counts as not set. When
// EnvEndpoint is set, its value is the only prefix.
//
// It is a pure function, so every operating system's list can be tested on
// any other. Unix paths are always joined with a forward slash.
func Prefixes(goos string, getenv func(string) string) []string {
	if override := getenv(EnvEndpoint); override != "" {
		return []string{override}
	}
	if goos == "windows" {
		return []string{windowsPrefix}
	}
	dir := fallbackDir
	for _, name := range runtimeDirVariables {
		if value := getenv(name); value != "" {
			dir = value
			break
		}
	}
	prefixes := []string{path.Join(dir, endpointName)}
	if goos == "linux" {
		for _, sub := range sandboxDirs {
			prefixes = append(prefixes, path.Join(dir, sub, endpointName))
		}
	}
	return prefixes
}

// Candidates lists every endpoint to try, in order: each prefix in turn, with
// the indices 0 to 9.
func Candidates(prefixes []string) []string {
	candidates := make([]string, 0, len(prefixes)*indices)
	for _, prefix := range prefixes {
		for i := range indices {
			candidates = append(candidates, prefix+strconv.Itoa(i))
		}
	}
	return candidates
}
