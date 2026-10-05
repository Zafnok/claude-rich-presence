package session_test

import (
	"fmt"
	"math/rand/v2"
	"strings"
	"testing"
	"time"

	"github.com/Zafnok/claude-rich-presence/internal/discord/session"
	"github.com/Zafnok/claude-rich-presence/internal/testutil/fakediscord"
)

// shownThing is what an update asks Discord to show: nothing, when name is
// empty, or the activity of that name.
type shownThing struct{ name string }

// model is what the test expects of the manager, kept as small as the rules
// themselves: an update is written at once if no write was made less than an
// interval ago, otherwise at the end of that interval, and a new connection
// starts with Discord showing nothing known. The test checks the model's
// idea of every write against what the fake server recorded, and then checks
// that no two writes are closer than the interval.
type model struct {
	interval time.Duration

	connected bool
	// desired is what Set and Clear last asked for. given is what the
	// scheduler has been given of it: only what was asked while connected, and
	// everything at the moment of connecting.
	desired, given shownThing
	asked, handed  bool

	shown shownThing
	known bool // shown is what Discord has on this connection
	last  time.Time
	sent  bool // last is the time of a write
}

func (m *model) pending() bool {
	return m.handed && (!m.known || m.given != m.shown)
}

func (m *model) due(now time.Time) bool {
	return !m.sent || now.Sub(m.last) >= m.interval
}

// timerAt is when the scheduler's timer is due, if one is armed.
func (m *model) timerAt(now time.Time) (time.Time, bool) {
	if m.pending() && !m.due(now) {
		return m.last.Add(m.interval), true
	}
	return time.Time{}, false
}

func (m *model) ask(t shownThing) {
	m.desired, m.asked = t, true
	if m.connected {
		m.given, m.handed = t, true
	}
}

func (m *model) connect() {
	m.connected = true
	m.given, m.handed = m.desired, m.asked
	m.known = false
}

func TestNoTwoWritesAreCloserThanTheInterval(t *testing.T) {
	for seed := uint64(1); seed <= 24; seed++ {
		t.Run(fmt.Sprintf("seed %d", seed), func(t *testing.T) {
			rng := rand.New(rand.NewPCG(seed, seed))
			// The configuration allows 4 seconds and up.
			interval := time.Duration(4+rng.IntN(13)) * time.Second
			h := newHarnessWithInterval(t, interval)
			m := &model{interval: interval}
			h.start()
			h.inBackoff(1, 0)
			disconnects := 1 // the first absence of Discord counts as one

			choose := func() shownThing {
				if rng.IntN(4) == 0 {
					return shownThing{}
				}
				return shownThing{name: string(rune('a' + rng.IntN(3)))}
			}
			ask := func(thing shownThing) {
				if thing.name == "" {
					h.m.Clear()
				} else {
					h.m.Set(activity(thing.name))
				}
				m.ask(thing)
			}

			// Some of what is asked before Discord appears.
			for range rng.IntN(4) {
				ask(choose())
			}
			srv := h.serve(fakediscord.Behavior{})
			h.clock.Advance(time.Second)
			h.ready(1)
			readies := 1
			m.connect()
			writes := 0
			settle := func() {
				t.Helper()
				now := h.clock.Now()
				if m.pending() && m.due(now) {
					writes++
					eventually(t, "the next write", func() bool { return len(written(srv)) >= writes })
					got := written(srv)[writes-1]
					want := m.given
					if (got.Kind == fakediscord.KindSetActivity) != (want.name != "") ||
						(want.name != "" && !strings.Contains(string(got.Activity), "details-"+want.name)) {
						t.Fatalf("write %d at %v is %v %s, want %q", writes, now, got.Kind, got.Activity, want.name)
					}
					m.shown, m.known, m.last, m.sent = want, true, now, true
				}
				_, armed := m.timerAt(now)
				timers := 0
				if armed {
					timers = 1
				}
				h.timers(timers)
			}
			settle()

			for range 40 {
				now := h.clock.Now()
				switch rng.IntN(5) {
				case 0, 1:
					ask(choose())
					settle()
				case 2, 3:
					d := time.Duration(rng.IntN(int(interval/time.Second)+3)) * time.Second
					if at, armed := m.timerAt(now); armed {
						// Land on the moment the timer is due at the latest,
						// so that the write is made at a time the test knows.
						d = min(d, at.Sub(now))
					}
					h.clock.Advance(d)
					settle()
				default:
					if err := srv.Disconnect(); err != nil {
						t.Fatal(err)
					}
					disconnects++
					h.awaitState(session.Disconnected, disconnects)
					m.connected = false
					timers := 1
					if _, armed := m.timerAt(now); armed {
						timers++
					}
					h.timers(timers)
					for range rng.IntN(4) {
						ask(choose())
					}
					h.clock.Advance(time.Second)
					readies++
					h.ready(readies)
					m.connect()
					settle()
				}
			}

			// What is still waiting for its interval is written when it ends,
			// and then Discord shows what was last asked for.
			if at, armed := m.timerAt(h.clock.Now()); armed {
				h.clock.Advance(at.Sub(h.clock.Now()))
				settle()
			}
			if m.asked && (!m.known || m.shown != m.desired) {
				t.Errorf("Discord was left showing %q, want %q", m.shown.name, m.desired.name)
			}

			// Every write is at least an interval after the one before it,
			// whatever connection each was made on.
			events := written(srv)
			if len(events) != writes {
				t.Fatalf("%d writes recorded, the model made %d", len(events), writes)
			}
			for i := 1; i < len(events); i++ {
				if gap := events[i].Time.Sub(events[i-1].Time); gap < interval {
					t.Errorf("writes %d and %d are %v apart, want at least %v", i, i+1, gap, interval)
				}
			}
		})
	}
}

// written is the updates the server has recorded, in order.
func written(srv *fakediscord.Server) []fakediscord.Event {
	var out []fakediscord.Event
	for _, ev := range srv.Events() {
		if ev.Kind == fakediscord.KindSetActivity || ev.Kind == fakediscord.KindClearActivity {
			out = append(out, ev)
		}
	}
	return out
}
