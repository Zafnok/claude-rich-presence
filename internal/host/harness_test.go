package host_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Zafnok/claude-rich-presence/internal/config"
	"github.com/Zafnok/claude-rich-presence/internal/control/protocol"
	"github.com/Zafnok/claude-rich-presence/internal/diag"
	"github.com/Zafnok/claude-rich-presence/internal/domain"
	"github.com/Zafnok/claude-rich-presence/internal/host"
	"github.com/Zafnok/claude-rich-presence/internal/presence"
	"github.com/Zafnok/claude-rich-presence/internal/testutil/fakeclock"
)

// The tests run nodes in a world that is all in memory: one lock, one
// control socket, connections that are queues of bytes, a Discord connection
// that records what it is told, and the fake clock. Nothing sleeps for a
// result. A test waits for a condition, and where the nodes need time to
// pass, it moves the clock while it waits.

// watchdog is how long a test waits, in real time, for something that should
// happen at once. Running out fails the test.
const watchdog = 20 * time.Second

// tick is how far settle moves the clock between two looks at its condition.
const tick = 10 * time.Millisecond

var start = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

var (
	errNoHost  = errors.New("nothing listens on the control socket")
	errBroken  = errors.New("injected failure")
	errNotHeld = errors.New("the lock is not held")
	errClosed  = errors.New("the listener is closed")
)

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

// world is one user's machine.
type world struct {
	t     *testing.T
	clock *clock
	// jitter is what every node's jitter source returns, unless a test
	// replaces the source.
	jitter func() float64

	mu       sync.Mutex
	holder   *lock     // nil while the lock is free
	listener *listener // the control socket, while someone listens on it
	trace    []string
	procs    []*proc
	conns    int
}

func newWorld(t *testing.T) *world {
	t.Helper()
	w := &world{
		t:      t,
		clock:  &clock{Clock: fakeclock.New(start)},
		jitter: func() float64 { return 1 },
	}
	t.Cleanup(w.cleanup)
	return w
}

// cleanup stops every node and checks that nothing of the package is left
// running, and that nothing of a session reached a log.
func (w *world) cleanup() {
	w.t.Helper()
	w.mu.Lock()
	procs := slices.Clone(w.procs)
	w.mu.Unlock()
	for _, p := range procs {
		p.stop()
	}
	wantNoGoroutines(w.t)
	for _, p := range procs {
		if log := p.logs.String(); strings.Contains(log, "session-") || strings.Contains(log, "project-") {
			w.t.Errorf("the log of %s holds session content:\n%s", p.name, log)
		}
	}
}

// note records something that happened, in order.
func (w *world) note(what string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.trace = append(w.trace, what)
}

// noted returns what has been recorded.
func (w *world) noted() []string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return slices.Clone(w.trace)
}

// eventually waits for cond without moving the clock.
func (w *world) eventually(what string, cond func() bool) {
	w.t.Helper()
	deadline := time.Now().Add(watchdog)
	for i := 0; !cond(); i++ {
		if time.Now().After(deadline) {
			w.t.Fatalf("timed out waiting for %s\n%s", what, w.dump())
		}
		pause(i)
	}
}

// settle waits for cond, moving the clock on while it does, so that nodes
// that are waiting to retry get to.
func (w *world) settle(what string, cond func() bool) {
	w.t.Helper()
	deadline := time.Now().Add(watchdog)
	for i := 0; !cond(); i++ {
		if time.Now().After(deadline) {
			w.t.Fatalf("timed out waiting for %s\n%s", what, w.dump())
		}
		pause(i)
		if cond() {
			return
		}
		w.clock.Advance(tick)
	}
}

// pause lets other goroutines run. The first looks only yield, which is
// enough when nothing is contended.
func pause(i int) {
	if i < 20 {
		runtime.Gosched()
		return
	}
	time.Sleep(time.Millisecond)
}

// dump describes the world, for a failure message.
func (w *world) dump() string {
	w.mu.Lock()
	procs := slices.Clone(w.procs)
	trace := slices.Clone(w.trace)
	w.mu.Unlock()
	var b strings.Builder
	for _, p := range procs {
		fmt.Fprintf(&b, "%s: role %v, alive %v, holds %d sessions, has %d\n", p.name, p.role(), p.alive(), len(p.held()), len(p.want()))
	}
	if len(trace) > 40 {
		trace = trace[len(trace)-40:]
	}
	fmt.Fprintf(&b, "trace: %s\n", strings.Join(trace, "; "))
	return b.String()
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

// asked returns every wait asked for so far.
func (c *clock) asked() []time.Duration {
	c.mu.Lock()
	defer c.mu.Unlock()
	return slices.Clone(c.waits)
}

// sink collects a log.
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

// half is one direction of a connection: a queue of bytes.
type half struct {
	mu      sync.Mutex
	change  *sync.Cond
	data    []byte
	ended   bool // the writing end is closed: a read drains the queue and ends
	broken  bool // the reading end is closed: a write fails
	stalled bool // a write waits, as when the reader has stopped reading
	waiting int  // how many writes are waiting
}

func newHalf() *half {
	h := &half{}
	h.change = sync.NewCond(&h.mu)
	return h
}

// conn is one end of a connection. Like a socket, it takes a write without
// waiting for the other end to read, a Close fails what is pending on it, and
// the other end then reads what was written and after that the end.
type conn struct {
	w       *world
	label   string
	in, out *half
	once    sync.Once
	// onClose runs before the first Close closes anything.
	onClose func()

	mu        sync.Mutex
	writes    int
	failAfter int  // writes beyond this many fail; negative means none do
	panicking bool // the next read panics
}

// pipe returns the two ends of a new connection.
func (w *world) pipe(client, server string) (*conn, *conn) {
	w.mu.Lock()
	w.conns++
	id := w.conns
	w.mu.Unlock()
	up, down := newHalf(), newHalf()
	c := &conn{w: w, label: fmt.Sprintf("%s#%d", client, id), in: down, out: up, failAfter: -1}
	s := &conn{w: w, label: fmt.Sprintf("%s#%d", server, id), in: up, out: down, failAfter: -1}
	return c, s
}

func (c *conn) Read(p []byte) (int, error) {
	h := c.in
	h.mu.Lock()
	defer h.mu.Unlock()
	for len(h.data) == 0 && !h.ended && !h.broken && !c.panics() {
		h.change.Wait()
	}
	if c.panics() {
		panic("injected panic in a read")
	}
	if h.broken {
		return 0, io.ErrClosedPipe
	}
	if len(h.data) == 0 {
		return 0, io.EOF
	}
	n := copy(p, h.data)
	h.data = h.data[n:]
	return n, nil
}

func (c *conn) Write(p []byte) (int, error) {
	c.mu.Lock()
	c.writes++
	fail := c.failAfter >= 0 && c.writes > c.failAfter
	c.mu.Unlock()
	if fail {
		return 0, errBroken
	}
	h := c.out
	h.mu.Lock()
	defer h.mu.Unlock()
	for h.stalled && !h.ended && !h.broken {
		h.waiting++
		h.change.Broadcast()
		h.change.Wait()
		h.waiting--
	}
	if h.ended || h.broken {
		return 0, io.ErrClosedPipe
	}
	h.data = append(h.data, p...)
	h.change.Broadcast()
	return len(p), nil
}

func (c *conn) Close() error {
	c.once.Do(func() {
		if c.onClose != nil {
			c.onClose()
		}
		c.w.note("closed " + c.label)
		c.in.mu.Lock()
		c.in.broken = true
		c.in.change.Broadcast()
		c.in.mu.Unlock()
		c.out.mu.Lock()
		c.out.ended = true
		c.out.change.Broadcast()
		c.out.mu.Unlock()
	})
	return nil
}

func (c *conn) panics() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.panicking
}

// panicOnRead makes the read that is pending on c, or the next one, panic.
func (c *conn) panicOnRead() {
	c.mu.Lock()
	c.panicking = true
	c.mu.Unlock()
	c.in.mu.Lock()
	c.in.change.Broadcast()
	c.in.mu.Unlock()
}

// failWritesAfter makes every write but the first n fail.
func (c *conn) failWritesAfter(n int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.failAfter = n
}

// stall makes writes on c wait, and release lets them through again.
func (c *conn) stall() { c.setStalled(true) }

func (c *conn) release() { c.setStalled(false) }

func (c *conn) setStalled(stalled bool) {
	c.out.mu.Lock()
	defer c.out.mu.Unlock()
	c.out.stalled = stalled
	c.out.change.Broadcast()
}

// blocked reports whether a write on c is waiting.
func (c *conn) blocked() bool {
	c.out.mu.Lock()
	defer c.out.mu.Unlock()
	return c.out.waiting > 0
}

// listener is the control socket.
type listener struct {
	w     *world
	owner *proc

	mu      sync.Mutex
	change  *sync.Cond
	waiting []*conn
	closed  bool
	failure error // returned by the Accept that is pending, or the next one
}

func (l *listener) Accept() (io.ReadWriteCloser, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	for {
		switch {
		case l.owner.knob(func(k *knobs) bool { return k.acceptPanics }):
			panic("injected panic in an accept")
		case l.failure != nil:
			return nil, l.failure
		case l.closed:
			return nil, errClosed
		case len(l.waiting) > 0:
			c := l.waiting[0]
			l.waiting = l.waiting[1:]
			return c, nil
		}
		l.change.Wait()
	}
}

// Close stops listening. A connection nobody accepted is reset.
func (l *listener) Close() error {
	l.mu.Lock()
	already := l.closed
	l.closed = true
	waiting := l.waiting
	l.waiting = nil
	l.change.Broadcast()
	l.mu.Unlock()
	if already {
		return nil
	}
	for _, c := range waiting {
		c.Close()
	}
	l.w.mu.Lock()
	if l.w.listener == l {
		l.w.listener = nil
	}
	l.w.mu.Unlock()
	l.w.note("listener closed by " + l.owner.name)
	return nil
}

// fail makes the Accept that is pending fail, as a listener that has
// broken does.
func (l *listener) fail() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.failure = errBroken
	l.change.Broadcast()
}

// wake makes the Accept that is pending look again.
func (l *listener) wake() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.change.Broadcast()
}

// offer queues a connection for Accept. It reports false if the listener is
// closed.
func (l *listener) offer(c *conn) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed {
		return false
	}
	l.waiting = append(l.waiting, c)
	l.change.Broadcast()
	return true
}

// lock is the host lock, held by a process.
type lock struct {
	w *world
	p *proc

	mu       sync.Mutex
	listener *listener
}

func (k *lock) Listen() (host.Listener, error) {
	if k.p.knob(func(kn *knobs) bool { return kn.listenPanics }) {
		panic("injected panic in a listen")
	}
	if wait := k.p.gate(func(kn *knobs) chan struct{} { return kn.listenGate }); wait != nil {
		<-wait
	}
	if k.p.knob(func(kn *knobs) bool {
		if kn.listenFailures == 0 {
			return false
		}
		kn.listenFailures--
		return true
	}) {
		return nil, errBroken
	}
	k.w.mu.Lock()
	defer k.w.mu.Unlock()
	if k.w.holder != k {
		return nil, errNotHeld
	}
	l := &listener{w: k.w, owner: k.p}
	l.change = sync.NewCond(&l.mu)
	k.mu.Lock()
	k.listener = l
	k.mu.Unlock()
	k.w.listener = l
	k.w.trace = append(k.w.trace, "listening: "+k.p.name)
	return l, nil
}

// Release gives up the lock. Like the real one it closes a listener that is
// still open first.
func (k *lock) Release() error {
	k.mu.Lock()
	l := k.listener
	k.mu.Unlock()
	if l != nil {
		l.Close()
	}
	k.w.mu.Lock()
	if k.w.holder == k {
		k.w.holder = nil
		k.w.trace = append(k.w.trace, "lock released by "+k.p.name)
	}
	k.w.mu.Unlock()
	if k.p.knob(func(kn *knobs) bool { return kn.releaseFails }) {
		return errBroken
	}
	return nil
}

// knobs are what a test can make go wrong for one process.
type knobs struct {
	acquireFails   bool
	acquirePanics  bool
	dialFails      bool
	dialRefused    bool // the dial reports an unsafe control socket
	dialStalls     bool
	listenFailures int // this many calls to Listen fail
	listenPanics   bool
	listenGate     chan struct{} // Listen waits until this is closed
	acceptPanics   bool
	releaseFails   bool
	discordPanics  bool // the Discord connection panics as it runs
	factoryPanics  bool // making the Discord connection panics
	renderPanics   bool
	renderGate     chan struct{} // Render waits until this is closed
	// onDial is given each connection the process dials, before the node
	// has it. onAccept is given each connection made to the process's
	// listener, before the node has it.
	onDial   func(*conn)
	onAccept func(*conn)
	// onDiscordStop runs when a Discord connection of the process is told
	// to stop, before it reports that it has.
	onDiscordStop func()
	discordState  protocol.DiscordState
}

// proc is one adapter process: a node and what the world gives it.
type proc struct {
	w        *world
	name     string
	version  string
	session  string
	node     *host.Node
	counters *diag.Counters
	logs     *sink

	cancel context.CancelFunc
	done   chan struct{}

	mu       sync.Mutex
	knobs    knobs
	running  bool
	dead     bool
	discords []*discord
	// rendering is what the renderer was last given. It becomes rendered,
	// and counts in renders, when the result has been handed to the Discord
	// connection, so that a test that sees one sees the other. Both count
	// from when the lock was last taken.
	rendering []domain.Session
	rendered  []domain.Session
	renders   int
	conns     []*conn // every end of a connection the process holds
	// expect is the process's own session, reduced by the test from what it
	// published.
	expect *domain.Registry
}

// spawn makes a process and its node. It does not run it. Each option
// changes the node's configuration.
func (w *world) spawn(name string, options ...func(*host.Config)) *proc {
	w.t.Helper()
	p := &proc{
		w:        w,
		name:     name,
		version:  "1.0.0",
		session:  "session-" + name,
		counters: &diag.Counters{},
		logs:     &sink{},
		done:     make(chan struct{}),
		expect:   domain.NewRegistry(),
	}
	p.knobs.discordState = protocol.DiscordConnected
	cfg := host.Config{
		Version:  p.version,
		Acquire:  p.acquire,
		Dial:     p.dial,
		Discord:  p.newDiscord,
		Render:   p.render,
		Clock:    w.clock,
		Jitter:   func() float64 { return w.jitter() },
		Counters: p.counters,
		Logger:   diag.NewLogger(p.logs, config.LogDebug, w.clock.Now),
	}
	for _, option := range options {
		option(&cfg)
	}
	p.version = cfg.Version
	node, err := host.New(cfg)
	if err != nil {
		w.t.Fatal(err)
	}
	p.node = node
	w.mu.Lock()
	w.procs = append(w.procs, p)
	w.mu.Unlock()
	return p
}

// version is an option that sets the binary version.
func version(v string) func(*host.Config) {
	return func(cfg *host.Config) { cfg.Version = v }
}

// idleClear is an option that sets the idle period.
func idleClear(d time.Duration) func(*host.Config) {
	return func(cfg *host.Config) { cfg.Settings = presence.Settings{IdleClear: d} }
}

// run starts the node's role loop.
func (p *proc) run() *proc {
	var ctx context.Context
	ctx, p.cancel = context.WithCancel(context.Background())
	p.mu.Lock()
	p.running = true
	p.mu.Unlock()
	go func() {
		defer close(p.done)
		p.node.Run(ctx)
	}()
	return p
}

// stop ends the node's role loop and waits for it. It does nothing to a
// process that is not running.
func (p *proc) stop() {
	p.w.t.Helper()
	p.mu.Lock()
	running := p.running
	p.running = false
	p.mu.Unlock()
	if !running {
		return
	}
	p.cancel()
	// Whatever the test held up is let go, so that the node can end.
	p.mu.Lock()
	release(p.knobs.listenGate)
	release(p.knobs.renderGate)
	p.knobs.listenGate, p.knobs.renderGate = nil, nil
	p.mu.Unlock()
	select {
	case <-p.done:
	case <-time.After(watchdog):
		p.w.t.Fatalf("timed out waiting for %s to stop\n%s", p.name, goroutines())
	}
}

// release closes a gate that is still shut.
func release(gate chan struct{}) {
	if gate == nil {
		return
	}
	select {
	case <-gate:
	default:
		close(gate)
	}
}

// kill ends the process as the operating system ends one that is killed:
// its lock is free and its connections are cut, with none of the steps of a
// shutdown. Then the node is stopped, in a world that no longer answers it.
func (p *proc) kill() {
	p.w.t.Helper()
	p.mu.Lock()
	p.dead = true
	conns := slices.Clone(p.conns)
	p.mu.Unlock()

	p.w.mu.Lock()
	var l *listener
	if k := p.w.holder; k != nil && k.p == p {
		p.w.holder = nil
		p.w.trace = append(p.w.trace, "killed, lock free: "+p.name)
		k.mu.Lock()
		l = k.listener
		k.mu.Unlock()
	}
	p.w.mu.Unlock()
	if l != nil {
		l.Close()
	}
	for _, c := range conns {
		c.Close()
	}
	p.stop()
}

// alive reports whether the process is running and has not been killed.
func (p *proc) alive() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.running && !p.dead
}

// killed reports whether the process has been killed.
func (p *proc) killed() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.dead
}

// set changes the process's knobs.
func (p *proc) set(change func(*knobs)) {
	p.mu.Lock()
	defer p.mu.Unlock()
	change(&p.knobs)
}

// knob reads, and may change, the knobs.
func (p *proc) knob(read func(*knobs) bool) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return read(&p.knobs)
}

// gate reads one of the gates.
func (p *proc) gate(read func(*knobs) chan struct{}) chan struct{} {
	p.mu.Lock()
	defer p.mu.Unlock()
	return read(&p.knobs)
}

func (p *proc) acquire() (host.Lock, error) {
	if p.knob(func(k *knobs) bool { return k.acquirePanics }) {
		panic("injected panic in an acquire")
	}
	if p.knob(func(k *knobs) bool { return k.acquireFails }) {
		return nil, errBroken
	}
	p.w.mu.Lock()
	defer p.w.mu.Unlock()
	if !p.alive() {
		return nil, errBroken
	}
	if p.w.holder != nil {
		return nil, fmt.Errorf("acquire: %w", host.ErrLocked)
	}
	k := &lock{w: p.w, p: p}
	p.w.holder = k
	p.w.trace = append(p.w.trace, "lock taken by "+p.name)
	p.mu.Lock()
	p.renders, p.rendered, p.rendering = 0, nil, nil
	p.mu.Unlock()
	return k, nil
}

func (p *proc) dial(ctx context.Context) (io.ReadWriteCloser, error) {
	if p.knob(func(k *knobs) bool { return k.dialStalls }) {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	if p.knob(func(k *knobs) bool { return k.dialRefused }) {
		return nil, fmt.Errorf("dial: %w", host.ErrUnsafe)
	}
	if p.knob(func(k *knobs) bool { return k.dialFails }) || !p.alive() {
		return nil, errBroken
	}
	c, err := p.w.connect(p.name)
	if err != nil {
		return nil, err
	}
	p.mu.Lock()
	p.conns = append(p.conns, c)
	onDial := p.knobs.onDial
	p.mu.Unlock()
	if onDial != nil {
		onDial(c)
	}
	return c, nil
}

// connect makes a connection to whoever listens on the control socket and
// returns the client's end.
func (w *world) connect(client string) (*conn, error) {
	w.mu.Lock()
	l := w.listener
	w.mu.Unlock()
	if l == nil {
		return nil, errNoHost
	}
	c, s := w.pipe(client, l.owner.name)
	l.owner.mu.Lock()
	l.owner.conns = append(l.owner.conns, s)
	onAccept := l.owner.knobs.onAccept
	l.owner.mu.Unlock()
	if onAccept != nil {
		onAccept(s)
	}
	if !l.offer(s) {
		return nil, errNoHost
	}
	return c, nil
}

// lastConn is the connection the process dialled most recently.
func (p *proc) lastConn() *conn {
	p.mu.Lock()
	defer p.mu.Unlock()
	for i := len(p.conns) - 1; i >= 0; i-- {
		if strings.HasPrefix(p.conns[i].label, p.name+"#") {
			return p.conns[i]
		}
	}
	return nil
}

func (p *proc) render(sessions []domain.Session, now time.Time, set presence.Settings) (domain.Activity, bool) {
	if wait := p.gate(func(k *knobs) chan struct{} { return k.renderGate }); wait != nil {
		<-wait
	}
	if p.knob(func(k *knobs) bool { return k.renderPanics }) {
		panic("injected panic in a render")
	}
	p.mu.Lock()
	p.rendering = slices.Clone(sessions)
	p.mu.Unlock()
	return presence.Render(sessions, now, set)
}

// held is what the process's registry holds as host: the sessions it last
// rendered and showed since it took the lock, in wire form.
func (p *proc) held() []protocol.SessionState {
	p.mu.Lock()
	defer p.mu.Unlock()
	return wire(p.rendered)
}

// rendersSinceLock is how many times the process has rendered since it took
// the lock.
func (p *proc) rendersSinceLock() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.renders
}

// wire puts sessions in the form the control channel carries, which compares
// with ==, ordered by id.
func wire(sessions []domain.Session) []protocol.SessionState {
	out := make([]protocol.SessionState, 0, len(sessions))
	for _, s := range sessions {
		out = append(out, protocol.SessionFromDomain(s))
	}
	slices.SortFunc(out, func(a, b protocol.SessionState) int { return strings.Compare(a.ID, b.ID) })
	return out
}

// want is the process's own session, as the test reduced it.
func (p *proc) want() []protocol.SessionState {
	p.mu.Lock()
	defer p.mu.Unlock()
	return wire(p.expect.Snapshot())
}

// union is the sessions of several processes together.
func union(procs ...*proc) []protocol.SessionState {
	var out []protocol.SessionState
	for _, p := range procs {
		out = append(out, p.want()...)
	}
	slices.SortFunc(out, func(a, b protocol.SessionState) int { return strings.Compare(a.ID, b.ID) })
	return out
}

// holds reports whether the process, as host, holds exactly the sessions of
// the given processes.
func (p *proc) holds(procs ...*proc) bool {
	return slices.Equal(p.held(), union(procs...))
}

// event is an event of the process's session, at the present.
func (p *proc) event(kind domain.Kind) domain.Event {
	e := domain.Event{SessionID: p.session, Surface: domain.SurfaceCode, At: p.w.clock.Now(), Kind: kind}
	switch kind {
	case domain.KindSessionOpened:
		e.Privacy = domain.PrivacyFull
		e.Project = "project-" + p.name
		e.Model = "opus"
	case domain.KindToolStarted:
		e.Tool = domain.ToolEditing
	case domain.KindModelChanged:
		e.Model = "sonnet"
	}
	return e
}

// publish gives the node events of the given kinds, and reduces them for
// the test as well.
func (p *proc) publish(kinds ...domain.Kind) {
	for _, kind := range kinds {
		p.send(p.event(kind))
	}
}

func (p *proc) send(e domain.Event) {
	p.mu.Lock()
	_, _ = p.expect.Apply(e)
	p.mu.Unlock()
	p.node.Publish(e)
}

// open opens the process's session.
func (p *proc) open() *proc {
	p.publish(domain.KindSessionOpened)
	return p
}

func (p *proc) role() host.Role { return p.node.Status().Role }

// isHost and isFollower are conditions to wait for.
func (p *proc) isHost() bool     { return p.role() == host.RoleHost }
func (p *proc) isFollower() bool { return p.role() == host.RoleFollower }

// discord is one Discord connection: it records what it is told to show.
type discord struct {
	p *proc

	mu      sync.Mutex
	shown   []shown
	stopped bool
}

// shown is one thing a Discord connection was told: an activity, or to show
// nothing.
type shown struct {
	activity domain.Activity
	show     bool
}

func (p *proc) newDiscord() host.Discord {
	if p.knob(func(k *knobs) bool { return k.factoryPanics }) {
		panic("injected panic in making the Discord connection")
	}
	d := &discord{p: p}
	p.mu.Lock()
	p.discords = append(p.discords, d)
	p.mu.Unlock()
	return d
}

func (d *discord) Set(a domain.Activity) { d.record(shown{a, true}) }

func (d *discord) Clear() { d.record(shown{}) }

func (d *discord) record(s shown) {
	d.mu.Lock()
	// What is said after the connection has ended goes nowhere, as with the
	// real one.
	if !d.stopped {
		d.shown = append(d.shown, s)
	}
	d.mu.Unlock()
	d.p.mu.Lock()
	d.p.renders++
	d.p.rendered = d.p.rendering
	d.p.mu.Unlock()
}

func (d *discord) Run(ctx context.Context) {
	if d.p.knob(func(k *knobs) bool { return k.discordPanics }) {
		panic("injected panic in the Discord connection")
	}
	<-ctx.Done()
	d.p.mu.Lock()
	onStop := d.p.knobs.onDiscordStop
	d.p.mu.Unlock()
	if onStop != nil {
		onStop()
	}
	d.mu.Lock()
	d.stopped = true
	d.mu.Unlock()
	if !d.p.killed() {
		d.p.w.note("presence cleared by " + d.p.name)
	}
}

func (d *discord) State() protocol.DiscordState {
	d.p.mu.Lock()
	defer d.p.mu.Unlock()
	return d.p.knobs.discordState
}

// all returns everything the connection was told.
func (d *discord) all() []shown {
	d.mu.Lock()
	defer d.mu.Unlock()
	return slices.Clone(d.shown)
}

// last is what the connection was told most recently, and false if it was
// told nothing.
func (d *discord) last() (shown, bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if len(d.shown) == 0 {
		return shown{}, false
	}
	return d.shown[len(d.shown)-1], true
}

// discord is the process's most recent Discord connection, or nil.
func (p *proc) discord() *discord {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.discords) == 0 {
		return nil
	}
	return p.discords[len(p.discords)-1]
}

// shows reports whether the process's Discord connection was last told to
// show an activity whose second line contains text.
func (p *proc) shows(text string) bool {
	d := p.discord()
	if d == nil {
		return false
	}
	s, ok := d.last()
	return ok && s.show && strings.Contains(s.activity.State, text)
}

// peer is the other end of a connection, held by the test: a follower that
// the test plays, or the host's end when the test plays the host.
type peer struct {
	t     *testing.T
	c     *conn
	lines *protocol.Decoder
}

func newPeer(t *testing.T, c *conn) *peer {
	return &peer{t: t, c: c, lines: protocol.NewDecoder(c)}
}

// join connects to the control socket as a follower would. It waits for a
// listener.
func (w *world) join(name string) *peer {
	w.t.Helper()
	var c *conn
	w.eventually("a host to listen", func() bool {
		var err error
		c, err = w.connect(name)
		return err == nil
	})
	return newPeer(w.t, c)
}

// say sends one message.
func (q *peer) say(m protocol.Message) {
	q.t.Helper()
	if err := protocol.Encode(q.c, m); err != nil {
		q.t.Fatalf("sending %T: %v", m, err)
	}
}

// write sends bytes as they are.
func (q *peer) write(raw string) {
	q.t.Helper()
	if _, err := q.c.Write([]byte(raw)); err != nil {
		q.t.Fatalf("writing: %v", err)
	}
}

// next reads one message, or the error that ended the connection.
func (q *peer) next() (m protocol.Message, err error) {
	q.t.Helper()
	within(q.t, "a message from "+q.c.label, func() { m, err = q.lines.Next() })
	return m, err
}

// hear reads one message and fails the test if there is none.
func (q *peer) hear() protocol.Message {
	q.t.Helper()
	m, err := q.next()
	if err != nil {
		q.t.Fatalf("reading from %s: %v", q.c.label, err)
	}
	return m
}

// update reads the next message that is not a request for status. A node
// that follows asks for status after whatever it sends, and whenever a test
// asks it for its role.
func (q *peer) update() protocol.Message {
	q.t.Helper()
	for {
		m := q.hear()
		if _, ok := m.(protocol.Status); !ok {
			return m
		}
	}
}

// hearEnd reads until the connection ends, and fails the test if any
// message comes first.
func (q *peer) hearEnd() {
	q.t.Helper()
	if m, err := q.next(); err == nil {
		q.t.Fatalf("read %#v from %s, want the connection to end", m, q.c.label)
	}
}

// hello says hello and returns the answer.
func (q *peer) hello(version string) protocol.Message {
	q.t.Helper()
	q.say(protocol.Hello{Protocol: protocol.Version, Version: version})
	return q.hear()
}

// welcomed says hello and expects a welcome.
func (q *peer) welcomed(version string) protocol.Welcome {
	q.t.Helper()
	m := q.hello(version)
	welcome, ok := m.(protocol.Welcome)
	if !ok {
		q.t.Fatalf("the hello was answered with %#v, want a welcome", m)
	}
	return welcome
}

// status asks for the host's summary.
func (q *peer) status() protocol.StatusResult {
	q.t.Helper()
	q.say(protocol.Status{})
	m := q.hear()
	result, ok := m.(protocol.StatusResult)
	if !ok {
		q.t.Fatalf("the status request was answered with %#v", m)
	}
	return result
}

// sessionAt is a session a test's follower can sync.
func sessionAt(id string, at time.Time) *protocol.SessionState {
	s := protocol.SessionFromDomain(domain.Session{
		ID: id, Surface: domain.SurfaceCode, Status: domain.StatusIdle,
		Privacy: domain.PrivacyStandard, Start: at, LastActivity: at,
	})
	return &s
}

// eventOf is an event a test's follower can send.
func eventOf(id string, kind domain.Kind, at time.Time) protocol.Event {
	return protocol.Event{Event: protocol.EventFromDomain(domain.Event{
		SessionID: id, Surface: domain.SurfaceCode, At: at, Kind: kind,
	})}
}

// ids are the ids of sessions.
func ids(sessions []protocol.SessionState) []string {
	out := make([]string, 0, len(sessions))
	for _, s := range sessions {
		out = append(out, s.ID)
	}
	return out
}

// stub is a host that the test plays: it holds the lock and, if asked,
// listens.
type stub struct {
	w    *world
	p    *proc
	lock host.Lock
	l    host.Listener
}

// stubHost takes the lock for the test. The process that holds it is not a
// node and is never run.
func (w *world) stubHost() *stub {
	w.t.Helper()
	p := &proc{w: w, name: "stub", logs: &sink{}, running: true, expect: domain.NewRegistry()}
	k, err := p.acquire()
	if err != nil {
		w.t.Fatalf("the test could not take the lock: %v", err)
	}
	s := &stub{w: w, p: p, lock: k}
	w.t.Cleanup(s.leave)
	return s
}

// listen opens the control socket.
func (s *stub) listen() *stub {
	s.w.t.Helper()
	l, err := s.lock.Listen()
	if err != nil {
		s.w.t.Fatalf("the test could not listen: %v", err)
	}
	s.l = l
	return s
}

// accept takes the next connection.
func (s *stub) accept() *peer {
	s.w.t.Helper()
	var c io.ReadWriteCloser
	within(s.w.t, "a connection to the test's host", func() {
		var err error
		if c, err = s.l.Accept(); err != nil {
			s.w.t.Errorf("accept: %v", err)
		}
	})
	if c == nil {
		s.w.t.FailNow()
	}
	return newPeer(s.w.t, c.(*conn))
}

// greeted takes the next connection, reads its hello and welcomes it.
func (s *stub) greeted(version string) (*peer, protocol.Hello) {
	s.w.t.Helper()
	q := s.accept()
	hello, ok := q.hear().(protocol.Hello)
	if !ok {
		s.w.t.Fatal("the first message was not a hello")
	}
	q.say(protocol.Welcome{Protocol: protocol.Version, Version: version})
	return q, hello
}

// leave gives up the lock, and the socket with it.
func (s *stub) leave() {
	_ = s.lock.Release()
}

// hostGoroutines lists the goroutines that are running the package's code.
func hostGoroutines() []string {
	var out []string
	for _, g := range strings.Split(goroutines(), "\n\n") {
		if strings.Contains(g, "internal/host.") {
			out = append(out, g)
		}
	}
	return out
}

func goroutines() string {
	buf := make([]byte, 1<<22)
	return string(buf[:runtime.Stack(buf, true)])
}

// wantNoGoroutines waits for every goroutine of the package to be gone. One
// that has finished its work may still be a moment from exiting, so the check
// polls, with a deadline that only turns a leak into a failure.
func wantNoGoroutines(t *testing.T) {
	t.Helper()
	deadline := time.Now().Add(watchdog)
	for i := 0; len(hostGoroutines()) > 0; i++ {
		if time.Now().After(deadline) {
			t.Fatalf("goroutines of the package are still running:\n%s", strings.Join(hostGoroutines(), "\n\n"))
		}
		pause(i)
	}
}
