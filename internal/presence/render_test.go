package presence_test

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/Zafnok/claude-rich-presence/internal/domain"
	"github.com/Zafnok/claude-rich-presence/internal/presence"
)

var update = flag.Bool("update", false, "rewrite the golden file")

var (
	t0  = time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)
	now = t0.Add(time.Hour)
)

// session is a valid working session at the full level, for a test to vary.
func session(id string, change func(*domain.Session)) domain.Session {
	s := domain.Session{
		ID:           id,
		Surface:      domain.SurfaceCode,
		Status:       domain.StatusWorking,
		Model:        "Opus",
		Project:      "proj-" + id,
		Privacy:      domain.PrivacyFull,
		Start:        t0,
		LastActivity: t0,
	}
	if change != nil {
		change(&s)
	}
	return s
}

func mustRender(t *testing.T, sessions []domain.Session, set presence.Settings) domain.Activity {
	t.Helper()
	a, ok := presence.Render(sessions, now, set)
	if !ok {
		t.Fatal("Render showed nothing, want an activity")
	}
	return a
}

// show makes an omitted field visible in the golden file.
func show(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

// TestRenderGolden renders every combination of surface, status, tool kind,
// privacy level, and one or several sessions, and compares the result with
// testdata/render.golden. Run with -update to rewrite the file, then read
// the diff.
func TestRenderGolden(t *testing.T) {
	type state struct {
		status domain.Status
		tool   domain.ToolKind
	}
	states := []state{{domain.StatusWorking, domain.ToolNone}}
	for _, k := range domain.ToolKinds() {
		states = append(states, state{domain.StatusWorking, k})
	}
	states = append(states,
		state{domain.StatusWaiting, domain.ToolNone},
		state{domain.StatusCompacting, domain.ToolNone},
		state{domain.StatusIdle, domain.ToolNone},
	)

	var b strings.Builder
	b.WriteString("# surface/status/tool/privacy/sessions | details | state | large image | large text | small image | small text\n")
	cases := 0
	for _, surface := range []domain.Surface{domain.SurfaceCode, domain.SurfaceDesktop} {
		for _, st := range states {
			for _, privacy := range []domain.Privacy{domain.PrivacyMinimal, domain.PrivacyStandard, domain.PrivacyFull} {
				for _, n := range []int{1, 3} {
					focus := session("a", func(s *domain.Session) {
						s.Surface, s.Status, s.Tool, s.Privacy = surface, st.status, st.tool, privacy
						s.Project = "myproject"
					})
					if err := focus.Validate(); err != nil {
						t.Fatal(err)
					}
					sessions := []domain.Session{focus}
					// The others are idle, on desktop and older, so they
					// never take the focus.
					for i := 1; i < n; i++ {
						sessions = append(sessions, session(fmt.Sprint("other", i), func(s *domain.Session) {
							s.Surface, s.Status = domain.SurfaceDesktop, domain.StatusIdle
							s.Start = t0.Add(-time.Hour)
							s.LastActivity = t0.Add(-time.Minute)
						}))
					}
					a := mustRender(t, sessions, presence.Settings{})
					if !a.Start.Equal(t0) || a.Type != domain.ActivityPlaying {
						t.Errorf("%s: start %v type %v, want the focus session's start and playing", surface, a.Start, a.Type)
					}
					fmt.Fprintf(&b, "%s/%s/%s/%s/%d | %s | %s | %s | %s | %s | %s\n",
						surface, st.status, show(string(st.tool)), privacy, n,
						show(a.Details), show(a.State), show(a.LargeImage), show(a.LargeText), show(a.SmallImage), show(a.SmallText))
					cases++
				}
			}
		}
	}
	if cases != 2*12*3*2 {
		t.Fatalf("rendered %d cases, want %d", cases, 2*12*3*2)
	}

	path := filepath.Join("testdata", "render.golden")
	if *update {
		if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want := strings.Split(strings.ReplaceAll(string(raw), "\r\n", "\n"), "\n")
	got := strings.Split(b.String(), "\n")
	if len(got) != len(want) {
		t.Fatalf("got %d lines, golden has %d", len(got), len(want))
	}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("line %d:\n got %s\nwant %s", i+1, got[i], want[i])
		}
	}
}

func TestRenderPrivacy(t *testing.T) {
	others := []domain.Session{
		session("y", func(s *domain.Session) { s.Status = domain.StatusIdle }),
		session("z", func(s *domain.Session) { s.Status = domain.StatusIdle }),
	}
	tests := []struct {
		name        string
		privacy     domain.Privacy
		several     bool
		wantProject bool
		wantCount   bool
		wantStatus  bool
	}{
		{"minimal alone", domain.PrivacyMinimal, false, false, false, false},
		{"minimal with others", domain.PrivacyMinimal, true, false, false, false},
		{"standard alone", domain.PrivacyStandard, false, false, false, true},
		{"standard with others", domain.PrivacyStandard, true, false, true, true},
		{"full alone", domain.PrivacyFull, false, true, false, true},
		{"full with others", domain.PrivacyFull, true, true, true, true},
		{"unknown level is treated as minimal", domain.Privacy("loud"), true, false, false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sessions := []domain.Session{session("a", func(s *domain.Session) {
				s.Privacy = tt.privacy
				s.Tool = domain.ToolEditing
				s.Project = "secretproject"
			})}
			if tt.several {
				sessions = append(sessions, others...)
			}
			a := mustRender(t, sessions, presence.Settings{})
			all := strings.Join([]string{a.Details, a.State, a.LargeText, a.SmallText, a.LargeImage, a.SmallImage}, "\n")
			if got := strings.Contains(all, "secretproject"); got != tt.wantProject {
				t.Errorf("project shown = %v, want %v, in %q", got, tt.wantProject, all)
			}
			// The other sessions' projects are never shown.
			if strings.Contains(all, "proj-") {
				t.Errorf("another session's project is shown in %q", all)
			}
			if got := strings.Contains(all, "sessions"); got != tt.wantCount {
				t.Errorf("session count shown = %v, want %v, in %q", got, tt.wantCount, all)
			}
			if tt.wantCount && !strings.Contains(a.State, "3 sessions") {
				t.Errorf("state = %q, want it to count 3 sessions", a.State)
			}
			gotStatus := a.State != "" || a.SmallImage != "" || a.SmallText != ""
			if gotStatus != tt.wantStatus {
				t.Errorf("status shown = %v, want %v: %+v", gotStatus, tt.wantStatus, a)
			}
			if tt.wantStatus && !strings.Contains(a.State, "Opus") {
				t.Errorf("state = %q, want the model", a.State)
			}
		})
	}
}

func TestRenderOptionalParts(t *testing.T) {
	tests := []struct {
		name        string
		change      func(*domain.Session)
		wantDetails string
		wantState   string
	}{
		{"no model", func(s *domain.Session) { s.Model = "" }, "Claude Code · proj-a", "Thinking"},
		{"no project at full", func(s *domain.Session) { s.Project = "" }, "Claude Code", "Thinking · Opus"},
		{
			"unknown status leaves only the model",
			func(s *domain.Session) { s.Status = "napping" },
			"Claude Code · proj-a", "Opus",
		},
		{
			"a one-character line is left out",
			func(s *domain.Session) { s.Status, s.Model = "napping", "X" },
			"Claude Code · proj-a", "",
		},
		{
			"a two-character line is kept",
			func(s *domain.Session) { s.Status, s.Model = "napping", "é4" },
			"Claude Code · proj-a", "é4",
		},
		{
			"unknown surface has no first line",
			func(s *domain.Session) { s.Surface, s.Project = "watch", "" },
			"", "Opus",
		},
		{
			"invalid bytes are dropped",
			func(s *domain.Session) { s.Project = "a\xffb" },
			"Claude Code · ab", "Thinking · Opus",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := mustRender(t, []domain.Session{session("a", tt.change)}, presence.Settings{})
			if a.Details != tt.wantDetails || a.State != tt.wantState {
				t.Errorf("details %q state %q, want %q and %q", a.Details, a.State, tt.wantDetails, tt.wantState)
			}
		})
	}
}

func TestRenderTruncation(t *testing.T) {
	const prefix = "Claude Code · " // 15 bytes
	tests := []struct {
		name    string
		project string
		want    string
	}{
		{"exactly at the limit is kept", strings.Repeat("x", 113), prefix + strings.Repeat("x", 113)},
		{"one over is cut", strings.Repeat("x", 114), prefix + strings.Repeat("x", 110) + "…"},
		// 37 three-byte characters end at byte 126, one past the cut, so the
		// cut backs up to the character boundary at 123.
		{"cut inside a three-byte character", strings.Repeat("世", 40), prefix + strings.Repeat("世", 36) + "…"},
		// 27 four-byte characters end at byte 123; the 28th would end at 127.
		{"cut inside a four-byte character", strings.Repeat("😀", 30), prefix + strings.Repeat("😀", 27) + "…"},
		{"cut after a two-byte character", strings.Repeat("é", 60), prefix + strings.Repeat("é", 55) + "…"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := mustRender(t, []domain.Session{session("a", func(s *domain.Session) { s.Project = tt.project })}, presence.Settings{})
			if a.Details != tt.want {
				t.Errorf("details = %q, want %q", a.Details, tt.want)
			}
			if len(a.Details) > 128 {
				t.Errorf("details is %d bytes, want at most 128", len(a.Details))
			}
			if !utf8.ValidString(a.Details) {
				t.Errorf("details %q is not valid UTF-8", a.Details)
			}
		})
	}
}

func TestRenderFocus(t *testing.T) {
	with := func(id string, status domain.Status, surface domain.Surface, last time.Duration) domain.Session {
		return session(id, func(s *domain.Session) {
			s.Status, s.Surface = status, surface
			s.LastActivity = t0.Add(last)
			// Each session has its own start, so the timer shows the focus.
			s.Start = t0.Add(-time.Duration(len(id)) * time.Hour)
		})
	}
	const code, desktop = domain.SurfaceCode, domain.SurfaceDesktop
	tests := []struct {
		name     string
		sessions []domain.Session
		want     string
	}{
		{"working over waiting", []domain.Session{
			with("a", domain.StatusWaiting, code, time.Minute),
			with("bb", domain.StatusWorking, desktop, 0),
		}, "bb"},
		{"waiting over compacting", []domain.Session{
			with("a", domain.StatusCompacting, code, time.Minute),
			with("bb", domain.StatusWaiting, desktop, 0),
		}, "bb"},
		{"compacting over idle", []domain.Session{
			with("a", domain.StatusIdle, code, time.Minute),
			with("bb", domain.StatusCompacting, desktop, 0),
		}, "bb"},
		{"code over desktop", []domain.Session{
			with("a", domain.StatusWorking, desktop, time.Minute),
			with("bb", domain.StatusWorking, code, 0),
		}, "bb"},
		{"most recent activity", []domain.Session{
			with("a", domain.StatusWorking, code, 0),
			with("bb", domain.StatusWorking, code, time.Minute),
		}, "bb"},
		{"lowest id last", []domain.Session{
			with("bb", domain.StatusWorking, code, 0),
			with("a", domain.StatusWorking, code, 0),
			with("ccc", domain.StatusWorking, code, 0),
		}, "a"},
		{"status outranks every later rule", []domain.Session{
			with("a", domain.StatusIdle, code, time.Minute),
			with("bb", domain.StatusWaiting, code, time.Minute),
			with("ccc", domain.StatusWorking, desktop, 0),
			with("dddd", domain.StatusCompacting, code, time.Minute),
		}, "ccc"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var first domain.Activity
			// Every rotation and the reverse: the order of the input does
			// not change the answer.
			for i := range 2 * len(tt.sessions) {
				in := slices.Clone(tt.sessions)
				if i >= len(in) {
					slices.Reverse(in)
				}
				k := i % len(in)
				in = append(in[k:], in[:k]...)
				a := mustRender(t, in, presence.Settings{})
				if want := "Claude Code · proj-" + tt.want; a.LargeText == "Claude Code" && a.Details != want {
					t.Errorf("order %d: details = %q, want %q", i, a.Details, want)
				}
				if want := "Claude Desktop · proj-" + tt.want; a.LargeText == "Claude Desktop" && a.Details != want {
					t.Errorf("order %d: details = %q, want %q", i, a.Details, want)
				}
				if want := t0.Add(-time.Duration(len(tt.want)) * time.Hour); !a.Start.Equal(want) {
					t.Errorf("order %d: start = %v, want %v, the focus session's", i, a.Start, want)
				}
				if i == 0 {
					first = a
				} else if !a.Equal(first) {
					t.Errorf("order %d: %+v, want the same as order 0: %+v", i, a, first)
				}
			}
		})
	}
}

func TestRenderShowsNothing(t *testing.T) {
	const period = 15 * time.Minute
	idle := func(id string, ago time.Duration) domain.Session {
		return session(id, func(s *domain.Session) {
			s.Status = domain.StatusIdle
			s.Start = now.Add(-24 * time.Hour)
			s.LastActivity = now.Add(-ago)
		})
	}
	tests := []struct {
		name     string
		sessions []domain.Session
		period   time.Duration
		wantShow bool
	}{
		{"nil registry", nil, period, false},
		{"empty registry", []domain.Session{}, period, false},
		{"empty registry, clearing off", nil, 0, false},
		{"idle just before the threshold", []domain.Session{idle("a", period-time.Nanosecond)}, period, true},
		{"idle exactly at the threshold", []domain.Session{idle("a", period)}, period, true},
		{"idle just after the threshold", []domain.Session{idle("a", period+time.Nanosecond)}, period, false},
		{"all idle past the threshold", []domain.Session{idle("a", time.Hour), idle("b", 2*time.Hour)}, period, false},
		{"one of two idle sessions is recent", []domain.Session{idle("a", time.Hour), idle("b", time.Minute)}, period, true},
		{"an old session that is not idle", []domain.Session{
			idle("a", time.Hour),
			session("b", func(s *domain.Session) {
				s.Status = domain.StatusWaiting
				s.Start = now.Add(-24 * time.Hour)
				s.LastActivity = now.Add(-time.Hour)
			}),
		}, period, true},
		{"clearing off", []domain.Session{idle("a", 100*time.Hour)}, 0, true},
		{"negative period is off", []domain.Session{idle("a", 100*time.Hour)}, -time.Second, true},
		{"desktop alone, long past the threshold", []domain.Session{onDesktop(idle("d", 100*time.Hour))}, period, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a, ok := presence.Render(tt.sessions, now, presence.Settings{IdleClear: tt.period})
			if ok != tt.wantShow {
				t.Fatalf("shown = %v, want %v", ok, tt.wantShow)
			}
			if !ok && !a.Equal(domain.Activity{}) {
				t.Errorf("activity = %+v, want the zero value when nothing is shown", a)
			}
		})
	}
}

// onDesktop moves a session to Claude Desktop.
func onDesktop(s domain.Session) domain.Session {
	s.Surface = domain.SurfaceDesktop
	return s
}

// TestDesktopIsShownForAsLongAsItIsOpen checks that the idle period ends the
// showing of Claude Code sessions and never of a Claude Desktop session.
func TestDesktopIsShownForAsLongAsItIsOpen(t *testing.T) {
	const period = 15 * time.Minute
	set := presence.Settings{IdleClear: period}
	idle := func(id string, ago time.Duration) domain.Session {
		return session(id, func(s *domain.Session) {
			s.Status = domain.StatusIdle
			s.LastActivity = now.Add(-ago)
		})
	}
	desktop := onDesktop(idle("d", 100*time.Hour))
	tests := []struct {
		name     string
		sessions []domain.Session
		want     string
	}{
		{"desktop beside code idle past the period", []domain.Session{idle("a", time.Hour), desktop, idle("b", 2*time.Hour)}, "Claude Desktop"},
		{"desktop beside code idle within the period", []domain.Session{idle("a", time.Minute), desktop}, "Claude Code"},
		{"desktop beside one recent and one old code session", []domain.Session{idle("a", time.Hour), desktop, idle("b", time.Minute)}, "Claude Code"},
		{"desktop beside old code that is not idle", []domain.Session{desktop, session("a", func(s *domain.Session) { s.LastActivity = now.Add(-time.Hour) })}, "Claude Code"},
		{"two desktop sessions", []domain.Session{desktop, onDesktop(idle("e", 200*time.Hour))}, "Claude Desktop"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if a := mustRender(t, tt.sessions, set); a.LargeText != tt.want {
				t.Errorf("the card is for %q, want %q", a.LargeText, tt.want)
			}
			if wait, ok := presence.ClearsIn(tt.sessions, now, set); ok {
				t.Errorf("ClearsIn = %v, want no moment while a Desktop session is open", wait)
			}
		})
	}
}

// TestClearsInIsTheMomentRenderShowsNothing checks ClearsIn against Render
// itself: the activity is still shown one step before the moment it names,
// and is not shown at it.
func TestClearsInIsTheMomentRenderShowsNothing(t *testing.T) {
	const period = 15 * time.Minute
	idle := func(id string, ago time.Duration) domain.Session {
		return session(id, func(s *domain.Session) {
			s.Status = domain.StatusIdle
			s.Start = now.Add(-24 * time.Hour)
			s.LastActivity = now.Add(-ago)
		})
	}
	working := session("w", func(s *domain.Session) { s.LastActivity = now.Add(-time.Hour) })
	tests := []struct {
		name     string
		sessions []domain.Session
		period   time.Duration
		want     time.Duration
		wantOK   bool
	}{
		{"no sessions", nil, period, 0, false},
		{"clearing off", []domain.Session{idle("a", time.Minute)}, 0, 0, false},
		{"negative period is off", []domain.Session{idle("a", time.Minute)}, -time.Second, 0, false},
		{"a session that is not idle", []domain.Session{idle("a", time.Minute), working}, period, 0, false},
		{"one idle session", []domain.Session{idle("a", time.Minute)}, period, 14*time.Minute + 1, true},
		{"idle since this moment", []domain.Session{idle("a", 0)}, period, period + 1, true},
		{"the most recent of several counts", []domain.Session{idle("a", 10*time.Minute), idle("b", 2*time.Minute), idle("c", 5*time.Minute)}, period, 13*time.Minute + 1, true},
		{"exactly at the threshold", []domain.Session{idle("a", period)}, period, 1, true},
		{"cleared already", []domain.Session{idle("a", period+1)}, period, 0, false},
		{"cleared long ago", []domain.Session{idle("a", time.Hour)}, period, period - time.Hour + 1, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			set := presence.Settings{IdleClear: tt.period}
			got, ok := presence.ClearsIn(tt.sessions, now, set)
			if ok != tt.wantOK || (ok && got != tt.want) {
				t.Fatalf("ClearsIn = %v, %v; want %v, %v", got, ok, tt.want, tt.wantOK)
			}
			if !ok {
				return
			}
			if _, shown := presence.Render(tt.sessions, now.Add(got-1), set); !shown {
				t.Errorf("nothing is shown one step before the moment, %v from now", got)
			}
			if _, shown := presence.Render(tt.sessions, now.Add(got), set); shown {
				t.Errorf("an activity is still shown at the moment, %v from now", got)
			}
		})
	}
}

// TestPhrasesFitDiscord checks every fixed phrase through the renderer: each
// is long enough for Discord to accept and short enough to read on a card.
func TestPhrasesFitDiscord(t *testing.T) {
	tools := append([]domain.ToolKind{domain.ToolNone}, domain.ToolKinds()...)
	for _, surface := range []domain.Surface{domain.SurfaceCode, domain.SurfaceDesktop} {
		for _, status := range domain.Statuses() {
			for _, tool := range tools {
				s := session("a", func(s *domain.Session) {
					s.Surface, s.Status, s.Tool, s.Privacy = surface, status, tool, domain.PrivacyStandard
					s.Model = ""
				})
				if s.Validate() != nil {
					continue
				}
				a := mustRender(t, []domain.Session{s}, presence.Settings{})
				for _, phrase := range []string{a.Details, a.State, a.LargeText, a.SmallText} {
					if n := utf8.RuneCountInString(phrase); n < 2 || n > 32 {
						t.Errorf("%s/%s/%s: phrase %q has %d characters, want 2 to 32", surface, status, tool, phrase, n)
					}
				}
				if a.LargeImage != "logo" || !slices.Contains([]string{"working", "waiting", "idle"}, a.SmallImage) {
					t.Errorf("%s/%s/%s: images %q and %q", surface, status, tool, a.LargeImage, a.SmallImage)
				}
			}
		}
	}
}
