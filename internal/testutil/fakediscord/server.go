package fakediscord

import (
	"errors"
	"io"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

// TB is the part of testing.TB the package uses.
type TB interface {
	Helper()
	Errorf(format string, args ...any)
	Fatalf(format string, args ...any)
	Cleanup(func())
}

// Behavior is how the server deviates from Discord. The zero value does not
// deviate. It can be replaced while the server runs with [Server.Set]; each
// frame is handled under the behavior in force when it was recorded.
type Behavior struct {
	// RejectHandshake, if not zero, answers every handshake with a close
	// frame carrying this code, then closes the connection.
	RejectHandshake int
	// ActivityError, if not zero, answers every set-activity and clear with
	// an ERROR event carrying this code and the command's nonce.
	ActivityError int
	// CloseOnConnect closes each connection as soon as it is accepted.
	CloseOnConnect bool
	// CloseAfterFrames, if positive, closes a connection once it has read
	// that many frames. The last one is recorded and not answered.
	CloseAfterFrames int
	// Silent keeps reading and recording but sends nothing, and never closes
	// the connection itself.
	Silent bool
	// Delay is waited before each answer, using the server's After function.
	Delay time.Duration
}

// Options configures a server. The zero value is usable.
type Options struct {
	// Now stamps recorded events. The default is time.Now.
	Now func() time.Time
	// After is what Behavior.Delay waits on. The default is time.After. A
	// test that must not depend on real time passes its own.
	After func(time.Duration) <-chan time.Time
	// Behavior is the behavior the server starts with.
	Behavior Behavior
	// Patience bounds, in real time, how long Await waits and how long Close
	// waits for the server's goroutines. Running out fails the test. It is a
	// watchdog, not a delay. The default is ten seconds.
	Patience time.Duration
}

// Malformed chooses what [Server.SendMalformed] writes.
type Malformed int

const (
	// MalformedJSON is a frame whose payload is not JSON.
	MalformedJSON Malformed = iota + 1
	// MalformedTruncated is a header that declares more payload than follows.
	MalformedTruncated
	// MalformedOversize is a header that declares a payload over the limit.
	MalformedOversize
	// MalformedOpcode is a frame with an opcode that does not exist.
	MalformedOpcode
)

var malformedBytes = map[Malformed][]byte{
	MalformedJSON:      frame(opFrame, []byte(`{"cmd":`)),
	MalformedTruncated: frame(opFrame, make([]byte, 64))[:headerSize+5],
	MalformedOversize:  frame(opFrame, make([]byte, MaxFrameSize))[:headerSize],
	MalformedOpcode:    frame(99, []byte(`{}`)),
}

// ErrNoConnection is returned by the Send methods when no connection is open.
var ErrNoConnection = errors.New("fakediscord: no open connection")

// listener is the platform's endpoint. Interrupt makes Accept fail, now and
// from then on. Close releases the endpoint once no Accept is running.
type listener interface {
	Accept() (io.ReadWriteCloser, error)
	Interrupt()
	Close() error
}

// Namespace is a place for endpoints that no other test and no real Discord
// shares: a fresh directory on Linux and macOS, a random pipe name prefix on
// Windows. Servers in one namespace differ by index, as Discord builds do.
type Namespace struct {
	location string
}

// NewNamespace makes a namespace that is removed when the test ends. If
// anything is still in it then, the test fails.
func NewNamespace(t TB) *Namespace {
	t.Helper()
	location, err := newLocation()
	if err != nil {
		t.Fatalf("fakediscord: creating a namespace: %v", err)
	}
	t.Cleanup(func() {
		if err := removeLocation(location); err != nil {
			t.Errorf("fakediscord: removing the namespace: %v", err)
		}
	})
	return &Namespace{location: location}
}

// Prefix is what a dialer appends an index to: the full endpoint name up to
// and including "discord-ipc-". On Windows it replaces `\\?\pipe\discord-ipc-`.
// Elsewhere it is the socket path in the directory that Env points at.
func (n *Namespace) Prefix() string { return locationPrefix(n.location) }

// Addr is the endpoint for an index, whether or not a server listens there.
func (n *Namespace) Addr(index int) string { return n.Prefix() + strconv.Itoa(index) }

// Env is the environment, as "NAME=value" entries, under which a dialer that
// follows Discord's search order finds this namespace. On Windows there is no
// such variable and it is empty; pass the dialer the Prefix instead.
func (n *Namespace) Env() []string { return locationEnv(n.location) }

// Start starts a server at index 0 of a namespace of its own.
func Start(t TB, o Options) *Server {
	t.Helper()
	return NewNamespace(t).Start(t, 0, o)
}

// Start starts a server at an index from 0 to 9. It is shut down when the test
// ends, if the test has not already called Close.
func (n *Namespace) Start(t TB, index int, o Options) *Server {
	t.Helper()
	if index < 0 || index > 9 {
		t.Fatalf("fakediscord: index %d is not between 0 and 9", index)
	}
	if o.Now == nil {
		o.Now = time.Now
	}
	if o.After == nil {
		o.After = time.After
	}
	if o.Patience == 0 {
		o.Patience = 10 * time.Second
	}
	ln, err := listen(n.Addr(index))
	if err != nil {
		t.Fatalf("fakediscord: listening at %s: %v", n.Addr(index), err)
	}
	s := &Server{
		t:         t,
		namespace: n,
		index:     index,
		now:       o.Now,
		after:     o.After,
		patience:  o.Patience,
		ln:        ln,
		behavior:  o.Behavior,
		conns:     map[int]*conn{},
		changed:   make(chan struct{}),
		stopping:  make(chan struct{}),
	}
	s.spawn(s.accept)
	t.Cleanup(s.Close)
	return s
}

// Server is one fake Discord endpoint.
type Server struct {
	t         TB
	namespace *Namespace
	index     int
	now       func() time.Time
	after     func(time.Duration) <-chan time.Time
	patience  time.Duration
	ln        listener

	closeOnce sync.Once
	stopping  chan struct{}
	wg        sync.WaitGroup
	running   atomic.Int64

	mu       sync.Mutex
	behavior Behavior
	closed   bool
	accepted int
	conns    map[int]*conn
	events   []Event
	changed  chan struct{} // closed and replaced whenever events grows
}

// conn is one accepted connection. Only its serving goroutine reads; anyone
// may write, one frame at a time.
type conn struct {
	id         int
	rw         io.ReadWriteCloser
	writeMu    sync.Mutex
	closeOnce  sync.Once
	frames     int
	handshaken bool
}

func (c *conn) write(b []byte) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	_, err := c.rw.Write(b)
	return err
}

func (c *conn) close() { c.closeOnce.Do(func() { _ = c.rw.Close() }) }

// Namespace returns the namespace the server listens in.
func (s *Server) Namespace() *Namespace { return s.namespace }

// Index returns the index the server listens at.
func (s *Server) Index() int { return s.index }

// Addr returns the socket path or pipe name the server listens at.
func (s *Server) Addr() string { return s.namespace.Addr(s.index) }

// Set replaces the behavior for frames read from now on.
func (s *Server) Set(b Behavior) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.behavior = b
}

// Events returns everything recorded so far, in order.
func (s *Server) Events() []Event {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]Event(nil), s.events...)
}

// Await blocks until at least n events of a kind are recorded and returns the
// events of that kind. If that takes longer than the server's patience, it
// fails the test.
func (s *Server) Await(kind Kind, n int) []Event {
	s.t.Helper()
	timeout := time.After(s.patience)
	for {
		s.mu.Lock()
		var found []Event
		for _, ev := range s.events {
			if ev.Kind == kind {
				found = append(found, ev)
			}
		}
		changed := s.changed
		s.mu.Unlock()
		if len(found) >= n {
			return found
		}
		select {
		case <-changed:
		case <-timeout:
			s.t.Fatalf("fakediscord: waited %v for %d %v events, have %d", s.patience, n, kind, len(found))
			return found
		}
	}
}

// Ping sends a ping frame on the most recent open connection.
func (s *Server) Ping(payload []byte) error { return s.send(frame(opPing, payload)) }

// SendClose sends a close frame with a code and message on the most recent
// open connection. The connection itself stays open.
func (s *Server) SendClose(code int, message string) error {
	return s.send(frame(opClose, closePayload(code, message)))
}

// SendMalformed sends a malformed frame on the most recent open connection.
func (s *Server) SendMalformed(m Malformed) error {
	b, ok := malformedBytes[m]
	if !ok {
		return errors.New("fakediscord: unknown kind of malformed frame")
	}
	return s.send(b)
}

// SendRaw writes bytes as they are on the most recent open connection.
func (s *Server) SendRaw(b []byte) error { return s.send(b) }

// Disconnect closes the most recent open connection.
func (s *Server) Disconnect() error {
	c := s.latest()
	if c == nil {
		return ErrNoConnection
	}
	c.close()
	return nil
}

func (s *Server) send(b []byte) error {
	c := s.latest()
	if c == nil {
		return ErrNoConnection
	}
	return c.write(b)
}

func (s *Server) latest() *conn {
	s.mu.Lock()
	defer s.mu.Unlock()
	var latest *conn
	for _, c := range s.conns {
		if latest == nil || c.id > latest.id {
			latest = c
		}
	}
	return latest
}

// Close shuts the server down: it stops listening, closes every connection
// and waits for its goroutines. If one is still running when patience runs
// out, the test fails. Close may be called more than once.
func (s *Server) Close() {
	s.t.Helper()
	s.closeOnce.Do(func() {
		close(s.stopping)
		s.ln.Interrupt()
		s.mu.Lock()
		s.closed = true
		for _, c := range s.conns {
			c.close()
		}
		s.mu.Unlock()

		done := make(chan struct{})
		go func() {
			s.wg.Wait()
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(s.patience):
			s.t.Errorf("fakediscord: %d goroutines still running %v after shutdown", s.running.Load(), s.patience)
			return
		}
		if err := s.ln.Close(); err != nil {
			s.t.Errorf("fakediscord: releasing %s: %v", s.Addr(), err)
		}
	})
}

// spawn runs f on a goroutine that Close waits for.
func (s *Server) spawn(f func()) {
	s.wg.Add(1)
	s.running.Add(1)
	go func() {
		defer s.wg.Done()
		defer s.running.Add(-1)
		f()
	}()
}

func (s *Server) accept() {
	for {
		rw, err := s.ln.Accept()
		if err != nil {
			select {
			case <-s.stopping:
			default:
				s.t.Errorf("fakediscord: accepting at %s: %v", s.Addr(), err)
			}
			return
		}
		s.mu.Lock()
		if s.closed {
			s.mu.Unlock()
			_ = rw.Close()
			continue
		}
		s.accepted++
		c := &conn{id: s.accepted, rw: rw}
		s.conns[c.id] = c
		s.mu.Unlock()
		s.spawn(func() { s.serve(c) })
	}
}

// record appends an event and returns the behavior in force at that moment,
// so that a test which waits for an event and then changes the behavior
// cannot change how that event is answered.
func (s *Server) record(c *conn, ev Event) Behavior {
	s.mu.Lock()
	defer s.mu.Unlock()
	ev.Conn, ev.Time = c.id, s.now()
	s.events = append(s.events, ev)
	close(s.changed)
	s.changed = make(chan struct{})
	return s.behavior
}

func (s *Server) current() Behavior {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.behavior
}

// serve reads frames from one connection until either side ends it.
func (s *Server) serve(c *conn) {
	defer func() {
		c.close()
		s.mu.Lock()
		delete(s.conns, c.id)
		s.mu.Unlock()
		s.record(c, Event{Kind: KindDisconnect})
	}()
	if s.current().CloseOnConnect {
		return
	}
	for {
		opcode, payload, err := readFrame(c.rw)
		oversize := errors.Is(err, errOversize)
		if err != nil && !oversize {
			return
		}
		c.frames++
		ev, nonce := classify(opcode, payload)
		if oversize {
			ev = Event{Kind: KindInvalid, Opcode: opcode}
		}
		b := s.record(c, ev)
		if b.CloseAfterFrames > 0 && c.frames >= b.CloseAfterFrames {
			return
		}
		answer, hangUp := reply(c, ev, nonce, b)
		if b.Silent {
			// Nothing is sent and the server does not hang up. After an
			// oversized header the stream cannot be followed any further,
			// so the rest is read and discarded.
			if oversize {
				_, _ = io.Copy(io.Discard, c.rw)
				return
			}
			if ev.Kind == KindClose {
				return
			}
			continue
		}
		if answer != nil {
			if b.Delay > 0 {
				select {
				case <-s.after(b.Delay):
				case <-s.stopping:
					return
				}
			}
			if c.write(answer) != nil {
				return
			}
		}
		if hangUp {
			return
		}
	}
}

// reply decides the answer to an event, if any, and whether the server then
// closes the connection.
func reply(c *conn, ev Event, nonce []byte, b Behavior) (answer []byte, hangUp bool) {
	unsupported := func(message string) ([]byte, bool) {
		return frame(opClose, closePayload(CloseUnsupported, message)), true
	}
	switch ev.Kind {
	case KindHandshake:
		switch {
		case c.handshaken:
			return unsupported("already handshaken")
		case b.RejectHandshake != 0:
			return frame(opClose, closePayload(b.RejectHandshake, "rejected by the test script")), true
		case ev.Version != 1:
			return frame(opClose, closePayload(CloseInvalidVersion, "Invalid Version")), true
		case ev.ClientID == "":
			return frame(opClose, closePayload(CloseInvalidClientID, "Invalid Client ID")), true
		}
		c.handshaken = true
		return frame(opFrame, readyPayload()), false
	case KindSetActivity, KindClearActivity, KindCommand:
		switch {
		case !c.handshaken:
			return unsupported("need to handshake first")
		case ev.Kind == KindCommand:
			return frame(opFrame, errorPayload(ev.Command, nonce, ErrorInvalidCommand, "Invalid command: "+ev.Command)), false
		case b.ActivityError != 0:
			return frame(opFrame, errorPayload(ev.Command, nonce, b.ActivityError, "rejected by the test script")), false
		}
		return frame(opFrame, ackPayload(ev, nonce)), false
	case KindPing:
		return frame(opPong, ev.Payload), false
	case KindPong:
		return nil, false
	case KindClose:
		return nil, true
	default:
		return unsupported("invalid frame")
	}
}
