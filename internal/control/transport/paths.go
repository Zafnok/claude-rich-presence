package transport

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"
)

// EnvRuntimeDir names the environment variable that overrides the runtime
// directory, for tests and for unusual setups (ADR-0006).
const EnvRuntimeDir = "RICH_PRESENCE_RUNTIME_DIR"

const (
	dirName    = "rich-presence"
	socketName = "control.sock"
	lockName   = "host.lock"

	// hashedPrefix starts the short directory name used when the preferred
	// socket path is too long for the platform.
	hashedPrefix = "rp-"
	// hashedDigits is how many hexadecimal digits of the hash the short
	// name keeps.
	hashedDigits = 8

	// The longest socket path, in bytes, that fits a socket address: the
	// size of its path field less the terminating zero.
	maxSocketPathDarwin = 103
	maxSocketPathOther  = 107
)

// Errors returned by Resolve.
var (
	// ErrRelativeOverride reports an override that is not an absolute path.
	// Processes started in different directories would not find each other.
	ErrRelativeOverride = errors.New(EnvRuntimeDir + " is not an absolute path")
	// ErrNoRuntimeDir reports an environment that names no usable temporary
	// directory. Only Windows can return it.
	ErrNoRuntimeDir = errors.New("no usable temporary directory: TEMP and TMP are unset, relative, or an application data folder")
	// ErrPathTooLong reports that no socket path fits the platform limit.
	// Only Windows can return it.
	ErrPathTooLong = errors.New("socket path is too long for this platform; set " + EnvRuntimeDir + " to a short absolute path")
)

// Paths locates the runtime files of the control channel. The lock file sits
// beside the socket.
type Paths struct {
	// Dir is the runtime directory.
	Dir string
	// Socket is the control socket.
	Socket string
	// Lock is the file whose lock elects the host.
	Lock string
}

// Locate resolves the runtime files for the running process.
func Locate(getenv func(string) string) (Paths, error) {
	return Resolve(runtime.GOOS, getenv, os.Getuid())
}

// Resolve implements the directory table of ADR-0006. It is a pure function:
// it reads nothing but its arguments, so every operating system's rules are
// tested on every operating system. uid is the current user id, and is not
// used for Windows.
//
// The preferred directory is the override if set, else the table's entry.
// When the socket path under it would not fit the platform limit, a short
// name derived from a hash of the preferred directory and the user is used
// under the temporary directory instead.
func Resolve(goos string, getenv func(string) string, uid int) (Paths, error) {
	p := platformOf(goos)

	preferred, err := p.preferredDir(getenv, uid)
	if err != nil {
		return Paths{}, err
	}
	if paths := p.pathsIn(preferred); p.fits(paths) {
		return paths, nil
	}

	root, ok := p.shortRoot(getenv)
	if !ok {
		return Paths{}, ErrPathTooLong
	}
	sum := sha256.Sum256([]byte(preferred + "\x00" + strconv.Itoa(uid)))
	paths := p.pathsIn(p.join(root, hashedPrefix+hex.EncodeToString(sum[:])[:hashedDigits]))
	if !p.fits(paths) {
		return Paths{}, ErrPathTooLong
	}
	return paths, nil
}

// platform holds what differs between operating systems in the resolver.
type platform struct {
	windows bool
	// xdg reports whether XDG_RUNTIME_DIR is honoured.
	xdg bool
	sep string
	// maxSocketPath is the longest socket path in bytes.
	maxSocketPath int
}

func platformOf(goos string) platform {
	switch goos {
	case "windows":
		return platform{windows: true, sep: `\`, maxSocketPath: maxSocketPathOther}
	case "darwin":
		return platform{sep: "/", maxSocketPath: maxSocketPathDarwin}
	default:
		return platform{xdg: true, sep: "/", maxSocketPath: maxSocketPathOther}
	}
}

func (p platform) preferredDir(getenv func(string) string, uid int) (string, error) {
	if override := getenv(EnvRuntimeDir); override != "" {
		if !p.isAbs(override) {
			return "", fmt.Errorf("%w: %q", ErrRelativeOverride, override)
		}
		return override, nil
	}
	if p.windows {
		temp, ok := windowsTemp(getenv)
		if !ok {
			return "", ErrNoRuntimeDir
		}
		return p.join(temp, dirName), nil
	}
	if xdg := getenv("XDG_RUNTIME_DIR"); p.xdg && p.isAbs(xdg) {
		return p.join(xdg, dirName), nil
	}
	// The temporary directory may be shared between users, so the name
	// carries the user id.
	temp := getenv("TMPDIR")
	if !p.isAbs(temp) {
		temp = "/tmp"
	}
	return p.join(temp, dirName+"-"+strconv.Itoa(uid)), nil
}

// shortRoot returns the directory that holds the short hashed name. On Unix
// that is /tmp whatever TMPDIR says, because TMPDIR is what made the path
// long. Windows has no short per-user location, so it is the temporary
// directory again.
func (p platform) shortRoot(getenv func(string) string) (string, bool) {
	if p.windows {
		return windowsTemp(getenv)
	}
	return "/tmp", true
}

func (p platform) pathsIn(dir string) Paths {
	return Paths{Dir: dir, Socket: p.join(dir, socketName), Lock: p.join(dir, lockName)}
}

func (p platform) fits(paths Paths) bool {
	return len(paths.Socket) <= p.maxSocketPath
}

func (p platform) join(dir, name string) string {
	return strings.TrimRight(dir, p.separators()) + p.sep + name
}

func (p platform) separators() string {
	if p.windows {
		return `\/`
	}
	return "/"
}

func (p platform) isAbs(path string) bool {
	if !p.windows {
		return strings.HasPrefix(path, "/")
	}
	if strings.HasPrefix(path, `\\`) {
		return true
	}
	return len(path) >= 3 && isDriveLetter(path[0]) && path[1] == ':' && strings.ContainsRune(`\/`, rune(path[2]))
}

func isDriveLetter(c byte) bool {
	return ('a' <= c && c <= 'z') || ('A' <= c && c <= 'Z')
}

// windowsTemp returns the user's temporary directory from the environment,
// not from a system call. A value that is itself %LOCALAPPDATA% or %APPDATA%
// is skipped: a folder created directly under those is redirected to a
// private copy for processes started by Claude Desktop, where the socket
// cannot work (CRP-002).
func windowsTemp(getenv func(string) string) (string, bool) {
	p := platformOf("windows")
	for _, name := range []string{"TEMP", "TMP"} {
		temp := getenv(name)
		if !p.isAbs(temp) {
			continue
		}
		if sameWindowsDir(temp, getenv("LOCALAPPDATA")) || sameWindowsDir(temp, getenv("APPDATA")) {
			continue
		}
		return temp, true
	}
	return "", false
}

func sameWindowsDir(a, b string) bool {
	norm := func(s string) string {
		return strings.TrimRight(strings.ReplaceAll(s, "/", `\`), `\`)
	}
	return strings.EqualFold(norm(a), norm(b))
}
