package fakediscord_test

import (
	"encoding/json"
	"fmt"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/Zafnok/claude-rich-presence/internal/testutil/fakediscord"
)

// The tests talk to the server over a raw connection and build and parse
// frames by hand, byte by byte, so that they check the server's framing
// against Discord's documentation and not against the server's own code.

const (
	opHandshake = 0
	opFrame     = 1
	opClose     = 2
	opPing      = 3
	opPong      = 4

	handshake = `{"v":1,"client_id":"1234567890"}`
)

// watchdog is how long a test waits, in real time, for something that should
// happen at once. Running out fails the test; nothing sleeps for it.
const watchdog = 10 * time.Second

func raw(opcode uint32, payload string) []byte {
	n := len(payload)
	header := []byte{
		byte(opcode), byte(opcode >> 8), byte(opcode >> 16), byte(opcode >> 24),
		byte(n), byte(n >> 8), byte(n >> 16), byte(n >> 24),
	}
	return append(header, payload...)
}

// within runs f and fails the test if it does not return in time.
func within[T any](t *testing.T, what string, f func() T) T {
	t.Helper()
	done := make(chan T, 1)
	go func() { done <- f() }()
	select {
	case v := <-done:
		return v
	case <-time.After(watchdog):
		t.Fatalf("timed out waiting for %s", what)
		panic("unreachable")
	}
}

type client struct {
	t  *testing.T
	rw io.ReadWriteCloser
}

func connect(t *testing.T, addr string) *client {
	t.Helper()
	rw, err := dial(addr)
	if err != nil {
		t.Fatalf("dialing %s: %v", addr, err)
	}
	t.Cleanup(func() { _ = rw.Close() })
	return &client{t: t, rw: rw}
}

func (c *client) write(b []byte) {
	c.t.Helper()
	if _, err := c.rw.Write(b); err != nil {
		c.t.Fatalf("writing: %v", err)
	}
}

func (c *client) send(opcode uint32, payload string) {
	c.t.Helper()
	c.write(raw(opcode, payload))
}

type received struct {
	opcode  uint32
	payload []byte
	err     error
}

func (c *client) tryRead() received {
	c.t.Helper()
	return within(c.t, "a frame", func() received {
		var h [8]byte
		if _, err := io.ReadFull(c.rw, h[:]); err != nil {
			return received{err: err}
		}
		opcode := uint32(h[0]) | uint32(h[1])<<8 | uint32(h[2])<<16 | uint32(h[3])<<24
		length := uint32(h[4]) | uint32(h[5])<<8 | uint32(h[6])<<16 | uint32(h[7])<<24
		payload := make([]byte, length)
		_, err := io.ReadFull(c.rw, payload)
		return received{opcode: opcode, payload: payload, err: err}
	})
}

// read reads one frame and checks its opcode.
func (c *client) read(wantOpcode uint32) []byte {
	c.t.Helper()
	got := c.tryRead()
	if got.err != nil {
		c.t.Fatalf("reading a frame: %v", got.err)
	}
	if got.opcode != wantOpcode {
		c.t.Fatalf("opcode = %d, want %d (payload %s)", got.opcode, wantOpcode, got.payload)
	}
	return got.payload
}

// readJSON reads one frame, checks its opcode and decodes its payload.
func (c *client) readJSON(wantOpcode uint32) map[string]any {
	c.t.Helper()
	payload := c.read(wantOpcode)
	var m map[string]any
	if err := json.Unmarshal(payload, &m); err != nil {
		c.t.Fatalf("payload %q is not a JSON object: %v", payload, err)
	}
	return m
}

// handshake sends a good handshake and consumes the ready event.
func (c *client) handshake() {
	c.t.Helper()
	c.send(opHandshake, handshake)
	if m := c.readJSON(opFrame); m["evt"] != "READY" {
		c.t.Fatalf("answer to the handshake = %v, want a READY event", m)
	}
}

// expectClosed checks that the server closed the connection and sent nothing
// more before doing so.
func (c *client) expectClosed() {
	c.t.Helper()
	rest := within(c.t, "the connection to close", func() []byte {
		// The error differs by platform; that the read ends is the point.
		b, _ := io.ReadAll(c.rw)
		return b
	})
	if len(rest) != 0 {
		c.t.Fatalf("server sent %d more bytes before closing: %q", len(rest), rest)
	}
}

// expectCloseFrame reads a close frame with a code, then the end of the
// connection.
func (c *client) expectCloseFrame(code int) {
	c.t.Helper()
	m := c.readJSON(opClose)
	if m["code"] != float64(code) {
		c.t.Fatalf("close code = %v, want %d", m["code"], code)
	}
	if s, _ := m["message"].(string); s == "" {
		c.t.Fatalf("close frame %v has no message", m)
	}
	c.expectClosed()
}

// stepClock returns a time function that advances a second on each call.
func stepClock() func() time.Time {
	var mu sync.Mutex
	now := time.Unix(1_700_000_000, 0)
	return func() time.Time {
		mu.Lock()
		defer mu.Unlock()
		now = now.Add(time.Second)
		return now
	}
}

// fakeTB stands in for the test when the thing under test is what the server
// reports to it.
type fakeTB struct {
	mu       sync.Mutex
	errors   []string
	fatal    string
	cleanups []func()
}

type fatalSignal struct{}

func (f *fakeTB) Helper() {}

func (f *fakeTB) Errorf(format string, args ...any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.errors = append(f.errors, fmt.Sprintf(format, args...))
}

func (f *fakeTB) Fatalf(format string, args ...any) {
	f.mu.Lock()
	f.fatal = fmt.Sprintf(format, args...)
	f.mu.Unlock()
	panic(fatalSignal{})
}

func (f *fakeTB) Cleanup(fn func()) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.cleanups = append(f.cleanups, fn)
}

// finish runs the cleanups, last first, as the testing package does.
func (f *fakeTB) finish() {
	f.mu.Lock()
	cleanups := f.cleanups
	f.cleanups = nil
	f.mu.Unlock()
	for i := len(cleanups) - 1; i >= 0; i-- {
		cleanups[i]()
	}
}

func (f *fakeTB) reported() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.errors...)
}

// fatalMessage runs f and returns the message it failed the fake test with,
// or "" if it did not.
func fatalMessage(tb *fakeTB, f func()) (message string) {
	defer func() {
		if r := recover(); r != nil {
			if _, ok := r.(fatalSignal); !ok {
				panic(r)
			}
			message = tb.fatal
		}
	}()
	f()
	return ""
}

var _ fakediscord.TB = (*fakeTB)(nil)
var _ fakediscord.TB = (*testing.T)(nil)
