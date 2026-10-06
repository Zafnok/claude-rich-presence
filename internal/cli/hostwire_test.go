package cli

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/Zafnok/claude-rich-presence/internal/config"
	ctransport "github.com/Zafnok/claude-rich-presence/internal/control/transport"
	"github.com/Zafnok/claude-rich-presence/internal/host"
)

func TestHostConfigPorts(t *testing.T) {
	w := newWorld(t)
	paths, err := ctransport.Locate(w.env.get)
	if err != nil {
		t.Fatal(err)
	}
	cfg := w.sys.hostConfig(config.Default(), paths, w.env.get, nil)

	t.Run("a held lock is reported as held, once", func(t *testing.T) {
		first, err := cfg.Acquire()
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = first.Release() })
		if _, err := cfg.Acquire(); !errors.Is(err, host.ErrLocked) {
			t.Errorf("second Acquire() error = %v, want host.ErrLocked", err)
		}
	})

	t.Run("a lock that cannot be tried is not reported as held", func(t *testing.T) {
		blocked := w.sys.hostConfig(config.Default(), ctransport.Paths{Dir: w.runtime + "-file", Socket: "x", Lock: "y"}, w.env.get, nil)
		if err := os.WriteFile(w.runtime+"-file", nil, 0o600); err != nil {
			t.Fatal(err)
		}
		_, err := blocked.Acquire()
		if err == nil || errors.Is(err, host.ErrLocked) {
			t.Errorf("Acquire() error = %v, want a failure that is not ErrLocked", err)
		}
	})

	t.Run("a socket in an unsafe directory is not dialled", func(t *testing.T) {
		file := w.runtime + "-not-a-directory"
		if err := os.WriteFile(file, nil, 0o600); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Remove(file) })
		socket := filepath.Join(file, "s")
		blocked := w.sys.hostConfig(config.Default(), ctransport.Paths{Dir: file, Socket: socket, Lock: "y"}, w.env.get, nil)
		conn, err := blocked.Dial(context.Background())
		// Only the node's own error comes back: the transport's is dropped.
		if conn != nil || err != host.ErrUnsafe {
			t.Errorf("Dial() = %v, %v, want nil and exactly ErrUnsafe", conn, err)
		}
	})

	t.Run("a listener cannot be opened once the lock is released", func(t *testing.T) {
		other := w.sys.hostConfig(config.Default(), ctransport.Paths{Dir: paths.Dir, Socket: paths.Socket, Lock: paths.Lock + "2"}, w.env.get, nil)
		held, err := other.Acquire()
		if err != nil {
			t.Fatal(err)
		}
		if err := held.Release(); err != nil {
			t.Fatal(err)
		}
		if _, err := held.Listen(); err == nil {
			t.Error("Listen() after Release() succeeded")
		}
	})

	t.Run("dialling a socket nobody listens on says no host", func(t *testing.T) {
		if _, err := cfg.Dial(context.Background()); !errors.Is(err, ctransport.ErrNoHost) {
			t.Errorf("Dial() error = %v, want ErrNoHost", err)
		}
	})
}
