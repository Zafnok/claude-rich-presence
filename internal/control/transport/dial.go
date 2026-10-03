package transport

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"time"
)

// ErrNoHost reports that nothing is listening on the control socket: the
// runtime directory or the socket file is missing, or the file is a leftover
// that refuses connections.
var ErrNoHost = errors.New("no host is listening on the control socket")

// Dial connects to the control socket, giving up after timeout or when ctx
// ends. It returns an error wrapping ErrNoHost when no host is listening,
// and the underlying error for anything else.
func Dial(ctx context.Context, socket string, timeout time.Duration) (net.Conn, error) {
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
