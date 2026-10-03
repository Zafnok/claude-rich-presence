package cli

import "runtime/debug"

// version is set by the linker for releases:
//
//	-ldflags "-X github.com/Zafnok/claude-rich-presence/internal/cli.version=1.2.3"
//
// When it is empty the version comes from the build information that the Go
// toolchain embeds in the binary.
var version string

// unknownVersion is reported when neither source has a version.
const unknownVersion = "unknown"

// resolveVersion returns the linker override if there is one, otherwise the
// version of the main module from the build information.
func resolveVersion(override string, read func() (*debug.BuildInfo, bool)) string {
	if override != "" {
		return override
	}
	if info, ok := read(); ok && info.Main.Version != "" {
		return info.Main.Version
	}
	return unknownVersion
}
