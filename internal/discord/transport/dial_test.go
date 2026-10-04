package transport

import (
	"context"
	"errors"
	"io/fs"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"
)

// errBusy stands in for a platform's "nothing is listening here" error.
var errBusy = errors.New("busy")

// fakeConn is a connection that was never opened.
type fakeConn struct {
	Conn
	addr string
}

// script is a fake open function: it answers each address from a table and
// records the order of the calls.
type script struct {
	answers map[string]error // nil means connect; a missing address is absent
	called  []string
}

func (s *script) open(_ context.Context, addr string) (Conn, error) {
	s.called = append(s.called, addr)
	err, ok := s.answers[addr]
	if !ok {
		return nil, &fs.PathError{Op: "open", Path: addr, Err: fs.ErrNotExist}
	}
	if err != nil {
		return nil, err
	}
	return fakeConn{addr: addr}, nil
}

func scripted(s *script, prefixes ...string) *dialer {
	d := newDialer(prefixes)
	d.open = s.open
	d.absent = []error{fs.ErrNotExist, errBusy}
	return d
}

func TestNewLooksWhereThisOperatingSystemPutsDiscord(t *testing.T) {
	getenv := env("XDG_RUNTIME_DIR", "/run/user/7")
	d := New(getenv).(*dialer)
	want := Candidates(Prefixes(runtime.GOOS, getenv))
	if !slices.Equal(d.candidates, want) {
		t.Errorf("candidates = %q, want %q", d.candidates, want)
	}
	if d.attempt != attemptTimeout {
		t.Errorf("attempt timeout = %v, want %v", d.attempt, attemptTimeout)
	}
}

func TestNewAtLooksOnlyAtThePrefix(t *testing.T) {
	d := NewAt("x-").(*dialer)
	if want := Candidates([]string{"x-"}); !slices.Equal(d.candidates, want) {
		t.Errorf("candidates = %q, want %q", d.candidates, want)
	}
}

func TestDialSelection(t *testing.T) {
	denied := errors.New("access denied")
	other := errors.New("another failure")
	cases := []struct {
		name       string
		answers    map[string]error
		wantAddr   string
		wantCalled int
		wantErr    error
	}{
		{"first index", map[string]error{"a-0": nil, "a-1": nil}, "a-0", 1, nil},
		{"a later index", map[string]error{"a-4": nil}, "a-4", 5, nil},
		{"the second prefix", map[string]error{"b-2": nil}, "b-2", 13, nil},
		{"the first prefix wins over the second", map[string]error{"a-9": nil, "b-0": nil}, "a-9", 10, nil},
		{"a busy endpoint is passed over", map[string]error{"a-0": errBusy, "a-1": nil}, "a-1", 2, nil},
		{"a failing endpoint is passed over", map[string]error{"a-0": denied, "a-1": nil}, "a-1", 2, nil},
		{"nothing anywhere", nil, "", 20, ErrNotRunning},
		{"only busy endpoints", map[string]error{"a-0": errBusy, "b-3": errBusy}, "", 20, ErrNotRunning},
		{"the first real failure is reported", map[string]error{"a-3": denied, "b-0": other}, "", 20, denied},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := &script{answers: c.answers}
			conn, err := scripted(s, "a-", "b-").Dial(context.Background())
			if !errors.Is(err, c.wantErr) {
				t.Fatalf("error = %v, want %v", err, c.wantErr)
			}
			if c.wantErr == nil {
				if got := conn.(fakeConn).addr; got != c.wantAddr {
					t.Errorf("connected to %q, want %q", got, c.wantAddr)
				}
			} else if conn != nil {
				t.Errorf("got a connection with the error %v", err)
			}
			if len(s.called) != c.wantCalled {
				t.Errorf("opened %d endpoints, want %d: %q", len(s.called), c.wantCalled, s.called)
			}
			if !slices.Equal(s.called, Candidates([]string{"a-", "b-"})[:len(s.called)]) {
				t.Errorf("opened out of order: %q", s.called)
			}
		})
	}
}

func TestDialFailureNamesTheEndpointAndIsNotNotRunning(t *testing.T) {
	denied := errors.New("access denied")
	s := &script{answers: map[string]error{"a-3": denied}}
	_, err := scripted(s, "a-").Dial(context.Background())
	if errors.Is(err, ErrNotRunning) {
		t.Errorf("error %v says Discord is not running", err)
	}
	if !strings.Contains(err.Error(), "a-3") {
		t.Errorf("error %q does not name the endpoint", err)
	}
}

func TestDialWithAContextAlreadyCancelledOpensNothing(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	s := &script{answers: map[string]error{"a-0": nil}}
	conn, err := scripted(s, "a-").Dial(ctx)
	if !errors.Is(err, context.Canceled) || conn != nil {
		t.Errorf("Dial = %v, %v, want context.Canceled", conn, err)
	}
	if len(s.called) != 0 {
		t.Errorf("opened %q", s.called)
	}
}

// blockingOpen waits for the context it is given, as a connect to a socket
// whose owner has stopped accepting does.
func blockingOpen(entered chan<- string) openFunc {
	return func(ctx context.Context, addr string) (Conn, error) {
		entered <- addr
		<-ctx.Done()
		return nil, ctx.Err()
	}
}

func TestCancellingTheContextAbortsADialInProgress(t *testing.T) {
	for _, addr := range []string{"a-0", "a-9"} {
		t.Run("while opening "+addr, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			entered := make(chan string)
			d := newDialer([]string{"a-"})
			d.attempt = time.Hour
			blocking := blockingOpen(entered)
			d.open = func(ctx context.Context, a string) (Conn, error) {
				if a != addr {
					return nil, fs.ErrNotExist
				}
				return blocking(ctx, a)
			}
			result := make(chan error, 1)
			go func() {
				_, err := d.Dial(ctx)
				result <- err
			}()
			within(t, "the open to start", func() string { return <-entered })
			cancel()
			if err := within(t, "Dial to return", func() error { return <-result }); !errors.Is(err, context.Canceled) {
				t.Errorf("error = %v, want context.Canceled", err)
			}
		})
	}
}

func TestAnAttemptThatHangsIsAbandonedAndTheNextIsTried(t *testing.T) {
	d := newDialer([]string{"a-"})
	d.attempt = time.Millisecond
	var called []string
	d.open = func(ctx context.Context, addr string) (Conn, error) {
		called = append(called, addr)
		if addr == "a-0" {
			<-ctx.Done()
			return nil, ctx.Err()
		}
		return fakeConn{addr: addr}, nil
	}
	conn, err := within2(t, "Dial to return", func() (Conn, error) { return d.Dial(context.Background()) })
	if err != nil {
		t.Fatal(err)
	}
	if got := conn.(fakeConn).addr; got != "a-1" {
		t.Errorf("connected to %q, want a-1 after %q", got, called)
	}
}

func TestOnlyHangingAttemptsIsNotRunning(t *testing.T) {
	d := newDialer([]string{"a-"})
	d.attempt = time.Millisecond
	d.open = func(ctx context.Context, addr string) (Conn, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	_, err := within2(t, "Dial to return", func() (Conn, error) { return d.Dial(context.Background()) })
	if !errors.Is(err, ErrNotRunning) {
		t.Errorf("error = %v, want ErrNotRunning", err)
	}
}
