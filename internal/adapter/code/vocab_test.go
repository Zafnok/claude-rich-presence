package code

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/Zafnok/claude-rich-presence/internal/domain"
	"github.com/Zafnok/claude-rich-presence/internal/testutil/fakeclock"
)

func TestToolKind(t *testing.T) {
	cases := []struct {
		name string
		want domain.ToolKind
	}{
		{"Edit", domain.ToolEditing},
		{"Write", domain.ToolEditing},
		{"MultiEdit", domain.ToolEditing},
		{"NotebookEdit", domain.ToolEditing},
		{"Bash", domain.ToolRunning},
		{"PowerShell", domain.ToolRunning},
		{"Read", domain.ToolReading},
		{"Grep", domain.ToolSearching},
		{"Glob", domain.ToolSearching},
		{"WebFetch", domain.ToolBrowsing},
		{"WebSearch", domain.ToolBrowsing},
		{"mcp__Claude_Browser__navigate", domain.ToolBrowsing},
		{"mcp__claude-in-chrome__computer", domain.ToolBrowsing},
		{"mcp__playwright__browser_click", domain.ToolBrowsing},
		{"mcp__puppeteer__screenshot", domain.ToolBrowsing},
		{"Agent", domain.ToolDelegating},
		{"Task", domain.ToolDelegating},
		{"mcp__github__create_issue", domain.ToolUsingTools},
		{"mcp__plugin_rich-presence_presence__presence_status", domain.ToolUsingTools},
		// Only the server's name decides, not the tool's.
		{"mcp__files__open_in_browser", domain.ToolUsingTools},
		{"mcp__", domain.ToolUsingTools},
		{"TodoWrite", domain.ToolGeneric},
		{"edit", domain.ToolGeneric},
		{"Bash ", domain.ToolGeneric},
		{"mcp_browser", domain.ToolGeneric},
		{"x", domain.ToolGeneric},
	}
	named := map[string]bool{}
	for _, c := range cases {
		named[c.name] = true
		if got := toolKind(c.name); got != c.want {
			t.Errorf("toolKind(%q) = %q, want %q", c.name, got, c.want)
		}
		if !toolKind(c.name).Valid() {
			t.Errorf("toolKind(%q) is not a tool kind", c.name)
		}
	}
	for name := range toolKinds {
		if !named[name] {
			t.Errorf("no case for %s", name)
		}
	}
}

func TestModelLabel(t *testing.T) {
	cases := []struct {
		id   string
		want string
	}{
		{"claude-opus-5-5", "Opus 5.5"},
		{"claude-sonnet-5-5", "Sonnet 5.5"},
		{"claude-fable-5-1", "Fable 5.1"},
		{"claude-haiku-4-5-20251001", "Haiku 4.5"},
		{"claude-opus-4-20250514", "Opus 4"},
		{"claude-opus-4-1-20250805", "Opus 4.1"},
		{"claude-3-5-sonnet-20241022", "Sonnet 3.5"},
		{"claude-3-opus-20240229", "Opus 3"},
		{"claude-3-7-sonnet-latest", "Sonnet 3.7"},
		{"claude-opus-5-5[1m]", "Opus 5.5"},
		{"claude-sonnet-4-5@20250929", "Sonnet 4.5"},
		{"us.anthropic.claude-opus-4-5-20251101-v1:0", "Opus 4.5"},
		{"CLAUDE-OPUS-5-5", "Opus 5.5"},
		{"claude opus 5 5", "Opus 5.5"},
		{"claude-sonnet-5-5-1", "Sonnet 5.5"},
		{"claude-opus-5-fast", "Opus 5"},
		{"claude-opus-opus-5", ""},
		// No vendor, no family, or no version: nothing to say.
		{"", ""},
		{"opus", ""},
		{"opus-5-5", ""},
		{"default", ""},
		{"claude", ""},
		{"claude-opus", ""},
		{"claude-opus-latest", ""},
		{"claude-5-5", ""},
		{"claude-20251001-opus-5", ""},
		{"claude-opus-555", ""},
		{"claude-opus-v5", ""},
		{"gpt-5", ""},
		// A family that is not known is never published, whatever it says.
		{"claude-instant-1", ""},
		{"claude-acmecorp-5-5", ""},
		{"claude-my-project-opus-5-5", ""},
	}
	for _, c := range cases {
		got := modelLabel(c.id)
		if got != c.want {
			t.Errorf("modelLabel(%q) = %q, want %q", c.id, got, c.want)
		}
		if len(got) > domain.MaxModelLen {
			t.Errorf("modelLabel(%q) is %d bytes", c.id, len(got))
		}
	}
	for _, family := range families {
		if got := modelLabel("claude-" + family + "-9"); !strings.EqualFold(got, family+" 9") {
			t.Errorf("family %s gives %q", family, got)
		}
	}
}

// TestProjectNameIsCleanedAsADisplayNameIs gives projectName directory names
// that Discord would read as something other than text. Each comes out as
// CleanName gives it, which is what the host accepts.
func TestProjectNameIsCleanedAsADisplayNameIs(t *testing.T) {
	cases := []struct {
		name string
		dir  string
		want string
	}{
		{"formatting characters", "**bold** `code` ~~gone~~ ||spoiler||", "bold code gone spoiler"},
		{"a mention", "@everyone", "everyone"},
		{"a user mention", "<@123456>", "123456"},
		{"link syntax", "[click](https:example.com)", "click(https:example.com)"},
		{"a zero-width character", "al\u200bpha", "alpha"},
		{"a right-to-left override", "alpha\u202egpj.exe", "alphagpj.exe"},
		{"a line break", "one\ntwo", "one two"},
		{"only markup", "*`~|<>[]@", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			for _, cwd := range []string{"/home/u/" + c.dir, `C:\Users\u\` + c.dir + `\`} {
				got := projectName(cwd)
				if got != c.want || got != domain.CleanName(c.dir) {
					t.Errorf("projectName(%q) = %q, want %q, which is what CleanName gives: %q", cwd, got, c.want, domain.CleanName(c.dir))
				}
				e := domain.Event{SessionID: "s1", Surface: domain.SurfaceCode, At: epoch, Kind: domain.KindSessionOpened, Project: got}
				if err := e.Validate(); err != nil {
					t.Errorf("the host would refuse %q: %v", got, err)
				}
			}
		})
	}
}

// FuzzPublishedProject opens a session at the full level in an arbitrary
// directory, with an arbitrary display name from the resolver when one is
// given. Whatever project the adapter publishes, the host accepts.
func FuzzPublishedProject(f *testing.F) {
	f.Add("/home/u/work/alpha", "")
	f.Add(`C:\Users\u\work\alpha\`, "Alpha")
	f.Add("/home/u/**@everyone**", "<@1> [a](b)")
	f.Add("/home/u/a\u200bb\u202ec", "a\u200bb\u202ec")
	f.Add("/home/u/\x00\xff", "\x00\xff\n")
	f.Add("/home/u/"+strings.Repeat("é", domain.MaxProjectLen), strings.Repeat(" n", domain.MaxProjectLen))
	f.Fuzz(func(t *testing.T, cwd, name string) {
		arguments, err := json.Marshal(map[string]string{"event": "UserPromptSubmit", "session_id": "s1", "cwd": cwd})
		if err != nil {
			t.Fatal(err)
		}
		rec := &recorder{}
		var resolve Resolver
		if name != "" {
			resolve = func(string) Settings { return Settings{Privacy: domain.PrivacyFull, Name: name} }
		}
		a, err := New(Options{Privacy: domain.PrivacyFull, ProvisionalID: provisional, Publisher: rec, Status: fixedStatus{}, Clock: fakeclock.New(epoch), Resolve: resolve})
		if err != nil {
			t.Fatal(err)
		}
		a.Open()
		if got := a.handleEvent(arguments, nil); got != constantResult {
			t.Errorf("result = %+v", got)
		}
		a.Close()
		// The registry is the host's: it refuses what the host would.
		replay(t, rec.events)
		for _, e := range rec.events {
			if e.Project != domain.CleanName(e.Project) {
				t.Errorf("published a project that is not clean: %q", e.Project)
			}
		}
	})
}

func TestProjectName(t *testing.T) {
	long := strings.Repeat("p", domain.MaxProjectLen-1)
	cases := []struct {
		cwd  string
		want string
	}{
		{"/home/u/work/alpha", "alpha"},
		{"/home/u/work/alpha/", "alpha"},
		{"/home/u/work/alpha//", "alpha"},
		{`C:\Users\u\work\alpha`, "alpha"},
		{`C:\Users\u\work\alpha\`, "alpha"},
		{`C:/Users/u\work/alpha`, "alpha"},
		{`\\server\share\alpha\`, "alpha"},
		{"alpha", "alpha"},
		{"/home/u/my project", "my project"},
		{"/home/u/über-projekt", "über-projekt"},
		{"/home/u/.config", ".config"},
		{"", ""},
		{"/", ""},
		{`\`, ""},
		{`C:\`, ""},
		{"C:", ""},
		{"/home/u/.", ""},
		{"/home/u/..", ""},
		// A name is cleaned as a display name is: control characters and line
		// breaks become one space, and what is not text is removed.
		{"/home/u/bad\x00name", "bad name"},
		{"/home/u/bad\nname", "bad name"},
		{"/home/u/bad\x7fname", "bad name"},
		{"/home/u/bad\ufffdname", "badname"},
		{"/home/u/\x00", ""},
		{"/home/u/**", ""},
		{"/home/u/" + long + "p", long + "p"},
		{"/home/u/" + long + "pp", long + "p"},
		// A character is never cut in half.
		{"/home/u/" + long + "é", long},
		{"/home/u/" + long + "éé", long},
	}
	for _, c := range cases {
		got := projectName(c.cwd)
		if got != c.want {
			t.Errorf("projectName(%q) = %q, want %q", c.cwd, got, c.want)
		}
		if len(got) > domain.MaxProjectLen || strings.ContainsAny(got, `/\`) {
			t.Errorf("projectName(%q) = %q", c.cwd, got)
		}
	}
}
