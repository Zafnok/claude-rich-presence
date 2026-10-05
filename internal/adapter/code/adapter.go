package code

import (
	"cmp"
	"encoding/json"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Zafnok/claude-rich-presence/internal/domain"
	"github.com/Zafnok/claude-rich-presence/internal/mcp"
)

// The names of the tools.
const (
	EventToolName  = "presence_event"
	StatusToolName = "presence_status"
)

// eventReply is the text of every result of the event tool, whatever the
// call held and whatever became of it. An empty result makes Claude Code add
// a line to the model's context on every prompt, and any other JSON is read
// as hook output and can steer the session (ADR-0007, rule 4).
const eventReply = "{}"

const eventDescription = "Internal. Hooks call this automatically to report session activity. Do not call it yourself."

// modelCallMeta is the key Claude Code puts in the _meta of a call the model
// makes. A hook's call has no _meta. Observed in CRP-001 and not documented,
// so refusing such calls is defence in depth and nothing relies on it.
const modelCallMeta = "claudecode/toolUseId"

// queueSize is how many tool calls may wait for the publisher. A call that
// arrives when the queue is full is dropped.
const queueSize = 256

// maxAgents is how many running subagents are tracked. A start beyond it is
// not counted, and so neither is its stop.
const maxAgents = 256

// Publisher takes presence events toward the host. Publish is called from a
// goroutine the adapter owns, never from a tool call, so a slow publisher
// delays presence and never Claude.
type Publisher interface {
	Publish(domain.Event)
}

// Clock tells the time.
type Clock interface {
	Now() time.Time
}

// Options configures an Adapter. Every field but Resolve is required.
type Options struct {
	// Privacy is the level for the whole session when Resolve is nil. When
	// Resolve is set it is the level for a directory that matches no profile,
	// and the session is at minimal until a hook supplies a working directory.
	Privacy domain.Privacy
	// Resolve gives the level and the project's display name for a working
	// directory. When it is set, the working directory is read at every level,
	// used to resolve and then discarded, and the level of the session follows
	// the directory of the latest hook that carries one. When it is nil, Privacy
	// applies throughout. It runs on the path Claude waits on and must perform
	// no I/O: see Resolver.
	Resolve Resolver
	// ProvisionalID names the session until a hook supplies the real id. It
	// must be unique to this process. CLAUDE_CODE_SESSION_ID must not be used
	// for it: it is stale after a clear and wrong under --continue.
	ProvisionalID string
	Publisher     Publisher
	Status        StatusSource
	Clock         Clock
}

// Adapter turns calls to the event tool into presence events for one session
// of the coding agent. The process is the session; the session id is a label
// the latest hook supplies.
//
// Make one with New, call Open when the client has initialized, and call
// Close when the server's input closes or the process is told to stop.
type Adapter struct {
	// privacy is the level that applies without a resolver, and the one a
	// resolver's settings fall back to for a directory with no profile. The
	// level in force is session.privacy, which is minimal until a hook with a
	// working directory has been resolved.
	privacy domain.Privacy
	resolve Resolver
	status  StatusSource
	clock   Clock
	pub     Publisher

	// queue carries one batch of events per tool call to the pump, which is
	// the only caller of the publisher. done closes when the pump has ended.
	queue chan []domain.Event
	done  chan struct{}

	// ignored counts malformed calls and calls made by the model. dropped
	// counts calls lost to a full queue and events the publisher failed on.
	ignored atomic.Int64
	dropped atomic.Int64

	mu     sync.Mutex
	opened bool
	closed bool
	start  time.Time
	session
	// agents holds the ids of the subagents whose start was seen and whose
	// stop was not.
	agents map[string]struct{}
	// last is published by the pump after the queue has drained, when set.
	last *domain.Event
}

// session is what the adapter remembers of its session between calls. It is
// what a rebind carries over to the new id. The settings resolved from the
// latest working directory are part of it, so that a call whose batch is
// dropped leaves them as they were.
type session struct {
	id      string
	model   string
	project string
	// privacy is the level in force, and name the display name resolved for
	// it, which is empty unless the level is full.
	privacy domain.Privacy
	name    string
}

// New checks the options, builds an adapter and starts its pump. The caller
// must call Close.
func New(opts Options) (*Adapter, error) {
	switch {
	case !opts.Privacy.Valid():
		return nil, errors.New("code: unknown privacy level")
	case opts.ProvisionalID == "" || len(opts.ProvisionalID) > domain.MaxIDLen:
		return nil, errors.New("code: provisional id must be 1 to 128 bytes")
	case opts.Publisher == nil:
		return nil, errors.New("code: publisher is nil")
	case opts.Status == nil:
		return nil, errors.New("code: status source is nil")
	case opts.Clock == nil:
		return nil, errors.New("code: clock is nil")
	}
	a := &Adapter{
		privacy: opts.Privacy,
		resolve: opts.Resolve,
		status:  opts.Status,
		clock:   opts.Clock,
		pub:     opts.Publisher,
		queue:   make(chan []domain.Event, queueSize),
		done:    make(chan struct{}),
		session: session{id: opts.ProvisionalID, privacy: openingLevel(opts)},
		agents:  map[string]struct{}{},
	}
	go a.pump()
	return a, nil
}

// openingLevel is the level a session is published at before any hook has
// supplied a working directory. With a resolver the directory decides the
// level, and until it is known the session may be in a profile more private
// than Privacy, so it opens at minimal.
func openingLevel(opts Options) domain.Privacy {
	if opts.Resolve != nil {
		return domain.PrivacyMinimal
	}
	return opts.Privacy
}

// Tools returns the event tool and the status tool.
func (a *Adapter) Tools() []mcp.Tool {
	// The names and schemas are constants that NewTool accepts, which the
	// tests show by listing the tools through a server.
	event, _ := mcp.NewMetaTool(EventToolName, eventDescription, eventSchema(), a.handleEvent)
	status, _ := mcp.NewTool(StatusToolName, statusDescription, json.RawMessage(`{"type":"object"}`), a.handleStatus)
	return []mcp.Tool{event, status}
}

// Open opens the session under its provisional id. Call it when MCP
// initialize completes: no hook has fired by then, and nothing may be
// published before (ADR-0007). It returns at once. A second call does
// nothing, and so does a call after Close.
func (a *Adapter) Open() {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.opened || a.closed {
		return
	}
	a.opened = true
	a.start = a.clock.Now()
	// The queue is empty: nothing is queued before the session opens.
	a.queue <- []domain.Event{a.openedEvent(a.session)}
}

// Close ends the session and waits until everything queued has reached the
// publisher, the session's end last. Call it when the server's input closes
// or on an interrupt or termination signal: there is no hook for the end of
// a session. Calls after the first only wait.
func (a *Adapter) Close() {
	a.mu.Lock()
	if !a.closed {
		a.closed = true
		if a.opened {
			last := a.event(a.id, domain.KindSessionEnded, a.clock.Now())
			a.last = &last
		}
		close(a.queue)
	}
	a.mu.Unlock()
	<-a.done
}

// pump hands queued events to the publisher, in order, until Close.
func (a *Adapter) pump() {
	defer close(a.done)
	for batch := range a.queue {
		for _, e := range batch {
			a.publish(e)
		}
	}
	// Close set last before it closed the queue.
	if a.last != nil {
		a.publish(*a.last)
	}
}

// publish gives one event to the publisher. A publisher that panics loses
// the event and nothing else (ADR-0008).
func (a *Adapter) publish(e domain.Event) {
	defer func() {
		if recover() != nil {
			a.dropped.Add(1)
		}
	}()
	a.pub.Publish(e)
}

// handleEvent is the event tool. Its result is the same for every call. It
// performs no I/O and cannot wait: it parses, updates the adapter's memory of
// the session and offers the events to the queue without blocking.
func (a *Adapter) handleEvent(arguments, meta json.RawMessage) mcp.Result {
	if !a.receive(arguments, meta) {
		a.ignored.Add(1)
	}
	return mcp.Result{Text: eventReply}
}

// receive handles one call to the event tool. It reports false when the call
// was ignored: made by the model, malformed, or failed inside.
func (a *Adapter) receive(arguments, meta json.RawMessage) (accepted bool) {
	defer func() {
		// What a panic carried is dropped, because it may hold the arguments.
		if recover() != nil {
			accepted = false
		}
	}()
	if fromModel(meta) {
		return false
	}
	// The directory is read when it can matter: to resolve the settings, and
	// to name the project at full.
	h, ok := parseHook(arguments, a.resolve != nil || a.privacy == domain.PrivacyFull)
	if !ok {
		return false
	}

	a.mu.Lock()
	defer a.mu.Unlock()
	if !a.opened || a.closed {
		return true
	}
	now := a.clock.Now()
	next := a.session
	agents := a.agents
	var batch []domain.Event

	cwd := h.get(fieldCwd)
	if a.resolve != nil && cwd != "" {
		settings := a.resolveSettings(cwd)
		next.privacy, next.name = settings.Privacy, settings.Name
	}
	raised := rank(next.privacy) > rank(a.session.privacy)
	lowered := rank(next.privacy) < rank(a.session.privacy)

	// The project is published at full only. Below it the session holds none,
	// so that a rebuilt session cannot have one either.
	renamed := false
	if next.privacy != domain.PrivacyFull {
		next.project = ""
	} else if project := cmp.Or(next.name, projectName(cwd)); project != "" && project != next.project {
		next.project = project
		renamed = true
	}
	moved := h.get(fieldSessionID) != next.id
	if id := h.get(fieldSessionID); moved || lowered {
		// A new id, as the first hook brings and as a clear does, or a lower
		// level. The session moves to the new id, or reopens under the same
		// one: the old session ends, and the new one opens with the same start
		// time, so no second session appears and none is left behind. Ending
		// is how the host forgets what was published at the higher level,
		// because no event takes a project or a model away.
		batch = append(batch, a.event(next.id, domain.KindSessionEnded, now))
		next.id = id
		agents = map[string]struct{}{}
		batch = append(batch, a.openedEvent(next))
	} else if renamed || raised {
		refresh := a.event(next.id, domain.KindSessionRefreshed, now)
		refresh.Project = next.project
		if raised {
			refresh.Privacy = next.privacy
		}
		batch = append(batch, refresh)
	}

	var agentStarted, agentStopped string
	e := a.event(next.id, h.row.kind, now)
	switch h.row.kind {
	case "":
		e.Kind = notificationKinds[h.get(fieldNotificationType)]
	case domain.KindSessionRefreshed:
		e.Model = modelLabel(h.get(fieldModel))
		if e.Model != "" {
			next.model = e.Model
		}
	case domain.KindToolStarted:
		e.Tool = toolKind(h.get(fieldToolName))
	case domain.KindModelChanged:
		// A model whose id cannot be read gives no label, and the domain has
		// no event that removes one, so the switch is not reported.
		e.Model = modelLabel(h.get(fieldToModel))
		if e.Model == "" {
			e.Kind = ""
		}
		next.model = e.Model
	case domain.KindSubagentStarted:
		agentStarted = h.get(fieldAgentID)
		if _, running := agents[agentStarted]; running || len(agents) >= maxAgents {
			agentStarted = ""
			e.Kind = ""
		}
	case domain.KindSubagentStopped:
		// Claude Code also sends a stop with no start, after compaction and
		// after ordinary turns. Only a stop whose start was seen counts.
		agentStopped = h.get(fieldAgentID)
		if _, running := agents[agentStopped]; !running {
			agentStopped = ""
			e.Kind = ""
		}
	}
	if e.Kind == "" && (moved || lowered) {
		// The session reopened at its original start time. Without an event
		// at the present, it would look idle since then.
		e = a.event(next.id, domain.KindSessionRefreshed, now)
	}
	if e.Kind != "" {
		batch = append(batch, e)
	}

	// Every event of the call is restricted at the level this call resolved,
	// and so no batch spans a change of level.
	batch = restrict(next.privacy, batch)
	if len(batch) > 0 {
		select {
		case a.queue <- batch:
		default:
			// The publisher is not keeping up. The call is lost whole and the
			// adapter's memory is left as it was, so that the next call makes
			// the same moves again.
			a.dropped.Add(1)
			return true
		}
	}
	a.session = next
	a.agents = agents
	if agentStarted != "" {
		a.agents[agentStarted] = struct{}{}
	}
	delete(a.agents, agentStopped)
	return true
}

// fromModel reports whether a call's _meta marks it as made by the model.
func fromModel(meta json.RawMessage) bool {
	var keys map[string]json.RawMessage
	if json.Unmarshal(meta, &keys) != nil {
		return false
	}
	_, marked := keys[modelCallMeta]
	return marked
}

// event is a bare event for the session.
func (a *Adapter) event(id string, kind domain.Kind, at time.Time) domain.Event {
	return domain.Event{SessionID: id, Surface: domain.SurfaceCode, At: at, Kind: kind}
}

// openedEvent opens s at the time the adapter's session began.
func (a *Adapter) openedEvent(s session) domain.Event {
	e := a.event(s.id, domain.KindSessionOpened, a.start)
	e.Model = s.model
	e.Project = s.project
	e.Privacy = s.privacy
	return restrictEvent(s.privacy, e)
}

// restrict applies the privacy level to a batch of events. It is the last
// thing done to an event before it is queued, so nothing above the level
// leaves the adapter (ADR-0008).
func restrict(level domain.Privacy, batch []domain.Event) []domain.Event {
	out := batch[:0]
	for _, e := range batch {
		// At minimal, an idle notice would become a refresh, which counts as
		// activity. It reports the opposite, so it is not published.
		if level == domain.PrivacyMinimal && e.Kind == domain.KindIdle {
			continue
		}
		out = append(out, restrictEvent(level, e))
	}
	return out
}

// restrictEvent applies the privacy level to one event.
//
//   - minimal: that the session exists, and its timestamps. Every event but
//     opened and ended becomes a bare refresh, which moves the time of last
//     activity and says nothing else.
//   - standard: also status, tool kind and model family.
//   - full: also the project name.
func restrictEvent(level domain.Privacy, e domain.Event) domain.Event {
	switch level {
	case domain.PrivacyFull:
		return e
	case domain.PrivacyStandard:
		e.Project = ""
		return e
	}
	kind := e.Kind
	if kind != domain.KindSessionOpened && kind != domain.KindSessionEnded {
		kind = domain.KindSessionRefreshed
	}
	return domain.Event{SessionID: e.SessionID, Surface: e.Surface, At: e.At, Kind: kind, Privacy: e.Privacy}
}
