package transport

import (
	"context"
	"errors"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/Microsoft/go-winio"

	"github.com/Zafnok/claude-rich-presence/internal/testutil/fakediscord"
)

// A go-winio listener that is not in Accept holds the name of its pipe with
// an instance that takes no client, so a client is told the pipe is busy for
// as long as it keeps asking.
func TestABusyPipeIsPassedOverAfterTheAttemptTimeout(t *testing.T) {
	ns := fakediscord.NewNamespace(t)
	ln, err := winio.ListenPipe(ns.Addr(0), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if _, err := open(ctx, ns.Addr(0)); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("open of the busy pipe = %v, want the deadline of the context", err)
	}

	ns.Start(t, 1, fakediscord.Options{})
	d := NewAt(ns.Prefix()).(*dialer)
	d.attempt = 50 * time.Millisecond
	var opened []string
	d.open = func(ctx context.Context, addr string) (Conn, error) {
		opened = append(opened, addr)
		return open(ctx, addr)
	}
	conn, err := within2(t, "Dial to return", func() (Conn, error) { return d.Dial(context.Background()) })
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer conn.Close()
	if len(opened) != 2 || opened[1] != ns.Addr(1) {
		t.Errorf("opened %q, want the busy pipe and then index 1", opened)
	}
}

// Discord documents the pipe as \\?\pipe\discord-ipc-N, which is the spelling
// Prefixes returns. The prefix of the fake server uses the \\.\ spelling.
func TestTheDocumentedSpellingOfThePipeConnects(t *testing.T) {
	ns := fakediscord.NewNamespace(t)
	ns.Start(t, 4, fakediscord.Options{})
	prefix := `\\?\pipe\` + strings.TrimPrefix(ns.Prefix(), `\\.\pipe\`)
	conn, err := NewAt(prefix).Dial(context.Background())
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	if err := conn.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}
}

func TestOpenOfAMissingPipe(t *testing.T) {
	ns := fakediscord.NewNamespace(t)
	conn, err := open(context.Background(), ns.Addr(0))
	if conn != nil || !newDialer(nil).isAbsent(err) {
		t.Errorf("open = %v, %v, want an absent error and a nil connection", conn, err)
	}
	if !errors.Is(err, syscall.ERROR_FILE_NOT_FOUND) {
		t.Errorf("error = %v, want ERROR_FILE_NOT_FOUND", err)
	}
}
