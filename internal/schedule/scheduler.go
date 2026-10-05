package schedule

import (
	"sync"
	"sync/atomic"
	"time"
)

// DiscordInterval is the minimum time between activity updates. Discord
// publishes two limits, five per 20 seconds and one per 15 seconds; this is
// the stricter.
const DiscordInterval = 15 * time.Second

// Clock is the time source a Scheduler needs. The real implementation wraps
// time.Now and time.AfterFunc and lives with the code that wires the host
// together; tests use internal/testutil/fakeclock.
type Clock interface {
	Now() time.Time
	// AfterFunc runs f on another goroutine once d has passed. The returned
	// function cancels that, as (*time.Timer).Stop does.
	AfterFunc(d time.Duration, f func()) (stop func() bool)
}

// Scheduler passes the latest submitted value to a consumer, no more often
// than once per interval. The first change after a quiet period is delivered
// at once; changes inside the interval replace one another, and the last is
// delivered when the interval has passed. A value equal to the one the
// consumer last received is not delivered again.
//
// A consumer that cannot use a value says so by returning false. The value is
// then as if it had not been delivered: it is not recorded as shown, and does
// not count towards the interval. Nothing is retried; the value is delivered
// again after the next Submit, Clear or Reset, if it is still the current one.
//
// All methods are safe for use from several goroutines and none of them
// blocks on the consumer.
type Scheduler[T comparable] struct {
	clock Clock
	emit  func(value T, show bool) bool

	mu      sync.Mutex
	limiter limiter[T]

	// delivering is held from the moment the loop looks at the limiter until
	// the consumer has answered, so that Settle can wait for a delivery that
	// is under way, or just decided.
	delivering sync.Mutex

	wake     chan struct{}
	stop     chan struct{}
	done     chan struct{}
	stopOnce sync.Once
}

// New starts a scheduler. The caller owns it and must call Stop.
//
// Updates are delivered by calling emit, with show false and the zero value
// for "show nothing". emit returns whether it took the update. It is called
// from the scheduler's own goroutine, one call at a time. It may take as long
// as it likes: submissions made meanwhile are coalesced, and the interval is
// measured from the start of each call. It must not call Stop.
func New[T comparable](clock Clock, interval time.Duration, emit func(value T, show bool) bool) *Scheduler[T] {
	s := &Scheduler[T]{
		clock:   clock,
		emit:    emit,
		limiter: limiter[T]{interval: interval},
		wake:    make(chan struct{}, 1),
		stop:    make(chan struct{}),
		done:    make(chan struct{}),
	}
	go s.run()
	return s
}

// Submit sets the value to show.
func (s *Scheduler[T]) Submit(value T) {
	s.submit(update[T]{value: value, show: true})
}

// Clear asks for nothing to be shown. It is an update like any other and
// obeys the same interval.
func (s *Scheduler[T]) Clear() {
	s.submit(update[T]{})
}

func (s *Scheduler[T]) submit(u update[T]) {
	s.mu.Lock()
	s.limiter.submit(u)
	s.mu.Unlock()
	s.signal()
}

// Reset forgets what the consumer last received, so the current value is
// delivered again, no sooner than the interval allows. Call it when the
// consumer has lost its state, as after a Discord reconnect.
func (s *Scheduler[T]) Reset() {
	s.mu.Lock()
	s.limiter.reset()
	s.mu.Unlock()
	s.signal()
}

// Settle returns once the delivery that is under way, if there is one, has
// finished: the consumer has answered and the answer is recorded. Calls to
// Submit, Clear and Reset made before it are then all taken into account by
// any delivery that follows. It is for a caller that must know that no
// delivery decided before its Reset is still on its way to the consumer. It
// must not be called from the consumer.
func (s *Scheduler[T]) Settle() {
	s.delivering.Lock()
	defer s.delivering.Unlock()
}

// Stop releases the scheduler's timer and waits for its goroutine to end,
// which includes waiting for a call to emit that is under way. Nothing is
// delivered afterwards, not even a pending value. Further calls to any method
// do nothing.
func (s *Scheduler[T]) Stop() {
	s.stopOnce.Do(func() { close(s.stop) })
	<-s.done
}

// signal asks the loop to look at the limiter again. One request is enough
// however many are made before the loop gets to it.
func (s *Scheduler[T]) signal() {
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

// timer is one armed wake-up. fired is set by the clock's goroutine and read
// by the loop.
type timer struct {
	stop  func() bool
	fired atomic.Bool
}

func (s *Scheduler[T]) arm(wait time.Duration) *timer {
	t := &timer{}
	t.stop = s.clock.AfterFunc(wait, func() {
		t.fired.Store(true)
		s.signal()
	})
	return t
}

func (s *Scheduler[T]) run() {
	defer close(s.done)
	var armed *timer
	for {
		select {
		case <-s.stop:
			if armed != nil {
				armed.stop()
			}
			return
		case <-s.wake:
		}

		s.delivering.Lock()
		s.mu.Lock()
		u, emit, wait := s.limiter.next(s.clock.Now())
		s.mu.Unlock()

		if wait > 0 {
			// The moment an emission is next allowed moves only when one is
			// made, so a timer that is still waiting is still right. One that
			// has fired while there is time left to wait, which takes a clock
			// that stepped backwards, is replaced.
			if armed == nil || armed.fired.Load() {
				armed = s.arm(wait)
			}
			s.delivering.Unlock()
			continue
		}
		if armed != nil {
			armed.stop()
			armed = nil
		}
		if emit && !s.emit(u.value, u.show) {
			s.mu.Lock()
			s.limiter.refused()
			s.mu.Unlock()
		}
		s.delivering.Unlock()
	}
}
