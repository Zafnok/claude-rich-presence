package diag_test

import (
	"io"
	"strings"
	"testing"

	"github.com/Zafnok/claude-rich-presence/internal/config"
	"github.com/Zafnok/claude-rich-presence/internal/diag"
)

func TestRedact(t *testing.T) {
	tests := []struct {
		name, path, home, want string
	}{
		{"inside home", "/home/zed/.config/rich-presence", "/home/zed", "~/.config/rich-presence"},
		{"home itself", "/home/zed", "/home/zed", "~"},
		{"home with a trailing separator", "/home/zed/logs", "/home/zed/", "~/logs"},
		{"windows", `C:\Users\Zed\.rich-presence\logs`, `C:\Users\Zed`, `~\.rich-presence\logs`},
		{"windows, other case", `c:\users\zed\AppData\Local\Temp\rich-presence`, `C:\Users\Zed`, `~\AppData\Local\Temp\rich-presence`},
		{"windows, forward slashes", `C:/Users/Zed/x`, `C:/Users/Zed`, `~/x`},
		{"a sibling whose name starts the same", "/home/zedekiah/x", "/home/zed", "/home/zedekiah/x"},
		{"the user's name outside home", "/tmp/zed/rich-presence", "/home/zed", "/tmp/<user>/rich-presence"},
		{"the user's name last", "/var/run/Zed", "/home/zed", "/var/run/<user>"},
		{"the user's name first", `zed\x`, "/home/zed", `<user>\x`},
		{"the home path spelled with other separators", `\home\zed\x`, "/home/zed", `\home\<user>\x`},
		{"nothing to redact", "/run/user/1000/rich-presence", "/home/zed", "/run/user/1000/rich-presence"},
		{"shorter than home", "/run", "/home/zed", "/run"},
		{"no home", "/home/zed/x", "", "/home/zed/x"},
		{"home is the root", "/home/zed/x", "/", "/home/zed/x"},
		{"home without a separator", "zed/x", "zed", "~/x"},
		{"empty path", "", "/home/zed", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := diag.Redact(tt.path, tt.home); got != tt.want {
				t.Errorf("Redact(%q, %q) = %q, want %q", tt.path, tt.home, got, tt.want)
			}
		})
	}
}

func TestFormat(t *testing.T) {
	report := diag.Report{
		Version: "v1.2.3",
		Findings: []diag.Finding{
			{Check: diag.CheckConfig, Detail: "loaded without warnings", Path: "/home/zed/.config/rich-presence/config.json"},
			{Check: diag.CheckRuntime, Result: diag.Warn, Detail: "has not been created yet", Action: "Start a Claude session", Path: "/tmp/zed/rich-presence"},
			{Check: diag.CheckEndpoint, Result: diag.Fail, Detail: "no Discord pipe or socket was found", Action: "Start the Discord desktop app"},
			{Check: diag.CheckHost, Detail: "a host answered, with 2 sessions"},
		},
		LogFile: "/home/zed/.config/rich-presence/logs/rich-presence.log",
	}
	want := `rich-presence doctor, version v1.2.3

[pass] Configuration: loaded without warnings
       At:  ~/.config/rich-presence/config.json
[warn] Runtime directory: has not been created yet
       At:  /tmp/<user>/rich-presence
       Try: Start a Claude session
[fail] Discord endpoint: no Discord pipe or socket was found
       Try: Start the Discord desktop app
[pass] Host: a host answered, with 2 sessions

Log file: ~/.config/rich-presence/logs/rich-presence.log
Result: fail
`
	if got := report.Format("/home/zed"); got != want {
		t.Errorf("Format =\n%s\nwant\n%s", got, want)
	}
}

func TestWorst(t *testing.T) {
	findings := func(results ...diag.Result) diag.Report {
		var r diag.Report
		for _, result := range results {
			r.Findings = append(r.Findings, diag.Finding{Result: result})
		}
		return r
	}
	tests := []struct {
		report diag.Report
		want   diag.Result
	}{
		{findings(), diag.Pass},
		{findings(diag.Pass, diag.Pass), diag.Pass},
		{findings(diag.Pass, diag.Warn, diag.Pass), diag.Warn},
		{findings(diag.Fail, diag.Warn), diag.Fail},
	}
	for _, tt := range tests {
		if got := tt.report.Worst(); got != tt.want {
			t.Errorf("Worst of %+v = %v, want %v", tt.report.Findings, got, tt.want)
		}
	}
}

// TestReportNamesNoUserAndNoProject runs the checks on each operating
// system's real directory layout, with everything going wrong that can put
// text into the report, and looks for the user's name and for a project name
// seeded wherever the checks read.
func TestReportNamesNoUserAndNoProject(t *testing.T) {
	const user, project = "zedekiah", "secret-project"
	tests := []struct {
		goos, homeVar, home, runtimeDir string
	}{
		{"linux", "HOME", "/home/" + user, "/run/user/1000/rich-presence"},
		{"linux", "HOME", "/home/" + user, "/tmp/" + user + "/rich-presence"},
		{"darwin", "HOME", "/Users/" + user, "/var/folders/ab/cdef/T/rich-presence"},
		{"windows", "USERPROFILE", `C:\Users\` + user, `c:\users\` + strings.ToUpper(user) + `\AppData\Local\Temp\rich-presence`},
	}
	states := []diag.DirState{diag.DirReady, diag.DirMissing, diag.DirUnsafe, diag.DirUnresolved}
	for _, tt := range tests {
		for _, state := range states {
			getenv := func(key string) string {
				switch key {
				case tt.homeVar:
					return tt.home
				case "RICH_PRESENCE_PRIVACY":
					return project
				}
				return ""
			}
			files := configFiles{`{"` + project + `":true,"log_level":"` + project + `"}`}
			ports := diag.Ports{
				Config: func() (config.Config, config.Dirs, []config.Warning) {
					return config.Load(tt.goos, getenv, files)
				},
				RuntimeDir:       func() (string, diag.DirState) { return tt.runtimeDir, state },
				DiscordEndpoints: func() (int, error) { return 0, nil },
				DialDiscord: func() (io.ReadWriteCloser, error) {
					return answering(t, closeWith(`{"code":4001,"message":"`+project+`"}`)), nil
				},
				Host: func() diag.HostAnswer {
					return diag.HostAnswer{State: diag.HostAnswered, Version: tt.home + "/" + project}
				},
			}
			got := diag.Run(ports, tt.home+`\`+project).Format(tt.home)
			for _, secret := range []string{user, project} {
				if strings.Contains(strings.ToLower(got), secret) {
					t.Errorf("%s: the report contains %q:\n%s", tt.goos, secret, got)
				}
			}
			if !strings.Contains(got, "~") || !strings.Contains(got, "3 ignored") {
				t.Errorf("%s: the report lacks a redacted path or the warnings:\n%s", tt.goos, got)
			}
		}
	}
}
