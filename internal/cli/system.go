package cli

import (
	"context"
	"math/rand/v2"
	"os"
	"os/signal"
	"runtime"
	"runtime/debug"
	"strconv"
	"syscall"
	"time"

	"github.com/Zafnok/claude-rich-presence/internal/config"
	"github.com/Zafnok/claude-rich-presence/internal/diag"
	dtransport "github.com/Zafnok/claude-rich-presence/internal/discord/transport"
)

// system is every real thing the commands reach for, as plain values, so that
// a test can replace the ones it must and run the rest for real. realSystem is
// the one place they are made, apart from the sockets and locks that
// hostConfig opens from the paths it is given.
type system struct {
	// goos is a runtime.GOOS value.
	goos string
	// version is this binary's version.
	version string
	clock   realtime
	fs      diag.FS
	files   config.FileReader
	// discord returns the way to reach Discord and every endpoint it may be
	// at, as the doctor lists them.
	discord func(getenv func(string) string) (dtransport.Dialer, []string)
	// exists reports whether a Discord endpoint is there.
	exists func(path string) bool
	// sessionID names this process's session until a hook gives the real one.
	// It must be unique to the process.
	sessionID func() string
	// notify returns a context that ends on an interrupt or termination
	// signal.
	notify func(context.Context) (context.Context, context.CancelFunc)
}

// realtime is the clock the host and the Discord session take.
type realtime interface {
	Now() time.Time
	AfterFunc(d time.Duration, f func()) (stop func() bool)
}

func realSystem() system {
	return system{
		goos:      runtime.GOOS,
		version:   resolveVersion(version, debug.ReadBuildInfo),
		clock:     wallClock{},
		fs:        diag.OSFS{},
		files:     osFiles{},
		discord:   realDiscord,
		exists:    exists,
		sessionID: processSessionID,
		notify: func(ctx context.Context) (context.Context, context.CancelFunc) {
			return signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
		},
	}
}

// wallClock is the time of the operating system.
type wallClock struct{}

func (wallClock) Now() time.Time { return time.Now() }

func (wallClock) AfterFunc(d time.Duration, f func()) func() bool {
	return time.AfterFunc(d, f).Stop
}

// osFiles reads the configuration file.
type osFiles struct{}

func (osFiles) ReadFile(name string) ([]byte, error) { return os.ReadFile(name) }

func realDiscord(getenv func(string) string) (dtransport.Dialer, []string) {
	prefixes := dtransport.Prefixes(runtime.GOOS, getenv)
	return dtransport.New(getenv), dtransport.Candidates(prefixes)
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// processSessionID is unique to the process: its id and the moment it asked.
// CLAUDE_CODE_SESSION_ID is not used, because it is stale after a clear.
func processSessionID() string {
	return "pending-" + strconv.Itoa(os.Getpid()) + "-" + strconv.FormatInt(time.Now().UnixNano(), 36)
}

// jitter is a number from 0 to 1. The package function is safe for several
// goroutines.
func jitter() float64 { return rand.Float64() }
