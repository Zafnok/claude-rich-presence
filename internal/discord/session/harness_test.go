package session_test

import (
	"context"
	"encoding/binary"
	"errors"
	"io"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Zafnok/claude-rich-presence/internal/config"
	"github.com/Zafnok/claude-rich-presence/internal/diag"
	"github.com/Zafnok/claude-rich-presence/internal/discord/session"
	"github.com/Zafnok/claude-rich-presence/internal/discord/transport"
	"github.com/Zafnok/claude-rich-presence/internal/domain"
	"github.com/Zafnok/claude-rich-presence/internal/testutil/fakeclock"
	"github.com/Zafnok/claude-rich-presence/internal/testutil/fakediscord"
)

// The tests run a Manager against the fake Discord server, through the real
// dialer of the operating system they run on, with the fake clock. Nothing
// sleeps for a result: a test waits for a log line, a recorded event or a
// timer, and then moves the clock by an exact amount.

// watchdog is how long a test waits, in real time, for something that should
// happen at once. Running out fails the test.
const watchdog = 10 * time.Second

const applicationID = "123456789012345678"

var start = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

// within runs f and fails the test if it does not return in time.
func within(t *testing.T, what string, f func()) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		defer close(done)
		f()
	}()
	select {
	case <-done:
	case <-time.After(watchdog):
		t.Fatalf("timed out waiting for %s", what)
	}
}

// eventually polls until cond holds.
func eventually(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(watchdog)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(time.Millisecond)
	}
}

// clock is the fake clock, and remembers every wait asked of it.
type clock struct {
	*fakeclock.Clock
	mu    sync.Mutex
	waits []time.Duration
}

func (c *clock) AfterFunc(d time.Duration, f func()) func() bool {
	c.mu.Lock()
	c.waits = append(c.waits, d)
	c.mu.Unlock()
	return c.Clock.AfterFunc(d, f)
}

// lastWait is the most recent wait asked for.
func (c *clock) lastWait() time.Duration {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.waits[len(c.waits)-1]
}

// sink collects the log.
type sink struct {
	mu  sync.Mutex
	buf strings.Builder
}

func (s *sink) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.Write(p)
}

func (s *sink) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.String()
}

func (s *sink) count(substr string) int { return strings.Count(s.String(), substr) }

// gate is a connection whose writes a test can hold: a held write returns
// only when the connection is closed, as a write to a peer that has stopped
// reading does.
type gate struct {
	io.ReadWriteCloser
	mu      sync.Mutex
	held    bool
	blocked chan struct{}
	once    sync.Once
	closed  chan struct{}
}

func newGate(conn io.ReadWriteCloser) *gate {
	return &gate{ReadWriteCloser: conn, blocked: make(chan struct{}, 8), closed: make(chan struct{})}
}

func (g *gate) hold() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.held = true
}

func (g *gate) Write(p []byte) (int, error) {
	g.mu.Lock()
	held := g.held
	g.mu.Unlock()
	if held {
		g.blocked <- struct{}{}
		<-g.closed
		return 0, io.ErrClosedPipe
	}
	return g.ReadWriteCloser.Write(p)
}

func (g *gate) Close() error {
	g.once.Do(func() { close(g.closed) })
	return g.ReadWriteCloser.Close()
}

// awaitBlocked returns once a write is being held.
func (g *gate) awaitBlocked(t *testing.T) {
	t.Helper()
	select {
	case <-g.blocked:
	case <-time.After(watchdog):
		t.Fatal("timed out waiting for a write to block")
	}
}

type harness struct {
	t     *testing.T
	clock *clock
	ns    *fakediscord.Namespace
	logs  *sink
	m     *session.Manager

	// dialer is what the manager dials with. A test may replace it before
	// start.
	dialer session.DialFunc

	mu    sync.Mutex
	gates []*gate

	cancel context.CancelFunc
	done   chan struct{}
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	h := &harness{
		t:     t,
		clock: &clock{Clock: fakeclock.New(start)},
		ns:    fakediscord.NewNamespace(t),
		logs:  &sink{},
		done:  make(chan struct{}),
	}
	h.dialer = h.dial
	h.m = session.New(session.Config{
		ApplicationID: applicationID,
		Dial:          func(ctx context.Context) (io.ReadWriteCloser, error) { return h.dialer(ctx) },
		Clock:         h.clock,
		Jitter:        func() float64 { return 1 },
		Logger:        diag.NewLogger(h.logs, config.LogDebug, h.clock.Now),
	})
	return h
}

// dial is what internal/cli will do: the real dialer, with its sentinel
// mapped to the session's.
func (h *harness) dial(ctx context.Context) (io.ReadWriteCloser, error) {
	conn, err := transport.NewAt(h.ns.Prefix()).Dial(ctx)
	if errors.Is(err, transport.ErrNotRunning) {
		return nil, session.ErrNotRunning
	}
	if err != nil {
		return nil, err
	}
	g := newGate(conn)
	h.mu.Lock()
	h.gates = append(h.gates, g)
	h.mu.Unlock()
	return g, nil
}

// conn is the connection dialled most recently.
func (h *harness) conn() *gate {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.gates[len(h.gates)-1]
}

// serve starts a fake Discord where the manager dials.
func (h *harness) serve(b fakediscord.Behavior) *fakediscord.Server {
	h.t.Helper()
	return h.ns.Start(h.t, 0, fakediscord.Options{Now: h.clock.Now, Behavior: b})
}

// start runs the manager. When the test ends it is stopped, and must leave
// nothing behind.
func (h *harness) start() {
	h.t.Helper()
	var ctx context.Context
	ctx, h.cancel = context.WithCancel(context.Background())
	go func() {
		defer close(h.done)
		h.m.Run(ctx)
	}()
	h.t.Cleanup(h.cleanup)
}

// wait returns when Run has.
func (h *harness) wait() {
	h.t.Helper()
	select {
	case <-h.done:
	case <-time.After(watchdog):
		h.t.Fatal("timed out waiting for Run to return")
	}
}

func (h *harness) cleanup() {
	h.t.Helper()
	h.cancel()
	deadline := time.After(watchdog)
	for stopped := false; !stopped; {
		select {
		case <-h.done:
			stopped = true
		case <-time.After(100 * time.Millisecond):
			// A test may end with a fake Discord that will not acknowledge
			// the final clear. Only the clock ends that wait.
			h.clock.Advance(session.ClearTimeout)
		case <-deadline:
			h.t.Fatal("timed out waiting for Run to return")
		}
	}
	wantNoGoroutines(h.t)
	if log := h.logs.String(); strings.Contains(log, "details-") || strings.Contains(log, "state-") {
		h.t.Errorf("the log holds activity content:\n%s", log)
	}
}

// awaitState waits until the manager has logged entering a state n times.
func (h *harness) awaitState(s session.State, n int) {
	h.t.Helper()
	line := "state=" + s.String()
	eventually(h.t, line, func() bool { return h.logs.count(line) >= n })
}

// timers waits until exactly n timers are armed on the clock.
func (h *harness) timers(n int) {
	h.t.Helper()
	within(h.t, "timers to be armed", func() { h.clock.WaitForTimers(n) })
}

// inBackoff waits until the manager has become disconnected for the nth time
// and its retry timer is armed. scheduled is how many timers the scheduler
// holds besides: one when an activity is waiting for its interval.
func (h *harness) inBackoff(n, scheduled int) {
	h.t.Helper()
	h.awaitState(session.Disconnected, n)
	h.timers(1 + scheduled)
}

// ready waits until the manager is ready for the nth time.
func (h *harness) ready(n int) {
	h.t.Helper()
	h.awaitState(session.Ready, n)
}

func (h *harness) wantStatus(state session.State, lastError string) {
	h.t.Helper()
	if got := h.m.Status(); got.State != state || got.LastError != lastError {
		h.t.Errorf("status is %v with error %q, want %v with %q", got.State, got.LastError, state, lastError)
	}
}

func activity(name string) domain.Activity {
	return domain.Activity{Details: "details-" + name, State: "state-" + name}
}

// count is how many events of a kind the server has recorded.
func count(srv *fakediscord.Server, kind fakediscord.Kind) int {
	n := 0
	for _, ev := range srv.Events() {
		if ev.Kind == kind {
			n++
		}
	}
	return n
}

// rawFrame encodes a frame for the server to send as it is.
func rawFrame(opcode uint32, payload string) []byte {
	b := binary.LittleEndian.AppendUint32(nil, opcode)
	b = binary.LittleEndian.AppendUint32(b, uint32(len(payload)))
	return append(b, payload...)
}

// managerGoroutines counts goroutines that are running the manager's code or
// its scheduler's. On Windows, go-winio keeps one goroutine of its own for
// the life of the process; it is not one of these.
func managerGoroutines() int {
	buf := make([]byte, 1<<20)
	buf = buf[:runtime.Stack(buf, true)]
	n := 0
	for _, g := range strings.Split(string(buf), "\n\n") {
		if strings.Contains(g, "internal/discord/session.") || strings.Contains(g, "internal/schedule.") {
			n++
		}
	}
	return n
}

// wantNoGoroutines waits for every goroutine of the manager to be gone. One
// that has finished its work may still be a moment from exiting, so the check
// polls, with a deadline that only turns a leak into a failure.
func wantNoGoroutines(t *testing.T) {
	t.Helper()
	eventually(t, "the manager's goroutines to end", func() bool { return managerGoroutines() == 0 })
}
