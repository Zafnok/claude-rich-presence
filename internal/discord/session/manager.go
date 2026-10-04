package session

import (
	"context"
	"errors"
	"io"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/Zafnok/claude-rich-presence/internal/diag"
	"github.com/Zafnok/claude-rich-presence/internal/discord/codec"
	"github.com/Zafnok/claude-rich-presence/internal/domain"
	"github.com/Zafnok/claude-rich-presence/internal/schedule"
)

const (
	// backoffMin and backoffMax bound the delay between connection attempts
	// before jitter. The delay doubles with each failure.
	backoffMin = time.Second
	backoffMax = 60 * time.Second
	// handshakeTimeout is how long Discord has to answer the handshake.
	handshakeTimeout = 5 * time.Second
	// writeTimeout is how long one write may take. It is shorter than the
	// scheduler's interval, so a write is never still pending when the next
	// activity is due.
	writeTimeout = 5 * time.Second
	// clearTimeout is how long shutdown waits for Discord to take and
	// acknowledge the final clear.
	clearTimeout = time.Second
)

// ErrNotRunning is what a DialFunc returns, alone or wrapped, when no Discord
// client is listening. It only changes the class of error that Status and the
// log report: every failure to dial is retried the same way.
var ErrNotRunning = errors.New("discord is not running")

// DialFunc opens the connection to the running Discord client. It returns
// when ctx ends, with the context's error. The connection it returns must
// allow a Write while a Read is pending, and Close must fail both.
type DialFunc func(ctx context.Context) (io.ReadWriteCloser, error)

// Config is what a Manager is built from. Every field but Logger is required.
type Config struct {
	// ApplicationID is the Discord application to appear as.
	ApplicationID string
	Dial          DialFunc
	Clock         schedule.Clock
	// Jitter returns a number from 0 to 1, which places each wait between
	// half of the backoff delay and all of it.
	Jitter func() float64
	// Logger receives state changes and error classes, never an activity.
	Logger *diag.Logger
}

// update is one emission of the scheduler: an activity, or a clear when show
// is false.
type update struct {
	activity domain.Activity
	show     bool
}

// Manager keeps Discord showing the desired activity for as long as Discord
// can be reached. See the package documentation.
type Manager struct {
	applicationID string
	dial          DialFunc
	clock         schedule.Clock
	jitter        func() float64
	log           *diag.Logger
	pid           int
	sched         *schedule.Scheduler[domain.Activity]

	// box holds the one update that is waiting to be written. It is filled
	// by the scheduler's goroutine and only while the state is Ready.
	box chan update

	mu     sync.Mutex
	status Status

	// The rest belongs to the goroutine in Run.
	nonces uint64
	warned bool // the invalid application id has been reported
}

// New returns a manager and starts its scheduler. The caller must call Run,
// once, which is also what releases the scheduler.
func New(cfg Config) *Manager {
	m := &Manager{
		applicationID: cfg.ApplicationID,
		dial:          cfg.Dial,
		clock:         cfg.Clock,
		jitter:        cfg.Jitter,
		log:           cfg.Logger,
		pid:           os.Getpid(),
		box:           make(chan update, 1),
	}
	m.sched = schedule.New(cfg.Clock, schedule.DiscordInterval, m.emit)
	return m
}

// Set makes a the activity to show. It returns at once in every state. After
// Run has returned it does nothing.
func (m *Manager) Set(a domain.Activity) { m.sched.Submit(a) }

// Clear asks for no activity to be shown. It returns at once in every state.
func (m *Manager) Clear() { m.sched.Clear() }

// Status returns a snapshot for diagnostics. It touches nothing but memory.
func (m *Manager) Status() Status {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.status
}

// emit is the scheduler's consumer. It never blocks: the update replaces any
// that is still waiting, or is dropped when there is no connection to send it
// on. A dropped update is sent after all when the connection is ready, because
// the scheduler is reset then.
func (m *Manager) emit(a domain.Activity, show bool) {
	m.mu.Lock()
	ready := m.status.State == Ready
	if ready {
		select {
		case <-m.box:
		default:
		}
		m.box <- update{a, show}
	}
	m.mu.Unlock()
	if !ready {
		m.log.Debug("activity update dropped while not connected")
	}
}

// setState records a state. An update left waiting from the state before is
// discarded with it.
func (m *Manager) setState(s State) {
	m.mu.Lock()
	m.status.State = s
	select {
	case <-m.box:
	default:
	}
	m.mu.Unlock()
	m.log.Info("discord connection state changed", stateAttrs[s])
}

func (m *Manager) setError(f failure) {
	m.mu.Lock()
	m.status.LastError = f.class
	m.mu.Unlock()
}

// Run keeps the connection until ctx ends. Then, if the connection is ready,
// it clears the activity, and it closes the connection and returns. No
// goroutine of the manager outlives it.
func (m *Manager) Run(ctx context.Context) {
	defer m.setState(Stopped)
	defer m.sched.Stop()
	delay := backoffMin
	for {
		m.setState(Connecting)
		l, f := m.connect(ctx)
		if l != nil {
			delay = backoffMin
			f = m.serve(ctx, l)
			l.close()
		}
		if ctx.Err() != nil {
			return
		}
		m.setError(f)
		m.log.Info("discord connection failed", f.attr)
		if f.class == ErrorInvalidApplicationID {
			// Trying again soon cannot help: go straight to the longest
			// delay, and say so once.
			delay = backoffMax
			if !m.warned {
				m.warned = true
				m.log.Warn("discord refused the application id: check discord_application_id in the configuration", f.attr)
			}
		}
		m.setState(Disconnected)
		if !m.sleep(ctx, jittered(delay, m.jitter())) {
			return
		}
		delay = min(2*delay, backoffMax)
	}
}

// jittered places a wait between half of delay and all of it.
func jittered(delay time.Duration, jitter float64) time.Duration {
	half := delay / 2
	return half + time.Duration(jitter*float64(half))
}

// after returns a channel that is closed once d has passed on the clock.
func (m *Manager) after(d time.Duration) (passed <-chan struct{}, stop func() bool) {
	ch := make(chan struct{})
	return ch, m.clock.AfterFunc(d, func() { close(ch) })
}

// sleep waits for d and reports false if ctx ended first.
func (m *Manager) sleep(ctx context.Context, d time.Duration) bool {
	passed, stop := m.after(d)
	defer stop()
	select {
	case <-ctx.Done():
		return false
	case <-passed:
		return true
	}
}

// connect dials and shakes hands. It returns a link that is ready for
// activities, or why there is none.
func (m *Manager) connect(ctx context.Context) (*link, failure) {
	conn, err := m.dial(ctx)
	if err != nil {
		if errors.Is(err, ErrNotRunning) {
			return nil, failNotRunning
		}
		return nil, failDial
	}
	l := open(conn)
	if f, ok := m.handshake(ctx, l); !ok {
		l.close()
		return nil, f
	}
	return l, failure{}
}

func (m *Manager) handshake(ctx context.Context, l *link) (failure, bool) {
	timeout, stop := m.after(handshakeTimeout)
	defer stop()
	if m.write(ctx, l, codec.Handshake(m.applicationID)) != nil {
		return failWrite, false
	}
	for {
		select {
		case <-ctx.Done():
			return failure{}, false
		case <-timeout:
			return failHandshakeTimeout, false
		case in := <-l.frames:
			msg, f, ok := receive(in)
			if !ok {
				return f, false
			}
			if msg.Kind == codec.KindReady {
				return failure{}, true
			}
		}
	}
}

// write sends one frame. A write that outlasts writeTimeout, or ctx, is ended
// by closing the connection.
func (m *Manager) write(ctx context.Context, l *link, f codec.Frame) error {
	stopTimer := m.clock.AfterFunc(writeTimeout, l.hangUp)
	defer stopTimer()
	stopWatch := context.AfterFunc(ctx, l.hangUp)
	defer stopWatch()
	return codec.WriteFrame(l.conn, f)
}

func (m *Manager) nonce() string {
	m.nonces++
	return strconv.FormatUint(m.nonces, 10)
}

// setActivity is the frame for an update, and the nonce its answer carries.
func (m *Manager) setActivity(u update) (codec.Frame, string) {
	var a *domain.Activity
	if u.show {
		a = &u.activity
	}
	return codec.SetActivity(m.pid, m.nonce, a)
}

// serve runs a ready connection until it fails, and says why, or until ctx
// ends.
func (m *Manager) serve(ctx context.Context, l *link) failure {
	m.warned = false
	m.setState(Ready)
	// Discord has forgotten what it was showing. The scheduler sends it
	// again, no sooner than its interval allows.
	m.sched.Reset()
	var awaiting string // the nonce of the last activity sent
	for {
		var out []codec.Frame
		select {
		case <-ctx.Done():
			m.goodbye(l)
			return failure{}
		case u := <-m.box:
			var frame codec.Frame
			frame, awaiting = m.setActivity(u)
			out = append(out, frame)
		case in := <-l.frames:
			msg, f, ok := receive(in)
			if !ok {
				return f
			}
			switch msg.Kind {
			case codec.KindPing:
				out = append(out, codec.Pong(msg.Payload))
			case codec.KindAck:
				if msg.Nonce == awaiting {
					m.mu.Lock()
					m.status.LastUpdate = m.clock.Now()
					m.mu.Unlock()
				}
			case codec.KindError:
				m.setError(failActivity)
				m.log.Warn("discord rejected an activity update", failActivity.attr, diag.Count("code", int64(msg.Code)))
			}
		}
		for _, frame := range out {
			if m.write(ctx, l, frame) != nil {
				return failWrite
			}
		}
	}
}

// goodbye clears the activity on a ready connection and waits for Discord to
// acknowledge that, for no longer than clearTimeout.
func (m *Manager) goodbye(l *link) {
	stop := m.clock.AfterFunc(clearTimeout, l.hangUp)
	defer stop()
	frame, nonce := m.setActivity(update{})
	if codec.WriteFrame(l.conn, frame) != nil {
		return
	}
	for {
		msg, _, ok := receive(<-l.frames)
		if !ok || (msg.Kind == codec.KindAck && msg.Nonce == nonce) {
			return
		}
	}
}
