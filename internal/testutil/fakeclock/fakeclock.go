package fakeclock

import (
	"sync"
	"time"
)

// Clock is a clock that moves only when a test calls Advance. It is safe for
// use from several goroutines.
//
// Its Now and AfterFunc methods have the shape production packages ask for in
// their own clock interfaces, so one fake serves all of them.
type Clock struct {
	mu      sync.Mutex
	changed *sync.Cond
	now     time.Time
	timers  []*timer
}

type timer struct {
	at time.Time
	f  func()
}

// New returns a clock that reads start until it is advanced.
func New(start time.Time) *Clock {
	c := &Clock{now: start}
	c.changed = sync.NewCond(&c.mu)
	return c
}

// Now returns the current fake time.
func (c *Clock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

// AfterFunc arranges for f to run once the clock has been advanced by at least
// d. The returned function cancels the timer, and reports whether it did so
// before the timer fired, as (*time.Timer).Stop does.
//
// f runs on the goroutine that calls Advance, never inside AfterFunc itself: a
// timer with a zero or negative d fires on the next call to Advance.
func (c *Clock) AfterFunc(d time.Duration, f func()) (stop func() bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	t := &timer{at: c.now.Add(d), f: f}
	c.timers = append(c.timers, t)
	c.changed.Broadcast()
	return func() bool { return c.remove(t) }
}

func (c *Clock) remove(t *timer) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	for i, other := range c.timers {
		if other == t {
			c.timers = append(c.timers[:i], c.timers[i+1:]...)
			c.changed.Broadcast()
			return true
		}
	}
	return false
}

// Advance moves the clock forward by d and runs every timer that falls due, in
// order of deadline and then of creation. Each timer's function sees the clock
// at that timer's deadline, and may itself create or stop timers. Advance
// returns when the functions have returned. It panics if d is negative.
func (c *Clock) Advance(d time.Duration) {
	if d < 0 {
		panic("fakeclock: Advance by a negative duration")
	}
	c.mu.Lock()
	end := c.now.Add(d)
	c.mu.Unlock()
	// The lock is not held while a timer's function runs, so the function may
	// use the clock, and a panic in it reaches the test unchanged.
	for f := c.nextDue(end); f != nil; f = c.nextDue(end) {
		f()
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if end.After(c.now) {
		c.now = end
	}
}

// nextDue removes the earliest timer due at or before end, moves the clock to
// its deadline and returns its function. It returns nil when none is due.
func (c *Clock) nextDue(end time.Time) func() {
	c.mu.Lock()
	defer c.mu.Unlock()
	at := -1
	for i, t := range c.timers {
		if t.at.After(end) {
			continue
		}
		if at < 0 || t.at.Before(c.timers[at].at) {
			at = i
		}
	}
	if at < 0 {
		return nil
	}
	t := c.timers[at]
	c.timers = append(c.timers[:at], c.timers[at+1:]...)
	c.changed.Broadcast()
	if t.at.After(c.now) {
		c.now = t.at
	}
	return t.f
}

// Timers returns how many timers are waiting to fire.
func (c *Clock) Timers() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.timers)
}

// WaitForTimers blocks until exactly n timers are waiting to fire. A test uses
// it instead of sleeping, to learn that another goroutine has armed or
// released its timer.
func (c *Clock) WaitForTimers(n int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for len(c.timers) != n {
		c.changed.Wait()
	}
}
