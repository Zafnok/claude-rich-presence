package transport_test

import (
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Zafnok/claude-rich-presence/internal/control/transport"
)

// listenIn listens on a socket in dir with no lock and no check, as another
// user who had made the directory could.
func listenIn(t *testing.T, dir string) (*net.UnixListener, string) {
	t.Helper()
	socket := filepath.Join(dir, "s")
	listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: socket, Net: "unix"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { listener.Close() })
	return listener, socket
}

// acceptsNothing fails the test if anything connected to the listener. A
// connect is complete before a dial returns, so one that was made is already
// waiting, and the deadline is only how long the listener is given to say
// that none is.
func acceptsNothing(t *testing.T, listener *net.UnixListener) {
	t.Helper()
	if err := listener.SetDeadline(time.Now().Add(100 * time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	conn, err := listener.Accept()
	if err == nil {
		conn.Close()
		t.Fatal("the listener accepted a connection, want none made")
	}
	if !errors.Is(err, os.ErrDeadlineExceeded) {
		t.Fatalf("Accept() error = %v, want the deadline", err)
	}
}

// refused fails the test unless err is the refusal of a directory for this
// reason, and safe to print: it must not name the path.
func refused(t *testing.T, conn net.Conn, err, reason error, dir string) {
	t.Helper()
	if conn != nil {
		conn.Close()
		t.Error("Dial() returned a connection, want none")
	}
	if !errors.Is(err, transport.ErrUnsafeDir) || !errors.Is(err, reason) {
		t.Fatalf("Dial() error = %v, want ErrUnsafeDir for %v", err, reason)
	}
	if errors.Is(err, transport.ErrNoHost) {
		t.Errorf("Dial() error = %v, which reads as no host", err)
	}
	if strings.Contains(err.Error(), dir) || strings.Contains(err.Error(), filepath.Base(filepath.Dir(dir))) {
		t.Errorf("Dial() error = %q, which names the directory", err)
	}
}

func TestDialAcceptsTheDirectoryPrepareMade(t *testing.T) {
	paths := testPaths(t)
	listener, err := acquire(t, paths).Listen()
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	served := echo(listener)
	roundTrip(t, paths.Socket)
	if err := <-served; err != nil {
		t.Errorf("server: %v", err)
	}
}

// Ownership is injected, as in TestCheckDir: the directory is the one
// Prepare made and somebody listens in it, and the dial is made as a user
// who does not own it.
func TestDialRefusesADirectoryOfAnotherUser(t *testing.T) {
	paths := testPaths(t)
	if err := transport.Prepare(paths.Dir); err != nil {
		t.Fatal(err)
	}
	listener, socket := listenIn(t, paths.Dir)

	conn, err := transport.DialAs(context.Background(), socket, patience, os.Getuid()+1, true)
	refused(t, conn, err, transport.ErrNotOwned, paths.Dir)
	acceptsNothing(t, listener)

	// The same dial as the owner connects, so it was the owner that decided.
	conn, err = transport.DialAs(context.Background(), socket, patience, os.Getuid(), false)
	if err != nil {
		t.Fatalf("Dial() as the owner: %v", err)
	}
	conn.Close()
}

func TestDialRefusesAFileInPlaceOfTheDirectory(t *testing.T) {
	paths := testPaths(t)
	if err := os.WriteFile(paths.Dir, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	conn, err := transport.Dial(context.Background(), paths.Socket, patience)
	refused(t, conn, err, transport.ErrNotDirectory, paths.Dir)
}

func TestDialDoesNotCreateTheDirectory(t *testing.T) {
	paths := testPaths(t)
	if _, err := transport.Dial(context.Background(), paths.Socket, patience); !errors.Is(err, transport.ErrNoHost) {
		t.Errorf("Dial() error = %v, want ErrNoHost", err)
	}
	if exists(t, paths.Dir) {
		t.Errorf("Dial() created %s", paths.Dir)
	}
}

// A directory that cannot be inspected is not connected to, and is not
// taken for an absent host.
func TestDialRefusesADirectoryItCannotInspect(t *testing.T) {
	socket := filepath.Join(t.TempDir(), "run\x00", "s")
	conn, err := transport.Dial(context.Background(), socket, patience)
	if err == nil || conn != nil || errors.Is(err, transport.ErrNoHost) {
		t.Errorf("Dial() = %v, %v, want nil and an error that is not ErrNoHost", conn, err)
	}
}
