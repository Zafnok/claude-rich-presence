package e2e

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/Zafnok/claude-rich-presence/internal/config"
	"github.com/Zafnok/claude-rich-presence/internal/testutil/fakediscord"
)

// button is a button as Discord documents it in an activity.
type button struct {
	Label string `json:"label"`
	URL   string `json:"url"`
}

// buttons are the buttons of an activity the fake Discord was told to show.
func buttons(t *testing.T, v shown) []button {
	t.Helper()
	var a struct {
		Buttons []button `json:"buttons"`
	}
	if err := json.Unmarshal([]byte(v.Raw), &a); err != nil {
		t.Fatalf("the activity %s is not what Discord documents: %v", v.Raw, err)
	}
	return a.Buttons
}

// writeConfig writes the scene's configuration file.
func (s *scene) writeConfig(settings map[string]any) {
	s.t.Helper()
	dirs, err := config.ResolveDirs(runtime.GOOS, s.getenv)
	if err != nil {
		s.t.Fatal(err)
	}
	raw, err := json.Marshal(settings)
	if err != nil {
		s.t.Fatal(err)
	}
	if err := os.MkdirAll(dirs.Config, 0o700); err != nil {
		s.t.Fatal(err)
	}
	if err := os.WriteFile(dirs.File, raw, 0o600); err != nil {
		s.t.Fatal(err)
	}
}

// E-link: a session in a directory whose profile has a link shows one
// button with that link, and a session anywhere else shows none. The button
// is the focus session's, and follows focus (ADR-0012, CRP-048).
func TestRepositoryLink(t *testing.T) {
	t.Parallel()
	const link = "https://github.com/example/shared-project"
	s := newScene(t)
	s.writeConfig(map[string]any{
		"privacy": "standard",
		"projects": []map[string]string{
			{"path": filepath.Join(s.home, "work", "project-of-shared"), "link": link + ".git"},
			// A link the configuration refuses is not published.
			{"path": filepath.Join(s.home, "work", "project-of-other"), "link": "https://user:token@github.com/example/other"},
		},
	})
	srv := s.discord(fakediscord.Behavior{})

	one := s.start("shared")
	one.awaitStatus("Role: host", "Discord: connected", "Sessions: 1")
	// Until a hook says where the session is, nothing says it has a link.
	opened := awaitShown(t, srv, "the session", lines(surfaceLine, ""))
	if got := buttons(t, opened); len(got) != 0 {
		t.Errorf("before the directory is known the buttons are %+v, want none", got)
	}

	withLink := func(v shown) bool {
		b := buttons(t, v)
		return !v.Clear && strings.HasPrefix(v.State, "Thinking") && len(b) == 1 && b[0] == button{Label: "View on GitHub", URL: link}
	}
	without := func(state string) func(shown) bool {
		return func(v shown) bool { return !v.Clear && strings.HasPrefix(v.State, state) && len(buttons(t, v)) == 0 }
	}

	// A hook that plants a link changes nothing: the link is the profile's.
	one.hook("UserPromptSubmit", "link", "https://github.com/planted/link", "url", "https://github.com/planted/link")
	awaitShown(t, srv, "the profiled session with its button", withLink)

	// A second session, in a directory whose profile has no valid link, takes
	// the focus when it works and the first is idle: no button.
	one.hook("Stop")
	two := s.start("other")
	two.hook("PreToolUse", "tool_name", "Bash")
	awaitShown(t, srv, "the other session without a button", without("Running commands"))

	// Focus returns to the first session, and so does its button.
	two.hook("Stop")
	one.hook("UserPromptSubmit")
	awaitShown(t, srv, "the profiled session with its button again", withLink)

	// Nothing shown at any time held the planted link or the refused one.
	for _, v := range showing(t, srv) {
		if strings.Contains(v.Raw, "planted") || strings.Contains(v.Raw, "token") {
			t.Errorf("Discord was shown a link that is not a profile's valid link: %s", v.Raw)
		}
	}
	two.finish()
	one.finish()
}
