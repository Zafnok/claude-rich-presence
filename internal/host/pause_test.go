package host_test

import (
	"testing"
	"time"

	"github.com/Zafnok/claude-rich-presence/internal/control/protocol"
	"github.com/Zafnok/claude-rich-presence/internal/domain"
	"github.com/Zafnok/claude-rich-presence/internal/host"
)

// The tests here are of pausing, and of the preview of the card.

// cleared reports whether the process's Discord connection was last told to
// show nothing.
func (p *proc) cleared() bool {
	d := p.discord()
	if d == nil {
		return false
	}
	s, ok := d.last()
	return ok && !s.show
}

// shownSince is how many times the process's Discord connection was told to
// show an activity, after the first n things it was told.
func (p *proc) shownSince(n int) int {
	count := 0
	for _, s := range p.discord().all()[n:] {
		if s.show {
			count++
		}
	}
	return count
}

// peek asks the host for its card. The answer comes after the host has dealt
// with everything q sent before. What the host said of a change of pause in
// the meantime is passed over.
func (q *peer) peek() protocol.PreviewResult {
	q.t.Helper()
	q.say(protocol.Preview{})
	for {
		switch m := q.hear().(type) {
		case protocol.StatusResult:
		case protocol.PreviewResult:
			return m
		default:
			q.t.Fatalf("the request for the card was answered with %#v", m)
		}
	}
}

// pauseHeld is the pause the host holds, asked on a connection of its own:
// whatever that connection is told was said after it was made.
func pauseHeld(w *world) protocol.PauseState {
	w.t.Helper()
	q := w.join("status")
	q.welcomed("1.0.0")
	defer q.c.Close()
	held := q.status().Pause
	if held == nil {
		w.t.Fatal("the host said nothing of a pause")
	}
	return *held
}

// told reads the summary the host sends, unasked, when its pause changes.
func (q *peer) told() protocol.PauseState {
	q.t.Helper()
	m := q.hear()
	result, ok := m.(protocol.StatusResult)
	if !ok || result.Pause == nil {
		q.t.Fatalf("the host said %#v, want its summary with the pause", m)
	}
	return *result.Pause
}

func TestSupersedes(t *testing.T) {
	const at = 1000
	pause := func(at, until int64) protocol.PauseState {
		return protocol.PauseState{Paused: true, Until: until, At: at}
	}
	resume := func(at int64) protocol.PauseState { return protocol.PauseState{At: at} }
	cases := []struct {
		name      string
		next, old protocol.PauseState
		want      bool
	}{
		{"a first pause", pause(at, 0), protocol.PauseState{}, true},
		{"a later pause", pause(at+1, 5000), pause(at, 9000), true},
		{"an earlier pause", pause(at-1, 9000), pause(at, 5000), false},
		{"a later resume", resume(at + 1), pause(at, 0), true},
		{"an earlier resume", resume(at - 1), pause(at, 5000), false},
		{"a pause and a resume at once: the pause", pause(at, 5000), resume(at), true},
		{"a resume and a pause at once: the pause", resume(at), pause(at, 5000), false},
		{"the same resume", resume(at), resume(at), false},
		{"two pauses at once: the later end", pause(at, 9000), pause(at, 5000), true},
		{"two pauses at once: not the earlier end", pause(at, 5000), pause(at, 9000), false},
		{"the same pause", pause(at, 5000), pause(at, 5000), false},
		{"two pauses at once: until resumed is the latest end", pause(at, 0), pause(at, 9000), true},
		{"two pauses at once: nothing ends later than until resumed", pause(at, 9000), pause(at, 0), false},
		{"the same pause until resumed", pause(at, 0), pause(at, 0), false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := host.Supersedes(c.next, c.old); got != c.want {
				t.Errorf("Supersedes(%+v, %+v) = %v, want %v", c.next, c.old, got, c.want)
			}
		})
	}
}

func TestAPauseWithADurationClearsAtOnceAndEndsByItself(t *testing.T) {
	w, a := hosting(t)
	q := follower(w, a, "one")
	until := w.clock.Now().Add(10 * time.Minute)

	if !a.node.Pause(until) {
		t.Fatal("the host would not pause")
	}
	w.eventually("the activity to be cleared", a.cleared)
	if got := q.told(); !got.Paused || got.Until != until.UnixMilli() {
		t.Errorf("the followers were told %+v, want a pause until %v", got, until)
	}
	st := a.node.Status()
	if !st.Paused || !st.PausedUntil.Equal(until) || st.NoPause {
		t.Errorf("status is %+v, want paused until %v", st, until)
	}
	if st.Sessions != 2 {
		t.Errorf("the host counts %d sessions while paused, want 2", st.Sessions)
	}

	// While paused, sessions go on being reduced and nothing is shown. The
	// answer to peek says the host has dealt with the event, so the clock is
	// not moved under it.
	told := len(a.discord().all())
	q.say(eventOf("session-one", domain.KindTurnStarted, w.clock.Now()))
	if got := q.peek(); got != (protocol.PreviewResult{}) {
		t.Errorf("the card while paused is %+v, want nothing", got)
	}
	w.clock.Advance(10*time.Minute - 1)
	if got := q.peek(); got.Shown {
		t.Errorf("the card just before the pause ends is %+v, want nothing", got)
	}
	if got := a.shownSince(told); got != 0 {
		t.Errorf("an activity was shown %d times during the pause, want none", got)
	}

	// The pause ends by itself, at its time and with no event, and what is
	// shown is the present: the turn that began during the pause.
	w.clock.Advance(1)
	w.eventually("the activity to return", func() bool { return a.shows("Thinking") && a.shows("2 sessions") })
	if st := a.node.Status(); st.Paused || !st.PausedUntil.IsZero() {
		t.Errorf("status after the pause is %+v, want not paused", st)
	}
}

func TestResumeRestoresTheActivityWithoutANewEvent(t *testing.T) {
	w, a := hosting(t)
	a.publish(domain.KindTurnStarted)
	w.eventually("the activity to be shown", func() bool { return a.shows("Thinking") })
	waits := len(w.clock.asked())

	if !a.node.Pause(time.Time{}) {
		t.Fatal("the host would not pause")
	}
	w.eventually("the activity to be cleared", a.cleared)
	if st := a.node.Status(); !st.Paused || !st.PausedUntil.IsZero() {
		t.Errorf("status is %+v, want paused until resumed", st)
	}
	// A pause until resumed waits for nothing.
	w.clock.Advance(1000 * time.Hour)
	if got := len(w.clock.asked()); got != waits {
		t.Errorf("%d timers were set for a pause until resumed, want none", got-waits)
	}
	if !a.cleared() {
		t.Error("the pause ended by itself")
	}

	if !a.node.Resume() {
		t.Fatal("the host would not resume")
	}
	w.eventually("the activity to return", func() bool { return a.shows("Thinking") })
	if st := a.node.Status(); st.Paused {
		t.Errorf("status after the resume is %+v, want not paused", st)
	}
}

func TestAResumeAtTheSameInstantAsThePauseStillResumes(t *testing.T) {
	// The fake clock does not move between the two, and a real one may not
	// either. The user's latest request is the latest all the same.
	w, a := hosting(t)
	a.node.Pause(time.Time{})
	a.node.Resume()
	a.publish(domain.KindTurnStarted)
	w.eventually("the activity to be shown", func() bool { return a.shows("Thinking") })
	if st := a.node.Status(); st.Paused {
		t.Errorf("status is %+v, want not paused", st)
	}
}

func TestAFollowerPausesThePresenceOfEverySession(t *testing.T) {
	w, a := hosting(t)
	b := w.spawn("b").open().run()
	w.settle("b to follow", func() bool { return b.isFollower() && a.holds(a, b) })
	q := follower(w, a, "one")

	if !b.node.Pause(time.Time{}) {
		t.Fatal("the follower would not pause")
	}
	// The follower reports its own request before the host has taken it.
	if st := b.node.Status(); !st.Paused {
		t.Errorf("the follower's status is %+v, want paused", st)
	}
	w.eventually("the activity to be cleared", a.cleared)
	// The host and every other follower learn of it.
	if st := a.node.Status(); !st.Paused {
		t.Errorf("the host's status is %+v, want paused", st)
	}
	if got := q.told(); !got.Paused || got.Until != 0 {
		t.Errorf("the other follower was told %+v, want a pause until resumed", got)
	}

	if !b.node.Resume() {
		t.Fatal("the follower would not resume")
	}
	w.eventually("the activity to return", func() bool { return a.shows("3 sessions") })
	if got := q.told(); got.Paused {
		t.Errorf("the other follower was told %+v, want a resume", got)
	}
	w.eventually("the host to know", func() bool { return !a.node.Status().Paused })
}

func TestTheLatestRequestWinsWhateverOrderItArrivesIn(t *testing.T) {
	w, a := hosting(t)
	one := follower(w, a, "one")
	two := follower(w, a, "two")
	at := w.clock.Now().UnixMilli()
	later := w.clock.Now().Add(20 * time.Minute).UnixMilli()
	sooner := w.clock.Now().Add(10 * time.Minute).UnixMilli()

	// Two followers offer the same pause with different ends, as after a
	// failover. The latest end is kept, whichever comes first.
	one.say(protocol.Pause{Until: sooner, At: at})
	two.say(protocol.Pause{Until: later, At: at})
	one.say(protocol.Pause{Until: sooner, At: at})
	// A resume from before the pause is stale.
	one.say(protocol.Resume{At: at - 1})
	one.peek()
	two.peek()
	if got := pauseHeld(w); got != (protocol.PauseState{Paused: true, Until: later, At: at}) {
		t.Fatalf("the host holds %+v, want the pause that ends last", got)
	}
	if !a.cleared() {
		t.Error("the activity is shown during the pause")
	}

	// A later resume ends it, and a stale pause does not bring it back.
	one.say(protocol.Resume{At: at + 1})
	two.say(protocol.Pause{Until: later, At: at})
	one.peek()
	two.peek()
	if got := pauseHeld(w); got != (protocol.PauseState{At: at + 1}) {
		t.Fatalf("the host holds %+v, want the resume", got)
	}
	w.eventually("the activity to return", func() bool { return a.shows("3 sessions") })
}

func TestAPauseSurvivesAFailoverForTheRemainingTime(t *testing.T) {
	w, a := hosting(t)
	b := w.spawn("b").open().run()
	w.settle("b to follow", func() bool { return b.isFollower() && a.holds(a, b) })
	until := w.clock.Now().Add(10 * time.Minute)

	// The host pauses. The follower does nothing, and is told.
	a.node.Pause(until)
	w.eventually("the activity to be cleared", a.cleared)
	w.eventually("the follower to learn of the pause", func() bool { return b.node.Status().Paused })
	b.publish(domain.KindTurnStarted)
	w.clock.Advance(4 * time.Minute)

	a.kill()
	w.settle("b to take over", b.isHost)
	w.eventually("b to hold its session", func() bool { return b.node.Status().Sessions == 1 })
	// The new host is paused from the start: it never shows the session.
	// The answer to peek says it has dealt with the session, so the clock is
	// not moved under it.
	q := w.join("probe")
	q.welcomed("1.0.0")
	if got := q.peek(); got.Shown {
		t.Errorf("the new host's card is %+v, want nothing", got)
	}
	if !b.cleared() || b.shownSince(0) != 0 {
		t.Errorf("the new host showed an activity %d times during the pause, want none", b.shownSince(0))
	}
	if st := b.node.Status(); !st.Paused || !st.PausedUntil.Equal(until) {
		t.Errorf("the new host's status is %+v, want paused until %v", st, until)
	}

	// For the remaining time, and no longer. Taking over moved the clock on
	// a little, so what remains is read from it.
	if left := until.Sub(w.clock.Now()); left > 6*time.Minute || left < 5*time.Minute {
		t.Fatalf("%v of the pause is left, want a little under six minutes", left)
	}
	w.clock.Advance(until.Sub(w.clock.Now()) - 1)
	if got := q.peek(); got.Shown {
		t.Errorf("just before the pause ends the new host's card is %+v, want nothing", got)
	}
	if got := b.shownSince(0); got != 0 {
		t.Errorf("the new host showed an activity %d times before the pause ended", got)
	}
	w.clock.Advance(1)
	w.eventually("the activity to return", func() bool { return b.shows("Thinking") })
}

func TestAFollowerOffersItsPauseToEachHostBeforeItsSession(t *testing.T) {
	w, s, b, q := following(t)
	until := w.clock.Now().Add(time.Hour)

	if !b.node.Pause(until) {
		t.Fatal("a follower whose host has said nothing yet would not pause")
	}
	sent, ok := q.update().(protocol.Pause)
	if !ok || sent.Until != until.UnixMilli() || sent.At != w.clock.Now().UnixMilli() {
		t.Fatalf("the host read %#v, want the pause", sent)
	}

	// The host is lost. The next one is offered the pause first, so that it
	// shows nothing of the session that follows.
	q.c.Close()
	w.sleeping(1)
	w.clock.Advance(w.lastWait())
	q, _ = s.greeted("1.0.0")
	if got := q.hear(); got != sent {
		t.Fatalf("the first message to the new host is %#v, want %#v", got, sent)
	}
	if _, ok := q.hear().(protocol.Sync); !ok {
		t.Fatal("the pause was not followed by a sync")
	}

	// A resume is offered the same way.
	w.clock.Advance(time.Second)
	b.node.Resume()
	resume := protocol.Resume{At: w.clock.Now().UnixMilli()}
	if got := q.update(); got != resume {
		t.Fatalf("the host read %#v, want %#v", got, resume)
	}
	q.c.Close()
	w.sleeping(1)
	w.clock.Advance(w.lastWait())
	q, _ = s.greeted("1.0.0")
	if got := q.hear(); got != resume {
		t.Fatalf("the first message to the next host is %#v, want %#v", got, resume)
	}
}

func TestAFollowerKeepsTheLatestPauseItHears(t *testing.T) {
	w, _, b, q := following(t)
	at := w.clock.Now().UnixMilli()
	say := func(p protocol.PauseState) {
		q.say(protocol.StatusResult{Discord: protocol.DiscordConnected, Version: "1.0.0", Pause: &p})
	}

	say(protocol.PauseState{Paused: true, At: at})
	w.eventually("the follower to report the pause", func() bool { return b.node.Status().Paused })
	// An older state does not replace it.
	say(protocol.PauseState{At: at - 1})
	say(protocol.PauseState{Paused: true, Until: at + 1, At: at + 1})
	w.eventually("the follower to report the later pause", func() bool {
		return b.node.Status().PausedUntil.Equal(time.UnixMilli(at + 1))
	})
	// A pause whose time has passed has ended.
	w.clock.Advance(time.Millisecond)
	if st := b.node.Status(); st.Paused {
		t.Errorf("status after the end of the pause is %+v, want not paused", st)
	}
}

func TestAnOlderHostCannotPause(t *testing.T) {
	w, s, b, q := following(t)

	// Before the host has said what it is, a pause is taken and sent.
	if !b.node.Pause(time.Time{}) {
		t.Fatal("a follower whose host has said nothing yet would not pause")
	}
	if _, ok := q.update().(protocol.Pause); !ok {
		t.Fatal("the pause was not sent")
	}
	// A host from before pausing says nothing of a pause. It shows presence,
	// so the pause the follower remembers is over, and is forgotten.
	q.say(protocol.StatusResult{Discord: protocol.DiscordConnected, Sessions: 1, Version: "1.0.0"})
	w.eventually("the follower to know its host cannot pause", func() bool { return b.node.Status().NoPause })
	if b.node.Pause(time.Time{}) || b.node.Pause(w.clock.Now().Add(time.Hour)) || b.node.Resume() {
		t.Error("the follower took a request its host would ignore")
	}
	if st := b.node.Status(); st.Paused {
		t.Errorf("status is %+v, want not paused", st)
	}
	// Nothing was sent for it, now or to the next host.
	b.publish(domain.KindTurnStarted)
	if event, ok := q.update().(protocol.Event); !ok || event.Event.Kind != string(domain.KindTurnStarted) {
		t.Fatalf("the host read %#v, want the event and no pause before it", event)
	}
	q.c.Close()
	w.sleeping(1)
	w.clock.Advance(w.lastWait())
	q, _ = s.greeted("1.0.0")
	if _, ok := q.hear().(protocol.Sync); !ok {
		t.Fatal("the first message to the next host was not the sync")
	}
	// The next host has not said what it is yet, so it may be asked.
	if st := b.node.Status(); st.NoPause {
		t.Errorf("status with a new host is %+v, want pausing not ruled out", st)
	}
}

func TestAPauseBeforeThereIsAHostIsKeptForIt(t *testing.T) {
	w := newWorld(t)
	a := w.spawn("a").open()
	if !a.node.Pause(time.Time{}) {
		t.Fatal("a node with no host would not pause")
	}
	if st := a.node.Status(); !st.Paused || st.Role != host.RoleNone {
		t.Errorf("status is %+v, want paused with no role", st)
	}
	a.run()
	w.eventually("a to become host and hold its session", func() bool { return a.isHost() && a.node.Status().Sessions == 1 })
	w.eventually("the host to say it shows nothing", a.cleared)
	if got := a.shownSince(0); got != 0 {
		t.Errorf("the host showed an activity %d times, want none", got)
	}
}

func TestARefusedConnectionCannotPause(t *testing.T) {
	w, a := hosting(t)
	q := w.join("newer")
	q.say(protocol.Hello{Protocol: protocol.Version + 1, Version: "9.0.0"})
	q.hear()
	q.say(protocol.Pause{At: w.clock.Now().UnixMilli()})
	q.say(protocol.Preview{})

	follower(w, a, "one")
	if got := pauseHeld(w); got.Paused {
		t.Errorf("the host holds %+v after a refused connection asked, want no pause", got)
	}
	w.sleeping(1)
	w.clock.Advance(host.GreetTimeout)
	// Nothing was answered.
	q.hearEnd()
}

func TestAFollowerThatStopsReadingDoesNotHoldUpAPause(t *testing.T) {
	w, a := hosting(t)
	var served *conn
	a.set(func(k *knobs) { k.onAccept = func(c *conn) { served = c } })
	follower(w, a, "deaf")
	a.set(func(k *knobs) { k.onAccept = nil })
	served.stall()

	a.node.Pause(time.Time{})
	w.eventually("the activity to be cleared", a.cleared)
	w.eventually("the announcement to wait for the follower", served.blocked)
	// The host carries on, and so does a second change of pause.
	a.node.Resume()
	w.eventually("the activity to return", func() bool { return a.shows("2 sessions") })
	// The term ends with the announcement still waiting.
	a.stop()
}

func TestThePreviewIsWhatDiscordWasTold(t *testing.T) {
	w, a := hosting(t)
	b := w.spawn("b").open().run()
	w.settle("b to follow", func() bool { return b.isFollower() && a.holds(a, b) })
	b.publish(domain.KindTurnStarted, domain.KindToolStarted)
	w.eventually("the activity to be shown", func() bool { return a.shows("Editing files") })

	last, _ := a.discord().last()
	want := host.Preview{Shown: true, Activity: last.activity}
	if got, known := a.node.Preview(); !known || got != want {
		t.Errorf("the host's preview is %+v (known %v), want %+v", got, known, want)
	}
	// A follower's is what its host said, which it asks for again each time.
	w.eventually("the follower's preview to be the same", func() bool {
		got, known := b.node.Preview()
		return known && got == want
	})
	if want.Activity.Details == "" || want.Activity.LargeText == "" || want.Activity.SmallText == "" {
		t.Fatalf("the activity %+v has an empty slot, so the test shows little", want.Activity)
	}

	// The answer to a request comes after what was sent before it.
	q := follower(w, a, "one")
	q.say(eventOf("session-one", domain.KindAttentionNeeded, w.clock.Now()))
	q.say(eventOf("session-a", domain.KindSessionEnded, w.clock.Now()))
	got := q.peek()
	last, _ = a.discord().last()
	if got != protocol.PreviewOf(last.activity, true) || got.State == "" {
		t.Errorf("the card is %+v, want what Discord was last told, %+v", got, last.activity)
	}

	// While paused the preview is of nothing.
	a.node.Pause(time.Time{})
	w.eventually("the host's preview to be of nothing", func() bool {
		got, known := a.node.Preview()
		return known && got == host.Preview{}
	})
	w.eventually("the follower's preview to be of nothing", func() bool {
		got, known := b.node.Preview()
		return known && got == host.Preview{}
	})
}

func TestThePreviewIsNotKnownWithoutAHostThatGivesIt(t *testing.T) {
	t.Run("no host", func(t *testing.T) {
		w := newWorld(t)
		a := w.spawn("a").open()
		if got, known := a.node.Preview(); known || got != (host.Preview{}) {
			t.Errorf("the preview is %+v (known %v), want not known", got, known)
		}
	})
	t.Run("a host that has not answered", func(t *testing.T) {
		w, _, b, q := following(t)
		if _, known := b.node.Preview(); known {
			t.Error("the preview is known before the host has said anything")
		}
		// Asking makes the node ask its host again.
		q.asked()
		q.say(protocol.PreviewResult{Shown: true, Details: "one line"})
		w.eventually("the answer to be reported", func() bool {
			got, known := b.node.Preview()
			return known && got == host.Preview{Shown: true, Activity: domain.Activity{Details: "one line"}}
		})
	})
	t.Run("a host with nothing shown", func(t *testing.T) {
		w := newWorld(t)
		a := w.spawn("a").run()
		w.eventually("a to become host", a.isHost)
		if got, known := a.node.Preview(); !known || got != (host.Preview{}) {
			t.Errorf("the preview is %+v (known %v), want nothing shown", got, known)
		}
	})
}

func TestAPreviewThatCannotBeSentEndsTheConnection(t *testing.T) {
	w, a := hosting(t)
	a.set(func(k *knobs) { k.onAccept = func(c *conn) { c.failWritesAfter(1) } })
	q := w.join("unlucky")
	q.welcomed("1.0.0")
	q.say(protocol.Preview{})
	q.hearEnd()
	if !a.isHost() {
		t.Errorf("role is %v, want host", a.role())
	}
}

func TestPauseAndPreviewNeverWait(t *testing.T) {
	// With the registry held up for good, and so with the host unable to
	// take anything.
	w := newWorld(t)
	a := w.spawn("a")
	a.set(func(k *knobs) { k.renderGate = make(chan struct{}) })
	a.open().run()
	w.eventually("a to become host", a.isHost)
	within(t, "Pause, Resume and Preview to return", func() {
		for range 4 * host.QueueSize {
			a.node.Pause(w.clock.Now().Add(time.Minute))
			a.node.Resume()
			a.node.Preview()
			a.node.Status()
		}
	})
}
