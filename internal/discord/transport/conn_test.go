package transport

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/Zafnok/claude-rich-presence/internal/testutil/fakediscord"
)

// The tests in this file run the real open function of the operating system
// they run on against the fake Discord server, under a name unique to the
// test, with the race detector on. They are the proof, on Windows as well as
// Unix, of deadlines and of concurrent reading and writing (risk R12).

// watchdog is how long a test waits, in real time, for something that should
// happen at once. Running out fails the test; nothing sleeps for it.
const watchdog = 10 * time.Second

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

func within2[A, B any](t *testing.T, what string, f func() (A, B)) (A, B) {
	t.Helper()
	type pair struct {
		a A
		b B
	}
	p := within(t, what, func() pair {
		a, b := f()
		return pair{a, b}
	})
	return p.a, p.b
}

const (
	opHandshake = 0
	opFrame     = 1
	opPing      = 3
	opPong      = 4
)

func frame(opcode uint32, payload string) []byte {
	b := binary.LittleEndian.AppendUint32(nil, opcode)
	b = binary.LittleEndian.AppendUint32(b, uint32(len(payload)))
	return append(b, payload...)
}

const handshake = `{"v":1,"client_id":"1"}`

type readResult struct {
	opcode  uint32
	payload []byte
	err     error
}

// readFrame reads one whole frame.
func readFrame(r io.Reader) readResult {
	header := make([]byte, 8)
	if _, err := io.ReadFull(r, header); err != nil {
		return readResult{err: err}
	}
	payload := make([]byte, binary.LittleEndian.Uint32(header[4:]))
	_, err := io.ReadFull(r, payload)
	return readResult{binary.LittleEndian.Uint32(header), payload, err}
}

// readFrameWithin reads one frame under the watchdog.
func readFrameWithin(t *testing.T, conn Conn) readResult {
	t.Helper()
	return within(t, "a frame", func() readResult { return readFrame(conn) })
}

func (r readResult) isReady() bool {
	return r.err == nil && r.opcode == opFrame && bytes.Contains(r.payload, []byte(`"READY"`))
}

// connectTo starts a fake Discord at an index and dials it through the prefix
// of its namespace.
func connectTo(t *testing.T, index int) (Conn, *fakediscord.Server) {
	t.Helper()
	ns := fakediscord.NewNamespace(t)
	srv := ns.Start(t, index, fakediscord.Options{})
	conn, err := within2(t, "Dial to return", func() (Conn, error) {
		return NewAt(ns.Prefix()).Dial(context.Background())
	})
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn, srv
}

// pendingRead is the function a reading goroutine runs in these tests, named
// so that awaitBlockedInRead can find it in a stack dump.
func pendingRead(conn Conn, result chan<- readResult) {
	result <- readFrame(conn)
}

// awaitBlockedInRead returns once the goroutine running pendingRead is
// parked. The only thing that function waits for is the read, so from then on
// the read is pending.
func awaitBlockedInRead(t *testing.T) {
	t.Helper()
	within(t, "the read to block", func() bool {
		buf := make([]byte, 1<<20)
		for {
			for _, g := range strings.Split(string(buf[:runtime.Stack(buf, true)]), "\n\n") {
				header, _, _ := strings.Cut(g, "\n")
				if strings.Contains(g, "transport.pendingRead(") && isParked(header) {
					return true
				}
			}
			runtime.Gosched()
		}
	})
}

// isParked reports whether the first line of a goroutine in a stack dump says
// it is waiting: in the poller on Unix, on a channel inside go-winio on
// Windows.
func isParked(header string) bool {
	for _, state := range []string{"[IO wait", "[chan receive", "[select"} {
		if strings.Contains(header, state) {
			return true
		}
	}
	return false
}

// isTimeout reports whether err is what a deadline produces. The standard
// library and go-winio use different error values, and both answer to this.
func isTimeout(err error) bool {
	var timeout interface{ Timeout() bool }
	return errors.As(err, &timeout) && timeout.Timeout()
}

func TestConnectsAtANonZeroIndexExchangesBytesAndCloses(t *testing.T) {
	conn, _ := connectTo(t, 3)
	if _, err := conn.Write(frame(opHandshake, handshake)); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if r := readFrameWithin(t, conn); !r.isReady() {
		t.Fatalf("read opcode %d, payload %s, error %v; want a READY frame", r.opcode, r.payload, r.err)
	}
	if err := conn.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}
	if _, err := conn.Write(frame(opPing, "{}")); err == nil {
		t.Error("Write after Close succeeded")
	}
}

func TestFirstListeningIndexWins(t *testing.T) {
	ns := fakediscord.NewNamespace(t)
	low := ns.Start(t, 2, fakediscord.Options{})
	ns.Start(t, 5, fakediscord.Options{})
	conn, err := NewAt(ns.Prefix()).Dial(context.Background())
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer conn.Close()
	if _, err := conn.Write(frame(opHandshake, handshake)); err != nil {
		t.Fatal(err)
	}
	if r := readFrameWithin(t, conn); !r.isReady() {
		t.Fatalf("read opcode %d, payload %s, error %v; want a READY frame", r.opcode, r.payload, r.err)
	}
	// Only the server that was dialled can make the client see a ping.
	if err := low.Ping([]byte(`{"from":2}`)); err != nil {
		t.Fatalf("the server at index 2 has no client: %v", err)
	}
	if r := readFrameWithin(t, conn); r.err != nil || r.opcode != opPing || string(r.payload) != `{"from":2}` {
		t.Errorf("read opcode %d, payload %s, error %v; want the ping from index 2", r.opcode, r.payload, r.err)
	}
}

func TestNoServerIsNotRunningAndReturnsPromptly(t *testing.T) {
	ns := fakediscord.NewNamespace(t)
	conn, err := within2(t, "Dial to return", func() (Conn, error) {
		return NewAt(ns.Prefix()).Dial(context.Background())
	})
	if !errors.Is(err, ErrNotRunning) || conn != nil {
		t.Errorf("Dial = %v, %v, want ErrNotRunning", conn, err)
	}
}

func TestDialOfARealEndpointWithACancelledContext(t *testing.T) {
	ns := fakediscord.NewNamespace(t)
	ns.Start(t, 0, fakediscord.Options{})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	conn, err := NewAt(ns.Prefix()).Dial(ctx)
	if !errors.Is(err, context.Canceled) || conn != nil {
		t.Errorf("Dial = %v, %v, want context.Canceled", conn, err)
	}
}

func TestReadDeadlineTimesOutWhenNothingArrives(t *testing.T) {
	conn, _ := connectTo(t, 1)
	if err := conn.SetReadDeadline(time.Now().Add(20 * time.Millisecond)); err != nil {
		t.Fatalf("SetReadDeadline: %v", err)
	}
	r := readFrameWithin(t, conn)
	if !isTimeout(r.err) {
		t.Fatalf("read error = %v, want a timeout", r.err)
	}

	// The connection is still good once the deadline is lifted.
	if err := conn.SetReadDeadline(time.Time{}); err != nil {
		t.Fatalf("clearing the deadline: %v", err)
	}
	if _, err := conn.Write(frame(opHandshake, handshake)); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if r := readFrameWithin(t, conn); !r.isReady() {
		t.Errorf("after the timeout read opcode %d, error %v", r.opcode, r.err)
	}
}

func TestWriteDeadlineInThePastFailsTheWrite(t *testing.T) {
	conn, _ := connectTo(t, 1)
	if err := conn.SetWriteDeadline(time.Now().Add(-time.Second)); err != nil {
		t.Fatalf("SetWriteDeadline: %v", err)
	}
	if _, err := conn.Write(frame(opHandshake, handshake)); !isTimeout(err) {
		t.Errorf("write error = %v, want a timeout", err)
	}
	if err := conn.SetDeadline(time.Time{}); err != nil {
		t.Fatalf("SetDeadline: %v", err)
	}
	if _, err := conn.Write(frame(opHandshake, handshake)); err != nil {
		t.Errorf("Write after clearing the deadline: %v", err)
	}
}

func TestWriteCompletesWhileAReadIsBlocked(t *testing.T) {
	conn, _ := connectTo(t, 1)
	result := make(chan readResult, 1)
	go pendingRead(conn, result)
	awaitBlockedInRead(t)

	// Nothing has been sent, so the read is still pending when this write
	// is made. Its answer is what the read then returns.
	n, err := within2(t, "the write", func() (int, error) { return conn.Write(frame(opHandshake, handshake)) })
	if err != nil || n != 8+len(handshake) {
		t.Fatalf("Write = %d, %v", n, err)
	}
	if r := within(t, "the read", func() readResult { return <-result }); !r.isReady() {
		t.Errorf("read opcode %d, payload %s, error %v; want a READY frame", r.opcode, r.payload, r.err)
	}

	// And once more on the established connection: a ping written while the
	// next read is pending.
	go pendingRead(conn, result)
	awaitBlockedInRead(t)
	if _, err := within2(t, "the write", func() (int, error) { return conn.Write(frame(opPing, `{"n":1}`)) }); err != nil {
		t.Fatalf("Write: %v", err)
	}
	r := within(t, "the read", func() readResult { return <-result })
	if r.err != nil || r.opcode != opPong || string(r.payload) != `{"n":1}` {
		t.Errorf("read opcode %d, payload %s, error %v; want the pong", r.opcode, r.payload, r.err)
	}
}

func TestCloseUnblocksAPendingRead(t *testing.T) {
	conn, _ := connectTo(t, 1)
	result := make(chan readResult, 1)
	go pendingRead(conn, result)
	awaitBlockedInRead(t)

	if err := conn.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	r := within(t, "the read to return", func() readResult { return <-result })
	if r.err == nil || isTimeout(r.err) || errors.Is(r.err, io.EOF) {
		t.Errorf("read error = %v, want a closed-connection error", r.err)
	}
}
