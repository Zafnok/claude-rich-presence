package code

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Zafnok/claude-rich-presence/internal/domain"
	"github.com/Zafnok/claude-rich-presence/internal/mcp"
	"github.com/Zafnok/claude-rich-presence/internal/testutil/fakeclock"
)

const provisional = "provisional-1"

var epoch = time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)

// constantResult is what the event tool must return, always.
var constantResult = mcp.Result{Text: "{}"}

// recorder is a Publisher that keeps what it is given.
type recorder struct {
	mu     sync.Mutex
	events []domain.Event
}

func (r *recorder) Publish(e domain.Event) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, e)
}

func (r *recorder) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.events)
}

// fixedStatus is a StatusSource that returns itself.
type fixedStatus Status

func (s fixedStatus) Status() Status { return Status(s) }

// harness is an adapter over a recorder and a fake clock.
type harness struct {
	t     *testing.T
	a     *Adapter
	rec   *recorder
	clock *fakeclock.Clock
}

func newHarness(t *testing.T, privacy domain.Privacy) *harness {
	t.Helper()
	h := &harness{t: t, rec: &recorder{}, clock: fakeclock.New(epoch)}
	a, err := New(Options{
		Privacy:       privacy,
		ProvisionalID: provisional,
		Publisher:     h.rec,
		Status:        fixedStatus{Role: RoleHost, Discord: DiscordConnected, Sessions: 1},
		Clock:         h.clock,
	})
	if err != nil {
		t.Fatal(err)
	}
	h.a = a
	return h
}

// call sends arguments to the event tool as a hook does, a second after the
// last call, and checks the result is the constant.
func (h *harness) call(arguments string) {
	h.t.Helper()
	h.callWithMeta(arguments, "")
}

func (h *harness) callWithMeta(arguments, meta string) {
	h.t.Helper()
	h.clock.Advance(time.Second)
	var rawArguments, rawMeta json.RawMessage
	if arguments != "" {
		rawArguments = json.RawMessage(arguments)
	}
	if meta != "" {
		rawMeta = json.RawMessage(meta)
	}
	if got := h.a.handleEvent(rawArguments, rawMeta); got != constantResult {
		h.t.Errorf("result for %s = %+v, want %+v", arguments, got, constantResult)
	}
}

// finish closes the adapter and returns everything it published, as text.
func (h *harness) finish() []string {
	h.t.Helper()
	h.a.Close()
	out := make([]string, 0, len(h.rec.events))
	for _, e := range h.rec.events {
		if err := e.Validate(); err != nil {
			h.t.Errorf("published an invalid event %+v: %v", e, err)
		}
		if e.Surface != domain.SurfaceCode {
			h.t.Errorf("event %+v has surface %q", e, e.Surface)
		}
		out = append(out, show(e))
	}
	return out
}

// show is an event without its time: the kind, the session id, and each
// optional field that is set.
func show(e domain.Event) string {
	s := string(e.Kind) + " " + e.SessionID
	if e.Tool != "" {
		s += " tool=" + string(e.Tool)
	}
	if e.Model != "" {
		s += " model=" + e.Model
	}
	if e.Project != "" {
		s += " project=" + e.Project
	}
	if e.Privacy != "" {
		s += " privacy=" + string(e.Privacy)
	}
	if e.Link != "" {
		s += " link=" + e.Link
	}
	return s
}

// bind is the call every bound scenario starts with, and bound is what an
// adapter publishes up to and including it at a privacy level.
const bind = `{"event":"UserPromptSubmit","session_id":"s1"}`

func bound(privacy domain.Privacy) []string {
	turn := "turn_started s1"
	if privacy == domain.PrivacyMinimal {
		turn = "session_refreshed s1"
	}
	return []string{
		"session_opened " + provisional + " privacy=" + string(privacy),
		"session_ended " + provisional,
		"session_opened s1 privacy=" + string(privacy),
		turn,
	}
}

// afterBind opens an adapter, binds it to s1, makes the calls and closes it.
// It returns what was published between the bind and the session's end.
func afterBind(t *testing.T, privacy domain.Privacy, calls ...string) []string {
	t.Helper()
	h := newHarness(t, privacy)
	h.a.Open()
	h.call(bind)
	for _, c := range calls {
		h.call(c)
	}
	got := h.finish()
	prefix := bound(privacy)
	if len(got) < len(prefix)+1 || !equal(got[:len(prefix)], prefix) || !strings.HasPrefix(got[len(got)-1], "session_ended ") {
		t.Fatalf("published %q, want it to begin %q and end with the session's end", got, prefix)
	}
	return got[len(prefix) : len(got)-1]
}

func equal(a, b []string) bool {
	return fmt.Sprintf("%q", a) == fmt.Sprintf("%q", b)
}

func wantEvents(t *testing.T, got []string, want ...string) {
	t.Helper()
	if !equal(got, want) {
		t.Errorf("published\n  %q\nwant\n  %q", got, want)
	}
}

// replay reduces published events in a registry, as the host does.
func replay(t *testing.T, events []domain.Event) []domain.Session {
	t.Helper()
	registry := domain.NewRegistry()
	for _, e := range events {
		if _, err := registry.Apply(e); err != nil {
			t.Fatalf("event %+v: %v", e, err)
		}
	}
	return registry.Snapshot()
}

// waitFor polls a condition with a deadline.
func waitFor(t *testing.T, what string, done func() bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for !done() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(time.Millisecond)
	}
}
