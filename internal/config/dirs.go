package config

import (
	"errors"
	"strings"
)

// FileName is the name of the configuration file inside Dirs.Config.
const FileName = "config.json"

// ErrNoHome means the environment does not say where the user's home is, so
// no directory can be resolved.
var ErrNoHome = errors.New("config: the home directory is not set")

// Dirs are the per-user locations of the configuration and the logs. The
// runtime directory of the control socket is not here.
type Dirs struct {
	Config string
	// File is the configuration file, inside Config.
	File string
	Logs string
}

// ResolveDirs returns the directories for the operating system named by goos,
// a runtime.GOOS value, from the environment alone. It is the same function
// on every operating system and touches nothing.
//
// On Windows the directories are under the user profile and never under
// AppData (ADR-0016). Elsewhere they follow the operating system's user
// configuration directory.
func ResolveDirs(goos string, getenv func(string) string) (Dirs, error) {
	sep, home, base := "/", getenv("HOME"), ""
	switch goos {
	case "windows":
		sep, home = `\`, getenv("USERPROFILE")
		base = `\.rich-presence`
	case "darwin":
		base = "/Library/Application Support/rich-presence"
	default:
		base = "/.config/rich-presence"
		// A relative XDG_CONFIG_HOME is invalid and ignored.
		if xdg := getenv("XDG_CONFIG_HOME"); strings.HasPrefix(xdg, "/") {
			home, base = xdg, "/rich-presence"
		}
	}
	if home == "" {
		return Dirs{}, ErrNoHome
	}
	dir := strings.TrimRight(home, `/\`) + base
	return Dirs{Config: dir, File: dir + sep + FileName, Logs: dir + sep + "logs"}, nil
}
