package schedule

import (
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Zafnok/claude-rich-presence/internal/testutil/fakeclock"
)

// emission is one call to the scheduler's emit function, with the fake time at
// which it was made.
type emission struct {
	value string
	show  bool
	at    time.Duration
}

// recorder is the consumer in these tests.
type recorder struct {
	t     *testing.T
	clock *fakeclock.Clock
	calls chan emission
}

func (r *recorder) emit(value string, show bool) {
	r.calls <- emission{value, show, r.clock.Now().Sub(start)}
}

// next waits for the next emission. The deadline only turns a hang into a
// failure; no test depends on real time passing.
func (r *recorder) next() emission {
	r.t.Helper()
	select {
	case e := <-r.calls:
		return e
	case <-time.After(30 * time.Second):
		r.t.Fatal("no emission")
		return emission{}
	}
}

func (r *recorder) want(want emission) {
	r.t.Helper()
	if got := r.next(); got != want {
		r.t.Fatalf("emitted %+v, want %+v", got, want)
	}
}

// wantNoMore is called once the scheduler can no longer emit: after Stop, or
// when it has nothing pending and no timer.
func (r *recorder) wantNoMore() {
	r.t.Helper()
	select {
	case e := <-r.calls:
		r.t.Fatalf("unexpected emission %+v", e)
	default:
	}
}

func newScheduler(t *testing.T) (*Scheduler[string], *fakeclock.Clock, *recorder) {
	t.Helper()
	clock := fakeclock.New(start)
	rec := &recorder{t: t, clock: clock, calls: make(chan emission, 16)}
	s := New(clock, interval, rec.emit)
	t.Cleanup(s.Stop)
	return s, clock, rec
}

// schedulerGoroutines counts goroutines that are running a scheduler's loop.
func schedulerGoroutines() int {
	buf := make([]byte, 1<<20)
	buf = buf[:runtime.Stack(buf, true)]
	n := 0
	for _, g := range strings.Split(string(buf), "\n\n") {
		if strings.Contains(g, "schedule.(*Scheduler[") && strings.Contains(g, ").run(") {
			n++
		}
	}
	return n
}

// wantNoGoroutines waits for every scheduler loop to be gone. A goroutine that
// has closed its done channel may still be a moment from exiting, so the check
// yields until it has, with a deadline that only turns a leak into a failure.
func wantNoGoroutines(t *testing.T) {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for schedulerGoroutines() != 0 {
		if time.Now().After(deadline) {
			t.Fatalf("%d scheduler goroutines still running", schedulerGoroutines())
		}
		runtime.Gosched()
	}
}

func TestDiscordInterval(t *testing.T) {
	if DiscordInterval != 15*time.Second {
		t.Fatalf("DiscordInterval = %v, want the stricter published limit, 15s", DiscordInterval)
	}
}

func TestFirstSubmissionIsEmittedWithNoDelay(t *testing.T) {
	s, clock, rec := newScheduler(t)
	s.Submit("a")
	rec.want(emission{"a", true, 0})

	clock.Advance(time.Hour)
	s.Submit("b")
	rec.want(emission{"b", true, time.Hour})
}

func TestBurstEmitsFirstAndThenLastAtTheBoundary(t *testing.T) {
	s, clock, rec := newScheduler(t)
	s.Submit("a")
	rec.want(emission{"a", true, 0})

	s.Submit("b")
	clock.WaitForTimers(1)
	clock.Advance(interval - 1)
	if clock.Timers() != 1 {
		t.Fatal("the timer fired before the boundary")
	}
	for _, v := range []string{"c", "d", "e"} {
		s.Submit(v)
	}
	clock.Advance(1)
	rec.want(emission{"e", true, interval})

	clock.WaitForTimers(0)
	s.Stop()
	rec.wantNoMore()
}

func TestBurstFromSeveralGoroutines(t *testing.T) {
	s, clock, rec := newScheduler(t)
	s.Submit("first")
	rec.want(emission{"first", true, 0})

	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			for range 100 {
				s.Submit("x")
				s.Submit("y")
				s.Clear()
				s.Reset()
			}
		})
	}
	wg.Wait()
	s.Submit("last")
	clock.WaitForTimers(1)
	clock.Advance(interval)
	rec.want(emission{"last", true, interval})

	s.Stop()
	rec.wantNoMore()
}

func TestSubmittingTheShownValueEmitsNothing(t *testing.T) {
	s, clock, rec := newScheduler(t)
	s.Submit("a")
	rec.want(emission{"a", true, 0})

	clock.Advance(interval)
	s.Submit("a")
	s.Submit("b")
	rec.want(emission{"b", true, interval})
	s.Stop()
	rec.wantNoMore()
}

func TestBackToTheShownValueEmitsNothingAtTheBoundary(t *testing.T) {
	s, clock, rec := newScheduler(t)
	s.Submit("a")
	rec.want(emission{"a", true, 0})

	s.Submit("b")
	clock.WaitForTimers(1)
	s.Submit("a")
	// With nothing left to send the scheduler gives up its timer, so nothing
	// can wake it at the boundary.
	clock.WaitForTimers(0)
	clock.Advance(interval)
	s.Stop()
	rec.wantNoMore()
}

func TestClearIsAnUpdateLikeAnyOther(t *testing.T) {
	s, clock, rec := newScheduler(t)
	s.Submit("a")
	rec.want(emission{"a", true, 0})

	s.Clear()
	clock.WaitForTimers(1)
	clock.Advance(interval)
	rec.want(emission{"", false, interval})
}

func TestResetEmitsTheCurrentValueAgain(t *testing.T) {
	t.Run("within the interval, at the boundary", func(t *testing.T) {
		s, clock, rec := newScheduler(t)
		s.Submit("a")
		rec.want(emission{"a", true, 0})

		clock.Advance(time.Second)
		s.Reset()
		clock.WaitForTimers(1)
		clock.Advance(interval - time.Second - 1)
		if clock.Timers() != 1 {
			t.Fatal("the timer fired before the boundary")
		}
		clock.Advance(1)
		rec.want(emission{"a", true, interval})
	})
	t.Run("after the interval, at once", func(t *testing.T) {
		s, clock, rec := newScheduler(t)
		s.Submit("a")
		rec.want(emission{"a", true, 0})

		clock.Advance(interval)
		s.Reset()
		rec.want(emission{"a", true, interval})
	})
}

func TestStopReleasesTheTimerAndTheGoroutine(t *testing.T) {
	// The loop of an earlier test's scheduler may still be on its way out.
	wantNoGoroutines(t)
	s, clock, rec := newScheduler(t)
	s.Submit("a")
	rec.want(emission{"a", true, 0})
	s.Submit("b")
	clock.WaitForTimers(1)
	if schedulerGoroutines() != 1 {
		t.Fatalf("%d scheduler goroutines before Stop, want 1", schedulerGoroutines())
	}

	s.Stop()
	if clock.Timers() != 0 {
		t.Fatalf("%d timers left after Stop", clock.Timers())
	}
	wantNoGoroutines(t)

	// A stopped scheduler accepts calls and ignores them.
	s.Stop()
	s.Submit("c")
	s.Clear()
	s.Reset()
	clock.Advance(interval)
	if clock.Timers() != 0 {
		t.Fatalf("%d timers armed after Stop", clock.Timers())
	}
	rec.wantNoMore()
}

func TestStopWithNothingPending(t *testing.T) {
	s, _, rec := newScheduler(t)
	s.Stop()
	wantNoGoroutines(t)
	rec.wantNoMore()
}

func TestSlowConsumerDoesNotBlockSubmitters(t *testing.T) {
	clock := fakeclock.New(start)
	entered := make(chan string)
	release := make(chan struct{})
	s := New(clock, interval, func(value string, _ bool) {
		entered <- value
		<-release
	})
	defer s.Stop()

	s.Submit("a")
	if got := <-entered; got != "a" {
		t.Fatalf("emitted %q, want %q", got, "a")
	}
	// The consumer is now stuck inside emit. Each of these returns anyway.
	s.Submit("b")
	s.Submit("c")
	s.Clear()
	s.Reset()
	s.Submit("d")

	clock.Advance(interval)
	release <- struct{}{}
	if got := <-entered; got != "d" {
		t.Fatalf("emitted %q, want only the latest, %q", got, "d")
	}
	release <- struct{}{}
}
