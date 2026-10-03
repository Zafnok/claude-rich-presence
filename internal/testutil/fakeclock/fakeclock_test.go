package fakeclock_test

import (
	"slices"
	"testing"
	"time"

	"github.com/Zafnok/claude-rich-presence/internal/testutil/fakeclock"
)

var start = time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)

func TestNowMovesOnlyOnAdvance(t *testing.T) {
	c := fakeclock.New(start)
	if got := c.Now(); !got.Equal(start) {
		t.Fatalf("Now() = %v, want %v", got, start)
	}
	c.Advance(0)
	c.Advance(3 * time.Second)
	if got, want := c.Now(), start.Add(3*time.Second); !got.Equal(want) {
		t.Fatalf("Now() = %v, want %v", got, want)
	}
}

func TestAdvancePanicsOnNegative(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("Advance(-1) did not panic")
		}
	}()
	fakeclock.New(start).Advance(-1)
}

func TestTimerFiresAtItsDeadline(t *testing.T) {
	c := fakeclock.New(start)
	var firedAt []time.Time
	c.AfterFunc(10*time.Second, func() { firedAt = append(firedAt, c.Now()) })

	c.Advance(10*time.Second - 1)
	if len(firedAt) != 0 || c.Timers() != 1 {
		t.Fatalf("one nanosecond early: fired %d times, %d timers waiting", len(firedAt), c.Timers())
	}
	c.Advance(time.Minute)
	if want := []time.Time{start.Add(10 * time.Second)}; !slices.Equal(firedAt, want) {
		t.Fatalf("fired at %v, want %v", firedAt, want)
	}
	if c.Timers() != 0 {
		t.Fatalf("%d timers waiting after firing", c.Timers())
	}
	if got, want := c.Now(), start.Add(time.Minute+10*time.Second-1); !got.Equal(want) {
		t.Fatalf("Now() = %v, want %v", got, want)
	}
}

func TestTimersFireInDeadlineThenCreationOrder(t *testing.T) {
	c := fakeclock.New(start)
	var order []string
	add := func(name string, d time.Duration) {
		c.AfterFunc(d, func() { order = append(order, name) })
	}
	add("late", 3*time.Second)
	add("early", time.Second)
	add("tie-first", 2*time.Second)
	add("tie-second", 2*time.Second)
	add("never", time.Hour)
	add("overdue", -time.Second)

	c.Advance(3 * time.Second)
	want := []string{"overdue", "early", "tie-first", "tie-second", "late"}
	if !slices.Equal(order, want) {
		t.Fatalf("order = %v, want %v", order, want)
	}
	if got, want := c.Now(), start.Add(3*time.Second); !got.Equal(want) {
		t.Fatalf("Now() = %v, want %v: an overdue timer must not move the clock back", got, want)
	}
}

func TestZeroDelayTimerWaitsForAdvance(t *testing.T) {
	c := fakeclock.New(start)
	fired := false
	c.AfterFunc(0, func() { fired = true })
	if fired {
		t.Fatal("fired inside AfterFunc")
	}
	c.Advance(0)
	if !fired {
		t.Fatal("did not fire on Advance(0)")
	}
}

func TestStop(t *testing.T) {
	c := fakeclock.New(start)
	fired := 0
	keep := c.AfterFunc(time.Second, func() { fired++ })
	stop := c.AfterFunc(time.Second, func() { t.Error("stopped timer fired") })

	if !stop() {
		t.Fatal("stop() = false for a waiting timer")
	}
	if stop() {
		t.Fatal("second stop() = true")
	}
	c.Advance(time.Second)
	if fired != 1 {
		t.Fatalf("the other timer fired %d times, want 1", fired)
	}
	if keep() {
		t.Fatal("stop() = true after the timer fired")
	}
}

func TestTimerFunctionMayUseTheClock(t *testing.T) {
	c := fakeclock.New(start)
	var firedAt []time.Duration
	var tick func()
	tick = func() {
		firedAt = append(firedAt, c.Now().Sub(start))
		if len(firedAt) < 3 {
			c.AfterFunc(time.Second, tick)
		}
	}
	c.AfterFunc(time.Second, tick)
	c.Advance(10 * time.Second)
	if want := []time.Duration{time.Second, 2 * time.Second, 3 * time.Second}; !slices.Equal(firedAt, want) {
		t.Fatalf("fired at %v, want %v", firedAt, want)
	}
}

func TestWaitForTimers(t *testing.T) {
	c := fakeclock.New(start)
	c.WaitForTimers(0)

	stops := make(chan func() bool)
	go func() { stops <- c.AfterFunc(time.Second, func() {}) }()
	c.WaitForTimers(1)

	stop := <-stops
	go stop()
	c.WaitForTimers(0)

	go c.AfterFunc(time.Second, func() {})
	c.WaitForTimers(1)
	go c.Advance(time.Second)
	c.WaitForTimers(0)
}

func TestPanicInTimerFunctionReachesTheCaller(t *testing.T) {
	c := fakeclock.New(start)
	c.AfterFunc(time.Second, func() { panic("boom") })
	func() {
		defer func() {
			if got := recover(); got != "boom" {
				t.Fatalf("recovered %v, want boom", got)
			}
		}()
		c.Advance(time.Second)
	}()
	// The clock is still usable.
	if got, want := c.Now(), start.Add(time.Second); !got.Equal(want) {
		t.Fatalf("Now() = %v, want %v", got, want)
	}
}
