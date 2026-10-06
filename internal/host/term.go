package host

import (
	"context"
	"io"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Zafnok/claude-rich-presence/internal/control/protocol"
	"github.com/Zafnok/claude-rich-presence/internal/diag"
	"github.com/Zafnok/claude-rich-presence/internal/domain"
	"github.com/Zafnok/claude-rich-presence/internal/presence"
)

// local is the source of the node's own session. Connections count from 1.
const local = 0

// term is one term as host: from taking the lock to giving it up.
type term struct {
	n       *Node
	lock    Lock
	discord Discord
	since   time.Time

	// inbox is the one way to the goroutine that owns the registry. quit
	// asks that goroutine to end, and reduced is closed when it has.
	inbox   chan request
	quit    chan struct{}
	reduced chan struct{}
	// sessions is how many sessions the registry holds, for Status.
	sessions atomic.Int64

	// yield is closed by a valid stand_down, and broken by a panic in a
	// goroutine of the term. Either ends the term.
	yield     chan struct{}
	yieldOnce sync.Once
	broken    chan struct{}
	breakOnce sync.Once

	// draining is set when the term has begun to end: a hello is refused.
	draining atomic.Bool
	// firm says the term does not end for a stand_down. It is set for good
	// when the term begins.
	firm bool

	// stopDiscord ends the Discord connection, and cleared is closed when
	// it has ended.
	stopDiscord context.CancelFunc
	cleared     chan struct{}

	// listening ends when the listener is to be closed for good, and
	// accepted is closed when the goroutine that listens has ended.
	listening     context.Context
	stopListening context.CancelFunc
	accepted      chan struct{}
	// handlers counts the goroutines that serve connections. Only the
	// goroutine that listens adds to it.
	handlers sync.WaitGroup

	mu       sync.Mutex
	conns    map[uint64]io.Closer
	lastConn uint64
}

// host is one term as host. It forwards the node's own session, and ends
// the term when ctx ends, when a valid stand_down arrives or when a
// goroutine of the term has panicked.
func (n *Node) host(ctx context.Context, lock Lock) outcome {
	// The deferred calls run in the order a term ends in: everything else
	// first, and the lock last.
	defer n.release(lock)
	// A node that stood down and has the lock again, without having
	// followed a newer host in between, stood down for a node that did not
	// take over. Doing so again would only clear presence again, so this
	// term it stays.
	t := n.begin(lock, n.yielded)
	defer t.end()

	n.haste = 0
	if n.followed {
		n.followed = false
		n.counters.FailedOver()
	}
	n.log.Info("became the presence host")
	n.attach(RoleHost, t, n.version)
	for {
		select {
		case <-ctx.Done():
			return lost
		case <-t.broken:
			return missed
		case <-t.yield:
			n.log.Info("standing down for a newer version")
			n.yielded = true
			return yielded
		case <-n.wake:
			for _, m := range n.take() {
				t.receive(local, m)
			}
		}
	}
}

// release gives up the lock.
func (n *Node) release(lock Lock) {
	if lock.Release() != nil {
		n.log.Warn("the host lock was not released cleanly", diag.ErrorClass("release_failed"))
	}
}

// begin starts a term: the registry, the Discord connection and the
// listener, each on a goroutine of its own.
func (n *Node) begin(lock Lock, firm bool) *term {
	t := &term{
		n:        n,
		lock:     lock,
		firm:     firm,
		discord:  n.discord(),
		since:    n.clock.Now(),
		inbox:    make(chan request),
		quit:     make(chan struct{}),
		reduced:  make(chan struct{}),
		yield:    make(chan struct{}),
		broken:   make(chan struct{}),
		cleared:  make(chan struct{}),
		accepted: make(chan struct{}),
		conns:    map[uint64]io.Closer{},
	}
	// Neither context comes from the one given to Run: a term ends its
	// parts itself, in order.
	showing, stopDiscord := context.WithCancel(context.Background())
	t.stopDiscord = stopDiscord
	t.listening, t.stopListening = context.WithCancel(context.Background())
	t.spawn(t.reduce, t.reduced)
	t.spawn(func() { t.discord.Run(showing) }, t.cleared)
	t.spawn(t.listen, t.accepted)
	return t
}

// spawn runs f on a goroutine of the term and closes done when it has
// ended. A panic in f ends the term.
func (t *term) spawn(f func(), done chan<- struct{}) {
	go func() {
		defer close(done)
		if !t.n.protect(f) {
			t.breakOnce.Do(func() { close(t.broken) })
		}
	}()
}

// end stops hosting, in the order the next host relies on. No follower
// joins a host that is leaving. Presence is cleared while the followers are
// still connected, and so before any of them can show it again as host. The
// listener is closed, which removes the socket file, before the lock is
// released, which the caller does next.
func (t *term) end() {
	// 1. Stop accepting: a hello is refused from here on.
	t.draining.Store(true)
	t.n.detach()

	// 2. Clear presence. The Discord connection clears as it ends.
	t.stopDiscord()
	<-t.cleared

	// 3. Close follower connections.
	t.hangUp()

	// 4. Close the listener.
	t.stopListening()
	<-t.accepted
	// A connection that was accepted during the last two steps is closed
	// too. No more can come.
	t.hangUp()
	t.handlers.Wait()
	// The registry goes last: a connection reports to it as it closes.
	close(t.quit)
	<-t.reduced
	t.n.log.Info("stopped hosting presence")
}

// hangUp closes every open connection, which ends the goroutine that
// serves it.
func (t *term) hangUp() {
	t.mu.Lock()
	conns := make([]io.Closer, 0, len(t.conns))
	for _, c := range t.conns {
		conns = append(conns, c)
	}
	t.mu.Unlock()
	for _, c := range conns {
		_ = c.Close()
	}
}

// listen keeps the control socket open for as long as the term listens. A
// socket that cannot be opened, or that fails, is tried again with backoff;
// until it works the node hosts its own session alone.
func (t *term) listen() {
	delay := retryBase
	warned := false
	for {
		if listener, err := t.lock.Listen(); err == nil && t.accept(listener) {
			// The socket worked. Its next failure is a new one.
			delay = retryBase
			warned = false
		}
		if t.listening.Err() != nil {
			return
		}
		if !warned {
			warned = true
			t.n.log.Warn("the control socket is not available, so no follower can join; retrying", diag.ErrorClass("listen_failed"))
		}
		if !t.n.sleep(t.listening.Done(), jittered(delay, t.n.jitter())) {
			return
		}
		delay = min(2*delay, retryCap)
	}
}

// accept serves each connection on a goroutine of its own, until the
// listener fails or the term stops listening. The listener is closed when
// it returns. It reports whether any connection was accepted.
func (t *term) accept(l Listener) (served bool) {
	defer closeWith(t.listening, l)()
	for {
		conn, err := l.Accept()
		if err != nil {
			return served
		}
		served = true
		id := t.admit(conn)
		t.handlers.Add(1)
		go func() {
			defer t.handlers.Done()
			// A panic costs this connection and no other.
			t.n.protect(func() {
				defer t.dismiss(id, conn)
				t.converse(id, conn)
			})
		}()
	}
}

// closeWith arranges for c to be closed when ctx ends, which fails a call
// that is pending on c. The function it returns ends the arrangement,
// closes c in any case, and returns when c is closed.
func closeWith(ctx context.Context, c io.Closer) (closeNow func()) {
	closed := make(chan struct{})
	unwatch := context.AfterFunc(ctx, func() {
		defer close(closed)
		_ = c.Close()
	})
	return func() {
		if !unwatch() {
			<-closed
		}
		_ = c.Close()
	}
}

// admit records a connection, so that hangUp can close it, and numbers it.
func (t *term) admit(conn io.Closer) uint64 {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.lastConn++
	t.conns[t.lastConn] = conn
	return t.lastConn
}

// dismiss closes a connection and forgets it.
func (t *term) dismiss(id uint64, conn io.Closer) {
	t.mu.Lock()
	delete(t.conns, id)
	t.mu.Unlock()
	_ = conn.Close()
}

// converse is the host's side of one connection, from the hello until the
// connection ends.
func (t *term) converse(id uint64, conn io.ReadWriteCloser) {
	// Nothing here has a deadline. A connection that is not welcomed in
	// time is closed, which ends the read below.
	stopGreeting := t.n.clock.AfterFunc(greetTimeout, func() { _ = conn.Close() })
	defer stopGreeting()

	lines := protocol.NewDecoder(conn)
	// An error leaves first nil, which is not a hello either.
	first, _ := lines.Next()
	hello, ok := first.(protocol.Hello)
	if !ok {
		t.n.log.Debug("a connection was closed because it did not begin with a valid hello")
		return
	}
	answer := t.answer(hello)
	if protocol.Encode(conn, answer) != nil {
		return
	}
	_, welcomed := answer.(protocol.Welcome)
	if welcomed {
		stopGreeting()
		// The sessions of a connection end with it.
		defer t.submit(request{op: opClosed, source: id})
	}
	for {
		m, err := lines.Next()
		switch {
		case err == nil:
		case skippable(err):
			t.n.counters.EventDropped()
			continue
		default:
			return
		}
		switch m := m.(type) {
		case protocol.StandDown:
			if t.outranked(hello, welcomed) {
				t.yieldOnce.Do(func() { close(t.yield) })
			}
		case protocol.Status:
			if welcomed && protocol.Encode(conn, t.status()) != nil {
				return
			}
		case protocol.Sync, protocol.Event:
			// After a refusal the only message still read is stand_down.
			if welcomed {
				t.receive(id, m)
			}
		}
	}
}

// answer is the reply to a hello: a welcome, unless the protocol is one this
// binary does not speak or the term is ending.
func (t *term) answer(h protocol.Hello) protocol.Message {
	answer := protocol.Answer(h, t.n.version)
	if _, ok := answer.(protocol.Welcome); ok && t.draining.Load() {
		return protocol.Refuse{Reason: protocol.ReasonStandingDown}
	}
	return answer
}

// outranked reports whether a stand_down from the sender of h is valid,
// which is when the sender is the newer binary. A welcomed sender is judged
// by its binary version. A refused one is judged by its protocol version,
// which this host could not speak if it is newer. A term that is firm takes
// none.
func (t *term) outranked(h protocol.Hello, welcomed bool) bool {
	if t.firm {
		return false
	}
	if !welcomed {
		return h.Protocol > protocol.Version
	}
	return newer(h.Version, t.n.version)
}

// status is the host's summary of itself. It reads memory only, and nothing
// of any session but their number.
func (t *term) status() protocol.StatusResult {
	state := t.discord.State()
	if !state.Valid() {
		state = protocol.DiscordUnknown
	}
	return protocol.StatusResult{
		Discord:       state,
		Sessions:      int(t.sessions.Load()),
		Version:       t.n.version,
		UptimeSeconds: max(0, int64(t.n.clock.Now().Sub(t.since)/time.Second)),
	}
}

// receive passes a sync or an event from a source to the registry. It is
// the one path for a follower's session and for the node's own. Any other
// message is ignored.
func (t *term) receive(source uint64, m protocol.Message) {
	switch m := m.(type) {
	case protocol.Sync:
		var session *domain.Session
		if m.Session != nil {
			s := m.Session.Domain()
			s.Link = t.n.checked(s.Link)
			session = &s
		}
		t.submit(request{op: opSync, source: source, session: session})
	case protocol.Event:
		e := m.Event.Domain()
		e.Link = t.n.checked(e.Link)
		t.submit(request{op: opEvent, source: source, event: e})
	}
}

// checked is a repository link as the host will publish it, or empty when
// there is none or it is not valid. The host trusts no follower's link: it
// validates each again, and warns of one it drops without repeating it.
func (n *Node) checked(raw string) string {
	if raw == "" {
		return ""
	}
	if n.link != nil {
		if link, ok := n.link(raw); ok {
			return link
		}
	}
	n.log.Warn("a repository link was not accepted and is not published", diag.ErrorClass("link_invalid"))
	return ""
}

// submit hands a request to the goroutine that owns the registry, which
// never waits on anything, or drops it if that goroutine has ended.
func (t *term) submit(r request) {
	select {
	case t.inbox <- r:
	case <-t.reduced:
	}
}

// reduce is the goroutine that owns the registry. After every change it
// shows the result.
func (t *term) reduce() {
	r := registry{sessions: domain.NewRegistry(), owners: map[string]uint64{}}
	stopTimer := noTimer
	defer func() { stopTimer() }()
	for {
		select {
		case <-t.quit:
			return
		case req := <-t.inbox:
			if !r.apply(req, t.n.counters) {
				continue
			}
			stopTimer()
			stopTimer = t.show(r.sessions.Snapshot())
		}
	}
}

// show renders the sessions and hands the result to the Discord connection,
// whose scheduler decides when it is sent. If the result would change by
// itself, because the idle period ends, it arms a timer that renders again
// then, and returns the function that cancels it.
func (t *term) show(sessions []domain.Session) (stopTimer func() bool) {
	now := t.n.clock.Now()
	t.sessions.Store(int64(len(sessions)))
	if activity, ok := t.n.render(sessions, now, t.n.settings); ok {
		t.discord.Set(activity)
	} else {
		t.discord.Clear()
	}
	wait, ok := presence.ClearsIn(sessions, now, t.n.settings)
	if !ok {
		return noTimer
	}
	return t.n.clock.AfterFunc(wait, func() { t.submit(request{op: opTick}) })
}

// noTimer cancels no timer.
func noTimer() bool { return false }
