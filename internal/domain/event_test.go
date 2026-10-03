package domain_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Zafnok/claude-rich-presence/internal/domain"
)

var t0 = time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)

// event returns a valid event of the given kind for session "s1".
func event(kind domain.Kind, at time.Time) domain.Event {
	e := domain.Event{SessionID: "s1", Surface: domain.SurfaceCode, At: at, Kind: kind}
	switch kind {
	case domain.KindToolStarted:
		e.Tool = domain.ToolEditing
	case domain.KindModelChanged:
		e.Model = "Opus"
	}
	return e
}

func TestEventValidateAcceptsEveryKind(t *testing.T) {
	for _, kind := range domain.Kinds() {
		if err := event(kind, t0).Validate(); err != nil {
			t.Errorf("%s: Validate() = %v, want nil", kind, err)
		}
	}
}

func TestEventValidateAcceptsLimits(t *testing.T) {
	e := event(domain.KindSessionOpened, t0)
	e.SessionID = strings.Repeat("i", domain.MaxIDLen)
	e.Model = strings.Repeat("m", domain.MaxModelLen)
	e.Project = strings.Repeat("p", domain.MaxProjectLen)
	e.Privacy = domain.PrivacyFull
	e.Tool = domain.ToolGeneric
	if err := e.Validate(); err != nil {
		t.Errorf("Validate() = %v, want nil", err)
	}
}

func TestEventValidateRejects(t *testing.T) {
	const marker = "MARKER"
	cases := []struct {
		name   string
		kind   domain.Kind
		mutate func(*domain.Event)
		want   string
	}{
		{"unknown kind", domain.KindIdle, func(e *domain.Event) { e.Kind = marker }, "unknown kind"},
		{"empty kind", domain.KindIdle, func(e *domain.Event) { e.Kind = "" }, "unknown kind"},
		{"zero time", domain.KindIdle, func(e *domain.Event) { e.At = time.Time{} }, "time is not set"},
		{"unknown tool", domain.KindToolStarted, func(e *domain.Event) { e.Tool = marker }, "unknown tool kind"},
		{"unknown tool on another kind", domain.KindIdle, func(e *domain.Event) { e.Tool = marker }, "unknown tool kind"},
		{"unknown privacy", domain.KindSessionOpened, func(e *domain.Event) { e.Privacy = marker }, "unknown privacy level"},
		{"tool started without tool", domain.KindToolStarted, func(e *domain.Event) { e.Tool = domain.ToolNone }, "tool started without a tool kind"},
		{"model changed without model", domain.KindModelChanged, func(e *domain.Event) { e.Model = "" }, "model changed without a model"},
		{"empty id", domain.KindIdle, func(e *domain.Event) { e.SessionID = "" }, "session id is empty"},
		{"long id", domain.KindIdle, func(e *domain.Event) { e.SessionID = strings.Repeat(marker, domain.MaxIDLen) }, "session id is too long"},
		{"id one over", domain.KindIdle, func(e *domain.Event) { e.SessionID = strings.Repeat("i", domain.MaxIDLen+1) }, "session id is too long"},
		{"unknown surface", domain.KindIdle, func(e *domain.Event) { e.Surface = marker }, "unknown surface"},
		{"empty surface", domain.KindIdle, func(e *domain.Event) { e.Surface = "" }, "unknown surface"},
		{"model one over", domain.KindModelChanged, func(e *domain.Event) { e.Model = strings.Repeat("m", domain.MaxModelLen+1) }, "model is too long"},
		{"long model on another kind", domain.KindIdle, func(e *domain.Event) { e.Model = strings.Repeat(marker, domain.MaxModelLen) }, "model is too long"},
		{"project one over", domain.KindSessionOpened, func(e *domain.Event) { e.Project = strings.Repeat("p", domain.MaxProjectLen+1) }, "project is too long"},
		{"long project on another kind", domain.KindIdle, func(e *domain.Event) { e.Project = strings.Repeat(marker, domain.MaxProjectLen) }, "project is too long"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := event(tc.kind, t0)
			tc.mutate(&e)
			err := e.Validate()
			if !errors.Is(err, domain.ErrInvalidEvent) {
				t.Fatalf("Validate() = %v, want an error wrapping ErrInvalidEvent", err)
			}
			if want := "invalid event: " + tc.want; err.Error() != want {
				t.Errorf("Validate() = %q, want %q", err, want)
			}
			if strings.Contains(err.Error(), marker) {
				t.Errorf("Validate() = %q, which repeats a field's value", err)
			}
		})
	}
}
