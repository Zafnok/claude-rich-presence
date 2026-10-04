//go:build !windows

package transport

import (
	"context"
	"io/fs"
	"net"
	"syscall"
)

// absentErrors are the errors for a socket nobody listens on: the file or its
// directory does not exist, or the file is a leftover that refuses
// connections.
var absentErrors = []error{fs.ErrNotExist, syscall.ECONNREFUSED}

// open connects to the Unix socket.
func open(ctx context.Context, addr string) (Conn, error) {
	var d net.Dialer
	return d.DialContext(ctx, "unix", addr)
}
