//go:build !windows

package fakediscord

import (
	"io"
	"net"
	"os"
	"path/filepath"
)

// newLocation makes the directory that stands in for the runtime directory.
// The name is short, because a socket path is limited to about a hundred
// bytes.
func newLocation() (string, error) { return os.MkdirTemp("", "fdisc-") }

// removeLocation fails if anything is left in the directory.
func removeLocation(location string) error { return os.Remove(location) }

func locationPrefix(location string) string { return filepath.Join(location, "discord-ipc-") }

func locationEnv(location string) []string { return []string{"XDG_RUNTIME_DIR=" + location} }

type socketListener struct{ ln net.Listener }

func listen(addr string) (listener, error) {
	ln, err := net.Listen("unix", addr)
	if err != nil {
		return nil, err
	}
	return socketListener{ln}, nil
}

func (l socketListener) Accept() (io.ReadWriteCloser, error) { return l.ln.Accept() }

// Interrupt closes the listener, which also removes the socket file.
func (l socketListener) Interrupt() { _ = l.ln.Close() }

func (l socketListener) Close() error { return nil }
