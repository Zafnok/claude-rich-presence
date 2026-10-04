package transport

import (
	"context"
	"errors"
	"fmt"
	"io"
	"runtime"
	"time"
)

// ErrNotRunning reports that no Discord client is listening on any candidate
// endpoint.
var ErrNotRunning = errors.New("discord is not running")

// Conn is an open connection to a Discord client. Deadlines are honoured, one
// goroutine may block in Read while another calls Write, and Close unblocks a
// pending Read.
type Conn interface {
	io.ReadWriteCloser
	SetDeadline(time.Time) error
	SetReadDeadline(time.Time) error
	SetWriteDeadline(time.Time) error
}

// Dialer finds and opens the connection to the running Discord client.
type Dialer interface {
	// Dial returns the first candidate endpoint that connects. When nothing
	// is listening on any of them the error is ErrNotRunning. When ctx ends
	// first the error is the context's.
	Dial(ctx context.Context) (Conn, error)
}

// attemptTimeout bounds one candidate. Opening a pipe or connecting to a
// local socket normally answers at once. The bound is for a socket whose
// owner has stopped accepting, and for a pipe with no free instance.
const attemptTimeout = 500 * time.Millisecond

// openFunc opens one endpoint. The real one is open, in the platform file.
type openFunc func(ctx context.Context, addr string) (Conn, error)

type dialer struct {
	candidates []string
	open       openFunc
	// absent are the errors from open that mean nothing is listening at
	// that address.
	absent  []error
	attempt time.Duration
}

// New returns a Dialer for the Discord client of this operating system, found
// through the environment that getenv reads.
func New(getenv func(string) string) Dialer {
	return newDialer(Prefixes(runtime.GOOS, getenv))
}

// NewAt returns a Dialer that looks only at the given endpoint name prefix,
// to which the indices 0 to 9 are added. Tests use it with a unique name, so
// that they never touch a real Discord.
func NewAt(prefix string) Dialer {
	return newDialer([]string{prefix})
}

func newDialer(prefixes []string) *dialer {
	return &dialer{
		candidates: Candidates(prefixes),
		open:       open,
		absent:     absentErrors,
		attempt:    attemptTimeout,
	}
}

func (d *dialer) Dial(ctx context.Context) (Conn, error) {
	var failure error
	for _, addr := range d.candidates {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		conn, err := d.try(ctx, addr)
		if err == nil {
			return conn, nil
		}
		if failure == nil && !d.isAbsent(err) {
			failure = fmt.Errorf("opening %s: %w", addr, err)
		}
	}
	// A cancellation during the last attempt is the caller's, not Discord's.
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if failure != nil {
		return nil, failure
	}
	return nil, ErrNotRunning
}

// try opens one candidate within the attempt timeout.
func (d *dialer) try(ctx context.Context, addr string) (Conn, error) {
	ctx, cancel := context.WithTimeout(ctx, d.attempt)
	defer cancel()
	return d.open(ctx, addr)
}

// isAbsent reports whether a failed attempt means nothing can be reached at
// that address, which is passed over without comment. An attempt that ran out
// of time is one of those: the endpoint is there but is not taking
// connections.
func (d *dialer) isAbsent(err error) bool {
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	for _, absent := range d.absent {
		if errors.Is(err, absent) {
			return true
		}
	}
	return false
}
