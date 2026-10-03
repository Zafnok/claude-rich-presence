package domain_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Zafnok/claude-rich-presence/internal/domain"
)

// session returns a valid session "s1" in the given status.
func session(status domain.Status) domain.Session {
	return domain.Session{
		ID:           "s1",
		Surface:      domain.SurfaceCode,
		Status:       status,
		Model:        "Sonnet",
		Project:      "demo",
		Privacy:      domain.PrivacyStandard,
		Start:        t0,
		LastActivity: t0.Add(time.Minute),
		Subagents:    1,
	}
}

// registryWith returns a registry holding the given sessions.
func registryWith(t *testing.T, sessions ...domain.Session) *domain.Registry {
	t.Helper()
	r := domain.NewRegistry()
	for _, s := range sessions {
		if err := r.Sync(s); err != nil {
			t.Fatalf("Sync(%+v) = %v", s, err)
		}
	}
	return r
}

// applyOne applies an event that must be valid and returns session "s1".
func applyOne(t *testing.T, r *domain.Registry, e domain.Event) domain.Session {
	t.Helper()
	snap, err := r.Apply(e)
	if err != nil {
		t.Fatalf("Apply(%+v) = %v", e, err)
	}
	if len(snap) != 1 {
		t.Fatalf("Apply(%+v) left %d sessions, want 1", e, len(snap))
	}
	return snap[0]
}

const (
	idle       = domain.StatusIdle
	working    = domain.StatusWorking
	waiting    = domain.StatusWaiting
	compacting = domain.StatusCompacting
)

// wantTransitions is the whole state machine: for each kind, the status that
// follows idle, working, waiting and compacting, in that order. It mirrors
// the table in docs/architecture/README.md.
var wantTransitions = map[domain.Kind][4]domain.Status{
	domain.KindSessionOpened:      {idle, working, waiting, compacting},
	domain.KindSessionRefreshed:   {idle, working, waiting, compacting},
	domain.KindTurnStarted:        {working, working, working, working},
	domain.KindToolStarted:        {working, working, working, working},
	domain.KindToolFinished:       {idle, working, working, compacting},
	domain.KindAttentionNeeded:    {waiting, waiting, waiting, compacting},
	domain.KindIdle:               {idle, idle, idle, idle},
	domain.KindTurnFinished:       {idle, idle, idle, idle},
	domain.KindCompactionStarted:  {idle, compacting, waiting, compacting},
	domain.KindCompactionFinished: {idle, working, waiting, working},
	domain.KindModelChanged:       {idle, working, waiting, compacting},
	domain.KindSubagentStarted:    {idle, working, waiting, compacting},
	domain.KindSubagentStopped:    {idle, working, waiting, compacting},
}

func TestTransitions(t *testing.T) {
	for _, kind := range domain.Kinds() {
		if kind == domain.KindSessionEnded {
			continue
		}
		want, ok := wantTransitions[kind]
		if !ok {
			t.Errorf("%s: no expected transitions", kind)
			continue
		}
		for i, from := range domain.Statuses() {
			t.Run(string(kind)+" from "+string(from), func(t *testing.T) {
				r := registryWith(t, session(from))
				got := applyOne(t, r, event(kind, t0.Add(2*time.Minute)))
				if got.Status != want[i] {
					t.Errorf("status = %s, want %s", got.Status, want[i])
				}
			})
		}
	}
}

func TestSessionEndedRemovesFromEveryStatus(t *testing.T) {
	for _, from := range domain.Statuses() {
		r := registryWith(t, session(from))
		snap, err := r.Apply(event(domain.KindSessionEnded, t0))
		if err != nil || len(snap) != 0 {
			t.Errorf("from %s: Apply(session ended) = %v, %v, want no sessions", from, snap, err)
		}
	}
}

func TestNewSessionDefaults(t *testing.T) {
	for _, kind := range domain.Kinds() {
		if kind == domain.KindSessionEnded {
			continue
		}
		t.Run(string(kind), func(t *testing.T) {
			e := event(kind, t0)
			e.Surface = domain.SurfaceDesktop
			got := applyOne(t, domain.NewRegistry(), e)

			want := domain.Session{
				ID:           "s1",
				Surface:      domain.SurfaceDesktop,
				Status:       wantTransitions[kind][0],
				Privacy:      domain.PrivacyMinimal,
				Start:        t0,
				LastActivity: t0,
			}
			switch kind {
			case domain.KindToolStarted:
				want.Tool = domain.ToolEditing
			case domain.KindModelChanged:
				want.Model = "Opus"
			case domain.KindSubagentStarted:
				want.Subagents = 1
			}
			if got != want {
				t.Errorf("session = %+v, want %+v", got, want)
			}
			if err := got.Validate(); err != nil {
				t.Errorf("created session is invalid: %v", err)
			}
		})
	}
}

func TestSessionEndedForUnknownSessionCreatesNothing(t *testing.T) {
	snap, err := domain.NewRegistry().Apply(event(domain.KindSessionEnded, t0))
	if err != nil || len(snap) != 0 {
		t.Errorf("Apply(session ended) = %v, %v, want no sessions", snap, err)
	}
}

func TestOpenedAndRefreshedSetOnlyWhatTheyCarry(t *testing.T) {
	for _, kind := range []domain.Kind{domain.KindSessionOpened, domain.KindSessionRefreshed} {
		t.Run(string(kind)+" with fields", func(t *testing.T) {
			e := event(kind, t0.Add(time.Minute))
			e.Model, e.Project, e.Privacy = "Haiku", "other", domain.PrivacyFull
			got := applyOne(t, registryWith(t, session(working)), e)
			if got.Model != "Haiku" || got.Project != "other" || got.Privacy != domain.PrivacyFull {
				t.Errorf("got model %q, project %q, privacy %q", got.Model, got.Project, got.Privacy)
			}
		})
		t.Run(string(kind)+" without fields", func(t *testing.T) {
			before := session(working)
			got := applyOne(t, registryWith(t, before), event(kind, t0.Add(time.Minute)))
			if got != before {
				t.Errorf("session = %+v, want it unchanged: %+v", got, before)
			}
		})
	}
}

func TestFieldsAreIgnoredOnKindsThatDoNotCarryThem(t *testing.T) {
	for _, kind := range domain.Kinds() {
		switch kind {
		case domain.KindSessionOpened, domain.KindSessionRefreshed, domain.KindSessionEnded:
			continue
		}
		t.Run(string(kind), func(t *testing.T) {
			before := session(working)
			e := event(kind, t0.Add(time.Minute))
			e.Project, e.Privacy = "other", domain.PrivacyFull
			if kind != domain.KindModelChanged {
				e.Model = "Haiku"
			}
			if kind != domain.KindToolStarted {
				e.Tool = domain.ToolBrowsing
			}
			e.Surface = domain.SurfaceDesktop
			got := applyOne(t, registryWith(t, before), e)

			if got.Project != before.Project || got.Privacy != before.Privacy || got.Surface != before.Surface {
				t.Errorf("project %q, privacy %q, surface %q changed", got.Project, got.Privacy, got.Surface)
			}
			if kind != domain.KindModelChanged && got.Model != before.Model {
				t.Errorf("model = %q, want %q", got.Model, before.Model)
			}
			if got.Tool == domain.ToolBrowsing {
				t.Errorf("tool was taken from a %s event", kind)
			}
		})
	}
}

func TestModelChanged(t *testing.T) {
	got := applyOne(t, registryWith(t, session(idle)), event(domain.KindModelChanged, t0))
	if got.Model != "Opus" {
		t.Errorf("model = %q, want Opus", got.Model)
	}
}

func TestToolKind(t *testing.T) {
	tool := func(k domain.ToolKind) domain.Event {
		e := event(domain.KindToolStarted, t0)
		e.Tool = k
		return e
	}
	cases := []struct {
		name   string
		events []domain.Event
		want   domain.ToolKind
	}{
		{"set by tool started", []domain.Event{tool(domain.ToolReading)}, domain.ToolReading},
		{"replaced by the next tool", []domain.Event{tool(domain.ToolReading), tool(domain.ToolRunning)}, domain.ToolRunning},
		{"kept when the tool finishes", []domain.Event{tool(domain.ToolReading), event(domain.KindToolFinished, t0)}, domain.ToolReading},
		{"kept through a model change", []domain.Event{tool(domain.ToolReading), event(domain.KindModelChanged, t0)}, domain.ToolReading},
		{"cleared by a new turn", []domain.Event{tool(domain.ToolReading), event(domain.KindTurnStarted, t0)}, domain.ToolNone},
		{"cleared when waiting", []domain.Event{tool(domain.ToolReading), event(domain.KindAttentionNeeded, t0)}, domain.ToolNone},
		{"cleared when compacting", []domain.Event{tool(domain.ToolReading), event(domain.KindCompactionStarted, t0)}, domain.ToolNone},
		{"cleared when the turn finishes", []domain.Event{tool(domain.ToolReading), event(domain.KindTurnFinished, t0)}, domain.ToolNone},
		{"cleared when idle", []domain.Event{tool(domain.ToolReading), event(domain.KindIdle, t0)}, domain.ToolNone},
		{"not restored after waiting", []domain.Event{tool(domain.ToolReading), event(domain.KindAttentionNeeded, t0), event(domain.KindToolFinished, t0)}, domain.ToolNone},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := domain.NewRegistry()
			var got domain.Session
			for _, e := range tc.events {
				got = applyOne(t, r, e)
			}
			if got.Tool != tc.want {
				t.Errorf("tool = %q, want %q", got.Tool, tc.want)
			}
		})
	}
}

func TestSubagentCount(t *testing.T) {
	start := event(domain.KindSubagentStarted, t0)
	stop := event(domain.KindSubagentStopped, t0)
	cases := []struct {
		name   string
		events []domain.Event
		want   int
	}{
		{"start", []domain.Event{start}, 1},
		{"two starts", []domain.Event{start, start}, 2},
		{"start then stop", []domain.Event{start, stop}, 0},
		{"stop without a start", []domain.Event{stop}, 0},
		{"more stops than starts", []domain.Event{start, stop, stop, stop}, 0},
		{"a start after extra stops still counts", []domain.Event{stop, stop, start}, 1},
		{"survives the end of the turn", []domain.Event{start, event(domain.KindTurnFinished, t0)}, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := domain.NewRegistry()
			var got domain.Session
			for _, e := range tc.events {
				got = applyOne(t, r, e)
			}
			if got.Subagents != tc.want {
				t.Errorf("subagents = %d, want %d", got.Subagents, tc.want)
			}
		})
	}
}

func TestTimes(t *testing.T) {
	// The session starts at t0 and was last active a minute later.
	early, between, late := t0.Add(-time.Hour), t0.Add(30*time.Second), t0.Add(time.Hour)
	last := t0.Add(time.Minute)
	cases := []struct {
		name      string
		event     domain.Event
		wantStart time.Time
		wantLast  time.Time
	}{
		{"a later event is activity", event(domain.KindTurnStarted, late), t0, late},
		{"an older event does not move last activity back", event(domain.KindTurnStarted, between), t0, last},
		{"an event at the same instant changes nothing", event(domain.KindTurnStarted, last), t0, last},
		{"an event before the start moves the start", event(domain.KindSessionOpened, early), early, last},
		{"the idle notice is not activity", event(domain.KindIdle, late), t0, last},
		{"the idle notice can still move the start", event(domain.KindIdle, early), early, last},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := applyOne(t, registryWith(t, session(working)), tc.event)
			if !got.Start.Equal(tc.wantStart) || !got.LastActivity.Equal(tc.wantLast) {
				t.Errorf("start %v, last %v; want start %v, last %v", got.Start, got.LastActivity, tc.wantStart, tc.wantLast)
			}
		})
	}
}

func TestSessionValidateAccepts(t *testing.T) {
	limits := session(working)
	limits.ID = strings.Repeat("i", domain.MaxIDLen)
	limits.Model = strings.Repeat("m", domain.MaxModelLen)
	limits.Project = strings.Repeat("p", domain.MaxProjectLen)
	limits.Tool = domain.ToolSearching
	limits.LastActivity = limits.Start
	limits.Subagents = 0

	bare := session(idle)
	bare.Model, bare.Project = "", ""

	for name, s := range map[string]domain.Session{"limits": limits, "bare": bare} {
		if err := s.Validate(); err != nil {
			t.Errorf("%s: Validate() = %v, want nil", name, err)
		}
	}
	for _, status := range domain.Statuses() {
		if err := session(status).Validate(); err != nil {
			t.Errorf("%s: Validate() = %v, want nil", status, err)
		}
	}
}

func TestSessionValidateRejects(t *testing.T) {
	const marker = "MARKER"
	cases := []struct {
		name   string
		mutate func(*domain.Session)
		want   string
	}{
		{"unknown status", func(s *domain.Session) { s.Status = marker }, "unknown status"},
		{"empty status", func(s *domain.Session) { s.Status = "" }, "unknown status"},
		{"unknown tool", func(s *domain.Session) { s.Tool = marker }, "unknown tool kind"},
		{"tool while idle", func(s *domain.Session) { s.Status, s.Tool = idle, domain.ToolEditing }, "tool kind on a session that is not working"},
		{"unknown privacy", func(s *domain.Session) { s.Privacy = marker }, "unknown privacy level"},
		{"empty privacy", func(s *domain.Session) { s.Privacy = "" }, "unknown privacy level"},
		{"zero start", func(s *domain.Session) { s.Start = time.Time{} }, "start time is not set"},
		{"last activity before start", func(s *domain.Session) { s.LastActivity = s.Start.Add(-time.Nanosecond) }, "last activity is before the start"},
		{"negative subagents", func(s *domain.Session) { s.Subagents = -1 }, "negative subagent count"},
		{"empty id", func(s *domain.Session) { s.ID = "" }, "session id is empty"},
		{"id one over", func(s *domain.Session) { s.ID = strings.Repeat("i", domain.MaxIDLen+1) }, "session id is too long"},
		{"unknown surface", func(s *domain.Session) { s.Surface = marker }, "unknown surface"},
		{"model one over", func(s *domain.Session) { s.Model = strings.Repeat("m", domain.MaxModelLen+1) }, "model is too long"},
		{"long project", func(s *domain.Session) { s.Project = strings.Repeat(marker, domain.MaxProjectLen) }, "project is too long"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := session(working)
			tc.mutate(&s)
			err := s.Validate()
			if !errors.Is(err, domain.ErrInvalidSession) {
				t.Fatalf("Validate() = %v, want an error wrapping ErrInvalidSession", err)
			}
			if want := "invalid session: " + tc.want; err.Error() != want {
				t.Errorf("Validate() = %q, want %q", err, want)
			}
			if strings.Contains(err.Error(), marker) {
				t.Errorf("Validate() = %q, which repeats a field's value", err)
			}
		})
	}
}
