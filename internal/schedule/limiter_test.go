package schedule

import (
	"testing"
	"time"

	"github.com/Zafnok/claude-rich-presence/internal/testutil/fakeclock"
)

const interval = 15 * time.Second

var start = time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)

func show(v string) update[string] { return update[string]{value: v, show: true} }

func submit(v string) func(*limiter[string]) {
	return func(l *limiter[string]) { l.submit(show(v)) }
}

func clear(l *limiter[string]) { l.submit(update[string]{}) }

func reset(l *limiter[string]) { l.reset() }

func all(ops ...func(*limiter[string])) func(*limiter[string]) {
	return func(l *limiter[string]) {
		for _, op := range ops {
			op(l)
		}
	}
}

// A step advances the clock, applies an operation, and then asks the limiter
// what to do. wantEmit of nil means nothing is emitted.
type step struct {
	advance  time.Duration
	do       func(*limiter[string])
	wantEmit *update[string]
	wantWait time.Duration
	// refuse takes back the emission, as a consumer that did not take the
	// value does. between runs first: something that happens while the
	// consumer is deciding.
	refuse  bool
	between func(*limiter[string])
}

func emits(v string) *update[string] { u := show(v); return &u }

var emitsNothingShown = &update[string]{}

func TestLimiter(t *testing.T) {
	tests := []struct {
		name     string
		interval time.Duration
		steps    []step
	}{
		{
			name:     "nothing submitted",
			interval: interval,
			steps: []step{
				{},
				{advance: interval, do: reset},
			},
		},
		{
			name:     "first submission is immediate",
			interval: interval,
			steps: []step{
				{do: submit("a"), wantEmit: emits("a")},
			},
		},
		{
			name:     "first show-nothing is immediate",
			interval: interval,
			steps: []step{
				{do: clear, wantEmit: emitsNothingShown},
				{do: clear},
			},
		},
		{
			name:     "submission after a quiet period is immediate",
			interval: interval,
			steps: []step{
				{do: submit("a"), wantEmit: emits("a")},
				{advance: interval, do: submit("b"), wantEmit: emits("b")},
				{advance: time.Hour, do: submit("c"), wantEmit: emits("c")},
			},
		},
		{
			name:     "value already shown emits nothing",
			interval: interval,
			steps: []step{
				{do: submit("a"), wantEmit: emits("a")},
				{do: submit("a")},
				{advance: interval, do: submit("a")},
				{advance: interval},
			},
		},
		{
			name:     "within the interval the update waits for the boundary",
			interval: interval,
			steps: []step{
				{do: submit("a"), wantEmit: emits("a")},
				{do: submit("b"), wantWait: interval},
				{advance: 5 * time.Second, wantWait: 10 * time.Second},
				{advance: 10*time.Second - 1, wantWait: 1},
				{advance: 1, wantEmit: emits("b")},
				{},
			},
		},
		{
			name:     "burst emits the first and then the last",
			interval: interval,
			steps: []step{
				{do: submit("a"), wantEmit: emits("a")},
				{advance: time.Second, do: all(submit("b"), submit("c")), wantWait: 14 * time.Second},
				{advance: time.Second, do: all(submit("d"), submit("e")), wantWait: 13 * time.Second},
				{advance: 13 * time.Second, wantEmit: emits("e")},
				{advance: interval},
			},
		},
		{
			name:     "a, b, a emits nothing at the boundary",
			interval: interval,
			steps: []step{
				{do: submit("a"), wantEmit: emits("a")},
				{advance: time.Second, do: submit("b"), wantWait: 14 * time.Second},
				{advance: time.Second, do: submit("a")},
				{advance: 13 * time.Second},
				{advance: time.Second},
			},
		},
		{
			name:     "show-nothing obeys the interval",
			interval: interval,
			steps: []step{
				{do: submit("a"), wantEmit: emits("a")},
				{advance: time.Second, do: clear, wantWait: 14 * time.Second},
				{advance: 14 * time.Second, wantEmit: emitsNothingShown},
				{advance: time.Second, do: submit("a"), wantWait: 14 * time.Second},
				{advance: 14 * time.Second, wantEmit: emits("a")},
			},
		},
		{
			name:     "show-nothing differs from a shown zero value",
			interval: 0,
			steps: []step{
				{do: submit(""), wantEmit: emits("")},
				{do: clear, wantEmit: emitsNothingShown},
				{do: submit(""), wantEmit: emits("")},
			},
		},
		{
			name:     "the interval runs from the last emission, not the last submission",
			interval: interval,
			steps: []step{
				{do: submit("a"), wantEmit: emits("a")},
				{advance: 5 * time.Second, do: submit("b"), wantWait: 10 * time.Second},
				{advance: 10 * time.Second, wantEmit: emits("b")},
				{advance: 5 * time.Second, do: submit("c"), wantWait: 10 * time.Second},
				{advance: 10 * time.Second, wantEmit: emits("c")},
			},
		},
		{
			name:     "reset within the interval re-emits at the boundary",
			interval: interval,
			steps: []step{
				{do: submit("a"), wantEmit: emits("a")},
				{advance: time.Second, do: reset, wantWait: 14 * time.Second},
				{advance: 14*time.Second - 1, wantWait: 1},
				{advance: 1, wantEmit: emits("a")},
				{advance: interval},
			},
		},
		{
			name:     "reset after the interval re-emits at once",
			interval: interval,
			steps: []step{
				{do: submit("a"), wantEmit: emits("a")},
				{advance: interval, do: reset, wantEmit: emits("a")},
			},
		},
		{
			name:     "reset re-emits the current value, not the one last shown",
			interval: interval,
			steps: []step{
				{do: submit("a"), wantEmit: emits("a")},
				{advance: time.Second, do: all(submit("b"), reset, submit("a")), wantWait: 14 * time.Second},
				{advance: 14 * time.Second, wantEmit: emits("a")},
			},
		},
		{
			name:     "reset re-emits show-nothing",
			interval: interval,
			steps: []step{
				{do: clear, wantEmit: emitsNothingShown},
				{advance: interval, do: reset, wantEmit: emitsNothingShown},
			},
		},
		{
			name:     "a refused first emission is as if it had not been made",
			interval: interval,
			steps: []step{
				{do: submit("a"), wantEmit: emits("a"), refuse: true},
				{wantEmit: emits("a")},
				{},
			},
		},
		{
			name:     "a refused emission does not start the interval",
			interval: interval,
			steps: []step{
				{do: submit("a"), wantEmit: emits("a"), refuse: true},
				{advance: time.Second, do: submit("b"), wantEmit: emits("b")},
				{advance: time.Second, do: submit("c"), wantWait: 14 * time.Second},
			},
		},
		{
			name:     "a refused emission leaves the interval of the one before it",
			interval: interval,
			steps: []step{
				{do: submit("a"), wantEmit: emits("a")},
				{advance: interval, do: submit("b"), wantEmit: emits("b"), refuse: true},
				{advance: time.Second, wantEmit: emits("b")},
				{do: submit("c"), wantWait: interval},
			},
		},
		{
			name:     "a refused emission leaves the value before it as the one shown",
			interval: interval,
			steps: []step{
				{do: submit("a"), wantEmit: emits("a")},
				{advance: interval, do: submit("b"), wantEmit: emits("b"), refuse: true},
				{do: submit("a")},
				{advance: interval},
			},
		},
		{
			name:     "a refused emission is not re-sent until something asks",
			interval: interval,
			steps: []step{
				{do: submit("a"), wantEmit: emits("a")},
				{advance: time.Second, do: submit("b"), wantWait: 14 * time.Second},
				{advance: 14 * time.Second, wantEmit: emits("b"), refuse: true},
				{advance: time.Second, do: reset, wantEmit: emits("b")},
			},
		},
		{
			name:     "a reset made while the consumer decided stays made",
			interval: interval,
			steps: []step{
				{do: submit("a"), wantEmit: emits("a")},
				{advance: interval, do: submit("b"), wantEmit: emits("b"), between: reset, refuse: true},
				{wantEmit: emits("b")},
			},
		},
		{
			name:     "zero interval never waits",
			interval: 0,
			steps: []step{
				{do: submit("a"), wantEmit: emits("a")},
				{do: submit("b"), wantEmit: emits("b")},
				{do: submit("b")},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clock := fakeclock.New(start)
			l := &limiter[string]{interval: tt.interval}
			for i, s := range tt.steps {
				clock.Advance(s.advance)
				if s.do != nil {
					s.do(l)
				}
				got, emit, wait := l.next(clock.Now())
				if s.refuse {
					if s.between != nil {
						s.between(l)
					}
					l.refused()
				}
				at := clock.Now().Sub(start)
				switch {
				case s.wantEmit == nil && emit:
					t.Fatalf("step %d at %v: emitted %+v, want nothing", i, at, got)
				case s.wantEmit != nil && !emit:
					t.Fatalf("step %d at %v: emitted nothing, want %+v", i, at, *s.wantEmit)
				case s.wantEmit != nil && got != *s.wantEmit:
					t.Fatalf("step %d at %v: emitted %+v, want %+v", i, at, got, *s.wantEmit)
				}
				if wait != s.wantWait {
					t.Fatalf("step %d at %v: wait = %v, want %v", i, at, wait, s.wantWait)
				}
			}
		})
	}
}
