package schedule

import "time"

// update is one thing to show: a value, or nothing when show is false.
type update[T comparable] struct {
	value T
	show  bool
}

// limiter holds the rate-limiting rule and nothing else. It has no clock, no
// timer and no lock: the caller passes the time in and acts on the answer.
type limiter[T comparable] struct {
	interval time.Duration

	desired update[T]
	wanted  bool // something has been submitted

	shown update[T]
	known bool // shown is what the consumer last received

	last time.Time
	sent bool // last is the time of an emission

	// before is what an emission replaced, for refused to put back.
	before effect[T]
}

// effect is the part of the limiter that an emission changes.
type effect[T comparable] struct {
	shown update[T]
	known bool
	last  time.Time
	sent  bool
}

func (l *limiter[T]) submit(u update[T]) {
	l.desired, l.wanted = u, true
}

// reset forgets what the consumer last received, but not when.
func (l *limiter[T]) reset() {
	l.known = false
}

// refused takes back the emission that next last returned, for a consumer that
// did not take it: the value is no longer recorded as shown, and the time is no
// longer recorded as the time of an emission, so the interval is as it was.
// A reset made since the emission stays made.
func (l *limiter[T]) refused() {
	l.last, l.sent = l.before.last, l.before.sent
	if l.known {
		l.shown, l.known = l.before.shown, l.before.known
	}
}

// next decides what to do at the time now. If emit is true the caller must
// deliver u, and the limiter has recorded it as shown at now, unless refused
// is called. Otherwise a
// positive wait is how long until an emission is allowed, and a zero wait
// means there is nothing to send.
func (l *limiter[T]) next(now time.Time) (u update[T], emit bool, wait time.Duration) {
	if !l.wanted || (l.known && l.desired == l.shown) {
		return u, false, 0
	}
	if l.sent {
		if elapsed := now.Sub(l.last); elapsed < l.interval {
			return u, false, l.interval - elapsed
		}
	}
	l.before = effect[T]{l.shown, l.known, l.last, l.sent}
	l.shown, l.known = l.desired, true
	l.last, l.sent = now, true
	return l.desired, true, 0
}
