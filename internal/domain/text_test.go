package domain_test

import (
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/Zafnok/claude-rich-presence/internal/domain"
)

func TestCleanName(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"plain", "Visions of Shuyi", "Visions of Shuyi"},
		{"other scripts and symbols", "修意の夢 C# (beta) my_project", "修意の夢 C# (beta) my_project"},
		{"line breaks become one space", "one\r\n\ntwo\tthree", "one two three"},
		{"spaces are trimmed and collapsed", "  a   b  ", "a b"},
		{"control characters", "a\x00b\x1bc\x7fd", "a b c d"},
		{"a no-break space and a line separator", "a\u00a0b\u2028c", "a b c"},
		{"invisible and direction characters", "a\u200bb\u202ec\ufeff", "abc"},
		{"markup", "**bold** `code` ~~gone~~ ||spoiler|| \\escaped", "bold code gone spoiler escaped"},
		{"a mention and a link", "<@123> [text](https://example.com)", "123 text(https://example.com)"},
		{"invalid UTF-8", "a\xffb", "ab"},
		{"only markup", "*`~|<>[]\\@", ""},
		{"only spaces", " \t\n", ""},
		{"empty", "", ""},
		{"at the cap", strings.Repeat("n", domain.MaxProjectLen), strings.Repeat("n", domain.MaxProjectLen)},
		{"over the cap", strings.Repeat("n", domain.MaxProjectLen+1), strings.Repeat("n", domain.MaxProjectLen)},
		{"cut between characters, not inside one", strings.Repeat("n", domain.MaxProjectLen-1) + "é", strings.Repeat("n", domain.MaxProjectLen-1)},
		{"no space is left at the cut", strings.Repeat("n", domain.MaxProjectLen-1) + " x", strings.Repeat("n", domain.MaxProjectLen-1)},
		{"a word that fits after a space", strings.Repeat("n", domain.MaxProjectLen-2) + " x", strings.Repeat("n", domain.MaxProjectLen-2) + " x"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := domain.CleanName(tc.in)
			if got != tc.want {
				t.Errorf("CleanName(%q) = %q, want %q", tc.in, got, tc.want)
			}
			if again := domain.CleanName(got); again != got {
				t.Errorf("CleanName(%q) = %q, want a clean name left as it is", got, again)
			}
		})
	}
}

func TestCleanTextCutsToItsLimit(t *testing.T) {
	for _, tc := range []struct {
		in    string
		limit int
		want  string
	}{
		{"battle engine", 32, "battle engine"},
		{"battle engine", 8, "battle e"},
		{"battle engine", 7, "battle"},
		{"battle engine", 0, ""},
	} {
		if got := domain.CleanText(tc.in, tc.limit); got != tc.want {
			t.Errorf("CleanText(%q, %d) = %q, want %q", tc.in, tc.limit, got, tc.want)
		}
	}
}

// FuzzCleanName checks what the host relies on: a cleaned name is valid text
// within the limit, holds nothing that cleaning removes, is left as it is by
// cleaning it again, and is accepted as the project of an event and of a
// session.
func FuzzCleanName(f *testing.F) {
	for _, seed := range []string{
		"", "alpha", "  a   b  ", "**bold** `code`", "<@123> [text](https://example.com)",
		"a\u200bb\u202ec\ufeff", "a\x00b\x1bc\x7fd", "a\xffb", "a\u00a0b\u2028c",
		strings.Repeat("n", domain.MaxProjectLen-1) + " x", strings.Repeat("é", domain.MaxProjectLen),
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, in string) {
		got := domain.CleanName(in)
		if len(got) > domain.MaxProjectLen || !utf8.ValidString(got) {
			t.Errorf("CleanName(%q) = %q, which is too long or not text", in, got)
		}
		if strings.ContainsAny(got, "*`~|<>[]\\@\n\r\t\x00\x7f\u200b\u202e\ufeff\ufffd") {
			t.Errorf("CleanName(%q) = %q, which holds what cleaning removes", in, got)
		}
		if again := domain.CleanName(got); again != got {
			t.Errorf("CleanName(%q) = %q, and cleaning that gives %q", in, got, again)
		}
		e := event(domain.KindSessionOpened, t0)
		e.Project = got
		if err := e.Validate(); err != nil {
			t.Errorf("an event with project %q: %v", got, err)
		}
		s := session(idle)
		s.Project = got
		if err := s.Validate(); err != nil {
			t.Errorf("a session with project %q: %v", got, err)
		}
	})
}

// TestTextThatIsNotCleanIsRefused gives an event and a session a project and
// a model that their adapter could not have produced. Each is refused, and
// the registry is left as it was.
func TestTextThatIsNotCleanIsRefused(t *testing.T) {
	const notClean, notLabel = "project is not clean", "model is not a model label"
	cases := []struct {
		name    string
		project string
		model   string
		want    string
	}{
		{"a control character in the project", "al\x00pha", "", notClean},
		{"a line break in the project", "alpha\nbeta", "", notClean},
		{"formatting in the project", "**alpha**", "", notClean},
		{"a mention in the project", "@everyone", "", notClean},
		{"link syntax in the project", "[alpha](https://example.com)", "", notClean},
		{"a zero-width character in the project", "al\u200bpha", "", notClean},
		{"a right-to-left override in the project", "alpha\u202egpj.exe", "", notClean},
		{"invalid UTF-8 in the project", "al\xffpha", "", notClean},
		{"a space before the project", " alpha", "", notClean},
		{"a space after the project", "alpha ", "", notClean},
		{"two spaces in the project", "my  project", "", notClean},
		{"a no-break space in the project", "my\u00a0project", "", notClean},
		{"markup in the model", "", "**Opus** 5.5", notLabel},
		{"a mention in the model", "", "@everyone", notLabel},
		{"a control character in the model", "", "Opus\x005.5", notLabel},
		{"a hyphen in the model", "", "claude-opus-5-5", notLabel},
		{"a letter outside ASCII in the model", "", "Opüs 5.5", notLabel},
		{"a space before the model", "", " Opus 5.5", notLabel},
		{"two spaces in the model", "", "Opus  5.5", notLabel},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := registryWith(t, session(idle))
			before := r.Snapshot()

			e := event(domain.KindSessionRefreshed, t0.Add(time.Second))
			e.Project, e.Model = tc.project, tc.model
			if _, err := r.Apply(e); err == nil || err.Error() != "invalid event: "+tc.want {
				t.Errorf("Apply = %v, want %q", err, tc.want)
			}
			s := session(working)
			s.Project, s.Model = tc.project, tc.model
			if tc.project == "" {
				s.Project = "demo"
			}
			if err := r.Sync(s); err == nil || err.Error() != "invalid session: "+tc.want {
				t.Errorf("Sync = %v, want %q", err, tc.want)
			}
			if after := r.Snapshot(); len(after) != 1 || after[0] != before[0] {
				t.Errorf("the registry holds %+v, want it unchanged, %+v", after, before)
			}
		})
	}
}

func TestCleanTextIsAccepted(t *testing.T) {
	for _, tc := range []struct{ project, model string }{
		{"", ""},
		{"alpha", "Opus 5.5"},
		{"修意の夢 C# (beta) my_project", "Sonnet 5"},
		{".config", "Haiku 4.5"},
		{"über-projekt", "Fable 5.1"},
	} {
		e := event(domain.KindSessionOpened, t0)
		e.Project, e.Model = tc.project, tc.model
		if err := e.Validate(); err != nil {
			t.Errorf("an event with project %q and model %q: %v", tc.project, tc.model, err)
		}
		s := session(idle)
		s.Project, s.Model = tc.project, tc.model
		if err := s.Validate(); err != nil {
			t.Errorf("a session with project %q and model %q: %v", tc.project, tc.model, err)
		}
	}
}
