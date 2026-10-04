package diag_test

import (
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Zafnok/claude-rich-presence/internal/config"
	"github.com/Zafnok/claude-rich-presence/internal/diag"
	"github.com/Zafnok/claude-rich-presence/internal/domain"
)

func TestLogLine(t *testing.T) {
	var out lines
	log := diag.NewLogger(&out, config.LogWarn, now)
	log.Warn("discord connection lost",
		diag.State("discord", "reconnecting"),
		diag.ErrorClass("timeout"),
		diag.Count("reconnects", 3),
		diag.Duration("backoff", 2*time.Second),
		diag.Version("host_version", "v1.2.3"),
		diag.Attr{},
	)
	want := `time=2026-10-03T12:00:00.000Z level=WARN msg="discord connection lost" discord=reconnecting error=timeout reconnects=3 backoff=2s host_version=v1.2.3` + "\n"
	if len(out.got) != 1 || out.got[0] != want {
		t.Errorf("wrote %q\nwant one write of %q", out.got, want)
	}
}

func TestLoggerWritesItsLevelAndAbove(t *testing.T) {
	methods := []string{"DEBUG", "INFO", "WARN", "ERROR"}
	tests := []struct {
		level config.LogLevel
		want  []string
	}{
		{config.LogDebug, methods},
		{config.LogInfo, methods[1:]},
		{config.LogWarn, methods[2:]},
		{config.LogError, methods[3:]},
		{"", methods[2:]},
		{"loud", methods[2:]},
	}
	for _, tt := range tests {
		t.Run(string(tt.level), func(t *testing.T) {
			var out lines
			log := diag.NewLogger(&out, tt.level, now)
			log.Debug("m")
			log.Info("m")
			log.Warn("m")
			log.Error("m")
			var got []string
			for _, line := range out.got {
				for _, m := range methods {
					if strings.Contains(line, "level="+m+" ") {
						got = append(got, m)
					}
				}
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("logged %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNilLoggerDiscards(t *testing.T) {
	var log *diag.Logger
	log.Error("m", diag.Count("n", 1))
	log.Warn("m")
	log.Info("m")
	log.Debug("m")
}

func TestCleanVersion(t *testing.T) {
	tests := []struct{ in, want string }{
		{"v1.2.3", "v1.2.3"},
		{"1.2.3", "1.2.3"},
		{"(devel)", "(devel)"},
		{"v0.0.0-20261003120000-abcdef123456+dirty", "v0.0.0-20261003120000-abcdef123456+dirty"},
		{"unknown", "unknown"},
		{"a_b", "a_b"},
		{"AZaz09", "AZaz09"},
		{strings.Repeat("1", 64), strings.Repeat("1", 64)},
		{strings.Repeat("1", 65), diag.InvalidVersion},
		{"", diag.InvalidVersion},
		{"v1 2", diag.InvalidVersion},
		{"/home/u/project", diag.InvalidVersion},
		{`C:\project`, diag.InvalidVersion},
		{"v1\n", diag.InvalidVersion},
		{"v1é", diag.InvalidVersion},
		{"a=b", diag.InvalidVersion},
		{"`", diag.InvalidVersion},
		{"{", diag.InvalidVersion},
		{"@", diag.InvalidVersion},
		{"[", diag.InvalidVersion},
		{"*", diag.InvalidVersion},
		{":", diag.InvalidVersion},
	}
	for _, tt := range tests {
		if got := diag.CleanVersion(tt.in); got != tt.want {
			t.Errorf("CleanVersion(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

// marker stands for work content. It is shaped like what the forbidden
// fields hold: a path, a sentence.
const marker = "/home/zed/Secret Project"

// TestForbiddenFieldsNeverReachTheLog puts the marker in every string field
// of an event and then logs about that event in every way the interface
// offers. The interface takes no event and no string from one, except as a
// version, so that is the only way in to try.
func TestForbiddenFieldsNeverReachTheLog(t *testing.T) {
	event := domain.Event{At: start, Kind: domain.KindToolStarted, Tool: domain.ToolNone}
	v := reflect.ValueOf(&event).Elem()
	seeded := 0
	for i := 0; i < v.NumField(); i++ {
		if v.Field(i).Kind() == reflect.String {
			v.Field(i).SetString(marker)
			seeded++
		}
	}
	if seeded < 7 {
		t.Fatalf("seeded %d string fields, want every one of the event's", seeded)
	}

	dir := t.TempDir()
	log := diag.Open(diag.OSFS{}, dir, config.LogDebug, now)
	var counters diag.Counters
	counters.EventReceived()
	if err := event.Validate(); err != nil {
		counters.EventDropped()
		log.Warn("event dropped", diag.ErrorClass("invalid event"))
	}
	for i := 0; i < v.NumField(); i++ {
		if v.Field(i).Kind() == reflect.String {
			log.Error("event", diag.Version("field", v.Field(i).String()))
		}
	}
	log.Debug("counters", counters.Snapshot().Attrs()...)

	data, err := os.ReadFile(diag.LogPath(dir))
	if err != nil {
		t.Fatal(err)
	}
	got := string(data)
	if n := strings.Count(got, "\n"); n != seeded+2 {
		t.Errorf("log has %d lines, want %d:\n%s", n, seeded+2, got)
	}
	for _, part := range []string{marker, "zed", "Secret", "Project"} {
		if strings.Contains(got, part) {
			t.Errorf("log contains %q:\n%s", part, got)
		}
	}
}

// TestInterfaceRefusesEventsAndFreeStrings type-checks small programs
// against the package. Each of the refused ones must fail to compile, and
// the accepted one shows that the check itself works.
func TestInterfaceRefusesEventsAndFreeStrings(t *testing.T) {
	const preamble = `package p

import (
	"log/slog"
	"time"

	"github.com/Zafnok/claude-rich-presence/internal/diag"
	"github.com/Zafnok/claude-rich-presence/internal/domain"
)

var (
	log   *diag.Logger
	event domain.Event
	text  string
	_     = slog.String
	_     = time.Second
)

func f() {
`
	tests := []struct {
		name, body string
		compiles   bool
	}{
		{"constants", `const state = "connected"
			log.Warn("lost", diag.State("discord", state), diag.ErrorClass("timeout"),
				diag.Count("n", 1), diag.Duration("d", time.Second), diag.Version("v", text))`, true},
		{"an event as an attribute", `log.Warn("m", event)`, false},
		{"a string as an attribute", `log.Warn("m", text)`, false},
		{"a standard library attribute", `log.Warn("m", slog.String("k", text))`, false},
		{"a standard library attribute inside Attr", `log.Warn("m", diag.Attr{slog.String("k", text)})`, false},
		{"an event field as the message", `log.Warn(event.Project)`, false},
		{"a string variable as the message", `log.Error(text)`, false},
		{"a built string as the message", `log.Info("project " + text)`, false},
		{"a string variable as a state", `log.Debug("m", diag.State("k", text))`, false},
		{"a typed string as a state", `log.Debug("m", diag.State("k", event.Kind))`, false},
		{"a string variable as a key", `log.Debug("m", diag.Count(text, 1))`, false},
		{"a string variable as an error class", `log.Debug("m", diag.ErrorClass(text))`, false},
		{"an error's text as an error class", `log.Debug("m", diag.ErrorClass(event.Validate().Error()))`, false},
		{"a string variable as a duration key", `log.Debug("m", diag.Duration(text, 0))`, false},
		{"a string variable as a version key", `log.Debug("m", diag.Version(text, "v1"))`, false},
		{"a conversion to the message type", `log.Warn(diag.text(text))`, false},
	}
	fset := token.NewFileSet()
	conf := types.Config{Importer: importer.ForCompiler(fset, "source", nil)}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			file, err := parser.ParseFile(fset, "p.go", preamble+tt.body+"\n}\n", 0)
			if err != nil {
				t.Fatal(err)
			}
			_, err = conf.Check("p", fset, []*ast.File{file}, nil)
			if tt.compiles && err != nil {
				t.Errorf("does not compile: %v", err)
			}
			if !tt.compiles && err == nil {
				t.Error("compiles, and must not")
			}
		})
	}
}
