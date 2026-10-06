package host_test

import (
	"testing"
	"time"

	"github.com/Zafnok/claude-rich-presence/internal/control/protocol"
	"github.com/Zafnok/claude-rich-presence/internal/domain"
	"github.com/Zafnok/claude-rich-presence/internal/host"
)

// The tests here are of a node as follower, with a host that the test plays.

// following returns a world in which the test is host and node b follows
// it. q is the host's end of b's connection, after the sync and the request
// for status that begin it.
func following(t *testing.T, options ...func(*host.Config)) (*world, *stub, *proc, *peer) {
	t.Helper()
	w := newWorld(t)
	s := w.stubHost().listen()
	b := w.spawn("b", options...).open().run()
	q, _ := s.greeted("1.0.0")
	if _, ok := q.hear().(protocol.Sync); !ok {
		t.Fatal("the first message after the welcome was not a sync")
	}
	q.asked()
	return w, s, b, q
}

// flood publishes far more events than the queue holds, and asks for the
// status, and fails the test if any of that waits for anything.
func flood(t *testing.T, p *proc) {
	t.Helper()
	within(t, "Publish and Status to return", func() {
		for range 4 * host.QueueSize {
			p.publish(domain.KindTurnStarted, domain.KindToolStarted, domain.KindTurnFinished)
			p.node.Status()
		}
	})
}

func TestPublishNeverWaits(t *testing.T) {
	// In every role and between roles, with whatever the node is doing at
	// the time held up for good.
	t.Run("before Run", func(t *testing.T) {
		w := newWorld(t)
		flood(t, w.spawn("a"))
	})
	t.Run("hosting, with the registry held up", func(t *testing.T) {
		w := newWorld(t)
		a := w.spawn("a")
		a.set(func(k *knobs) { k.renderGate = make(chan struct{}) })
		a.open().run()
		w.eventually("a to become host", a.isHost)
		flood(t, a)
	})
	t.Run("hosting, while ending with the Discord connection held up", func(t *testing.T) {
		w := newWorld(t)
		a := w.spawn("a").open().run()
		w.eventually("a to become host", a.isHost)
		reached, hold := make(chan struct{}), make(chan struct{})
		a.set(func(k *knobs) {
			k.onDiscordStop = func() {
				close(reached)
				<-hold
			}
		})
		stopped := make(chan struct{})
		go func() {
			defer close(stopped)
			a.stop()
		}()
		<-reached
		flood(t, a)
		close(hold)
		<-stopped
	})
	t.Run("joining, with the dial held up", func(t *testing.T) {
		w := newWorld(t)
		w.stubHost()
		b := w.spawn("b")
		b.set(func(k *knobs) { k.dialStalls = true })
		b.run()
		flood(t, b)
	})
	t.Run("joining, with no answer to the hello", func(t *testing.T) {
		w := newWorld(t)
		s := w.stubHost().listen()
		b := w.spawn("b").run()
		s.accept().hear()
		flood(t, b)
	})
	t.Run("following a host that has stopped reading", func(t *testing.T) {
		w, _, b, _ := following(t)
		c := b.lastConn()
		c.stall()
		b.publish(domain.KindTurnStarted)
		w.eventually("the write to block", c.blocked)
		flood(t, b)
	})
	t.Run("retrying", func(t *testing.T) {
		w := newWorld(t)
		w.stubHost()
		b := w.spawn("b").run()
		w.sleeping(1)
		flood(t, b)
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
		flood(t, a)
	})
}

func TestAFullQueueIsReplacedByASync(t *testing.T) {
	w, _, b, q := following(t)
	c := b.lastConn()

	// The host stops reading. One event is taken and its write waits.
	c.stall()
	b.publish(domain.KindTurnStarted)
	w.eventually("the write to block", c.blocked)
	// The queue fills, and one event more replaces everything in it.
	for range host.QueueSize {
		b.publish(domain.KindToolStarted)
	}
	b.publish(domain.KindAttentionNeeded)
	// What is published while the sync is due is carried by it.
	b.publish(domain.KindSubagentStarted)
	c.release()

	first, ok := q.update().(protocol.Event)
	if !ok || first.Event.Kind != string(domain.KindTurnStarted) {
		t.Fatalf("the host read %#v first, want the event that was being written", first)
	}
	// None of the queued events follows: the next thing is the session as
	// it now is.
	sync, ok := q.update().(protocol.Sync)
	if !ok || sync.Session == nil {
		t.Fatalf("the host read %#v next, want a sync", sync)
	}
	if want := b.want()[0]; *sync.Session != want {
		t.Errorf("the sync carries %+v, want the session as it is now, %+v", *sync.Session, want)
	}
	if sync.Session.Status != string(domain.StatusWaiting) || sync.Session.Subagents != 1 {
		t.Errorf("the sync carries status %q and %d subagents, want what the dropped events led to", sync.Session.Status, sync.Session.Subagents)
	}
	// After it, events are forwarded again.
	b.publish(domain.KindTurnFinished)
	next, ok := q.update().(protocol.Event)
	if !ok || next.Event.Kind != string(domain.KindTurnFinished) {
		t.Errorf("after the sync the host read %#v, want the next event", next)
	}
}

func TestAFollowerThatFellBehindConvergesOnTheHost(t *testing.T) {
	w := newWorld(t)
	a := w.spawn("a").open().run()
	w.eventually("a to become host", a.isHost)
	b := w.spawn("b").open().run()
	w.settle("b to follow", func() bool { return b.isFollower() && a.holds(a, b) })

	c := b.lastConn()
	c.stall()
	kinds := domain.Kinds()
	for i := range 5 * host.QueueSize {
		// Every kind but the one that ends the session.
		b.publish(kinds[i%(len(kinds)-1)])
	}
	b.publish(domain.KindToolStarted)
	c.release()

	w.eventually("the host's view to be the follower's state", func() bool { return a.holds(a, b) })
	if got := b.want()[0].Status; got != string(domain.StatusWorking) {
		t.Fatalf("the follower's session is %q, want working", got)
	}
}

func TestAnEventTheChannelCannotCarryIsDropped(t *testing.T) {
	_, _, b, q := following(t)

	// The domain accepts a time before 1970 and the channel does not.
	e := b.event(domain.KindTurnStarted)
	e.At = time.UnixMilli(-1)
	b.send(e)
	// The connection stays, and the next event arrives.
	b.publish(domain.KindToolStarted)
	next, ok := q.update().(protocol.Event)
	if !ok || next.Event.Kind != string(domain.KindToolStarted) {
		t.Fatalf("the host read %#v, want the event after the one that was dropped", next)
	}
	if got := b.counters.Snapshot().EventsDropped; got != 1 {
		t.Errorf("%d events counted as dropped, want 1", got)
	}
}

func TestAFollowerReportsWhatItsHostLastSaid(t *testing.T) {
	w, _, b, q := following(t)

	// Until the host has answered, only its version is known.
	want := host.Status{Role: host.RoleFollower, Discord: protocol.DiscordUnknown, Version: "1.0.0"}
	if got := b.node.Status(); got != want {
		t.Errorf("status before the host answered is %+v, want %+v", got, want)
	}
	// Asking makes the node ask its host again.
	q.asked()

	q.say(protocol.StatusResult{Discord: protocol.DiscordConnecting, Sessions: 7, Version: "1.0.0", UptimeSeconds: 30, Pause: &protocol.PauseState{}})
	want = host.Status{Role: host.RoleFollower, Discord: protocol.DiscordConnecting, Sessions: 7, Version: "1.0.0", Uptime: 30 * time.Second}
	w.eventually("the answer to be reported", func() bool { return b.node.Status() == want })

	// What the follower does not understand is passed over.
	q.write("this is not a message\n")
	q.write(`{"type":"from_a_later_version"}` + "\n")
	q.say(protocol.Welcome{Protocol: protocol.Version, Version: "9.9.9"})
	q.say(protocol.StatusResult{Discord: protocol.DiscordConnected, Sessions: 8, Version: "1.0.0", UptimeSeconds: 31, Pause: &protocol.PauseState{}})
	want = host.Status{Role: host.RoleFollower, Discord: protocol.DiscordConnected, Sessions: 8, Version: "1.0.0", Uptime: 31 * time.Second}
	w.eventually("the later answer to be reported", func() bool { return b.node.Status() == want })
}

func TestAPanicWhileHearingTheHostCostsTheConnection(t *testing.T) {
	w, s, b, q := following(t)

	b.lastConn().panicOnRead()
	q.hearEnd()
	w.sleeping(1)
	if got := b.logs.count("recovered from a panic"); got != 1 {
		t.Errorf("the panic was logged %d times, want once", got)
	}
	if got := b.role(); got != host.RoleNone {
		t.Errorf("role is %v, want none", got)
	}
	// It joins again like after any lost connection.
	w.clock.Advance(w.lastWait())
	s.greeted("1.0.0")
	w.eventually("b to follow again", b.isFollower)
}

func TestFollowingToRetryingWhenAWriteFails(t *testing.T) {
	w := newWorld(t)
	s := w.stubHost().listen()
	b := w.spawn("b").open()
	// The hello is written, and nothing after it.
	b.set(func(k *knobs) { k.onDial = func(c *conn) { c.failWritesAfter(1) } })
	b.run()

	q, _ := s.greeted("1.0.0")
	q.hearEnd()
	w.sleeping(1)
	if got := b.role(); got != host.RoleNone {
		t.Errorf("role is %v, want none", got)
	}
}

func TestANewerFollowerAsksAnOlderHostToStandDownOnce(t *testing.T) {
	w := newWorld(t)
	s := w.stubHost().listen()
	b := w.spawn("b", version("2.0.0")).open().run()

	q, _ := s.greeted("1.9.9")
	if got := q.hear(); got != (protocol.StandDown{}) {
		t.Fatalf("the first message after the welcome is %#v, want stand_down", got)
	}
	// The host does not stand down. The follower behaves as any other.
	if _, ok := q.hear().(protocol.Sync); !ok {
		t.Fatal("the stand_down was not followed by a sync")
	}
	w.eventually("b to follow", b.isFollower)
	for _, kind := range []domain.Kind{domain.KindTurnStarted, domain.KindToolStarted, domain.KindTurnFinished} {
		b.publish(kind)
		if event, ok := q.update().(protocol.Event); !ok || event.Event.Kind != string(kind) {
			t.Fatalf("the host read %#v, want the event %s", event, kind)
		}
	}
}

func TestANodeHasOneSession(t *testing.T) {
	w, s, b, q := following(t)
	other := domain.Event{SessionID: "session-other", Surface: domain.SurfaceCode, At: w.clock.Now(), Kind: domain.KindTurnStarted}

	// An event for another session replaces the node's session. The host
	// is told with a sync, which replaces what it holds for the connection.
	b.node.Publish(other)
	sync, ok := q.update().(protocol.Sync)
	if !ok || sync.Session == nil || sync.Session.ID != "session-other" || sync.Session.Status != string(domain.StatusWorking) {
		t.Fatalf("the host read %#v, want a sync of the new session", sync)
	}

	// The end of a session that is not the node's is passed on and leaves
	// the node's session alone.
	b.node.Publish(b.event(domain.KindSessionEnded))
	if event, ok := q.update().(protocol.Event); !ok || event.Event.SessionID != b.session {
		t.Fatalf("the host read %#v, want the event passed on", event)
	}
	q.c.Close()
	w.sleeping(1)
	w.clock.Advance(w.lastWait())
	q, _ = s.greeted("1.0.0")
	if sync, ok := q.hear().(protocol.Sync); !ok || sync.Session == nil || sync.Session.ID != "session-other" {
		t.Fatalf("the sync after reconnecting is %#v, want the node's one session", sync)
	}

	// When its session ends the node has none, and says so.
	other.Kind = domain.KindSessionEnded
	b.node.Publish(other)
	if event, ok := q.update().(protocol.Event); !ok || event.Event.Kind != string(domain.KindSessionEnded) {
		t.Fatalf("the host read %#v, want the end of the session", event)
	}
	q.c.Close()
	w.sleeping(1)
	w.clock.Advance(w.lastWait())
	q, _ = s.greeted("1.0.0")
	if sync, ok := q.hear().(protocol.Sync); !ok || sync.Session != nil {
		t.Fatalf("the sync after the session ended is %#v, want one with no session", sync)
	}
}
