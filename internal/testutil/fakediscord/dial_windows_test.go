package fakediscord_test

import (
	"io"
	"os"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/Zafnok/claude-rich-presence/internal/testutil/fakediscord"
)

// dial opens the pipe for overlapped I/O, so that a test can write while a
// read is pending.
func dial(addr string) (io.ReadWriteCloser, error) {
	return os.OpenFile(addr, os.O_RDWR|syscall.FILE_FLAG_OVERLAPPED, 0)
}

func TestWindowsHasNoEnvironmentAndAUniquePipeName(t *testing.T) {
	a := fakediscord.NewNamespace(t)
	b := fakediscord.NewNamespace(t)
	if env := a.Env(); len(env) != 0 {
		t.Errorf("Env() = %q, want none", env)
	}
	if !strings.HasPrefix(a.Prefix(), `\\.\pipe\`) || !strings.HasSuffix(a.Prefix(), `-discord-ipc-`) {
		t.Errorf("Prefix() = %q", a.Prefix())
	}
	if a.Prefix() == b.Prefix() {
		t.Errorf("two namespaces share the prefix %q", a.Prefix())
	}
}

// Discord documents the pipe as \\?\pipe\discord-ipc-N. Both spellings name
// the same pipe.
func TestWindowsPipeAnswersToTheDocumentedSpelling(t *testing.T) {
	srv := fakediscord.Start(t, fakediscord.Options{})
	addr := `\\?\pipe\` + strings.TrimPrefix(srv.Addr(), `\\.\pipe\`)
	connect(t, addr).handshake()
}

// A test that starts a child process reads its output with a blocking read,
// which occupies a thread for as long as the child lives. If that is the
// thread on which the server began waiting for clients, shutting the server
// down must still not wait for it: Windows cancels a pending operation
// through the thread that started it.
func TestWindowsCloseWhileTheStartingThreadIsBlockedInARead(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = r.Close() }()

	// The server is started on a thread that then blocks in a read, as the
	// reader of a child's output does.
	started := make(chan *fakediscord.Server)
	unblocked := make(chan struct{})
	go func() {
		defer close(unblocked)
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		started <- fakediscord.Start(t, fakediscord.Options{})
		_, _ = r.Read(make([]byte, 1))
	}()
	srv := <-started

	closed := make(chan struct{})
	go func() {
		defer close(closed)
		srv.Close()
	}()
	select {
	case <-closed:
	case <-time.After(10 * time.Second):
		t.Error("Close did not return while the thread that started the server was blocked in a read")
	}
	_ = w.Close()
	<-unblocked
	<-closed
}
