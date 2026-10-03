//go:build !windows

package fakediscord_test

import (
	"errors"
	"io"
	"io/fs"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Zafnok/claude-rich-presence/internal/testutil/fakediscord"
)

func dial(addr string) (io.ReadWriteCloser, error) { return net.Dial("unix", addr) }

func runtimeDir(t *testing.T, ns *fakediscord.Namespace) string {
	t.Helper()
	env := ns.Env()
	if len(env) != 1 || !strings.HasPrefix(env[0], "XDG_RUNTIME_DIR=") {
		t.Fatalf("Env() = %q, want one XDG_RUNTIME_DIR entry", env)
	}
	return strings.TrimPrefix(env[0], "XDG_RUNTIME_DIR=")
}

func TestEnvPointsAtTheSocketDirectory(t *testing.T) {
	srv := fakediscord.Start(t, fakediscord.Options{})
	dir := runtimeDir(t, srv.Namespace())
	if want := filepath.Join(dir, "discord-ipc-0"); srv.Addr() != want {
		t.Errorf("Addr() = %q, want %q", srv.Addr(), want)
	}
	if info, err := os.Stat(srv.Addr()); err != nil || info.Mode()&fs.ModeSocket == 0 {
		t.Errorf("no socket at %s: %v", srv.Addr(), err)
	}
}

func TestNothingIsLeftOnDisk(t *testing.T) {
	var dir, addr string
	t.Run("server", func(t *testing.T) {
		srv := fakediscord.Start(t, fakediscord.Options{})
		dir, addr = runtimeDir(t, srv.Namespace()), srv.Addr()
		connect(t, addr).handshake()
	})
	for _, path := range []string{addr, dir} {
		if _, err := os.Stat(path); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("%s still exists after the test: %v", path, err)
		}
	}
}

func TestNamespaceThatCannotBeCreated(t *testing.T) {
	t.Setenv("TMPDIR", filepath.Join(t.TempDir(), "missing"))
	tb := &fakeTB{}
	message := fatalMessage(tb, func() { fakediscord.NewNamespace(tb) })
	if !strings.Contains(message, "creating a namespace") {
		t.Errorf("fatal message = %q", message)
	}
}

func TestNamespaceWithSomethingLeftInItFailsTheTest(t *testing.T) {
	tb := &fakeTB{}
	ns := fakediscord.NewNamespace(tb)
	dir := runtimeDir(t, ns)
	stray := filepath.Join(dir, "stray")
	if err := os.WriteFile(stray, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	tb.finish()
	if errs := tb.reported(); len(errs) != 1 || !strings.Contains(errs[0], "removing the namespace") {
		t.Errorf("reported %q, want one error about removing the namespace", errs)
	}
}
