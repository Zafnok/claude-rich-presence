package fakediscord_test

import (
	"io"
	"os"
	"strings"
	"syscall"
	"testing"

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
