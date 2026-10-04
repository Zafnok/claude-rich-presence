//go:build !windows

package transport

import (
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/Zafnok/claude-rich-presence/internal/testutil/fakediscord"
)

// The namespace of the fake server is a directory, handed to the client as
// XDG_RUNTIME_DIR, so here the whole path from environment to connection is
// exercised.
func TestNewFindsTheServerThroughTheEnvironment(t *testing.T) {
	ns := fakediscord.NewNamespace(t)
	ns.Start(t, 6, fakediscord.Options{})
	values := map[string]string{}
	for _, entry := range ns.Env() {
		name, value, _ := strings.Cut(entry, "=")
		values[name] = value
	}
	conn, err := New(func(name string) string { return values[name] }).Dial(context.Background())
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	if err := conn.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}
}

// A Discord that crashed leaves its socket file behind. Connecting to it is
// refused, and the dialer moves on.
func TestALeftoverSocketFileCountsAsAbsent(t *testing.T) {
	dir, err := os.MkdirTemp("", "crp-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	prefix := filepath.Join(dir, "discord-ipc-")
	ln, err := net.ListenUnix("unix", &net.UnixAddr{Name: prefix + "0", Net: "unix"})
	if err != nil {
		t.Fatal(err)
	}
	ln.SetUnlinkOnClose(false)
	if err := ln.Close(); err != nil {
		t.Fatal(err)
	}

	conn, err := open(context.Background(), prefix+"0")
	if conn != nil || !errors.Is(err, syscall.ECONNREFUSED) {
		t.Errorf("open = %v, %v, want connection refused and a nil connection", conn, err)
	}
	if _, err := NewAt(prefix).Dial(context.Background()); !errors.Is(err, ErrNotRunning) {
		t.Errorf("Dial error = %v, want ErrNotRunning", err)
	}
}
