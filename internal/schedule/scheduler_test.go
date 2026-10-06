package schedule

import (
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
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
	// refuse makes emit report that it did not take the update.
	refuse atomic.Bool
	// settle is the scheduler's Settle.
	settle func()
}

// record starts a scheduler on clock whose consumer is the recorder returned.
// fake is the clock the emissions are timed by, which clock wraps or is.
func record(t *testing.T, clock Clock, fake *fakeclock.Clock) (*Scheduler[string], *recorder) {
	t.Helper()
	rec := &recorder{t: t, clock: fake, calls: make(chan emission, 16)}
	s := New(clock, interval, rec.emit)
	rec.settle = s.Settle
	t.Cleanup(s.Stop)
	return s, rec
}

// emit takes every update, unless refuse is set.
func (r *recorder) emit(value string, show bool) bool {
	r.calls <- emission{value, show, r.clock.Now().Sub(start)}
	return !r.refuse.Load()
}

// next waits for the next emission, and for the scheduler to have recorded
// the answer to it. An emission arrives here from inside emit, before emit has
// returned: without the second wait a test would go on while the scheduler was
// still reading refuse, or had yet to record a refusal. The deadline only
// turns a hang into a failure; no test depends on real time passing.
func (r *recorder) next() emission {
	r.t.Helper()
	select {
	case e := <-r.calls:
		r.settle()
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
	s, rec := record(t, clock, clock)
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

// blockedInSettle reports whether a goroutine is parked inside Settle, waiting
// for the delivery under way.
func blockedInSettle() bool {
	buf := make([]byte, 1<<20)
	buf = buf[:runtime.Stack(buf, true)]
	for _, g := range strings.Split(string(buf), "\n\n") {
		header, _, _ := strings.Cut(g, "\n")
		if strings.Contains(header, "sync.Mutex.Lock") && strings.Contains(g, ").Settle(") {
			return true
		}
	}
	return false
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

func TestRefusedUpdateDoesNotCountTowardsTheInterval(t *testing.T) {
	s, clock, rec := newScheduler(t)
	rec.refuse.Store(true)
	s.Submit("a")
	rec.want(emission{"a", true, 0})

	// Nothing is retried by itself, and no timer waits for the interval.
	clock.Advance(time.Second)
	if clock.Timers() != 0 {
		t.Fatalf("%d timers armed after a refused update, want none", clock.Timers())
	}
	rec.wantNoMore()

	// The consumer is ready: the same value is delivered at once, with the
	// clock not moved.
	rec.refuse.Store(false)
	s.Reset()
	rec.want(emission{"a", true, time.Second})

	// That one was taken, so it does count.
	s.Submit("b")
	clock.WaitForTimers(1)
	clock.Advance(interval)
	rec.want(emission{"b", true, time.Second + interval})
}

func TestRefusedUpdateIsDeliveredAgainWhenSubmittedAgain(t *testing.T) {
	s, clock, rec := newScheduler(t)
	s.Submit("a")
	rec.want(emission{"a", true, 0})

	clock.Advance(interval)
	rec.refuse.Store(true)
	s.Submit("b")
	rec.want(emission{"b", true, interval})

	// The one before it is still the one shown, so going back to it needs no
	// delivery, and a new value is not held back by the refused one.
	rec.refuse.Store(false)
	s.Submit("a")
	clock.Advance(time.Second)
	s.Submit("c")
	rec.want(emission{"c", true, interval + time.Second})
}

func TestSettleWaitsForADeliveryThatIsUnderWay(t *testing.T) {
	clock := fakeclock.New(start)
	entered := make(chan struct{})
	release := make(chan struct{})
	s := New(clock, interval, func(string, bool) bool {
		close(entered)
		<-release
		return true
	})
	defer s.Stop()
	// A test that fails must still let the consumer go, or Stop never returns.
	letGo := sync.OnceFunc(func() { close(release) })
	defer letGo()

	// With nothing under way it returns at once.
	s.Settle()

	s.Submit("a")
	<-entered
	settled := make(chan struct{})
	go func() {
		s.Settle()
		close(settled)
	}()
	// Settle must be seen waiting, which a Settle that returns early never is.
	deadline := time.Now().Add(30 * time.Second)
	for !blockedInSettle() {
		select {
		case <-settled:
			t.Fatal("Settle returned while the consumer was still deciding")
		default:
		}
		if time.Now().After(deadline) {
			t.Fatal("Settle neither returned nor waited")
		}
		runtime.Gosched()
	}
	letGo()
	select {
	case <-settled:
	case <-time.After(30 * time.Second):
		t.Fatal("Settle did not return after the delivery finished")
	}
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
	s := New(clock, interval, func(value string, _ bool) bool {
		entered <- value
		<-release
		return true
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

// steppingClock is a fake clock whose reading can be set back, as a wall clock
// can be. Its timers are unaffected.
type steppingClock struct {
	*fakeclock.Clock
	back atomic.Int64
}

func (c *steppingClock) Now() time.Time {
	return c.Clock.Now().Add(-time.Duration(c.back.Load()))
}

func TestClockSteppingBackDelaysButDoesNotStall(t *testing.T) {
	clock := &steppingClock{Clock: fakeclock.New(start)}
	s, rec := record(t, clock, clock.Clock)

	s.Submit("a")
	rec.want(emission{"a", true, 0})
	s.Submit("b")
	clock.WaitForTimers(1)

	// The timer fires on time, but the clock now reads five seconds short of
	// the boundary. The scheduler must wait those five seconds, not forever.
	clock.back.Store(int64(5 * time.Second))
	clock.Advance(interval)
	clock.WaitForTimers(1)
	clock.Advance(5 * time.Second)
	rec.want(emission{"b", true, interval + 5*time.Second})
}

func TestTimerIsKeptThroughABurst(t *testing.T) {
	clock := &countingClock{Clock: fakeclock.New(start)}
	s, rec := record(t, clock, clock.Clock)

	s.Submit("a")
	rec.want(emission{"a", true, 0})
	// The scheduler is let finish with each submission before the next thing
	// happens. One that it was still looking at when the clock moved would
	// find its timer fired with time left to wait, and arm another.
	for _, v := range []string{"b", "c", "d", "e"} {
		looks := clock.looks.Load()
		s.Submit(v)
		deadline := time.Now().Add(30 * time.Second)
		for clock.looks.Load() == looks {
			if time.Now().After(deadline) {
				t.Fatalf("the scheduler did not look at %q", v)
			}
			runtime.Gosched()
		}
		// It reads the clock inside the step that Settle waits for.
		s.Settle()
	}
	if clock.Timers() != 1 {
		t.Fatalf("%d timers armed during the burst, want 1", clock.Timers())
	}
	clock.Advance(interval)
	rec.want(emission{"e", true, interval})
	s.Stop()
	if got := clock.armed.Load(); got != 1 {
		t.Fatalf("armed %d timers for one burst, want 1", got)
	}
}

// countingClock counts the timers armed on it, and the times it is read,
// which the scheduler does once each time it looks at what to do.
type countingClock struct {
	*fakeclock.Clock
	armed atomic.Int64
	looks atomic.Int64
}

func (c *countingClock) Now() time.Time {
	c.looks.Add(1)
	return c.Clock.Now()
}

func (c *countingClock) AfterFunc(d time.Duration, f func()) func() bool {
	c.armed.Add(1)
	return c.Clock.AfterFunc(d, f)
}
