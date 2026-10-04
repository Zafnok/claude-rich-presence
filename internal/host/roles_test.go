package host_test

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Zafnok/claude-rich-presence/internal/control/protocol"
	"github.com/Zafnok/claude-rich-presence/internal/domain"
	"github.com/Zafnok/claude-rich-presence/internal/host"
)

// Each transition of the role state machine in the package documentation has
// a test here that is named after it.

// sleeping waits until exactly n timers are armed on the clock, which is how
// a test learns that a node has begun a wait.
func (w *world) sleeping(n int) {
	w.t.Helper()
	within(w.t, "timers to be armed", func() { w.clock.WaitForTimers(n) })
}

// lastWait is the most recent wait asked of the clock.
func (w *world) lastWait() time.Duration {
	w.t.Helper()
	waits := w.clock.asked()
	if len(waits) == 0 {
		w.t.Fatal("no wait was asked of the clock")
	}
	return waits[len(waits)-1]
}

// lockFree reports whether nobody holds the lock.
func (w *world) lockFree() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.holder == nil
}

// socket is the listener on the control socket, once there is one.
func (w *world) socket() *listener {
	w.t.Helper()
	var l *listener
	w.eventually("a host to listen", func() bool {
		w.mu.Lock()
		defer w.mu.Unlock()
		l = w.listener
		return l != nil
	})
	return l
}

func TestElectingToHostingWhenTheLockIsTaken(t *testing.T) {
	w := newWorld(t)
	a := w.spawn("a").run()
	w.eventually("a to become host", a.isHost)

	a.open()
	a.publish(domain.KindTurnStarted, domain.KindToolStarted)
	w.eventually("the events to reach the renderer", func() bool { return a.holds(a) })
	w.eventually("the Discord connection to be told", func() bool { return a.shows("Editing files") })
	if got := a.counters.Snapshot().Failovers; got != 0 {
		t.Errorf("%d failovers counted for a node that was never a follower, want 0", got)
	}
}

func TestElectingToJoiningWhenTheLockIsHeld(t *testing.T) {
	w := newWorld(t)
	s := w.stubHost().listen()
	b := w.spawn("b", version("1.2.3")).run()

	q := s.accept()
	want := protocol.Hello{Protocol: protocol.Version, Version: "1.2.3"}
	if got := q.hear(); got != want {
		t.Errorf("the first message is %#v, want %#v", got, want)
	}
	if got := b.role(); got != host.RoleNone {
		t.Errorf("role before the welcome is %v, want none", got)
	}
}

func TestElectingToJoiningWhenTheLockCannotBeTried(t *testing.T) {
	w := newWorld(t)
	s := w.stubHost().listen()
	b := w.spawn("b")
	b.set(func(k *knobs) { k.acquireFails = true })
	b.run()

	// It joins all the same, and is refused, so that it comes round again.
	for range 2 {
		q := s.accept()
		q.hear()
		q.say(protocol.Refuse{Reason: protocol.ReasonOther})
		q.hearEnd()
		w.sleeping(1)
		w.clock.Advance(w.lastWait())
	}
	s.accept()
	if got := b.logs.count("the host lock cannot be tried"); got != 1 {
		t.Errorf("the failing lock was reported %d times, want once", got)
	}
}

func TestJoiningToFollowingOnWelcome(t *testing.T) {
	w := newWorld(t)
	s := w.stubHost().listen()
	b := w.spawn("b").open().run()

	q, _ := s.greeted("1.0.0")
	// The session as a whole comes before anything else.
	sync, ok := q.hear().(protocol.Sync)
	if !ok || sync.Session == nil || *sync.Session != b.want()[0] {
		t.Fatalf("the first message after the welcome is %#v, want a sync of the session", sync)
	}
	w.eventually("b to follow", b.isFollower)

	// Then events are forwarded.
	b.publish(domain.KindTurnStarted)
	event, ok := q.update().(protocol.Event)
	if !ok || event.Event.Kind != string(domain.KindTurnStarted) || event.Event.SessionID != b.session {
		t.Errorf("after the sync came %#v, want the event", event)
	}
}

func TestJoiningToRetryingWhenTheDialFails(t *testing.T) {
	w := newWorld(t)
	w.stubHost() // holds the lock and does not listen
	b := w.spawn("b").open().run()

	// It cannot take the lock and cannot reach a host. It keeps trying, a
	// little less often each time, and Publish returns at once throughout.
	for _, want := range []time.Duration{1, 2, 4, 8, 16, 32, 64, 64, 64} {
		want *= host.RetryBase
		w.sleeping(1)
		if got := w.lastWait(); got != want {
			t.Fatalf("waiting %v before trying again, want %v", got, want)
		}
		within(t, "Publish to return", func() {
			for range 2 * host.QueueSize {
				b.publish(domain.KindTurnStarted)
			}
		})
		if got := b.role(); got != host.RoleNone {
			t.Fatalf("role is %v, want none", got)
		}
		w.clock.Advance(want)
	}
}

func TestJoiningToRetryingWhenTheHostRefuses(t *testing.T) {
	for _, tc := range []struct {
		reason    protocol.Reason
		standDown bool
	}{
		{protocol.ReasonStandingDown, false},
		{protocol.ReasonOther, false},
		// Only an older host cannot speak the protocol. It is asked to
		// stand down on the refused connection.
		{protocol.ReasonUnsupportedProtocol, true},
	} {
		t.Run(string(tc.reason), func(t *testing.T) {
			w := newWorld(t)
			s := w.stubHost().listen()
			b := w.spawn("b").run()

			q := s.accept()
			q.hear()
			q.say(protocol.Refuse{Reason: tc.reason})
			if tc.standDown {
				if got := q.hear(); got != (protocol.StandDown{}) {
					t.Errorf("after the refusal came %#v, want stand_down", got)
				}
			}
			q.hearEnd()

			w.sleeping(1)
			if got := b.role(); got != host.RoleNone {
				t.Errorf("role is %v, want none", got)
			}
			// A refusal is never final: it tries again.
			w.clock.Advance(w.lastWait())
			s.accept()
		})
	}
}

func TestJoiningToRetryingWhenTheConnectionEndsWithoutAWelcome(t *testing.T) {
	t.Run("closed by the host", func(t *testing.T) {
		w := newWorld(t)
		s := w.stubHost().listen()
		b := w.spawn("b").run()

		q := s.accept()
		q.hear()
		q.c.Close()

		w.sleeping(1)
		if got := b.role(); got != host.RoleNone {
			t.Errorf("role is %v, want none", got)
		}
		w.clock.Advance(w.lastWait())
		s.accept()
	})
	t.Run("the hello cannot be sent", func(t *testing.T) {
		w := newWorld(t)
		s := w.stubHost().listen()
		b := w.spawn("b")
		b.set(func(k *knobs) { k.onDial = func(c *conn) { c.failWritesAfter(0) } })
		b.run()

		s.accept().hearEnd()
		w.sleeping(1)
		if got := b.role(); got != host.RoleNone {
			t.Errorf("role is %v, want none", got)
		}
	})
}

func TestJoiningPassesOverWhatItDoesNotUnderstand(t *testing.T) {
	w := newWorld(t)
	s := w.stubHost().listen()
	b := w.spawn("b").run()

	q := s.accept()
	q.hear()
	q.write("this is not a message\n")
	q.write(`{"type":"from_a_later_version","protocol":"no longer a number"}` + "\n")
	q.say(protocol.Welcome{Protocol: protocol.Version, Version: "1.0.0"})
	w.eventually("b to follow", b.isFollower)
}

func TestFollowingToRetryingWhenTheConnectionIsLost(t *testing.T) {
	w := newWorld(t)
	s := w.stubHost()
	b := w.spawn("b").open().run()

	// One failed round first, so that the delay has grown.
	w.sleeping(1)
	if got := w.lastWait(); got != host.RetryBase {
		t.Fatalf("waiting %v, want %v", got, host.RetryBase)
	}
	s.listen()
	w.clock.Advance(host.RetryBase)
	q, _ := s.greeted("1.0.0")
	q.hear() // the sync
	q.hear() // the request for status that follows it

	q.c.Close()
	w.sleeping(1)
	// The delay starts again once a host has been reached.
	if got := w.lastWait(); got != host.RetryBase {
		t.Errorf("waiting %v after losing the host, want %v", got, host.RetryBase)
	}
	if got := b.role(); got != host.RoleNone {
		t.Errorf("role is %v, want none", got)
	}

	// On reaching a host again, the session comes first, whatever was
	// published meanwhile.
	b.publish(domain.KindTurnStarted, domain.KindToolStarted)
	w.clock.Advance(host.RetryBase)
	q, _ = s.greeted("1.0.0")
	sync, ok := q.hear().(protocol.Sync)
	if !ok || sync.Session == nil || *sync.Session != b.want()[0] {
		t.Errorf("the first message after reconnecting is %#v, want a sync of the session as it is now", sync)
	}
}

func TestRetryingToElectingWhenTheBackoffHasPassed(t *testing.T) {
	w := newWorld(t)
	s := w.stubHost()
	b := w.spawn("b").open().run()
	w.sleeping(1)

	s.leave()
	w.clock.Advance(host.RetryBase - 1)
	if got := b.role(); got != host.RoleNone {
		t.Fatalf("role is %v before the backoff has passed, want none", got)
	}
	w.clock.Advance(1)
	w.eventually("b to become host", b.isHost)
	w.eventually("b to hold its session", func() bool { return b.holds(b) })
}

func TestHostingToYieldingOnStandDown(t *testing.T) {
	w := newWorld(t)
	a := w.spawn("a", version("1.0.0")).open().run()
	w.eventually("a to become host", a.isHost)

	q := w.join("newer")
	q.welcomed("1.1.0")
	q.say(protocol.StandDown{})

	q.hearEnd()
	w.eventually("a to give up the lock", w.lockFree)
	if got := a.role(); got != host.RoleNone {
		t.Errorf("role is %v after standing down, want none", got)
	}
	w.sleeping(1)
	if got := w.lastWait(); got != host.StandDownDelay {
		t.Errorf("waiting %v after standing down, want %v", got, host.StandDownDelay)
	}
	if got := a.logs.count("standing down for a newer version"); got != 1 {
		t.Errorf("standing down was logged %d times, want once", got)
	}
}

func TestHostingToYieldingOnStandDownFromANewerProtocol(t *testing.T) {
	w := newWorld(t)
	a := w.spawn("a").run()
	w.eventually("a to become host", a.isHost)

	q := w.join("newer")
	q.say(protocol.Hello{Protocol: protocol.Version + 1, Version: "9.0.0"})
	if got, want := q.hear(), (protocol.Refuse{Reason: protocol.ReasonUnsupportedProtocol}); got != want {
		t.Fatalf("the hello was answered with %#v, want %#v", got, want)
	}
	q.say(protocol.StandDown{})

	q.hearEnd()
	w.eventually("a to give up the lock", w.lockFree)
}

func TestYieldingToElectingWhenTheDelayHasPassed(t *testing.T) {
	w := newWorld(t)
	a := w.spawn("a").open().run()
	w.eventually("a to become host", a.isHost)
	q := w.join("newer")
	q.welcomed("1.1.0")
	q.say(protocol.StandDown{})
	w.eventually("a to give up the lock", w.lockFree)
	w.sleeping(1)

	w.clock.Advance(host.StandDownDelay - 1)
	if !w.lockFree() {
		t.Fatal("the lock was taken before the delay had passed")
	}
	// Nobody took the lock, so the node that gave it up has it again.
	w.clock.Advance(1)
	w.eventually("a to become host again", a.isHost)
	w.eventually("a to hold its session", func() bool { return a.holds(a) })
}

func TestHostingToRetryingWhenAGoroutineOfTheTermPanics(t *testing.T) {
	// recovers checks that the panic cost the term and no more: the node
	// gave up the lock, and is host again once the fault is gone.
	recovers := func(t *testing.T, w *world, a *proc, mend func(*knobs)) {
		t.Helper()
		w.eventually("a to give up the lock", w.lockFree)
		w.eventually("the panic to be logged", func() bool { return a.logs.count("recovered from a panic") > 0 })
		a.set(mend)
		w.settle("a to become host again", func() bool { return a.isHost() && a.holds(a) })
	}
	t.Run("the registry", func(t *testing.T) {
		w := newWorld(t)
		a := w.spawn("a").run()
		w.eventually("a to become host", a.isHost)
		// A follower is connected when it happens. Its connection is closed
		// with the others, and what it reports as it closes goes nowhere.
		q := w.join("follower")
		q.welcomed("1.0.0")
		q.say(protocol.Sync{Session: sessionAt("session-follower", start)})
		w.eventually("the follower's session to be held", func() bool { return len(a.held()) == 1 })

		a.set(func(k *knobs) { k.renderPanics = true })
		a.open()
		q.hearEnd()
		recovers(t, w, a, func(k *knobs) { k.renderPanics = false })
	})
	t.Run("the Discord connection", func(t *testing.T) {
		w := newWorld(t)
		a := w.spawn("a").open()
		a.set(func(k *knobs) { k.discordPanics = true })
		a.run()
		recovers(t, w, a, func(k *knobs) { k.discordPanics = false })
	})
	t.Run("the listener", func(t *testing.T) {
		w := newWorld(t)
		a := w.spawn("a").open()
		a.set(func(k *knobs) { k.listenPanics = true })
		a.run()
		recovers(t, w, a, func(k *knobs) { k.listenPanics = false })
	})
	t.Run("an accept", func(t *testing.T) {
		w := newWorld(t)
		a := w.spawn("a").open().run()
		w.eventually("a to become host", a.isHost)
		l := w.socket()
		a.set(func(k *knobs) { k.acceptPanics = true })
		l.wake()
		recovers(t, w, a, func(k *knobs) { k.acceptPanics = false })
	})
}

func TestAPanicInARoundIsRecoveredAndTheRoundIsRetried(t *testing.T) {
	w := newWorld(t)
	a := w.spawn("a").open()
	a.set(func(k *knobs) { k.acquirePanics = true })
	a.run()

	w.sleeping(1)
	if got := a.logs.count("recovered from a panic"); got != 1 {
		t.Errorf("the panic was logged %d times, want once", got)
	}
	a.set(func(k *knobs) { k.acquirePanics = false })
	w.clock.Advance(w.lastWait())
	w.eventually("a to become host", a.isHost)
}

func TestAPanicWhileATermBeginsDoesNotKeepTheLock(t *testing.T) {
	w := newWorld(t)
	a := w.spawn("a").open()
	a.set(func(k *knobs) { k.factoryPanics = true })
	a.run()

	w.sleeping(1)
	if !w.lockFree() {
		t.Fatal("the lock is held by a node that is not host")
	}
	if got := a.role(); got != host.RoleNone {
		t.Errorf("role is %v, want none", got)
	}
	// Another node can be host meanwhile.
	b := w.spawn("b").open().run()
	w.eventually("b to become host", b.isHost)
	a.set(func(k *knobs) { k.factoryPanics = false })
	w.settle("a to follow it", func() bool { return a.isFollower() && b.holds(a, b) })
}

func TestEveryStateToStoppedWhenTheContextEnds(t *testing.T) {
	// Every case ends with the node stopped in the state it was left in.
	// The cleanup of the world then checks that no goroutine is left.
	t.Run("before electing", func(t *testing.T) {
		w := newWorld(t)
		a := w.spawn("a")
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		within(t, "Run to return", func() { a.node.Run(ctx) })
		if !w.lockFree() {
			t.Error("the lock was taken by a node whose context had ended")
		}
	})
	t.Run("hosting", func(t *testing.T) {
		w := newWorld(t)
		a := w.spawn("a").open().run()
		w.eventually("a to hold its session", func() bool { return a.isHost() && a.holds(a) })
		a.stop()
		if !w.lockFree() {
			t.Error("the lock is still held")
		}
		if got := a.role(); got != host.RoleNone {
			t.Errorf("role is %v, want none", got)
		}
	})
	t.Run("hosting while the socket cannot be opened", func(t *testing.T) {
		w := newWorld(t)
		a := w.spawn("a")
		a.set(func(k *knobs) { k.listenFailures = 1 << 20 })
		a.run()
		w.sleeping(1)
		a.stop()
		if !w.lockFree() {
			t.Error("the lock is still held")
		}
	})
	t.Run("hosting while the socket is being opened", func(t *testing.T) {
		w := newWorld(t)
		a := w.spawn("a")
		a.set(func(k *knobs) { k.listenGate = make(chan struct{}) })
		a.run()
		w.eventually("a to become host", a.isHost)
		// Stopping lets the Listen through. The listener it returns is
		// closed again at once.
		a.stop()
		if !w.lockFree() {
			t.Error("the lock is still held")
		}
	})
	t.Run("joining, while dialling", func(t *testing.T) {
		w := newWorld(t)
		w.stubHost()
		b := w.spawn("b")
		b.set(func(k *knobs) { k.dialStalls = true })
		b.run()
		b.stop()
	})
	t.Run("joining, waiting for the welcome", func(t *testing.T) {
		w := newWorld(t)
		s := w.stubHost().listen()
		b := w.spawn("b").run()
		q := s.accept()
		q.hear()
		b.stop()
		q.hearEnd()
	})
	t.Run("following", func(t *testing.T) {
		w := newWorld(t)
		s := w.stubHost().listen()
		b := w.spawn("b").open().run()
		q, _ := s.greeted("1.0.0")
		q.hear() // the sync
		q.hear() // the request for status
		b.stop()
		q.hearEnd()
		if got := b.role(); got != host.RoleNone {
			t.Errorf("role is %v, want none", got)
		}
	})
	t.Run("following a host that has stopped reading", func(t *testing.T) {
		w := newWorld(t)
		s := w.stubHost().listen()
		b := w.spawn("b").open()
		var c *conn
		b.set(func(k *knobs) {
			k.onDial = func(dialled *conn) { c = dialled }
		})
		b.run()
		q, _ := s.greeted("1.0.0")
		q.hear()
		q.hear()
		c.stall()
		b.publish(domain.KindTurnStarted)
		w.eventually("the write to block", c.blocked)
		b.stop()
	})
	t.Run("retrying", func(t *testing.T) {
		w := newWorld(t)
		w.stubHost()
		b := w.spawn("b").run()
		w.sleeping(1)
		b.stop()
	})
	t.Run("yielding", func(t *testing.T) {
		w := newWorld(t)
		a := w.spawn("a").run()
		w.eventually("a to become host", a.isHost)
		q := w.join("newer")
		q.welcomed("1.1.0")
		q.say(protocol.StandDown{})
		w.eventually("a to give up the lock", w.lockFree)
		w.sleeping(1)
		a.stop()
		if !w.lockFree() {
			t.Error("the lock was taken again")
		}
	})
}

// failover is the scenario of a host with two followers, ended by end. One
// follower takes over and the other follows it.
func failover(t *testing.T, end func(*proc)) {
	w := newWorld(t)
	a := w.spawn("a").open().run()
	w.eventually("a to become host", a.isHost)
	// The sessions begin a minute apart, so that each start time is its own.
	w.clock.Advance(time.Minute)
	b := w.spawn("b").open().run()
	w.settle("b to follow", b.isFollower)
	w.clock.Advance(time.Minute)
	c := w.spawn("c").open().run()
	w.settle("c to follow", c.isFollower)
	b.publish(domain.KindTurnStarted)
	w.eventually("a to hold all three sessions", func() bool { return a.holds(a, b, c) })
	if !a.shows("3 sessions") {
		t.Fatal("the activity does not count three sessions")
	}
	// What the old host held of the two followers, start times included.
	before := a.held()[1:]
	if before[0].Start == before[1].Start {
		t.Fatal("the two sessions began at the same time, so the test would show nothing")
	}

	end(a)

	hosts := func() int {
		n := 0
		for _, p := range []*proc{b, c} {
			if p.isHost() {
				n++
			}
		}
		return n
	}
	w.settle("one follower to take over and the other to follow it", func() bool {
		if hosts() > 1 {
			t.Fatal("both followers are host")
		}
		return (b.isHost() && c.isFollower() && b.holds(b, c)) || (c.isHost() && b.isFollower() && c.holds(b, c))
	})
	next, other := b, c
	if c.isHost() {
		next, other = c, b
	}
	// The registry was rebuilt from what the two hold, so each session has
	// the start time it had under the old host.
	for i, session := range next.held() {
		if session.Start != before[i].Start {
			t.Errorf("%s starts at %d under the new host, want its original start, %d", session.ID, session.Start, before[i].Start)
		}
	}
	w.eventually("the new host to show both sessions", func() bool { return next.shows("2 sessions") })
	if got := next.counters.Snapshot().Failovers; got != 1 {
		t.Errorf("the new host counted %d failovers, want 1", got)
	}
	if got := other.counters.Snapshot().Failovers; got != 0 {
		t.Errorf("the other follower counted %d failovers, want 0", got)
	}
}

func TestAFollowerTakesOverWhenTheHostStops(t *testing.T) {
	failover(t, (*proc).stop)
}

func TestAFollowerTakesOverWhenTheHostIsKilled(t *testing.T) {
	failover(t, (*proc).kill)
}

func TestANewerFollowerTakesOverFromAnOlderHost(t *testing.T) {
	w := newWorld(t)
	a := w.spawn("a", version("1.0.0")).open().run()
	w.eventually("a to become host", a.isHost)
	b := w.spawn("b", version("1.1.0")).open().run()

	w.settle("the newer node to host and the older to follow", func() bool {
		return b.isHost() && a.isFollower() && b.holds(a, b)
	})
	if got := b.logs.count("asked an older presence host to stand down"); got != 1 {
		t.Errorf("the newer node asked %d times, want once", got)
	}
	if got := a.logs.count("standing down for a newer version"); got != 1 {
		t.Errorf("the older node stood down %d times, want once", got)
	}
	// The older node does not ask in turn.
	if got := a.logs.count("asked an older presence host to stand down"); got != 0 {
		t.Errorf("the older node asked the newer host to stand down %d times", got)
	}
}

func TestShutdownHappensInOrder(t *testing.T) {
	w := newWorld(t)
	a := w.spawn("a").open().run()
	w.eventually("a to become host", a.isHost)
	follower := w.join("follower") // connection 1
	follower.welcomed("1.0.0")
	follower.say(protocol.Sync{Session: sessionAt("session-follower", start)})
	w.eventually("the follower's session to be held", func() bool { return len(a.held()) == 2 })

	// While presence is being cleared the host still holds the lock, still
	// listens and still has its follower, and it takes no new one.
	var during []string
	a.set(func(k *knobs) {
		k.onDiscordStop = func() {
			c, err := w.connect("late") // connection 2
			if err != nil {
				t.Errorf("connecting during shutdown: %v", err)
				return
			}
			_ = protocol.Encode(c, protocol.Hello{Protocol: protocol.Version, Version: "1.0.0"})
			m, err := protocol.NewDecoder(c).Next()
			if want := (protocol.Refuse{Reason: protocol.ReasonStandingDown}); err != nil || m != want {
				t.Errorf("a hello during shutdown was answered with %#v, %v; want %#v", m, err, want)
			}
			during = w.noted()
		}
	})
	a.stop()

	for _, entry := range during {
		for _, early := range []string{"closed a#", "listener closed by a", "lock released by a", "presence cleared by a"} {
			if strings.HasPrefix(entry, early) {
				t.Errorf("%q happened before presence was cleared", entry)
			}
		}
	}
	// What followed, in the order it must have: presence cleared, both
	// connections closed, the listener closed, the lock released.
	var after []string
	for _, entry := range w.noted()[len(during):] {
		if strings.HasSuffix(entry, " a") || strings.HasPrefix(entry, "closed a#") {
			after = append(after, entry)
		}
	}
	want := []string{"presence cleared by a", "closed a#1", "closed a#2", "listener closed by a", "lock released by a"}
	if len(after) == len(want) {
		slices.Sort(after[1:3]) // the two connections are closed in no particular order
	}
	if !slices.Equal(after, want) {
		t.Errorf("shutdown did\n%s\nwant\n%s", strings.Join(after, "\n"), strings.Join(want, "\n"))
	}
	follower.hearEnd()
	if got := a.role(); got != host.RoleNone {
		t.Errorf("role after shutdown is %v, want none", got)
	}
}
