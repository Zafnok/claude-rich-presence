package transport

import (
	"context"
	"io/fs"

	"github.com/Microsoft/go-winio"
)

// absentErrors are the errors for a pipe that does not exist.
var absentErrors = []error{fs.ErrNotExist}

// open opens the named pipe for overlapped I/O. While the pipe exists but
// has no free instance, which Windows reports as ERROR_PIPE_BUSY, the call
// keeps trying until the context ends and then returns the error of the
// context.
//
// The standard library can open the pipe too, but its file type has a data
// race between a read and a write in flight together (ADR-0017).
func open(ctx context.Context, addr string) (Conn, error) {
	return winio.DialPipeContext(ctx, addr)
}
