package transport

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"time"
)

// ErrNoHost reports that nothing is listening on the control socket: the
// runtime directory or the socket file is missing, or the file is a leftover
// that refuses connections.
var ErrNoHost = errors.New("no host is listening on the control socket")

// ErrNoTimeout reports a dial with no positive timeout. A dial that could
// wait without limit is refused, because nothing here may hang (ADR-0008).
var ErrNoTimeout = errors.New("dial timeout must be positive")

// ErrUnsafeDir reports a control socket whose directory Prepare would
// refuse: whoever listens there may be another user, so nothing is sent. It
// wraps the reason, one of ErrNotDirectory, ErrNotOwned and ErrAccessible,
// and never names the path, so that it is safe to print. It is not
// ErrNoHost: a host may well be there.
var ErrUnsafeDir = errors.New("the runtime directory is not safe to use")

// Dial connects to the control socket, giving up after timeout or when ctx
// ends. It returns an error wrapping ErrNoHost when no host is listening,
// one wrapping ErrUnsafeDir when the directory of the socket is not the
// user's alone, and the underlying error for anything else.
//
// The directory is checked as Prepare checks it, and before anything is
// connected to, because a follower sends its session to whoever answers
// (CRP-065). Dial never creates the directory.
func Dial(ctx context.Context, socket string, timeout time.Duration) (net.Conn, error) {
	return dial(ctx, socket, timeout, os.Getuid(), runtime.GOOS != "windows")
}

// dial is Dial with the user and the rules of checkDir given.
func dial(ctx context.Context, socket string, timeout time.Duration, self int, unixRules bool) (net.Conn, error) {
	if timeout <= 0 {
		return nil, ErrNoTimeout
	}
	// Lstat, so that a link is refused and not followed. A directory that
	// cannot be inspected is not connected to either.
	info, err := os.Lstat(filepath.Dir(socket))
	if err != nil {
		return nil, classifyDialError(err)
	}
	if err := checkInfo(info, self, unixRules); err != nil {
		return nil, fmt.Errorf("%w: it %w", ErrUnsafeDir, err)
	}
	// Between the check and the connect the directory could change. Under
	// the Unix rules that gap is closed by what was checked: the directory
	// is the user's and owner-only, so only the user can replace the socket
	// in it or the directory itself. (A parent that others can write, such
	// as /tmp, is sticky, and lets nobody else rename or remove it.)
	dialer := net.Dialer{Timeout: timeout}
	conn, err := dialer.DialContext(ctx, "unix", socket)
	if err != nil {
		return nil, classifyDialError(err)
	}
	return conn, nil
}

func classifyDialError(err error) error {
	if errors.Is(err, fs.ErrNotExist) || errors.Is(err, errRefused) || errors.Is(err, errNoDir) {
		return fmt.Errorf("%w: %w", ErrNoHost, err)
	}
	return err
}
