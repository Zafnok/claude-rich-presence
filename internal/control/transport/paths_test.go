package transport_test

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"runtime"
	"strings"
	"testing"

	"github.com/Zafnok/claude-rich-presence/internal/control/transport"
)

const (
	winLocal   = `C:\Users\u\AppData\Local`
	winRoaming = `C:\Users\u\AppData\Roaming`
	winTemp    = `C:\Users\u\AppData\Local\Temp`
	// The temporary directory macOS assigns a user is this long.
	macTemp = "/var/folders/zz/zyxvpxvq6csfxvn_n0000000000000/T/"
)

func env(pairs ...string) func(string) string {
	m := map[string]string{}
	for i := 0; i < len(pairs); i += 2 {
		m[pairs[i]] = pairs[i+1]
	}
	return func(name string) string { return m[name] }
}

// hashed is the short directory name the resolver derives, computed here
// independently of it.
func hashed(preferred, uid string) string {
	sum := sha256.Sum256([]byte(preferred + "\x00" + uid))
	return "rp-" + hex.EncodeToString(sum[:])[:8]
}

// padded returns an absolute directory, starting with prefix, of exactly n
// bytes.
func padded(prefix string, n int) string {
	return prefix + strings.Repeat("a", n-len(prefix))
}

func TestResolve(t *testing.T) {
	const (
		unixTail = len("/control.sock")
		long     = 200
	)
	longUnix := padded("/long/", long)
	longWin := padded(`C:\long\`, long)

	cases := []struct {
		name    string
		goos    string
		env     func(string) string
		uid     int
		wantDir string
		sep     string
		wantErr error
	}{
		// Linux.
		{name: "linux prefers the XDG runtime directory", goos: "linux",
			env: env("XDG_RUNTIME_DIR", "/run/user/1000", "TMPDIR", "/var/tmp"), uid: 1000,
			wantDir: "/run/user/1000/rich-presence"},
		{name: "linux trims a trailing separator", goos: "linux",
			env: env("XDG_RUNTIME_DIR", "/run/user/1000/"), uid: 1000,
			wantDir: "/run/user/1000/rich-presence"},
		{name: "linux without XDG falls back to a per-user directory under TMPDIR", goos: "linux",
			env: env("TMPDIR", "/var/tmp"), uid: 1000,
			wantDir: "/var/tmp/rich-presence-1000"},
		{name: "linux ignores a relative XDG runtime directory", goos: "linux",
			env: env("XDG_RUNTIME_DIR", "run/user/1000"), uid: 1000,
			wantDir: "/tmp/rich-presence-1000"},
		{name: "linux without TMPDIR uses /tmp", goos: "linux",
			env: env(), uid: 1000,
			wantDir: "/tmp/rich-presence-1000"},
		{name: "linux ignores a relative TMPDIR", goos: "linux",
			env: env("TMPDIR", "tmp"), uid: 7,
			wantDir: "/tmp/rich-presence-7"},
		{name: "linux override wins", goos: "linux",
			env: env(transport.EnvRuntimeDir, "/custom/dir", "XDG_RUNTIME_DIR", "/run/user/1000"), uid: 1000,
			wantDir: "/custom/dir"},
		{name: "linux relative override is refused", goos: "linux",
			env: env(transport.EnvRuntimeDir, "custom/dir"), uid: 1000,
			wantErr: transport.ErrRelativeOverride},
		{name: "linux socket path one under the limit is kept", goos: "linux",
			env: env(transport.EnvRuntimeDir, padded("/", transport.MaxSocketPathOther-unixTail-1)), uid: 1000,
			wantDir: padded("/", transport.MaxSocketPathOther-unixTail-1)},
		{name: "linux socket path at the limit is kept", goos: "linux",
			env: env(transport.EnvRuntimeDir, padded("/", transport.MaxSocketPathOther-unixTail)), uid: 1000,
			wantDir: padded("/", transport.MaxSocketPathOther-unixTail)},
		{name: "linux socket path one over the limit is hashed", goos: "linux",
			env: env(transport.EnvRuntimeDir, padded("/", transport.MaxSocketPathOther-unixTail+1)), uid: 1000,
			wantDir: "/tmp/" + hashed(padded("/", transport.MaxSocketPathOther-unixTail+1), "1000")},
		{name: "linux long XDG runtime directory is hashed under /tmp, not TMPDIR", goos: "linux",
			env: env("XDG_RUNTIME_DIR", longUnix, "TMPDIR", longUnix), uid: 1000,
			wantDir: "/tmp/" + hashed(longUnix+"/rich-presence", "1000")},
		{name: "an unknown operating system follows the Linux rules", goos: "freebsd",
			env: env("XDG_RUNTIME_DIR", "/run/user/1000"), uid: 1000,
			wantDir: "/run/user/1000/rich-presence"},

		// macOS.
		{name: "macOS uses a per-user directory under TMPDIR", goos: "darwin",
			env: env("TMPDIR", macTemp), uid: 501,
			wantDir: "/var/folders/zz/zyxvpxvq6csfxvn_n0000000000000/T/rich-presence-501"},
		{name: "macOS does not use the XDG runtime directory", goos: "darwin",
			env: env("XDG_RUNTIME_DIR", "/run/user/501", "TMPDIR", "/private/tmp"), uid: 501,
			wantDir: "/private/tmp/rich-presence-501"},
		{name: "macOS without TMPDIR uses /tmp", goos: "darwin",
			env: env(), uid: 501,
			wantDir: "/tmp/rich-presence-501"},
		{name: "macOS override wins", goos: "darwin",
			env: env(transport.EnvRuntimeDir, "/custom/dir", "TMPDIR", macTemp), uid: 501,
			wantDir: "/custom/dir"},
		{name: "macOS relative override is refused", goos: "darwin",
			env: env(transport.EnvRuntimeDir, "./dir"), uid: 501,
			wantErr: transport.ErrRelativeOverride},
		{name: "macOS socket path one under the limit is kept", goos: "darwin",
			env: env(transport.EnvRuntimeDir, padded("/", transport.MaxSocketPathDarwin-unixTail-1)), uid: 501,
			wantDir: padded("/", transport.MaxSocketPathDarwin-unixTail-1)},
		{name: "macOS socket path at the limit is kept", goos: "darwin",
			env: env(transport.EnvRuntimeDir, padded("/", transport.MaxSocketPathDarwin-unixTail)), uid: 501,
			wantDir: padded("/", transport.MaxSocketPathDarwin-unixTail)},
		{name: "macOS socket path one over the limit is hashed", goos: "darwin",
			env: env(transport.EnvRuntimeDir, padded("/", transport.MaxSocketPathDarwin-unixTail+1)), uid: 501,
			wantDir: "/tmp/" + hashed(padded("/", transport.MaxSocketPathDarwin-unixTail+1), "501")},
		{name: "macOS limit is lower than the Linux one", goos: "darwin",
			env: env(transport.EnvRuntimeDir, padded("/", transport.MaxSocketPathOther-unixTail)), uid: 501,
			wantDir: "/tmp/" + hashed(padded("/", transport.MaxSocketPathOther-unixTail), "501")},
		{name: "macOS long TMPDIR is hashed under /tmp", goos: "darwin",
			env: env("TMPDIR", longUnix), uid: 501,
			wantDir: "/tmp/" + hashed(longUnix+"/rich-presence-501", "501")},

		// Windows.
		{name: "windows uses TEMP", goos: "windows",
			env:     env("TEMP", winTemp, "TMP", `D:\tmp`, "LOCALAPPDATA", winLocal, "APPDATA", winRoaming),
			wantDir: winTemp + `\rich-presence`, sep: `\`},
		{name: "windows trims a trailing separator", goos: "windows",
			env:     env("TEMP", winTemp+`\`),
			wantDir: winTemp + `\rich-presence`, sep: `\`},
		{name: "windows accepts forward slashes and a lower-case drive", goos: "windows",
			env:     env("TEMP", "c:/Temp/"),
			wantDir: `c:/Temp\rich-presence`, sep: `\`},
		{name: "windows accepts a network path", goos: "windows",
			env:     env("TEMP", `\\server\share\tmp`),
			wantDir: `\\server\share\tmp\rich-presence`, sep: `\`},
		{name: "windows without TEMP falls back to TMP", goos: "windows",
			env:     env("TMP", `D:\tmp`, "LOCALAPPDATA", winLocal),
			wantDir: `D:\tmp\rich-presence`, sep: `\`},
		{name: "windows skips a relative TEMP", goos: "windows",
			env:     env("TEMP", `Temp`, "TMP", `D:\tmp`),
			wantDir: `D:\tmp\rich-presence`, sep: `\`},
		{name: "windows skips a TEMP with no path after the drive", goos: "windows",
			env:     env("TEMP", `C:`, "TMP", `D:\tmp`),
			wantDir: `D:\tmp\rich-presence`, sep: `\`},
		{name: "windows skips a TEMP that only looks like a drive", goos: "windows",
			env:     env("TEMP", `1:\Temp`, "TMP", `D:\tmp`),
			wantDir: `D:\tmp\rich-presence`, sep: `\`},
		{name: "windows skips a TEMP that is LOCALAPPDATA", goos: "windows",
			env:     env("TEMP", `c:/users/U/appdata/local/`, "TMP", `D:\tmp`, "LOCALAPPDATA", winLocal),
			wantDir: `D:\tmp\rich-presence`, sep: `\`},
		{name: "windows skips a TEMP that is APPDATA", goos: "windows",
			env:     env("TEMP", winRoaming, "TMP", `D:\tmp`, "APPDATA", winRoaming),
			wantDir: `D:\tmp\rich-presence`, sep: `\`},
		{name: "windows with TEMP and TMP both application data folders has no directory", goos: "windows",
			env:     env("TEMP", winLocal, "TMP", winRoaming, "LOCALAPPDATA", winLocal, "APPDATA", winRoaming),
			wantErr: transport.ErrNoRuntimeDir},
		{name: "windows with no temporary directory has no directory", goos: "windows",
			env:     env("LOCALAPPDATA", winLocal, "APPDATA", winRoaming, "USERPROFILE", `C:\Users\u`),
			wantErr: transport.ErrNoRuntimeDir},
		{name: "windows override wins", goos: "windows",
			env:     env(transport.EnvRuntimeDir, `E:\run`, "TEMP", winTemp),
			wantDir: `E:\run`, sep: `\`},
		{name: "windows relative override is refused", goos: "windows",
			env:     env(transport.EnvRuntimeDir, `run`, "TEMP", winTemp),
			wantErr: transport.ErrRelativeOverride},
		{name: "windows override with a Unix-style path is refused", goos: "windows",
			env:     env(transport.EnvRuntimeDir, `/run`, "TEMP", winTemp),
			wantErr: transport.ErrRelativeOverride},
		{name: "windows socket path one under the limit is kept", goos: "windows",
			env:     env(transport.EnvRuntimeDir, padded(`C:\`, transport.MaxSocketPathOther-unixTail-1), "TEMP", winTemp),
			wantDir: padded(`C:\`, transport.MaxSocketPathOther-unixTail-1), sep: `\`},
		{name: "windows socket path at the limit is kept", goos: "windows",
			env:     env(transport.EnvRuntimeDir, padded(`C:\`, transport.MaxSocketPathOther-unixTail), "TEMP", winTemp),
			wantDir: padded(`C:\`, transport.MaxSocketPathOther-unixTail), sep: `\`},
		{name: "windows socket path one over the limit is hashed under TEMP", goos: "windows",
			env:     env(transport.EnvRuntimeDir, padded(`C:\`, transport.MaxSocketPathOther-unixTail+1), "TEMP", winTemp),
			wantDir: winTemp + `\` + hashed(padded(`C:\`, transport.MaxSocketPathOther-unixTail+1), "-1"), sep: `\`, uid: -1},
		{name: "windows long override with no temporary directory has no path", goos: "windows",
			env:     env(transport.EnvRuntimeDir, longWin),
			wantErr: transport.ErrPathTooLong},
		{name: "windows TEMP too long even for the hashed name has no path", goos: "windows",
			env:     env("TEMP", longWin),
			wantErr: transport.ErrPathTooLong},
		{name: "windows hashed name at the limit is used", goos: "windows",
			env: env("TEMP", padded(`C:\`, transport.MaxSocketPathOther-unixTail-len(`\rp-01234567`))), uid: -1,
			wantDir: padded(`C:\`, transport.MaxSocketPathOther-unixTail-len(`\rp-01234567`)) + `\` +
				hashed(padded(`C:\`, transport.MaxSocketPathOther-unixTail-len(`\rp-01234567`))+`\rich-presence`, "-1"), sep: `\`},
		{name: "windows hashed name one over the limit has no path", goos: "windows",
			env: env("TEMP", padded(`C:\`, transport.MaxSocketPathOther-unixTail-len(`\rp-01234567`)+1)), uid: -1,
			wantErr: transport.ErrPathTooLong},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := transport.Resolve(c.goos, c.env, c.uid)
			if c.wantErr != nil {
				if !errors.Is(err, c.wantErr) {
					t.Fatalf("Resolve() error = %v, want %v", err, c.wantErr)
				}
				if got != (transport.Paths{}) {
					t.Errorf("Resolve() = %+v with an error, want no paths", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("Resolve() error = %v", err)
			}
			sep := c.sep
			if sep == "" {
				sep = "/"
			}
			want := transport.Paths{
				Dir:    c.wantDir,
				Socket: c.wantDir + sep + "control.sock",
				Lock:   c.wantDir + sep + "host.lock",
			}
			if got != want {
				t.Errorf("Resolve() =\n%+v, want\n%+v", got, want)
			}
			limit := transport.MaxSocketPathOther
			if c.goos == "darwin" {
				limit = transport.MaxSocketPathDarwin
			}
			if len(got.Socket) > limit {
				t.Errorf("socket path is %d bytes, over the limit of %d", len(got.Socket), limit)
			}
		})
	}
}

// The hashed name is part of the contract between binaries of different
// versions: a follower and a host that disagree on it never meet.
func TestResolveHashedNameIsStable(t *testing.T) {
	dir := "/" + strings.Repeat("x", 150)
	got, err := transport.Resolve("linux", env(transport.EnvRuntimeDir, dir), 1000)
	if err != nil {
		t.Fatal(err)
	}
	if want := "/tmp/rp-45ff876f"; got.Dir != want {
		t.Errorf("Dir = %q, want %q", got.Dir, want)
	}
}

func TestResolveHashedNameSeparatesUsersAndDirectories(t *testing.T) {
	long := "/" + strings.Repeat("x", 150)
	resolve := func(dir string, uid int) string {
		t.Helper()
		got, err := transport.Resolve("linux", env(transport.EnvRuntimeDir, dir), uid)
		if err != nil {
			t.Fatal(err)
		}
		return got.Dir
	}
	base := resolve(long, 1000)
	if again := resolve(long, 1000); again != base {
		t.Errorf("same input gave %q then %q", base, again)
	}
	if other := resolve(long, 1001); other == base {
		t.Errorf("users 1000 and 1001 share %q", base)
	}
	if other := resolve(long+"y", 1000); other == base {
		t.Errorf("two directories share %q", base)
	}
}

// ADR-0006: on Windows the directory is never a folder directly under
// %LOCALAPPDATA% or %APPDATA%, whatever TEMP and TMP say.
func TestResolveWindowsNeverDirectlyUnderAppData(t *testing.T) {
	values := []string{"", winLocal, winRoaming, winTemp, winLocal + `\`, strings.ToUpper(winRoaming), `D:\tmp`}
	for _, temp := range values {
		for _, tmp := range values {
			got, err := transport.Resolve("windows", env("TEMP", temp, "TMP", tmp, "LOCALAPPDATA", winLocal, "APPDATA", winRoaming), -1)
			if err != nil {
				continue
			}
			parent := strings.ToLower(got.Dir[:strings.LastIndex(got.Dir, `\`)])
			if parent == strings.ToLower(winLocal) || parent == strings.ToLower(winRoaming) {
				t.Errorf("TEMP=%q TMP=%q: Dir = %q, directly under an application data folder", temp, tmp, got.Dir)
			}
		}
	}
}

// The directory macOS assigns is long. This records that the preferred path
// still fits there, so the hashed name is not the usual case.
func TestResolveFitsTheRealMacOSTemporaryDirectory(t *testing.T) {
	got, err := transport.Resolve("darwin", env("TMPDIR", macTemp), 501)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(got.Dir, macTemp) {
		t.Errorf("Dir = %q, want it under %q", got.Dir, macTemp)
	}
}

func TestLocateResolvesForTheRunningProcess(t *testing.T) {
	getenv := env(
		"XDG_RUNTIME_DIR", "/run/user/1", "TMPDIR", "/var/tmp",
		"TEMP", winTemp, "LOCALAPPDATA", winLocal,
	)
	got, err := transport.Locate(getenv)
	if err != nil {
		t.Fatal(err)
	}
	want, err := transport.Resolve(runtime.GOOS, getenv, os.Getuid())
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Errorf("Locate() = %+v, want %+v", got, want)
	}
	if got.Dir == "" {
		t.Error("Locate() returned no directory")
	}
}

// The real environment of the machine running the tests resolves, and to a
// socket path the platform accepts. On macOS this is the check against the
// temporary directory the system actually assigns.
func TestLocateInTheRealEnvironment(t *testing.T) {
	getenv := func(name string) string {
		if name == transport.EnvRuntimeDir {
			return ""
		}
		return os.Getenv(name)
	}
	got, err := transport.Locate(getenv)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("runtime directory on this machine: %s (socket path %d bytes)", got.Dir, len(got.Socket))
	if limit := socketPathLimit(); len(got.Socket) > limit {
		t.Errorf("socket path is %d bytes, over the limit of %d", len(got.Socket), limit)
	}
}

func socketPathLimit() int {
	if runtime.GOOS == "darwin" {
		return transport.MaxSocketPathDarwin
	}
	return transport.MaxSocketPathOther
}
