package host_test

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Zafnok/claude-rich-presence/internal/control/protocol"
	"github.com/Zafnok/claude-rich-presence/internal/domain"
	"github.com/Zafnok/claude-rich-presence/internal/host"
)

// The tests here are of a node as host, with followers that are other nodes
// or that the test plays.

// hosting returns a world with one node, a, that is host and holds its own
// session.
func hosting(t *testing.T, options ...func(*host.Config)) (*world, *proc) {
	t.Helper()
	w := newWorld(t)
	a := w.spawn("a", options...).open().run()
	w.eventually("a to become host and hold its session", func() bool { return a.isHost() && a.holds(a) })
	return w, a
}

// follower connects a follower that the test plays, with one session.
func follower(w *world, a *proc, name string) *peer {
	w.t.Helper()
	before := len(a.held())
	q := w.join(name)
	q.welcomed("1.0.0")
	q.say(protocol.Sync{Session: sessionAt("session-"+name, w.clock.Now())})
	w.eventually("the session of "+name+" to be held", func() bool { return len(a.held()) == before+1 })
	return q
}

// mark sends, on q, an event that the host renders, and waits until it has.
// The host takes the messages of a connection in order, so everything q sent
// before has been dealt with by then. It returns how many times the host
// rendered since before was read, the mark's own render not counted.
func mark(w *world, a *proc, q *peer, before int) int {
	w.t.Helper()
	at := w.clock.Now().Add(time.Duration(before+1) * time.Hour)
	q.say(eventOf("session-mark", domain.KindSessionRefreshed, at))
	w.eventually("the mark to be rendered", func() bool {
		for _, s := range a.held() {
			if s.ID == "session-mark" && s.LastActivity == at.UnixMilli() {
				return true
			}
		}
		return false
	})
	return a.rendersSinceLock() - before - 1
}

// closedByHost reports whether the host has closed its end of connection n.
func closedByHost(w *world, n int) func() bool {
	return func() bool { return slices.Contains(w.noted(), fmt.Sprintf("closed a#%d", n)) }
}

// holdsIDs reports whether the host holds exactly the sessions with these
// ids.
func holdsIDs(a *proc, want ...string) func() bool {
	return func() bool { return strings.Join(ids(a.held()), " ") == strings.Join(want, " ") }
}

func TestASecondNodeFollowsAndItsSessionIsHeld(t *testing.T) {
	w, a := hosting(t)
	b := w.spawn("b").open().run()
	w.settle("b to follow", b.isFollower)
	w.eventually("b's session to be in the host's registry", func() bool { return a.holds(a, b) })
	w.eventually("the activity to count both", func() bool { return a.shows("2 sessions") })

	b.publish(domain.KindTurnStarted, domain.KindToolStarted)
	w.eventually("b's events to reach the host", func() bool { return a.holds(a, b) && a.shows("Editing files") })
}

func TestASessionEndsWithItsConnection(t *testing.T) {
	w, a := hosting(t)
	b := w.spawn("b").open().run()
	w.settle("b to follow", func() bool { return b.isFollower() && a.holds(a, b) })
	w.eventually("the activity to count both", func() bool { return a.shows("2 sessions") })
	shown := len(a.discord().all())

	b.stop()
	w.eventually("b's session to disappear", func() bool { return a.holds(a) })
	// Presence was rendered again, for the one session that is left.
	w.eventually("the activity to change", func() bool { return len(a.discord().all()) > shown })
	if last, _ := a.discord().last(); !last.show || strings.Contains(last.activity.State, "sessions") {
		t.Errorf("the activity after the follower left is %+v, want one session", last)
	}
}

func TestEverySessionOfAConnectionEndsWithIt(t *testing.T) {
	w, a := hosting(t)
	q := follower(w, a, "one")
	// A follower may send events for a session it did not sync.
	q.say(eventOf("session-two", domain.KindTurnStarted, w.clock.Now()))
	w.eventually("both sessions to be held", holdsIDs(a, "session-a", "session-one", "session-two"))

	q.c.Close()
	w.eventually("both sessions to disappear", holdsIDs(a, "session-a"))
}

func TestASyncReplacesWhatTheConnectionHolds(t *testing.T) {
	w, a := hosting(t)
	q := follower(w, a, "one")
	q.say(eventOf("session-extra", domain.KindTurnStarted, w.clock.Now()))
	w.eventually("the sessions to be held", holdsIDs(a, "session-a", "session-extra", "session-one"))

	q.say(protocol.Sync{Session: sessionAt("session-two", w.clock.Now())})
	w.eventually("the sync to replace them", holdsIDs(a, "session-a", "session-two"))

	// The same sync again holds the same: it is idempotent.
	q.say(protocol.Sync{Session: sessionAt("session-two", w.clock.Now())})
	mark(w, a, q, a.rendersSinceLock())
	if !holdsIDs(a, "session-a", "session-mark", "session-two")() {
		t.Errorf("after the same sync twice the host holds %v", ids(a.held()))
	}

	q.say(protocol.Sync{})
	w.eventually("a sync with no session to remove everything", holdsIDs(a, "session-a"))
	// With nothing left to remove, a sync with no session changes nothing.
	renders := a.rendersSinceLock()
	q.say(protocol.Sync{})
	if got := mark(w, a, q, renders); got != 0 {
		t.Errorf("rendered %d times for a sync that changed nothing, want none", got)
	}
}

func TestAnEventEndsASession(t *testing.T) {
	w, a := hosting(t)
	q := follower(w, a, "one")
	q.say(eventOf("session-one", domain.KindSessionEnded, w.clock.Now()))
	w.eventually("the session to end", holdsIDs(a, "session-a"))

	// The host's own session goes the same way.
	a.publish(domain.KindSessionEnded)
	w.eventually("the host's own session to end", holdsIDs(a))
	w.eventually("nothing to be shown", func() bool {
		last, ok := a.discord().last()
		return ok && !last.show
	})
}

func TestASessionBelongsToTheConnectionThatNamedItLast(t *testing.T) {
	for name, claim := range map[string]func(*world) protocol.Message{
		"in a sync": func(w *world) protocol.Message {
			return protocol.Sync{Session: sessionAt("session-one", w.clock.Now())}
		},
		"in an event": func(w *world) protocol.Message {
			return eventOf("session-one", domain.KindSessionRefreshed, w.clock.Now())
		},
	} {
		t.Run(name, func(t *testing.T) { sessionMovesToTheLaterConnection(t, claim) })
	}
}

func sessionMovesToTheLaterConnection(t *testing.T, claim func(*world) protocol.Message) {
	w, a := hosting(t)
	first := follower(w, a, "one")
	// The same follower connects again before the host has seen its first
	// connection close.
	second := w.join("one-again")
	second.welcomed("1.0.0")
	second.say(claim(w))
	mark(w, a, second, a.rendersSinceLock())

	first.c.Close()
	w.eventually("the host to see the first connection close", closedByHost(w, 1))
	mark(w, a, second, a.rendersSinceLock())
	if !holdsIDs(a, "session-a", "session-mark", "session-one")() {
		t.Fatalf("the host holds %v after the old connection closed, want the session kept", ids(a.held()))
	}

	second.c.Close()
	w.eventually("the session to end with the connection that had it", holdsIDs(a, "session-a"))
}

func TestAConnectionThatDoesNotBeginWithAValidHelloIsClosed(t *testing.T) {
	for name, first := range map[string]string{
		"another message":        `{"type":"sync","session":null}`,
		"not JSON":               `hello`,
		"no version":             `{"type":"hello","protocol":1}`,
		"a version with a space": `{"type":"hello","protocol":1,"version":"1 0"}`,
		"an unknown type":        `{"type":"from_a_later_version"}`,
		"an oversized line":      `{"type":"hello","protocol":1,"version":"` + strings.Repeat("1", protocol.MaxLineBytes) + `"}`,
	} {
		t.Run(name, func(t *testing.T) {
			w, a := hosting(t)
			b := w.spawn("b").open().run()
			w.settle("b to follow", func() bool { return b.isFollower() && a.holds(a, b) })

			q := w.join("stranger")
			q.write(first + "\n")
			q.hearEnd()

			// Nobody else noticed.
			if !b.isFollower() || !a.holds(a, b) {
				t.Errorf("after the bad connection, b has role %v and the host holds %v", b.role(), ids(a.held()))
			}
			b.publish(domain.KindTurnStarted)
			w.eventually("b's next event to arrive", func() bool { return a.holds(a, b) })
		})
	}
}

func TestAnOversizedLineClosesOnlyItsConnection(t *testing.T) {
	w, a := hosting(t)
	b := w.spawn("b").open().run()
	w.settle("b to follow", func() bool { return b.isFollower() && a.holds(a, b) })
	q := follower(w, a, "long")

	q.write(`{"type":"event","event":{"session_id":"` + strings.Repeat("x", protocol.MaxLineBytes) + `"}}` + "\n")
	q.hearEnd()
	w.eventually("the session of the closed connection to end", func() bool { return a.holds(a, b) })
	if !b.isFollower() {
		t.Errorf("b has role %v, want follower", b.role())
	}
}

func TestADisconnectInTheMiddleOfAMessage(t *testing.T) {
	w, a := hosting(t)
	b := w.spawn("b").open().run()
	w.settle("b to follow", func() bool { return b.isFollower() && a.holds(a, b) })
	q := follower(w, a, "cut")
	dropped := a.counters.Snapshot().EventsDropped

	q.write(`{"type":"event","event":{"session_id":"session-cut","surf`)
	q.c.Close()
	w.eventually("the session of the cut connection to end", func() bool { return a.holds(a, b) })
	// The half of a message is dropped like any line that cannot be read.
	if got := a.counters.Snapshot().EventsDropped; got != dropped+1 {
		t.Errorf("%d lines counted as dropped, want 1", got-dropped)
	}
	if !b.isFollower() {
		t.Errorf("b has role %v, want follower", b.role())
	}
}

func TestWhatCannotBeUsedIsSkippedAndTheConnectionStays(t *testing.T) {
	w, a := hosting(t)
	q := follower(w, a, "one")
	before := a.counters.Snapshot()
	renders := a.rendersSinceLock()
	now := w.clock.Now().UnixMilli()

	for _, line := range []string{
		// Malformed, incomplete and out of range: each is skipped.
		`not a message`,
		`{"type":"event"}`,
		`{"type":"event","event":{"session_id":"session-one","surface":"code","at":-1,"kind":"turn_started"}}`,
		// A word this binary does not know, in an event and in a sync.
		`{"type":"event","event":{"session_id":"session-one","surface":"code","at":1,"kind":"from_a_later_version"}}`,
		`{"type":"sync","session":{"id":"session-one","surface":"code","status":"from_a_later_version","privacy":"standard","start":1,"last_activity":1}}`,
	} {
		q.write(line + "\n")
	}
	// What a follower has no reason to send is ignored without being
	// counted: an unknown type, and the messages of the handshake.
	q.write(`{"type":"from_a_later_version","session":7}` + "\n")
	q.say(protocol.Hello{Protocol: protocol.Version, Version: "1.0.0"})
	q.say(protocol.Welcome{Protocol: protocol.Version, Version: "1.0.0"})
	// A blank line is not a line.
	q.write("\n")

	// The connection is still open and still answers, and nothing changed.
	if got := mark(w, a, q, renders); got != 0 {
		t.Errorf("rendered %d times for messages that changed nothing", got)
	}
	if got := q.status().Sessions; got != 3 {
		t.Errorf("the host reports %d sessions, want 3", got)
	}
	if got := a.counters.Snapshot().EventsDropped - before.EventsDropped; got != 5 {
		t.Errorf("%d messages counted as dropped, want 5", got)
	}

	q.say(protocol.Event{Event: protocol.EventData{SessionID: "session-one", Surface: "code", At: now, Kind: "turn_started"}})
	w.eventually("the next good event to be applied", func() bool { return a.shows("Thinking") })
	if got := a.counters.Snapshot().EventsReceived - before.EventsReceived; got != 3 {
		t.Errorf("%d events counted as received, want 3: the one with an unknown word, the mark and the good one", got)
	}
}

func TestStatusIsAnsweredFromMemory(t *testing.T) {
	w, a := hosting(t, version("1.4.0"))
	one := follower(w, a, "one")
	told := len(a.discord().all())
	renders := a.rendersSinceLock()
	w.clock.Advance(90 * time.Second)

	// The status command: a connection of its own, and never a sync.
	q := w.join("status")
	q.welcomed("1.0.0")
	want := protocol.StatusResult{Discord: protocol.DiscordConnected, Sessions: 2, Version: "1.4.0", UptimeSeconds: 90}
	if got := q.status(); got != want {
		t.Errorf("status is %+v, want %+v", got, want)
	}

	a.set(func(k *knobs) { k.discordState = protocol.DiscordConnecting })
	if got := q.status().Discord; got != protocol.DiscordConnecting {
		t.Errorf("Discord is reported as %q, want connecting", got)
	}
	// A state that is not one of the protocol's is reported as unknown, and
	// still answered.
	a.set(func(k *knobs) { k.discordState = "stopped" })
	if got := q.status().Discord; got != protocol.DiscordUnknown {
		t.Errorf("Discord is reported as %q, want unknown", got)
	}
	q.c.Close()
	w.eventually("the host to see the connection close", closedByHost(w, 2))

	// Asking changed nothing: nothing was rendered, and the Discord
	// connection was told nothing but the mark that shows it.
	if got := mark(w, a, one, renders); got != 0 {
		t.Errorf("rendered %d times while status was asked", got)
	}
	if got := len(a.discord().all()); got != told+1 {
		t.Errorf("the Discord connection was told %d things while status was asked", got-told-1)
	}

	// The node answers the same from its own Status.
	a.set(func(k *knobs) { k.discordState = protocol.DiscordDisconnected })
	wantOwn := host.Status{Role: host.RoleHost, Discord: protocol.DiscordDisconnected, Sessions: 3, Version: "1.4.0", Uptime: 90 * time.Second}
	if got := a.node.Status(); got != wantOwn {
		t.Errorf("the host's own status is %+v, want %+v", got, wantOwn)
	}
}

func TestARefusedConnectionIsReadOnlyForStandDown(t *testing.T) {
	w, a := hosting(t)
	q := w.join("newer")
	q.say(protocol.Hello{Protocol: protocol.Version + 1, Version: "9.0.0"})
	if got, want := q.hear(), (protocol.Refuse{Reason: protocol.ReasonUnsupportedProtocol}); got != want {
		t.Fatalf("the hello was answered with %#v, want %#v", got, want)
	}

	// Nothing else it sends has any effect, and nothing is answered.
	q.say(protocol.Sync{Session: sessionAt("session-refused", w.clock.Now())})
	q.say(eventOf("session-refused", domain.KindTurnStarted, w.clock.Now()))
	q.say(protocol.Status{})

	// The host does not wait for long: a refused connection that stays
	// silent is closed.
	w.sleeping(1)
	w.clock.Advance(host.GreetTimeout - 1)
	if closedByHost(w, 1)() {
		t.Fatal("the refused connection was closed before the time was up")
	}
	w.clock.Advance(1)
	q.hearEnd()
	if !holdsIDs(a, "session-a")() || !a.isHost() {
		t.Errorf("after the refused connection the host has role %v and holds %v", a.role(), ids(a.held()))
	}
}

func TestAConnectionThatSaysNothingIsClosed(t *testing.T) {
	w, a := hosting(t)
	silent := w.join("silent")
	welcomed := w.join("welcomed")
	welcomed.welcomed("1.0.0")

	w.sleeping(1)
	w.clock.Advance(host.GreetTimeout)
	silent.hearEnd()
	// A welcomed connection has no such limit.
	w.clock.Advance(10 * host.GreetTimeout)
	if got := welcomed.status().Sessions; got != 1 {
		t.Errorf("the host reports %d sessions, want 1", got)
	}
	if !a.isHost() {
		t.Errorf("role is %v, want host", a.role())
	}
}

func TestAStandDownThatIsNotFromANewerBinaryIsIgnored(t *testing.T) {
	for name, theirs := range map[string]string{
		"the same version":             "1.0.0",
		"an older version":             "0.9.0",
		"a version with no known rank": "(devel)",
	} {
		t.Run(name, func(t *testing.T) {
			w, a := hosting(t, version("1.0.0"))
			q := w.join("other")
			q.welcomed(theirs)
			q.say(protocol.StandDown{})
			// It still answers on that connection, so it has read the
			// request.
			if got := q.status().Sessions; got != 1 {
				t.Errorf("the host reports %d sessions, want 1", got)
			}
			// And it is still host after its role loop has come round many
			// times, any one of which would have ended the term had the
			// request been taken.
			for range 16 {
				a.publish(domain.KindSessionRefreshed)
				w.clock.Advance(time.Millisecond)
				w.eventually("the host's own event to be shown", func() bool { return a.holds(a) })
			}
			if !a.isHost() || w.lockFree() {
				t.Errorf("role is %v after a stand_down that is not valid, want host", a.role())
			}
			a.stop()
			if got := a.logs.count("standing down"); got != 0 {
				t.Errorf("the host stood down %d times, want none", got)
			}
		})
	}
}

func TestAnAnswerThatCannotBeSentEndsTheConnection(t *testing.T) {
	t.Run("the welcome", func(t *testing.T) {
		w, a := hosting(t)
		a.set(func(k *knobs) { k.onAccept = func(c *conn) { c.failWritesAfter(0) } })
		q := w.join("unlucky")
		q.say(protocol.Hello{Protocol: protocol.Version, Version: "1.0.0"})
		q.hearEnd()
		if !a.isHost() {
			t.Errorf("role is %v, want host", a.role())
		}
	})
	t.Run("the status", func(t *testing.T) {
		w, a := hosting(t)
		a.set(func(k *knobs) { k.onAccept = func(c *conn) { c.failWritesAfter(1) } })
		q := w.join("unlucky")
		q.welcomed("1.0.0")
		q.say(protocol.Sync{Session: sessionAt("session-unlucky", w.clock.Now())})
		w.eventually("its session to be held", holdsIDs(a, "session-a", "session-unlucky"))
		q.say(protocol.Status{})
		q.hearEnd()
		w.eventually("its session to end", holdsIDs(a, "session-a"))
	})
}

func TestAPanicWhileServingAConnectionCostsOnlyThatConnection(t *testing.T) {
	w, a := hosting(t)
	b := w.spawn("b").open().run()
	w.settle("b to follow", func() bool { return b.isFollower() && a.holds(a, b) })
	var served *conn
	a.set(func(k *knobs) { k.onAccept = func(c *conn) { served = c } })
	q := follower(w, a, "one")

	served.panicOnRead()
	q.hearEnd()
	w.eventually("the session of that connection to end", func() bool { return a.holds(a, b) })
	if got := a.logs.count("recovered from a panic"); got != 1 {
		t.Errorf("the panic was logged %d times, want once", got)
	}
	if !a.isHost() || !b.isFollower() {
		t.Errorf("after the panic a has role %v and b has %v", a.role(), b.role())
	}
}

func TestAStalledFollowerDelaysNobody(t *testing.T) {
	w, a := hosting(t)
	// The host's writes to this follower never finish, and the follower
	// never reads: it asks for status until the host is stuck answering.
	var served *conn
	a.set(func(k *knobs) { k.onAccept = func(c *conn) { served = c } })
	stalled := follower(w, a, "stalled")
	a.set(func(k *knobs) { k.onAccept = nil })
	served.stall()
	stalled.say(protocol.Status{})
	w.eventually("the host's answer to block", served.blocked)

	// Another follower and the host's own session are served as before.
	b := w.spawn("b").open().run()
	w.settle("b to follow", func() bool { return b.isFollower() })
	b.publish(domain.KindTurnStarted)
	a.publish(domain.KindTurnStarted, domain.KindToolStarted)
	w.eventually("both to be up to date", func() bool {
		return len(a.held()) == 3 && a.held()[0] == a.want()[0] && a.held()[1] == b.want()[0]
	})
	other := w.join("other")
	other.welcomed("1.0.0")
	if got := other.status().Sessions; got != 3 {
		t.Errorf("the host reports %d sessions, want 3", got)
	}
}

func TestTheActivityIsClearedWhenTheIdlePeriodEnds(t *testing.T) {
	const period = 15 * time.Minute
	w, a := hosting(t, idleClear(period))
	if last, _ := a.discord().last(); !last.show {
		t.Fatal("an idle session is not shown at first")
	}

	// Activity moves the moment: five minutes in, the session is refreshed.
	w.sleeping(1)
	w.clock.Advance(5 * time.Minute)
	a.publish(domain.KindSessionRefreshed)
	w.eventually("the refresh to reach the host", func() bool { return a.holds(a) })
	w.sleeping(1)
	if got := w.lastWait(); got != period+1 {
		t.Fatalf("the timer is set for %v, want one step past the idle period", got)
	}
	told := len(a.discord().all())

	w.clock.Advance(period)
	if got := len(a.discord().all()); got != told {
		t.Fatalf("the Discord connection was told %d things before the idle period had ended", got-told)
	}
	// One step later the period has ended, and no event is needed.
	w.clock.Advance(1)
	w.eventually("the activity to be cleared", func() bool {
		last, _ := a.discord().last()
		return !last.show
	})
	if got := w.clock.Timers(); got != 0 {
		t.Errorf("%d timers are armed after the activity was cleared, want none", got)
	}

	// The next event restores it. A working session is never cleared.
	a.publish(domain.KindTurnStarted)
	w.eventually("the activity to be shown again", func() bool { return a.shows("Thinking") })
	if got := w.clock.Timers(); got != 0 {
		t.Errorf("%d timers are armed while a session is working, want none", got)
	}
}

func TestAHostWhoseSocketCannotBeOpenedServesItselfAndRetries(t *testing.T) {
	w := newWorld(t)
	a := w.spawn("a").open()
	a.set(func(k *knobs) { k.listenFailures = 2 })
	a.run()

	// It is host all the same, alone.
	w.eventually("a to become host and hold its session", func() bool { return a.isHost() && a.holds(a) })
	for _, want := range []time.Duration{host.RetryBase, 2 * host.RetryBase} {
		w.sleeping(1)
		if got := w.lastWait(); got != want {
			t.Fatalf("waiting %v to open the socket again, want %v", got, want)
		}
		w.clock.Advance(want)
	}
	if got := a.logs.count("the control socket is not available"); got != 1 {
		t.Errorf("the socket was reported %d times, want once", got)
	}

	b := w.spawn("b").open().run()
	w.settle("b to follow once the socket is open", func() bool { return b.isFollower() && a.holds(a, b) })
}

func TestAListenerThatFailsIsOpenedAgain(t *testing.T) {
	w, a := hosting(t)
	b := w.spawn("b").open().run()
	w.settle("b to follow", func() bool { return b.isFollower() && a.holds(a, b) })

	w.socket().fail()
	w.sleeping(1)
	if got := a.logs.count("the control socket is not available"); got != 1 {
		t.Errorf("the failure was reported %d times, want once", got)
	}
	// The follower that was connected is still served meanwhile.
	b.publish(domain.KindTurnStarted)
	w.eventually("b's event to arrive", func() bool { return a.holds(a, b) })

	w.clock.Advance(w.lastWait())
	c := w.spawn("c").open().run()
	w.settle("c to follow on the new listener", func() bool { return c.isFollower() && a.holds(a, b, c) })
}

func TestALockThatIsNotReleasedCleanlyIsReported(t *testing.T) {
	w, a := hosting(t)
	a.set(func(k *knobs) { k.releaseFails = true })
	a.stop()
	if got := a.logs.count("the host lock was not released cleanly"); got != 1 {
		t.Errorf("the failure was reported %d times, want once", got)
	}
	if !w.lockFree() {
		t.Error("the lock is still held")
	}
}
