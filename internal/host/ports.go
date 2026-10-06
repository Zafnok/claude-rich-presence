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

// ErrUnsafe is what Config.Dial returns, alone or wrapped, when it will not
// connect because the control socket is somewhere another user could be
// listening. The node sends nothing, reports it once, and tries again as
// after any round that reached no host.
var ErrUnsafe = errors.New("the control socket is not in a safe place")

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

// Config is what a Node is built from. Every field but Settings, Link and
// Logger is required.
type Config struct {
	// Version is this binary's version. It is what decides, between two
	// nodes, which should be host.
	Version string
	// Acquire tries once, without waiting, to take the host lock.
	Acquire func() (Lock, error)
	// Dial connects to the control socket. It gives up when ctx ends, and
	// must not wait without limit otherwise. It returns ErrUnsafe when it
	// refuses to connect.
	Dial func(ctx context.Context) (io.ReadWriteCloser, error)
	// Discord makes the connection to Discord for one term as host.
	Discord func() Discord
	// Render is presence.Render. It is called from one goroutine, with
	// Settings.
	Render   func(sessions []domain.Session, now time.Time, set presence.Settings) (domain.Activity, bool)
	Settings presence.Settings
	// Link validates a repository link a session arrives with, and returns it
	// in its published form. It is config.ValidateLink with the user's hosts,
	// the function the adapter's link was validated with (ADR-0012). A link
	// it refuses is dropped and the session is kept. When it is nil every
	// link is dropped.
	Link  func(raw string) (string, bool)
	Clock Clock
	// Jitter returns a number from 0 to 1, which places each wait between
	// half of the backoff delay and all of it. It is called from more than
	// one goroutine.
	Jitter   func() float64
	Counters *diag.Counters
	// Logger receives changes of role and classes of error, never an event.
	Logger *diag.Logger
}
