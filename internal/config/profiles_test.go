package config_test

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/Zafnok/claude-rich-presence/internal/config"
	"github.com/Zafnok/claude-rich-presence/internal/domain"
)

// loadOn loads a configuration file for an operating system.
func loadOn(t *testing.T, goos, file string, pairs ...string) (config.Config, []config.Warning) {
	t.Helper()
	pairs = append(pairs, "USERPROFILE", `C:\Users\u`)
	cfg, _, warnings := config.Load(goos, env(pairs...), &fakeFiles{data: file})
	return cfg, warnings
}

// projects is a configuration file whose profiles have the given paths, each
// named after its position: p0, p1 and so on.
func projects(paths ...string) string {
	entries := make([]map[string]string, len(paths))
	for i, p := range paths {
		entries[i] = map[string]string{"path": p, "name": fmt.Sprintf("p%d", i)}
	}
	file, _ := json.Marshal(map[string]any{"projects": entries})
	return string(file)
}

func fileWarning(setting, problem string) config.Warning {
	return config.Warning{Source: config.SourceFile, Setting: setting, Problem: problem}
}

func TestEffectiveMatching(t *testing.T) {
	cases := []struct {
		name     string
		goos     string
		profiles []string
		cwd      string
		want     string // the name of the profile that matches, or empty
	}{
		{"exact path", "linux", []string{"/home/u/proj"}, "/home/u/proj", "p0"},
		{"nested path", "linux", []string{"/home/u/proj"}, "/home/u/proj/internal/x", "p0"},
		{"worktree under the project", "linux", []string{"/home/u/proj"}, "/home/u/proj/.claude/worktrees/fix-1a2b", "p0"},
		{"sibling with a common prefix", "linux", []string{"/home/u/proj"}, "/home/u/project", ""},
		{"parent of the profile", "linux", []string{"/home/u/proj"}, "/home/u", ""},
		{"no match", "linux", []string{"/home/u/proj"}, "/srv/other", ""},
		{"no profiles", "linux", nil, "/home/u/proj", ""},
		{"longest wins, outer first", "linux", []string{"/home/u", "/home/u/proj"}, "/home/u/proj/x", "p1"},
		{"longest wins, inner first", "linux", []string{"/home/u/proj", "/home/u"}, "/home/u/proj/x", "p0"},
		{"outer applies beside the inner", "linux", []string{"/home/u/proj", "/home/u"}, "/home/u/other", "p1"},
		{"trailing separator on the profile", "linux", []string{"/home/u/proj/"}, "/home/u/proj", "p0"},
		{"trailing separator on the directory", "linux", []string{"/home/u/proj"}, "/home/u/proj/", "p0"},
		{"dot segments are cleaned", "linux", []string{"/home/u/./x/../proj"}, "/home/u/proj//sub/..", "p0"},
		{"the root holds everything", "linux", []string{"/"}, "/home/u", "p0"},
		{"the root is itself", "linux", []string{"/"}, "/", "p0"},
		{"case differs on Linux", "linux", []string{"/home/u/Proj"}, "/home/u/proj", ""},
		{"a backslash is a name on Linux", "linux", []string{`/home/u\proj`}, "/home/u/proj", ""},
		{"relative directory", "linux", []string{"/home/u/proj"}, "home/u/proj", ""},
		{"empty directory", "linux", []string{"/"}, "", ""},
		{"over-long directory", "linux", []string{"/"}, "/" + strings.Repeat("d", 4096), ""},

		{"case differs on macOS", "darwin", []string{"/Users/U/Proj"}, "/users/u/proj/src", "p0"},
		{"sibling on macOS", "darwin", []string{"/Users/u/proj"}, "/Users/u/PROJECT", ""},

		{"exact on Windows", "windows", []string{`C:\Users\u\proj`}, `C:\Users\u\proj`, "p0"},
		{"nested on Windows", "windows", []string{`C:\Users\u\proj`}, `C:\Users\u\proj\internal`, "p0"},
		{"worktree on Windows", "windows", []string{`D:\proj`}, `D:\proj\.claude\worktrees\fix-1a2b`, "p0"},
		{"sibling on Windows", "windows", []string{`C:\Users\u\proj`}, `C:\Users\u\proj2`, ""},
		{"forward slashes in the profile", "windows", []string{"C:/Users/u/proj"}, `C:\Users\u\proj\x`, "p0"},
		{"forward slashes in the directory", "windows", []string{`C:\Users\u\proj`}, "C:/Users/u/proj/x", "p0"},
		{"mixed separators", "windows", []string{`C:\Users/u\proj`}, `C:/Users\u/proj\x`, "p0"},
		{"case differs on Windows", "windows", []string{`C:\Users\U\Proj`}, `c:\users\u\PROJ\x`, "p0"},
		{"trailing separator on Windows", "windows", []string{`C:\Users\u\proj\`}, `C:\Users\u\proj`, "p0"},
		{"another drive", "windows", []string{`C:\proj`}, `D:\proj`, ""},
		{"a drive holds everything on it", "windows", []string{`C:\`}, `C:\Users\u`, "p0"},
		{"dot segments do not leave the drive", "windows", []string{`C:\proj`}, `C:\..\..\proj\x`, "p0"},
		{"longest wins on Windows", "windows", []string{`C:\a`, `C:\a\b`}, `c:/A/B/c`, "p1"},
		{"network share", "windows", []string{`\\server\share\proj`}, `//SERVER/share/proj/x`, "p0"},
		{"another share", "windows", []string{`\\server\share\proj`}, `\\server\other\proj`, ""},
		{"extended-length prefix", "windows", []string{`C:\proj`}, `\\?\C:\proj\x`, "p0"},
		{"no drive on Windows", "windows", []string{`C:\proj`}, `\proj`, ""},
		{"drive-relative directory", "windows", []string{`C:\proj`}, `C:proj`, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg, warnings := loadOn(t, tc.goos, projects(tc.profiles...))
			if len(cfg.Projects) != len(tc.profiles) || len(warnings) != 0 {
				t.Fatalf("loaded %d profiles, %v", len(cfg.Projects), warnings)
			}
			if got := cfg.Effective(tc.goos, tc.cwd).Name; got != tc.want {
				t.Errorf("Effective(%q) matched %q, want %q", tc.cwd, got, tc.want)
			}
		})
	}
}

func TestEffectiveSettings(t *testing.T) {
	file := `{
		"privacy": "standard",
		"projects": [
			{"path": "/work/open", "privacy": "full", "name": "Visions of Shuyi",
			 "areas": ["battle engine", "story"], "link": "https://github.com/owner/visions.git"},
			{"path": "/work/client", "privacy": "minimal"},
			{"path": "/work/plain", "name": "Plain"}
		]
	}`
	cfg, warnings := loadOn(t, "linux", file)
	if len(warnings) != 0 {
		t.Fatalf("warnings: %v", warnings)
	}
	cases := []struct {
		name string
		cwd  string
		want config.Settings
	}{
		{"a profile raises the level", "/work/open/src", config.Settings{
			Privacy: domain.PrivacyFull, Name: "Visions of Shuyi",
			Areas: []string{"battle engine", "story"}, Link: "https://github.com/owner/visions",
		}},
		{"a profile lowers the level", "/work/client", config.Settings{Privacy: domain.PrivacyMinimal}},
		{"a profile without a level keeps the global one", "/work/plain", config.Settings{Privacy: domain.PrivacyStandard, Name: "Plain"}},
		{"no profile gives the global settings", "/work/secret", config.Settings{Privacy: domain.PrivacyStandard}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := cfg.Effective("linux", tc.cwd); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("got %+v, want %+v", got, tc.want)
			}
		})
	}
	t.Run("the environment's level is the global one", func(t *testing.T) {
		cfg, _ := loadOn(t, "linux", file, "RICH_PRESENCE_PRIVACY", "full")
		if got := cfg.Effective("linux", "/work/secret").Privacy; got != domain.PrivacyFull {
			t.Errorf("unprofiled: got %q", got)
		}
		if got := cfg.Effective("linux", "/work/client").Privacy; got != domain.PrivacyMinimal {
			t.Errorf("profiled: got %q", got)
		}
	})
}

func TestOffHidesAProjectOrEverything(t *testing.T) {
	cases := []struct {
		name  string
		file  string
		hides bool
		want  map[string]domain.Privacy
	}{
		{"a profile hides its project", `{"projects": [{"path": "/work/secret", "privacy": "off"}]}`, true,
			map[string]domain.Privacy{"/work/secret/src": domain.PrivacyOff, "/work/other": domain.PrivacyStandard}},
		{"the global level hides everything but a profile that shows", `{"privacy": "off", "projects": [{"path": "/work/open", "privacy": "full"}]}`, true,
			map[string]domain.Privacy{"/work/open": domain.PrivacyFull, "/work/other": domain.PrivacyOff}},
		{"no level is off", `{"privacy": "minimal", "projects": [{"path": "/work/open", "privacy": "full"}]}`, false,
			map[string]domain.Privacy{"/work/open": domain.PrivacyFull, "/work/other": domain.PrivacyMinimal}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg, warnings := loadOn(t, "linux", tc.file)
			if len(warnings) != 0 {
				t.Fatalf("warnings: %v", warnings)
			}
			if got := cfg.Hides(); got != tc.hides {
				t.Errorf("Hides() = %v, want %v", got, tc.hides)
			}
			for cwd, want := range tc.want {
				if got := cfg.Effective("linux", cwd).Privacy; got != want {
					t.Errorf("%s: got %q, want %q", cwd, got, want)
				}
			}
		})
	}
	t.Run("the environment can set it", func(t *testing.T) {
		cfg, warnings := loadOn(t, "linux", `{}`, "RICH_PRESENCE_PRIVACY", "off")
		if len(warnings) != 0 || !cfg.Hides() {
			t.Errorf("Hides() = %v with warnings %v", cfg.Hides(), warnings)
		}
	})
}

func TestValidateLink(t *testing.T) {
	hosts := []string{"github.com", "gitlab.example.org"}
	cases := []struct {
		name string
		link string
		want string // empty when the link is rejected
	}{
		{"valid GitHub link", "https://github.com/owner/repo", "https://github.com/owner/repo"},
		{"trailing .git is removed", "https://github.com/owner/repo.git", "https://github.com/owner/repo"},
		{"dots, dashes and underscores", "https://github.com/my-org/my_repo.go", "https://github.com/my-org/my_repo.go"},
		{"an added host", "https://gitlab.example.org/group/repo", "https://gitlab.example.org/group/repo"},
		{"host and scheme in capitals", "HTTPS://GitHub.COM/Owner/Repo", "https://github.com/Owner/Repo"},
		{"segments at the length limit", "https://github.com/" + strings.Repeat("o", 100) + "/" + strings.Repeat("r", 100) + ".git",
			"https://github.com/" + strings.Repeat("o", 100) + "/" + strings.Repeat("r", 100)},

		{"http", "http://github.com/owner/repo", ""},
		{"another scheme", "ssh://github.com/owner/repo", ""},
		{"no scheme", "github.com/owner/repo", ""},
		{"the SSH form", "git@github.com:owner/repo.git", ""},
		{"scheme without slashes", "https:github.com/owner/repo", ""},
		{"empty scheme", ":github.com/owner/repo", ""},
		{"a user name", "https://user@github.com/owner/repo", ""},
		{"a user name and token", "https://user:ghp_token@github.com/owner/repo", ""},
		{"a query string", "https://github.com/owner/repo?tab=readme", ""},
		{"an empty query", "https://github.com/owner/repo?", ""},
		{"a fragment", "https://github.com/owner/repo#readme", ""},
		{"an empty fragment", "https://github.com/owner/repo#", ""},
		{"an extra path segment", "https://github.com/owner/repo/tree/main", ""},
		{"a trailing slash", "https://github.com/owner/repo/", ""},
		{"owner only", "https://github.com/owner", ""},
		{"no path", "https://github.com", ""},
		{"empty owner", "https://github.com//repo", ""},
		{"empty repository", "https://github.com/owner/", ""},
		{"repository that is only .git", "https://github.com/owner/.git", ""},
		{"repository that ends in .git twice", "https://github.com/owner/repo.git.git", ""},
		{"repository that is .git twice", "https://github.com/owner/.git.git", ""},
		{"dot owner", "https://github.com/./repo", ""},
		{"dot-dot repository", "https://github.com/owner/..", ""},
		{"a colon in the path", "https://github.com/owner/re:po", ""},
		{"a host not on the list", "https://example.com/owner/repo", ""},
		{"a host that ends with the allowed name", "https://evilgithub.com/owner/repo", ""},
		{"a subdomain of the allowed name", "https://gist.github.com/owner/repo", ""},
		{"a host that contains the allowed name", "https://github.com.evil.example/owner/repo", ""},
		{"a host that begins with the allowed name", "https://github.community/owner/repo", ""},
		{"a trailing dot on the host", "https://github.com./owner/repo", ""},
		{"a port", "https://github.com:443/owner/repo", ""},
		{"no host", "https:///owner/repo", ""},
		{"an escape", "https://github.com/owner/re%2Fpo", ""},
		{"a backslash", `https://github.com\@evil.example/owner/repo`, ""},
		{"a space", "https://github.com/owner/my repo", ""},
		{"a line break", "https://github.com/owner/repo\n", ""},
		{"a letter outside ASCII", "https://github.com/owner/répo", ""},
		{"an over-long owner", "https://github.com/" + strings.Repeat("o", 101) + "/repo", ""},
		{"an over-long repository", "https://github.com/owner/" + strings.Repeat("r", 101), ""},
		{"an over-long value", "https://github.com/owner/repo" + strings.Repeat("/", 512), ""},
		{"empty", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := config.ValidateLink(tc.link, hosts)
			if got != tc.want || ok != (tc.want != "") {
				t.Errorf("ValidateLink(%q) = %q, %v, want %q", tc.link, got, ok, tc.want)
			}
		})
	}
	t.Run("no hosts accept nothing", func(t *testing.T) {
		if got, ok := config.ValidateLink("https://github.com/owner/repo", nil); ok {
			t.Errorf("accepted as %q", got)
		}
	})
	t.Run("the limit is Discord's", func(t *testing.T) {
		if config.MaxLinkLen != 512 {
			t.Errorf("MaxLinkLen = %d", config.MaxLinkLen)
		}
	})
}

func FuzzValidateLink(f *testing.F) {
	f.Add("https://github.com/owner/repo.git", "github.com")
	f.Add("https://user:token@github.com/owner/repo", "github.com")
	f.Add("https://github.com/owner/repo?x=1#frag", "github.com")
	f.Add("https://github.com@evil.example/owner/repo", "evil.example")
	f.Add("HTTPS://GITHUB.COM/a/b", "github.com")
	f.Add("https://[::1]/a/b", "[::1]")
	f.Add("https://github.com/%40/%3f", "github.com")
	f.Add("\x00https://\xff/a/b", "\xff")
	f.Add("https:a/b", "")
	f.Add("https://githuB.Com/0/.git.git", "0")
	f.Fuzz(func(t *testing.T, link, host string) {
		got, ok := config.ValidateLink(link, []string{"github.com", host})
		if !ok {
			if got != "" {
				t.Errorf("rejected %q but returned %q", link, got)
			}
			return
		}
		if strings.ContainsAny(link, "@?#") || strings.ContainsAny(got, "@?#") {
			t.Errorf("accepted %q as %q", link, got)
		}
		if len(got) > config.MaxLinkLen || !strings.HasPrefix(got, "https://") || strings.Count(got, "/") != 4 {
			t.Errorf("accepted %q as %q", link, got)
		}
		if again, ok := config.ValidateLink(got, []string{"github.com", host}); !ok || again != got {
			t.Errorf("the result %q does not validate to itself: %q, %v", got, again, ok)
		}
	})
}

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
		{"at the cap", strings.Repeat("n", 128), strings.Repeat("n", 128)},
		{"over the cap", strings.Repeat("n", 129), strings.Repeat("n", 128)},
		{"cut between characters, not inside one", strings.Repeat("n", 127) + "é", strings.Repeat("n", 127)},
		{"no space is left at the cut", strings.Repeat("n", 127) + " x", strings.Repeat("n", 127)},
		{"a word that fits after a space", strings.Repeat("n", 126) + " x", strings.Repeat("n", 126) + " x"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := config.CleanName(tc.in); got != tc.want {
				t.Errorf("CleanName(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
	if config.MaxNameLen != domain.MaxProjectLen {
		t.Errorf("MaxNameLen = %d, want the project name's limit", config.MaxNameLen)
	}
}

func TestCleanAreas(t *testing.T) {
	cases := []struct {
		name     string
		in       []string
		want     []string
		wantOver bool
	}{
		{"plain", []string{"battle engine", "story"}, []string{"battle engine", "story"}, false},
		{"none", nil, nil, false},
		{"at the entry cap", []string{"1", "2", "3", "4", "5", "6", "7", "8"}, []string{"1", "2", "3", "4", "5", "6", "7", "8"}, false},
		{"over the entry cap", []string{"1", "2", "3", "4", "5", "6", "7", "8", "9", "10"}, []string{"1", "2", "3", "4", "5", "6", "7", "8"}, true},
		{"an over-long entry is cut", []string{strings.Repeat("a", 33)}, []string{strings.Repeat("a", 32)}, false},
		{"duplicates differing in case or spacing", []string{"Battle Engine", "battle  engine", " BATTLE ENGINE ", "story"}, []string{"Battle Engine", "story"}, false},
		{"duplicates after cleaning", []string{"story", "*story*", "sto\u200bry"}, []string{"story"}, false},
		{"empty after cleaning", []string{"", "  ", "**", "\x00", "ui"}, []string{"ui"}, false},
		{"what is dropped does not count toward the cap", []string{"1", "1", "", "2", "3", "4", "5", "6", "7", "8"}, []string{"1", "2", "3", "4", "5", "6", "7", "8"}, false},
		{"a repeat past the cap is not over", []string{"1", "2", "3", "4", "5", "6", "7", "8", "8", ""}, []string{"1", "2", "3", "4", "5", "6", "7", "8"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, over := config.CleanAreas(tc.in)
			if !reflect.DeepEqual(got, tc.want) || over != tc.wantOver {
				t.Errorf("CleanAreas(%q) = %q, %v, want %q, %v", tc.in, got, over, tc.want, tc.wantOver)
			}
		})
	}
}

func TestRejectedLinkWarnsOnceWithoutTheValue(t *testing.T) {
	const secret = "https://user:ghp_secret@github.com/owner/private"
	file := fmt.Sprintf(`{"projects": [
		{"path": "/a", "link": "https://github.com/owner/a"},
		{"path": "/b", "privacy": "minimal", "name": "B", "link": %q},
		{"path": "/c", "link": 7}
	]}`, secret)
	cfg, warnings := loadOn(t, "linux", file)
	const problem = "is not an accepted repository link and is not published"
	want := []config.Warning{fileWarning("projects[1].link", problem), fileWarning("projects[2].link", problem)}
	if !reflect.DeepEqual(warnings, want) {
		t.Errorf("got %v, want %v", warnings, want)
	}
	for _, w := range warnings {
		if strings.Contains(w.String(), "secret") || strings.Contains(w.String(), "owner") {
			t.Errorf("the warning echoes the link: %s", w)
		}
	}
	// The rest of the profile stands, so a bad link cannot raise its level.
	wantB := config.Settings{Privacy: domain.PrivacyMinimal, Name: "B"}
	if got := cfg.Effective("linux", "/b"); !reflect.DeepEqual(got, wantB) {
		t.Errorf("profile with the bad link: got %+v, want %+v", got, wantB)
	}
	if got := cfg.Effective("linux", "/a").Link; got != "https://github.com/owner/a" {
		t.Errorf("the good link: got %q", got)
	}
}

func TestBadProfileIsSkippedAndTheRestLoad(t *testing.T) {
	const skipped = "must have a path that is an absolute directory and was skipped"
	cases := []struct {
		name  string
		entry string
		warn  config.Warning
	}{
		{"not an object", `"/bad"`, fileWarning("projects[1]", "must be an object and was skipped")},
		{"null", `null`, fileWarning("projects[1]", "must be an object and was skipped")},
		{"no path", `{"name": "x"}`, fileWarning("projects[1]", skipped)},
		{"path of the wrong type", `{"path": 1}`, fileWarning("projects[1]", skipped)},
		{"empty path", `{"path": ""}`, fileWarning("projects[1]", skipped)},
		{"relative path", `{"path": "work/bad"}`, fileWarning("projects[1]", skipped)},
		{"home shorthand", `{"path": "~/bad"}`, fileWarning("projects[1]", skipped)},
		{"over-long path", `{"path": "/` + strings.Repeat("p", 4096) + `"}`, fileWarning("projects[1]", skipped)},
		{"duplicate path", `{"path": "/first"}`, fileWarning("projects[1]", "repeats the path of an earlier profile and was skipped")},
		{"duplicate after cleaning", `{"path": "/x/../first/", "name": "again"}`, fileWarning("projects[1]", "repeats the path of an earlier profile and was skipped")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			file := `{"projects": [{"path": "/first", "name": "first"}, ` + tc.entry + `, {"path": "/last", "name": "last"}]}`
			cfg, warnings := loadOn(t, "linux", file)
			if !reflect.DeepEqual(warnings, []config.Warning{tc.warn}) {
				t.Errorf("got %v, want %v", warnings, tc.warn)
			}
			if len(cfg.Projects) != 2 || cfg.Effective("linux", "/first").Name != "first" || cfg.Effective("linux", "/last").Name != "last" {
				t.Errorf("profiles: %+v", cfg.Projects)
			}
			if got := cfg.Effective("linux", "/bad"); !reflect.DeepEqual(got, config.Settings{Privacy: domain.PrivacyStandard}) {
				t.Errorf("the skipped profile applies: %+v", got)
			}
		})
	}
	t.Run("a duplicate that differs in case on Windows", func(t *testing.T) {
		cfg, warnings := loadOn(t, "windows", projects(`C:\Proj`, `c:/proj/`))
		want := []config.Warning{fileWarning("projects[1]", "repeats the path of an earlier profile and was skipped")}
		if len(cfg.Projects) != 1 || !reflect.DeepEqual(warnings, want) {
			t.Errorf("got %+v, %v", cfg.Projects, warnings)
		}
	})
	t.Run("not a duplicate on Linux", func(t *testing.T) {
		cfg, warnings := loadOn(t, "linux", projects("/Proj", "/proj"))
		if len(cfg.Projects) != 2 || len(warnings) != 0 {
			t.Errorf("got %+v, %v", cfg.Projects, warnings)
		}
	})
}

func TestBadProfileFieldIsLeftUnset(t *testing.T) {
	cases := []struct {
		name  string
		entry string
		warn  config.Warning
		want  config.Settings
	}{
		{"unknown level", `"privacy": "loud", "name": "N"`,
			fileWarning("projects[0].privacy", "must be off, minimal, standard or full"),
			config.Settings{Privacy: domain.PrivacyFull, Name: "N"}},
		{"level of the wrong type", `"privacy": 2`,
			fileWarning("projects[0].privacy", "must be off, minimal, standard or full"),
			config.Settings{Privacy: domain.PrivacyFull}},
		{"name of the wrong type", `"privacy": "minimal", "name": ["N"]`,
			fileWarning("projects[0].name", "must be text with something to show"),
			config.Settings{Privacy: domain.PrivacyMinimal}},
		{"name that is empty after cleaning", `"name": "**\n"`,
			fileWarning("projects[0].name", "must be text with something to show"),
			config.Settings{Privacy: domain.PrivacyFull}},
		{"areas of the wrong type", `"areas": "story", "name": "N"`,
			fileWarning("projects[0].areas", "must be a list of strings"),
			config.Settings{Privacy: domain.PrivacyFull, Name: "N"}},
		{"an area of the wrong type", `"areas": ["story", 2]`,
			fileWarning("projects[0].areas", "must be a list of strings"),
			config.Settings{Privacy: domain.PrivacyFull}},
		{"too many areas", `"areas": ["1", "2", "3", "4", "5", "6", "7", "8", "9"]`,
			fileWarning("projects[0].areas", "has more than 8 entries, and the rest are ignored"),
			config.Settings{Privacy: domain.PrivacyFull, Areas: []string{"1", "2", "3", "4", "5", "6", "7", "8"}}},
		{"an unknown setting", `"name": "N", "personality": "pirate", "LINK": "x"`,
			fileWarning("projects[0]", "has a setting that is not known"),
			config.Settings{Privacy: domain.PrivacyFull, Name: "N"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg, warnings := loadOn(t, "linux", `{"privacy": "full", "projects": [{"path": "/p", `+tc.entry+`}]}`)
			if !reflect.DeepEqual(warnings, []config.Warning{tc.warn}) {
				t.Errorf("got %v, want %v", warnings, tc.warn)
			}
			if got := cfg.Effective("linux", "/p"); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("got %+v, want %+v", got, tc.want)
			}
		})
	}
	t.Run("areas are cleaned without a warning", func(t *testing.T) {
		cfg, warnings := loadOn(t, "linux", `{"projects": [{"path": "/p", "areas": ["Story", "story ", "", "`+strings.Repeat("a", 40)+`"]}]}`)
		want := []string{"Story", strings.Repeat("a", 32)}
		if got := cfg.Effective("linux", "/p").Areas; !reflect.DeepEqual(got, want) || len(warnings) != 0 {
			t.Errorf("got %q, %v", got, warnings)
		}
	})
	t.Run("every problem in a profile is reported", func(t *testing.T) {
		_, warnings := loadOn(t, "linux", `{"projects": [{"path": "/p", "privacy": "", "name": "", "areas": null, "link": "", "x": 1}]}`)
		want := []config.Warning{
			fileWarning("projects[0].privacy", "must be off, minimal, standard or full"),
			fileWarning("projects[0].name", "must be text with something to show"),
			fileWarning("projects[0].areas", "must be a list of strings"),
			fileWarning("projects[0].link", "is not an accepted repository link and is not published"),
			fileWarning("projects[0]", "has a setting that is not known"),
		}
		if !reflect.DeepEqual(warnings, want) {
			t.Errorf("got %v, want %v", warnings, want)
		}
	})
}

func TestProjectsSetting(t *testing.T) {
	for _, value := range []string{`{}`, `"/p"`, `null`, `7`} {
		cfg, warnings := loadOn(t, "linux", `{"projects": `+value+`, "privacy": "full"}`)
		want := []config.Warning{fileWarning("projects", "must be a list of profiles")}
		if len(cfg.Projects) != 0 || cfg.Privacy != domain.PrivacyFull || !reflect.DeepEqual(warnings, want) {
			t.Errorf("%s: got %+v, %v", value, cfg.Projects, warnings)
		}
	}
	t.Run("an empty list", func(t *testing.T) {
		cfg, warnings := loadOn(t, "linux", `{"projects": []}`)
		if !isDefault(cfg) || len(warnings) != 0 {
			t.Errorf("got %+v, %v", cfg, warnings)
		}
	})
	t.Run("more than the limit", func(t *testing.T) {
		paths := make([]string, config.MaxProfiles+2)
		for i := range paths {
			paths[i] = fmt.Sprintf("/p/%d", i)
		}
		cfg, warnings := loadOn(t, "linux", projects(paths...))
		want := []config.Warning{fileWarning("projects", "has more than 64 entries, and the rest are ignored")}
		if len(cfg.Projects) != config.MaxProfiles || !reflect.DeepEqual(warnings, want) {
			t.Errorf("got %d profiles, %v", len(cfg.Projects), warnings)
		}
		if got := cfg.Effective("linux", "/p/63").Name; got != "p63" {
			t.Errorf("the last profile kept: got %q", got)
		}
		if got := cfg.Effective("linux", "/p/64").Name; got != "" {
			t.Errorf("the first profile past the limit applies: %q", got)
		}
	})
}

func TestLinkHosts(t *testing.T) {
	file := `{
		"link_hosts": ["GitLab.Example.org", "codeberg.org", "github.com", "codeberg.org"],
		"projects": [
			{"path": "/a", "link": "https://gitlab.example.org/group/a"},
			{"path": "/b", "link": "https://github.com/owner/b"},
			{"path": "/c", "link": "https://bitbucket.org/owner/c"}
		]
	}`
	cfg, warnings := loadOn(t, "linux", file)
	if want := []string{"github.com", "gitlab.example.org", "codeberg.org"}; !reflect.DeepEqual(cfg.LinkHosts, want) {
		t.Errorf("LinkHosts = %q, want %q", cfg.LinkHosts, want)
	}
	want := []config.Warning{fileWarning("projects[2].link", "is not an accepted repository link and is not published")}
	if !reflect.DeepEqual(warnings, want) {
		t.Errorf("got %v, want %v", warnings, want)
	}
	for cwd, link := range map[string]string{"/a": "https://gitlab.example.org/group/a", "/b": "https://github.com/owner/b", "/c": ""} {
		if got := cfg.Effective("linux", cwd).Link; got != link {
			t.Errorf("%s: link %q, want %q", cwd, got, link)
		}
	}

	t.Run("the order in the file does not matter", func(t *testing.T) {
		cfg, warnings := loadOn(t, "linux", `{"projects": [{"path": "/a", "link": "https://codeberg.org/o/a"}], "link_hosts": ["codeberg.org"]}`)
		if got := cfg.Effective("linux", "/a").Link; got != "https://codeberg.org/o/a" || len(warnings) != 0 {
			t.Errorf("got %q, %v", got, warnings)
		}
	})
	t.Run("invalid hosts are skipped with a warning each", func(t *testing.T) {
		bad := []string{
			"", "https://gitlab.com", "gitlab.com/group", "gitlab.com:443", "user@gitlab.com", "*.gitlab.com",
			".gitlab.com", "gitlab.com.", "git..lab", "-gitlab.com", "gitlab-.com", "gitläb.com", "git lab.com",
			strings.Repeat("a", 64) + ".com", strings.Repeat("a.", 127),
		}
		list, _ := json.Marshal(append(bad, "ok.example"))
		cfg, warnings := loadOn(t, "linux", `{"link_hosts": `+string(list)+`}`)
		if want := []string{"github.com", "ok.example"}; !reflect.DeepEqual(cfg.LinkHosts, want) {
			t.Errorf("LinkHosts = %q, want %q", cfg.LinkHosts, want)
		}
		if len(warnings) != len(bad) {
			t.Fatalf("got %d warnings, want %d: %v", len(warnings), len(bad), warnings)
		}
		for i, w := range warnings {
			if want := fileWarning(fmt.Sprintf("link_hosts[%d]", i), "must be a host name such as gitlab.com"); w != want {
				t.Errorf("got %v, want %v", w, want)
			}
		}
	})
	t.Run("labels and names at their length limits", func(t *testing.T) {
		label := strings.Repeat("a", 63)
		host := label + "." + label + "." + label + "." + strings.Repeat("b", 61)
		cfg, warnings := loadOn(t, "linux", `{"link_hosts": ["`+host+`"]}`)
		if len(host) != 253 || len(cfg.LinkHosts) != 2 || len(warnings) != 0 {
			t.Errorf("a %d-byte host: got %q, %v", len(host), cfg.LinkHosts, warnings)
		}
	})
	t.Run("wrong type", func(t *testing.T) {
		for _, value := range []string{`"gitlab.com"`, `[1]`, `null`, `{"gitlab.com": true}`} {
			cfg, warnings := loadOn(t, "linux", `{"link_hosts": `+value+`}`)
			want := []config.Warning{fileWarning("link_hosts", "must be a list of host names")}
			if !isDefault(cfg) || !reflect.DeepEqual(warnings, want) {
				t.Errorf("%s: got %q, %v", value, cfg.LinkHosts, warnings)
			}
		}
	})
	t.Run("more than the limit", func(t *testing.T) {
		hosts := make([]string, config.MaxLinkHosts+1)
		for i := range hosts {
			hosts[i] = fmt.Sprintf("h%d.example", i)
		}
		list, _ := json.Marshal(hosts)
		cfg, warnings := loadOn(t, "linux", `{"link_hosts": `+string(list)+`}`)
		want := []config.Warning{fileWarning("link_hosts", "has more than 16 entries, and the rest are ignored")}
		if len(cfg.LinkHosts) != config.MaxLinkHosts+1 || cfg.LinkHosts[16] != "h15.example" || !reflect.DeepEqual(warnings, want) {
			t.Errorf("got %q, %v", cfg.LinkHosts, warnings)
		}
	})
	t.Run("the default cannot be changed through a loaded configuration", func(t *testing.T) {
		loadOn(t, "linux", `{"link_hosts": ["codeberg.org"]}`)
		if got := config.Default().LinkHosts; !reflect.DeepEqual(got, []string{config.DefaultLinkHost}) {
			t.Errorf("Default().LinkHosts = %q", got)
		}
	})
}

func TestEnvironmentDoesNotDefineProfiles(t *testing.T) {
	cfg, warnings := loadOn(t, "linux", "{}",
		"RICH_PRESENCE_PROJECTS", `[{"path": "/p", "privacy": "full"}]`,
		"RICH_PRESENCE_LINK_HOSTS", `["evil.example"]`,
		"RICH_PRESENCE_LINK", "https://github.com/owner/repo",
	)
	if !isDefault(cfg) || len(warnings) != 0 {
		t.Errorf("got %+v, %v", cfg, warnings)
	}
}

func TestProfileWarningsKeepTheirPlace(t *testing.T) {
	_, warnings := loadOn(t, "linux", `{"zeta": 1, "projects": [1], "link_hosts": [""], "privacy": "loud"}`)
	want := []config.Warning{
		fileWarning("privacy", "must be off, minimal, standard or full"),
		fileWarning("link_hosts[0]", "must be a host name such as gitlab.com"),
		fileWarning("projects[0]", "must be an object and was skipped"),
		fileWarning("zeta", "is not a known setting"),
	}
	if !reflect.DeepEqual(warnings, want) {
		t.Errorf("got %v, want %v", warnings, want)
	}
}

// TestProfilesReadNothingFromTheProject shows that the only file read is the
// configuration file: Effective has no way to read one, and loading profiles
// opens nothing under their paths.
func TestProfilesReadNothingFromTheProject(t *testing.T) {
	files := &fakeFiles{data: `{"projects": [{"path": "/work/proj", "name": "P", "link": "https://github.com/o/r"}]}`}
	cfg, dirs, _ := config.Load("linux", env(), files)
	cfg.Effective("linux", "/work/proj/sub")
	if !reflect.DeepEqual(files.read, []string{dirs.File}) {
		t.Errorf("read %q, want only %q", files.read, dirs.File)
	}
}

func FuzzEffective(f *testing.F) {
	f.Add(`{"projects": [{"path": "/a/b", "privacy": "full", "name": "N", "areas": ["x"], "link": "https://github.com/o/r"}]}`, "/a/b/c", "linux")
	f.Add(`{"projects": [{"path": "C:\\a"}, {"path": "c:/A/b", "name": "*"}], "link_hosts": ["x.y"]}`, `C:\a\B\..`, "windows")
	f.Add(`{"projects": [{"path": "//s/h"}, {"path": "/"}, 1, null, {"path": 2}]}`, `\\?\UNC\s\h`, "windows")
	f.Add(`{"projects": {"path": "/"}, "link_hosts": "x"}`, "/", "darwin")
	f.Add(`{"link_hosts": ["\u00e9", "."], "projects": [{"path": "/", "link": "https://./a/b", "areas": ["\ud800"]}]}`, "\x00", "")
	f.Add("{\"\x8d\x8d\x8d\x8d\x8d\x8d\x8d\x8d\x8d\x8d\x8d\x8d\x8d\x8d\x8d\x8d\x8d\x8d0000000\":\"\"}", "0", "0")
	f.Fuzz(func(t *testing.T, file, cwd, goos string) {
		cfg, _, warnings := config.Load(goos, env("USERPROFILE", `C:\Users\u`), &fakeFiles{data: file})
		for _, w := range warnings {
			// The name of an unknown setting is the user's text, cut short.
			if w.Problem != config.ProblemUnknownSetting && len(w.Setting) > 60 || len(w.Problem) > 80 {
				t.Errorf("a long warning: %s", w)
			}
		}
		if len(cfg.Projects) > config.MaxProfiles || cfg.LinkHosts[0] != config.DefaultLinkHost {
			t.Errorf("got %d profiles and hosts %q", len(cfg.Projects), cfg.LinkHosts)
		}
		for _, p := range cfg.Projects {
			if p.Privacy != "" && !p.Privacy.Valid() {
				t.Errorf("profile privacy %q", p.Privacy)
			}
			if len(p.Name) > config.MaxNameLen || len(p.Areas) > config.MaxAreas || strings.ContainsAny(p.Link, "@?#") {
				t.Errorf("profile out of bounds: %+v", p)
			}
		}
		if s := cfg.Effective(goos, cwd); !s.Privacy.Valid() {
			t.Errorf("Effective privacy %q", s.Privacy)
		}
	})
}
