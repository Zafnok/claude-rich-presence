package host

import (
	"context"
	"errors"
	"io"
	"time"

	"github.com/Zafnok/claude-rich-presence/internal/control/protocol"
	"github.com/Zafnok/claude-rich-presence/internal/diag"
	"github.com/Zafnok/claude-rich-presence/internal/domain"
	"github.com/Zafnok/claude-rich-presence/internal/presence"
)

// ErrLocked is what Config.Acquire returns, alone or wrapped, when another
// process holds the host lock. It only changes what the log says: after any
// failure to take the lock the node tries to follow a host.
var ErrLocked = errors.New("host lock is held by another process")

// Clock is the time source. The real implementation wraps time.Now and
// time.AfterFunc; tests use internal/testutil/fakeclock.
type Clock interface {
	Now() time.Time
	// AfterFunc runs f on another goroutine once d has passed. The returned
	// function cancels that, as (*time.Timer).Stop does.
	AfterFunc(d time.Duration, f func()) (stop func() bool)
}

// Lock is the host lock, held (ADR-0005).
type Lock interface {
	// Listen opens the control socket. It is called again, after a wait, if
	// it fails or if the listener it returned fails; that listener has been
	// closed by then.
	Listen() (Listener, error)
	// Release gives up the lock. The node has closed its listener by then.
	Release() error
}

// Listener accepts follower connections on the control socket.
//
// A connection, here and from Config.Dial, must allow a Write while a Read is
// pending. Its Close must fail both, and may be called from any goroutine,
// more than once.
type Listener interface {
	Accept() (io.ReadWriteCloser, error)
	// Close fails a pending Accept. Closing twice is harmless.
	Close() error
}

// Discord is the connection to Discord for one term as host. internal/cli
// adapts the Discord session manager to it.
type Discord interface {
	// Set and Clear say what to show. They return at once.
	Set(domain.Activity)
	Clear()
	// Run keeps the connection until ctx ends, then clears the activity,
	// closes the connection and returns. It is called once.
	Run(ctx context.Context)
	// State is the state of the connection, read from memory.
	State() protocol.DiscordState
}

// Config is what a Node is built from. Every field but Settings and Logger is
// required.
type Config struct {
	// Version is this binary's version. It is what decides, between two
	// nodes, which should be host.
	Version string
	// Acquire tries once, without waiting, to take the host lock.
	Acquire func() (Lock, error)
	// Dial connects to the control socket. It gives up when ctx ends, and
	// must not wait without limit otherwise.
	Dial func(ctx context.Context) (io.ReadWriteCloser, error)
	// Discord makes the connection to Discord for one term as host.
	Discord func() Discord
	// Render is presence.Render. It is called from one goroutine, with
	// Settings.
	Render   func(sessions []domain.Session, now time.Time, set presence.Settings) (domain.Activity, bool)
	Settings presence.Settings
	Clock    Clock
	// Jitter returns a number from 0 to 1, which places each wait between
	// half of the backoff delay and all of it.
	Jitter   func() float64
	Counters *diag.Counters
	// Logger receives changes of role and classes of error, never an event.
	Logger *diag.Logger
}
