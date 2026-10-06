package protocol_test

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/Zafnok/claude-rich-presence/internal/control/protocol"
	"github.com/Zafnok/claude-rich-presence/internal/domain"
)

const sessionID = "3f2a9c1e-7b64-4d0a-9e51-0c8d2b6a4f17"

// examples is one of each message, keyed by the name of its golden file.
func examples() map[string]protocol.Message {
	return map[string]protocol.Message{
		"hello":   protocol.Hello{Protocol: 1, Version: "1.4.0"},
		"welcome": protocol.Welcome{Protocol: 1, Version: "1.3.2"},
		"refuse":  protocol.Refuse{Reason: protocol.ReasonUnsupportedProtocol},
		"sync": protocol.Sync{Session: &protocol.SessionState{
			ID: sessionID, Surface: "code", Status: "working", Tool: "editing",
			Model: "opus", Project: "example-project", Privacy: "standard",
			Start: 1790985600000, LastActivity: 1790985723500, Subagents: 2,
		}},
		"sync_none": protocol.Sync{},
		"event": protocol.Event{Event: protocol.EventData{
			SessionID: sessionID, Surface: "code", At: 1790985723500,
			Kind: "tool_started", Tool: "editing",
		}},
		"stand_down": protocol.StandDown{},
		"status":     protocol.Status{},
		"status_result": protocol.StatusResult{
			Discord: protocol.DiscordConnected, Sessions: 3, Version: "1.4.0", UptimeSeconds: 5400,
			Pause: &protocol.PauseState{Paused: true, Until: 1790989323500, At: 1790985723500},
		},
		"pause":               protocol.Pause{Until: 1790989323500, At: 1790985723500},
		"pause_until_resumed": protocol.Pause{At: 1790985723500},
		"resume":              protocol.Resume{At: 1790985999000},
		"preview":             protocol.Preview{},
		"preview_result": protocol.PreviewResult{
			Shown: true, Details: "Claude Code · example-project", State: "Editing files · Opus · 2 sessions",
			Start: 1790985600000, LargeImage: "logo", LargeText: "Claude Code",
			SmallImage: "working", SmallText: "Editing files",
			ButtonLabel: "View on GitHub", ButtonURL: "https://github.com/owner/example-project",
		},
		"preview_result_nothing": protocol.PreviewResult{},
	}
}

func encode(t *testing.T, m protocol.Message) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := protocol.Encode(&buf, m); err != nil {
		t.Fatalf("Encode(%#v): %v", m, err)
	}
	return buf.Bytes()
}

func golden(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "golden", name+".json"))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestGoldenFilesMatchTheCodec(t *testing.T) {
	files, err := filepath.Glob(filepath.Join("testdata", "golden", "*.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != len(examples()) {
		t.Errorf("%d golden files, want one for each of %d examples", len(files), len(examples()))
	}
	types := map[string]bool{}
	for name, m := range examples() {
		t.Run(name, func(t *testing.T) {
			want := golden(t, name)
			if got := encode(t, m); !bytes.Equal(got, want) {
				t.Errorf("encoded\n got %s\nwant %s", got, want)
			}
			got, err := protocol.Decode(want)
			if err != nil {
				t.Fatalf("Decode: %v", err)
			}
			if !reflect.DeepEqual(got, m) {
				t.Errorf("decoded\n got %#v\nwant %#v", got, m)
			}
			if !strings.Contains(string(want), `{"type":"`+m.Type()+`"`) {
				t.Errorf("Type() = %q, not the type in %s", m.Type(), want)
			}
		})
		types[m.Type()] = true
	}
	if len(types) != 12 {
		t.Errorf("examples cover %d message types, want 12", len(types))
	}
}

// TestSpecificationShowsEveryGoldenLine ties the document to the code: each
// example in it is a golden file, byte for byte.
func TestSpecificationShowsEveryGoldenLine(t *testing.T) {
	spec, err := os.ReadFile(filepath.Join("..", "..", "..", "docs", "protocol", "control.md"))
	if err != nil {
		t.Fatal(err)
	}
	for name := range examples() {
		line := strings.TrimSpace(string(golden(t, name)))
		if !strings.Contains(string(spec), "\n"+line+"\n") {
			t.Errorf("docs/protocol/control.md does not show the %s example:\n%s", name, line)
		}
	}
}

func TestEncodeWritesOneLineInOneWrite(t *testing.T) {
	for name, m := range examples() {
		w := &countingWriter{}
		if err := protocol.Encode(w, m); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if w.writes != 1 {
			t.Errorf("%s: %d writes, want 1", name, w.writes)
		}
		if got := bytes.Count(w.buf.Bytes(), []byte("\n")); got != 1 || !bytes.HasSuffix(w.buf.Bytes(), []byte("\n")) {
			t.Errorf("%s: want exactly one newline, at the end: %q", name, w.buf.Bytes())
		}
	}
}

type countingWriter struct {
	buf    bytes.Buffer
	writes int
}

func (w *countingWriter) Write(p []byte) (int, error) {
	w.writes++
	return w.buf.Write(p)
}

type failingWriter struct{ err error }

func (w failingWriter) Write([]byte) (int, error) { return 0, w.err }

func TestEncodeReturnsTheWriterError(t *testing.T) {
	want := errors.New("pipe closed")
	if err := protocol.Encode(failingWriter{want}, protocol.Status{}); !errors.Is(err, want) {
		t.Errorf("got %v, want %v", err, want)
	}
}

func validSession() protocol.SessionState {
	return protocol.SessionState{
		ID: "s", Surface: "code", Status: "idle", Privacy: "minimal", Start: 1, LastActivity: 1,
	}
}

func validEvent() protocol.EventData {
	return protocol.EventData{SessionID: "s", Surface: "code", At: 1, Kind: "idle"}
}

func session(change func(*protocol.SessionState)) protocol.Message {
	s := validSession()
	change(&s)
	return protocol.Sync{Session: &s}
}

func event(change func(*protocol.EventData)) protocol.Message {
	e := validEvent()
	change(&e)
	return protocol.Event{Event: e}
}

func TestEncodeRejectsWhatWouldNotDecode(t *testing.T) {
	long := strings.Repeat("x", 200)
	word := strings.Repeat("x", protocol.MaxWordLen+1)
	card := strings.Repeat("x", protocol.MaxCardTextLen+1)
	cases := []struct {
		name  string
		m     protocol.Message
		want  error
		field string
	}{
		{"unknown message", protocol.Unknown{}, protocol.ErrInvalidValue, "type"},
		{"hello without protocol", protocol.Hello{Version: "1"}, protocol.ErrMissingField, "hello.protocol"},
		{"hello negative protocol", protocol.Hello{Protocol: -1, Version: "1"}, protocol.ErrInvalidValue, "hello.protocol"},
		{"hello without version", protocol.Hello{Protocol: 1}, protocol.ErrMissingField, "hello.version"},
		{"hello long version", protocol.Hello{Protocol: 1, Version: strings.Repeat("1", protocol.MaxVersionLen+1)}, protocol.ErrInvalidValue, "hello.version"},
		{"hello version with a slash", protocol.Hello{Protocol: 1, Version: "a/b"}, protocol.ErrInvalidValue, "hello.version"},
		{"hello version with a backslash", protocol.Hello{Protocol: 1, Version: `a\b`}, protocol.ErrInvalidValue, "hello.version"},
		{"hello version with a space", protocol.Hello{Protocol: 1, Version: "a b"}, protocol.ErrInvalidValue, "hello.version"},
		{"hello version with a colon", protocol.Hello{Protocol: 1, Version: "C:"}, protocol.ErrInvalidValue, "hello.version"},
		{"hello version not ASCII", protocol.Hello{Protocol: 1, Version: "é"}, protocol.ErrInvalidValue, "hello.version"},
		{"welcome without protocol", protocol.Welcome{Version: "1"}, protocol.ErrMissingField, "welcome.protocol"},
		{"welcome without version", protocol.Welcome{Protocol: 1}, protocol.ErrMissingField, "welcome.version"},
		{"refuse without reason", protocol.Refuse{}, protocol.ErrMissingField, "refuse.reason"},
		{"refuse free-form reason", protocol.Refuse{Reason: "because"}, protocol.ErrInvalidValue, "refuse.reason"},
		{"status without discord", protocol.StatusResult{Version: "1"}, protocol.ErrMissingField, "status_result.discord"},
		{"status free-form discord", protocol.StatusResult{Discord: "sort of", Version: "1"}, protocol.ErrInvalidValue, "status_result.discord"},
		{"status negative sessions", protocol.StatusResult{Discord: protocol.DiscordConnected, Sessions: -1, Version: "1"}, protocol.ErrInvalidValue, "status_result.sessions"},
		{"status negative uptime", protocol.StatusResult{Discord: protocol.DiscordConnected, UptimeSeconds: -1, Version: "1"}, protocol.ErrInvalidValue, "status_result.uptime_seconds"},
		{"status without version", protocol.StatusResult{Discord: protocol.DiscordConnected}, protocol.ErrMissingField, "status_result.version"},
		{"status version is a path", protocol.StatusResult{Discord: protocol.DiscordConnected, Version: "/home/me"}, protocol.ErrInvalidValue, "status_result.version"},
		{"status negative pause end", protocol.StatusResult{Discord: protocol.DiscordConnected, Version: "1", Pause: &protocol.PauseState{Until: -1}}, protocol.ErrInvalidValue, "status_result.pause.until"},
		{"status negative pause time", protocol.StatusResult{Discord: protocol.DiscordConnected, Version: "1", Pause: &protocol.PauseState{At: -1}}, protocol.ErrInvalidValue, "status_result.pause.at"},
		{"pause without time", protocol.Pause{Until: 5}, protocol.ErrMissingField, "pause.at"},
		{"pause negative time", protocol.Pause{At: -1}, protocol.ErrInvalidValue, "pause.at"},
		{"pause negative end", protocol.Pause{Until: -1, At: 1}, protocol.ErrInvalidValue, "pause.until"},
		{"resume without time", protocol.Resume{}, protocol.ErrMissingField, "resume.at"},
		{"resume negative time", protocol.Resume{At: -1}, protocol.ErrInvalidValue, "resume.at"},
		{"preview long details", protocol.PreviewResult{Details: card}, protocol.ErrInvalidValue, "preview_result.details"},
		{"preview long state", protocol.PreviewResult{State: card}, protocol.ErrInvalidValue, "preview_result.state"},
		{"preview negative start", protocol.PreviewResult{Start: -1}, protocol.ErrInvalidValue, "preview_result.start"},
		{"preview long large image", protocol.PreviewResult{LargeImage: card}, protocol.ErrInvalidValue, "preview_result.large_image"},
		{"preview long large text", protocol.PreviewResult{LargeText: card}, protocol.ErrInvalidValue, "preview_result.large_text"},
		{"preview long small image", protocol.PreviewResult{SmallImage: card}, protocol.ErrInvalidValue, "preview_result.small_image"},
		{"preview long small text", protocol.PreviewResult{SmallText: card}, protocol.ErrInvalidValue, "preview_result.small_text"},
		{"preview long button label", protocol.PreviewResult{ButtonLabel: card}, protocol.ErrInvalidValue, "preview_result.button_label"},
		{"preview long button link", protocol.PreviewResult{ButtonURL: card}, protocol.ErrInvalidValue, "preview_result.button_url"},

		{"session without id", session(func(s *protocol.SessionState) { s.ID = "" }), protocol.ErrMissingField, "session.id"},
		{"session long id", session(func(s *protocol.SessionState) { s.ID = long }), protocol.ErrInvalidValue, "session.id"},
		{"session without surface", session(func(s *protocol.SessionState) { s.Surface = "" }), protocol.ErrMissingField, "session.surface"},
		{"session long surface", session(func(s *protocol.SessionState) { s.Surface = word }), protocol.ErrInvalidValue, "session.surface"},
		{"session without status", session(func(s *protocol.SessionState) { s.Status = "" }), protocol.ErrMissingField, "session.status"},
		{"session long status", session(func(s *protocol.SessionState) { s.Status = word }), protocol.ErrInvalidValue, "session.status"},
		{"session long tool", session(func(s *protocol.SessionState) { s.Tool = word }), protocol.ErrInvalidValue, "session.tool"},
		{"session long model", session(func(s *protocol.SessionState) { s.Model = long }), protocol.ErrInvalidValue, "session.model"},
		{"session long project", session(func(s *protocol.SessionState) { s.Project = long }), protocol.ErrInvalidValue, "session.project"},
		{"session without privacy", session(func(s *protocol.SessionState) { s.Privacy = "" }), protocol.ErrMissingField, "session.privacy"},
		{"session long privacy", session(func(s *protocol.SessionState) { s.Privacy = word }), protocol.ErrInvalidValue, "session.privacy"},
		{"session without start", session(func(s *protocol.SessionState) { s.Start = 0 }), protocol.ErrMissingField, "session.start"},
		{"session negative start", session(func(s *protocol.SessionState) { s.Start = -1 }), protocol.ErrInvalidValue, "session.start"},
		{"session without last activity", session(func(s *protocol.SessionState) { s.LastActivity = 0 }), protocol.ErrMissingField, "session.last_activity"},
		{"session negative last activity", session(func(s *protocol.SessionState) { s.LastActivity = -1 }), protocol.ErrInvalidValue, "session.last_activity"},
		{"session negative subagents", session(func(s *protocol.SessionState) { s.Subagents = -1 }), protocol.ErrInvalidValue, "session.subagents"},

		{"event without session id", event(func(e *protocol.EventData) { e.SessionID = "" }), protocol.ErrMissingField, "event.session_id"},
		{"event long session id", event(func(e *protocol.EventData) { e.SessionID = long }), protocol.ErrInvalidValue, "event.session_id"},
		{"event without surface", event(func(e *protocol.EventData) { e.Surface = "" }), protocol.ErrMissingField, "event.surface"},
		{"event long surface", event(func(e *protocol.EventData) { e.Surface = word }), protocol.ErrInvalidValue, "event.surface"},
		{"event without time", event(func(e *protocol.EventData) { e.At = 0 }), protocol.ErrMissingField, "event.at"},
		{"event negative time", event(func(e *protocol.EventData) { e.At = -1 }), protocol.ErrInvalidValue, "event.at"},
		{"event without kind", event(func(e *protocol.EventData) { e.Kind = "" }), protocol.ErrMissingField, "event.kind"},
		{"event long kind", event(func(e *protocol.EventData) { e.Kind = word }), protocol.ErrInvalidValue, "event.kind"},
		{"event long tool", event(func(e *protocol.EventData) { e.Tool = word }), protocol.ErrInvalidValue, "event.tool"},
		{"event long model", event(func(e *protocol.EventData) { e.Model = long }), protocol.ErrInvalidValue, "event.model"},
		{"event long project", event(func(e *protocol.EventData) { e.Project = long }), protocol.ErrInvalidValue, "event.project"},
		{"event long privacy", event(func(e *protocol.EventData) { e.Privacy = word }), protocol.ErrInvalidValue, "event.privacy"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var buf bytes.Buffer
			err := protocol.Encode(&buf, c.m)
			if !errors.Is(err, c.want) {
				t.Fatalf("got %v, want %v", err, c.want)
			}
			if !strings.HasSuffix(err.Error(), ": "+c.field) {
				t.Errorf("error %q does not name %s", err, c.field)
			}
			if buf.Len() != 0 {
				t.Errorf("wrote %q, want nothing", buf.Bytes())
			}
		})
	}
}

func TestEncodeAcceptsValuesAtTheLimits(t *testing.T) {
	word := strings.Repeat("w", protocol.MaxWordLen)
	card := strings.Repeat("c", protocol.MaxCardTextLen)
	messages := []protocol.Message{
		protocol.Hello{Protocol: 99, Version: strings.Repeat("1", protocol.MaxVersionLen)},
		protocol.Welcome{Protocol: 1, Version: "v0.0.0-20261003120000-0123456789ab+dirty"},
		protocol.Welcome{Protocol: 1, Version: "(devel)"},
		protocol.Hello{Protocol: 1, Version: "AZaz09._-+()"},
		protocol.Refuse{Reason: protocol.ReasonStandingDown},
		protocol.Refuse{Reason: protocol.ReasonOther},
		protocol.StatusResult{Discord: protocol.DiscordConnecting, Version: "1"},
		protocol.StatusResult{Discord: protocol.DiscordDisconnected, Version: "1"},
		protocol.StatusResult{Discord: protocol.DiscordUnknown, Version: "1"},
		protocol.StatusResult{Discord: protocol.DiscordConnected, Version: "1", Pause: &protocol.PauseState{}},
		protocol.Pause{At: 1},
		protocol.Resume{At: 1},
		protocol.PreviewResult{
			Shown: true, Details: card, State: card, Start: 1, LargeImage: card, LargeText: card,
			SmallImage: card, SmallText: card, ButtonLabel: card, ButtonURL: card,
		},
		protocol.Sync{Session: &protocol.SessionState{
			ID: strings.Repeat("i", domain.MaxIDLen), Surface: word, Status: word, Tool: word,
			Model: strings.Repeat("m", domain.MaxModelLen), Project: strings.Repeat("p", domain.MaxProjectLen),
			Privacy: word, Start: 1, LastActivity: 1,
		}},
		protocol.Event{Event: protocol.EventData{
			SessionID: strings.Repeat("i", domain.MaxIDLen), Surface: word, At: 1, Kind: word, Tool: word,
			Model: strings.Repeat("m", domain.MaxModelLen), Project: strings.Repeat("p", domain.MaxProjectLen),
			Privacy: word,
		}},
	}
	for _, m := range messages {
		got, err := protocol.Decode(encode(t, m))
		if err != nil {
			t.Errorf("Decode(%#v): %v", m, err)
			continue
		}
		if !reflect.DeepEqual(got, m) {
			t.Errorf("round trip\n got %#v\nwant %#v", got, m)
		}
	}
}

func TestDecode(t *testing.T) {
	cases := []struct {
		name string
		line string
		want protocol.Message
	}{
		{"unknown type", `{"type":"teleport","to":"mars"}`, protocol.Unknown{}},
		{"unknown type reusing a known field with another shape", `{"type":"teleport","protocol":"x","session":7}`, protocol.Unknown{}},
		{"unknown fields", `{"type":"hello","protocol":1,"version":"1","colour":"blue","nested":{"a":[1]}}`, protocol.Hello{Protocol: 1, Version: "1"}},
		{"unknown fields on a message with none", `{"type":"status","verbose":true}`, protocol.Status{}},
		{"unknown fields in an event", `{"type":"event","event":{"session_id":"s","surface":"code","at":1,"kind":"idle","prompt":"x"}}`, protocol.Event{Event: validEvent()}},
		{"unknown fields in a sync", `{"type":"sync","session":{"id":"s","surface":"code","status":"idle","privacy":"minimal","start":1,"last_activity":1,"from_a_later_version":"https://example.org/a/b"}}`, protocol.Sync{Session: ptr(validSession())}},
		{"unknown reason", `{"type":"refuse","reason":"moon_phase"}`, protocol.Refuse{Reason: protocol.ReasonOther}},
		{"unknown discord state", `{"type":"status_result","discord":"napping","version":"1"}`, protocol.StatusResult{Discord: protocol.DiscordUnknown, Version: "1"}},
		{"sessions and uptime default to zero", `{"type":"status_result","discord":"connected","version":"1"}`, protocol.StatusResult{Discord: protocol.DiscordConnected, Version: "1"}},
		{"a status from before pausing has no pause", `{"type":"status_result","discord":"connected","version":"1","pause":null}`, protocol.StatusResult{Discord: protocol.DiscordConnected, Version: "1"}},
		{"a pause that was never asked for", `{"type":"status_result","discord":"connected","version":"1","pause":{}}`, protocol.StatusResult{Discord: protocol.DiscordConnected, Version: "1", Pause: &protocol.PauseState{}}},
		{"a preview of nothing", `{"type":"preview_result"}`, protocol.PreviewResult{}},
		{"sync without a session field", `{"type":"sync"}`, protocol.Sync{}},
		{"a word the domain does not know", `{"type":"event","event":{"session_id":"s","surface":"code","at":1,"kind":"levitating"}}`, protocol.Event{Event: protocol.EventData{SessionID: "s", Surface: "code", At: 1, Kind: "levitating"}}},
		{"a newer protocol version", `{"type":"hello","protocol":7,"version":"9.0.0"}`, protocol.Hello{Protocol: 7, Version: "9.0.0"}},
		{"surrounding white space", " \t{\"type\":\"stand_down\"}\r\n", protocol.StandDown{}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := protocol.Decode([]byte(c.line))
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, c.want) {
				t.Errorf("got %#v, want %#v", got, c.want)
			}
		})
	}
}

func TestUnknownHasNoType(t *testing.T) {
	if got := (protocol.Unknown{}).Type(); got != "" {
		t.Errorf("Type() = %q, want empty", got)
	}
}

func TestDecodeErrors(t *testing.T) {
	const secret = "hunter2"
	cases := []struct {
		name string
		line string
		want error
	}{
		{"over the limit", `{"type":"status","pad":"` + strings.Repeat("x", protocol.MaxLineBytes) + `"}`, protocol.ErrLineTooLong},
		{"empty", ``, protocol.ErrMalformed},
		{"not JSON", `hello ` + secret, protocol.ErrMalformed},
		{"truncated", `{"type":"hello","protocol":1,"version":"` + secret, protocol.ErrMalformed},
		{"an array", `["` + secret + `"]`, protocol.ErrMalformed},
		{"a number", `7`, protocol.ErrMalformed},
		{"two objects", `{"type":"status"} {"type":"status"}`, protocol.ErrMalformed},
		{"type is not a string", `{"type":7}`, protocol.ErrMalformed},
		{"field of the wrong shape", `{"type":"hello","protocol":"` + secret + `","version":"1"}`, protocol.ErrMalformed},
		{"reason is not a string", `{"type":"refuse","reason":7}`, protocol.ErrMalformed},
		{"discord is not a string", `{"type":"status_result","discord":7,"version":"1"}`, protocol.ErrMalformed},
		{"session is not an object", `{"type":"sync","session":"` + secret + `"}`, protocol.ErrMalformed},
		{"pause is not an object", `{"type":"status_result","discord":"connected","version":"1","pause":"` + secret + `"}`, protocol.ErrMalformed},
		{"pause end is not a number", `{"type":"pause","until":"` + secret + `","at":1}`, protocol.ErrMalformed},
		{"pause without time", `{"type":"pause","until":5}`, protocol.ErrMissingField},
		{"resume without time", `{"type":"resume"}`, protocol.ErrMissingField},
		{"preview text is not a string", `{"type":"preview_result","details":7}`, protocol.ErrMalformed},
		{"null", `null`, protocol.ErrMissingField},
		{"no type", `{"protocol":1}`, protocol.ErrMissingField},
		{"empty type", `{"type":""}`, protocol.ErrMissingField},
		{"hello without version", `{"type":"hello","protocol":1}`, protocol.ErrMissingField},
		{"hello without protocol", `{"type":"hello","version":"1"}`, protocol.ErrMissingField},
		{"welcome without version", `{"type":"welcome","protocol":1}`, protocol.ErrMissingField},
		{"refuse without reason", `{"type":"refuse"}`, protocol.ErrMissingField},
		{"event without event", `{"type":"event"}`, protocol.ErrMissingField},
		{"event without kind", `{"type":"event","event":{"session_id":"` + secret + `","surface":"code","at":1}}`, protocol.ErrMissingField},
		{"session without id", `{"type":"sync","session":{"surface":"code"}}`, protocol.ErrMissingField},
		{"status result without discord", `{"type":"status_result","version":"1"}`, protocol.ErrMissingField},
		{"version is a path", `{"type":"status_result","discord":"connected","version":"/home/` + secret + `"}`, protocol.ErrInvalidValue},
		{"negative protocol", `{"type":"hello","protocol":-1,"version":"1"}`, protocol.ErrInvalidValue},
		{"project over its limit", `{"type":"event","event":{"session_id":"s","surface":"code","at":1,"kind":"idle","project":"` + strings.Repeat("p", domain.MaxProjectLen+1) + `"}}`, protocol.ErrInvalidValue},
	}
	all := []error{protocol.ErrLineTooLong, protocol.ErrMalformed, protocol.ErrMissingField, protocol.ErrInvalidValue}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := protocol.Decode([]byte(c.line))
			if got != nil {
				t.Errorf("got message %#v with an error", got)
			}
			for _, e := range all {
				if errors.Is(err, e) != (e == c.want) {
					t.Errorf("errors.Is(%v, %v) = %t", err, e, errors.Is(err, e))
				}
			}
			if strings.Contains(err.Error(), secret) {
				t.Errorf("error quotes the input: %v", err)
			}
		})
	}
}

func TestDecodeLineLimitIsExact(t *testing.T) {
	pad := func(n int) []byte {
		const frame = `{"type":"status","pad":""}`
		return []byte(`{"type":"status","pad":"` + strings.Repeat("x", n-len(frame)) + `"}`)
	}
	if _, err := protocol.Decode(pad(protocol.MaxLineBytes)); err != nil {
		t.Errorf("a line at the limit: %v", err)
	}
	if _, err := protocol.Decode(pad(protocol.MaxLineBytes + 1)); !errors.Is(err, protocol.ErrLineTooLong) {
		t.Errorf("a line one over the limit: got %v", err)
	}

	// The same through the stream decoder, with and without a final newline.
	for _, end := range []string{"\n", ""} {
		d := protocol.NewDecoder(strings.NewReader(string(pad(protocol.MaxLineBytes)) + end))
		if m, err := d.Next(); err != nil || m != (protocol.Status{}) {
			t.Errorf("stream, line at the limit, end %q: %#v, %v", end, m, err)
		}
		d = protocol.NewDecoder(strings.NewReader(string(pad(protocol.MaxLineBytes+1)) + end))
		if _, err := d.Next(); !errors.Is(err, protocol.ErrLineTooLong) {
			t.Errorf("stream, line over the limit, end %q: got %v", end, err)
		}
	}
}

func TestDecoderReadsAStream(t *testing.T) {
	stream := `{"type":"hello","protocol":1,"version":"1"}` + "\n" +
		"\n" +
		"   \r\n" +
		`not json` + "\n" +
		`{"type":"later"}` + "\r\n" +
		`{"type":"status"}`
	d := protocol.NewDecoder(strings.NewReader(stream))

	if m, err := d.Next(); err != nil || m != (protocol.Hello{Protocol: 1, Version: "1"}) {
		t.Fatalf("first: %#v, %v", m, err)
	}
	if m, err := d.Next(); m != nil || !errors.Is(err, protocol.ErrMalformed) {
		t.Fatalf("bad line: %#v, %v", m, err)
	}
	if m, err := d.Next(); err != nil || m != (protocol.Unknown{}) {
		t.Fatalf("after a bad line: %#v, %v", m, err)
	}
	if m, err := d.Next(); err != nil || m != (protocol.Status{}) {
		t.Fatalf("last line without a newline: %#v, %v", m, err)
	}
	for range 2 {
		if m, err := d.Next(); m != nil || err != io.EOF {
			t.Fatalf("end: %#v, %v", m, err)
		}
	}
}

func TestDecoderLongLineIsFinal(t *testing.T) {
	stream := `{"type":"status"}` + "\n" + strings.Repeat("x", protocol.MaxLineBytes+1) + "\n" + `{"type":"status"}` + "\n"
	d := protocol.NewDecoder(strings.NewReader(stream))
	if _, err := d.Next(); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if m, err := d.Next(); m != nil || !errors.Is(err, protocol.ErrLineTooLong) {
			t.Fatalf("got %#v, %v", m, err)
		}
	}
}

func TestDecoderReturnsTheReaderError(t *testing.T) {
	want := errors.New("connection reset")
	d := protocol.NewDecoder(io.MultiReader(strings.NewReader(`{"type":"status"}`+"\n"), failingReader{want}))
	if _, err := d.Next(); err != nil {
		t.Fatal(err)
	}
	if m, err := d.Next(); m != nil || !errors.Is(err, want) {
		t.Fatalf("got %#v, %v", m, err)
	}
}

type failingReader struct{ err error }

func (r failingReader) Read([]byte) (int, error) { return 0, r.err }

func TestAnswer(t *testing.T) {
	cases := []struct {
		protocol int
		want     protocol.Message
	}{
		{protocol.MinVersion - 1, protocol.Refuse{Reason: protocol.ReasonUnsupportedProtocol}},
		{protocol.MinVersion, protocol.Welcome{Protocol: protocol.Version, Version: "1.3.2"}},
		{protocol.Version, protocol.Welcome{Protocol: protocol.Version, Version: "1.3.2"}},
		{protocol.Version + 1, protocol.Refuse{Reason: protocol.ReasonUnsupportedProtocol}},
	}
	for _, c := range cases {
		got := protocol.Answer(protocol.Hello{Protocol: c.protocol, Version: "1.4.0"}, "1.3.2")
		if got != c.want {
			t.Errorf("protocol %d: got %#v, want %#v", c.protocol, got, c.want)
		}
		if protocol.Supported(c.protocol) != (got.Type() == protocol.TypeWelcome) {
			t.Errorf("protocol %d: Supported disagrees with Answer", c.protocol)
		}
	}
}

func TestDomainEventRoundTrips(t *testing.T) {
	at := time.Date(2026, 10, 3, 12, 0, 0, 500_000_000, time.UTC)
	for _, kind := range domain.Kinds() {
		e := domain.Event{
			SessionID: sessionID, Surface: domain.SurfaceDesktop, At: at, Kind: kind,
			Tool: domain.ToolSearching, Model: "sonnet", Project: "example-project", Privacy: domain.PrivacyFull,
			Link: "https://github.com/me/example-project",
		}
		got, err := protocol.Decode(encode(t, protocol.Event{Event: protocol.EventFromDomain(e)}))
		if err != nil {
			t.Fatalf("%s: %v", kind, err)
		}
		back := got.(protocol.Event).Event.Domain()
		if back != e {
			t.Errorf("%s:\n got %#v\nwant %#v", kind, back, e)
		}
		if err := back.Validate(); err != nil {
			t.Errorf("%s: %v", kind, err)
		}
	}
}

func TestDomainSessionRoundTrips(t *testing.T) {
	start := time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)
	for _, status := range domain.Statuses() {
		s := domain.Session{
			ID: sessionID, Surface: domain.SurfaceCode, Status: status, Model: "opus",
			Project: "example-project", Privacy: domain.PrivacyStandard,
			Start: start, LastActivity: start.Add(90 * time.Second), Subagents: 3,
			Link: "https://github.com/me/example-project",
		}
		if status == domain.StatusWorking {
			s.Tool = domain.ToolEditing
		}
		got, err := protocol.Decode(encode(t, protocol.Sync{Session: ptr(protocol.SessionFromDomain(s))}))
		if err != nil {
			t.Fatalf("%s: %v", status, err)
		}
		back := got.(protocol.Sync).Session.Domain()
		if back != s {
			t.Errorf("%s:\n got %#v\nwant %#v", status, back, s)
		}
		if err := back.Validate(); err != nil {
			t.Errorf("%s: %v", status, err)
		}
	}
}

// TestTheLinkIsAdditive is the compatibility of the link field in both
// directions. A message without a link is, byte for byte, what it was before
// the field existed, so an older host reads it as before; and a host reads a
// follower that sends none as having none. An older host ignores the field
// as it ignores any it does not know, which TestDecode shows.
func TestTheLinkIsAdditive(t *testing.T) {
	without := string(encode(t, protocol.Sync{Session: ptr(validSession())})) + string(encode(t, protocol.Event{Event: validEvent()}))
	if strings.Contains(without, "link") {
		t.Errorf("messages without a link mention one: %s", without)
	}

	got, err := protocol.Decode([]byte(`{"type":"sync","session":{"id":"s","surface":"code","status":"idle","privacy":"minimal","start":1,"last_activity":1}}`))
	if err != nil {
		t.Fatal(err)
	}
	if link := got.(protocol.Sync).Session.Link; link != "" {
		t.Errorf("a session sent without a link has %q", link)
	}

	// What the link holds is not judged here. An invalid one is the host's
	// to drop, and must not cost the session by failing the line.
	for _, link := range []string{"https://github.com/me/visions", "not a link", "https://user:token@github.com/me/visions", strings.Repeat("a", 2000)} {
		for _, m := range []protocol.Message{
			session(func(s *protocol.SessionState) { s.Link = link }),
			event(func(e *protocol.EventData) { e.Link = link }),
		} {
			back, err := protocol.Decode(encode(t, m))
			if err != nil {
				t.Errorf("a %s with a link of %d bytes: %v", m.Type(), len(link), err)
				continue
			}
			if !reflect.DeepEqual(back, m) {
				t.Errorf("round trip\n got %#v\nwant %#v", back, m)
			}
		}
	}
}

func ptr[T any](v T) *T { return &v }

func TestConversionCutsTimesToTheMillisecond(t *testing.T) {
	at := time.Date(2026, 10, 3, 12, 0, 0, 123_456_789, time.FixedZone("x", 3600))
	got := protocol.EventFromDomain(domain.Event{At: at}).Domain().At
	if want := at.Truncate(time.Millisecond).UTC(); got != want {
		t.Errorf("got %v, want %v", got, want)
	}
}

// A time that is not set stays not set, so the domain still rejects it.
func TestConversionKeepsTheZeroTime(t *testing.T) {
	if got := protocol.EventFromDomain(domain.Event{}); got.At != 0 {
		t.Errorf("wire time %d, want 0", got.At)
	}
	if got := (protocol.EventData{}).Domain(); !got.At.IsZero() {
		t.Errorf("domain time %v, want zero", got.At)
	}
	s := protocol.SessionFromDomain(domain.Session{}).Domain()
	if !s.Start.IsZero() || !s.LastActivity.IsZero() {
		t.Errorf("session times %v and %v, want zero", s.Start, s.LastActivity)
	}
}

// fields lists a struct's wire fields as "name kind", descending into nested
// structs and pointers to them.
func fields(t reflect.Type, prefix string) []string {
	var out []string
	for i := range t.NumField() {
		f := t.Field(i)
		name, _, _ := strings.Cut(f.Tag.Get("json"), ",")
		ft := f.Type
		if ft.Kind() == reflect.Pointer {
			ft = ft.Elem()
		}
		if ft.Kind() == reflect.Struct {
			out = append(out, fields(ft, prefix+name+".")...)
			continue
		}
		out = append(out, prefix+name+" "+ft.Kind().String())
	}
	sort.Strings(out)
	return out
}

// TestMessagesCarryOnlyTheListedFields is the whole of what can cross the
// channel. There is no field for prompt text, tool input, a file path or
// anything else free-form: the only strings are ids, closed vocabularies, the
// model label, the project name, the repository link and the binary version.
// Each is bounded, the link by the limit on a line and then by the host,
// which validates it. Adding a field fails this test until it is listed here and in
// docs/protocol/control.md.
func TestMessagesCarryOnlyTheListedFields(t *testing.T) {
	eventFields := []string{
		"event.at int64", "event.kind string", "event.link string", "event.model string", "event.privacy string",
		"event.project string", "event.session_id string", "event.surface string", "event.tool string",
	}
	sessionFields := []string{
		"session.id string", "session.last_activity int64", "session.link string", "session.model string",
		"session.privacy string", "session.project string", "session.start int64",
		"session.status string", "session.subagents int", "session.surface string", "session.tool string",
	}
	cases := []struct {
		m    protocol.Message
		want []string
	}{
		{protocol.Hello{}, []string{"protocol int", "version string"}},
		{protocol.Welcome{}, []string{"protocol int", "version string"}},
		{protocol.Refuse{}, []string{"reason string"}},
		{protocol.Sync{}, sessionFields},
		{protocol.Event{}, eventFields},
		{protocol.StandDown{}, nil},
		{protocol.Status{}, nil},
		// Nothing here names a project, a path or a session.
		{protocol.StatusResult{}, []string{
			"discord string", "pause.at int64", "pause.paused bool", "pause.until int64",
			"sessions int", "uptime_seconds int64", "version string",
		}},
		// A pause and a resume carry times and nothing that could be shown.
		{protocol.Pause{}, []string{"at int64", "until int64"}},
		{protocol.Resume{}, []string{"at int64"}},
		{protocol.Preview{}, nil},
		// The card as the host already publishes it, and nothing else.
		{protocol.PreviewResult{}, []string{
			"button_label string", "button_url string", "details string", "large_image string", "large_text string",
			"shown bool", "small_image string", "small_text string", "start int64", "state string",
		}},
		{protocol.Unknown{}, nil},
	}
	for _, c := range cases {
		typ := reflect.TypeOf(c.m)
		if got := fields(typ, ""); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s carries\n got %q\nwant %q", typ.Name(), got, c.want)
		}
	}
}

// TestPayloadsMirrorTheDomain checks that the wire event and session have a
// field for each field of the domain type and no others.
func TestPayloadsMirrorTheDomain(t *testing.T) {
	names := func(t reflect.Type) []string {
		var out []string
		for i := range t.NumField() {
			out = append(out, t.Field(i).Name)
		}
		sort.Strings(out)
		return out
	}
	cases := []struct{ wire, domain any }{
		{protocol.EventData{}, domain.Event{}},
		{protocol.SessionState{}, domain.Session{}},
	}
	for _, c := range cases {
		got, want := names(reflect.TypeOf(c.wire)), names(reflect.TypeOf(c.domain))
		if !reflect.DeepEqual(got, want) {
			t.Errorf("%T has fields %q, the domain has %q", c.wire, got, want)
		}
	}
}

func TestPreviewRoundTripsAnActivity(t *testing.T) {
	a := domain.Activity{
		Details: "one", State: "two", Start: time.UnixMilli(1790985600000).UTC(),
		LargeImage: "logo", LargeText: "large", SmallImage: "working", SmallText: "small",
		Button: domain.Button{Label: "View on GitHub", URL: "https://github.com/owner/repo"},
	}
	got, shown := protocol.PreviewOf(a, true).Activity()
	if !shown || got != a {
		t.Errorf("got %+v shown %v, want %+v", got, shown, a)
	}
	// Whatever the activity holds, nothing shown is nothing on the wire.
	nothing := protocol.PreviewOf(a, false)
	if nothing != (protocol.PreviewResult{}) {
		t.Errorf("a preview of nothing carries %+v", nothing)
	}
	if got, shown := nothing.Activity(); shown || got != (domain.Activity{}) {
		t.Errorf("got %+v shown %v, want nothing", got, shown)
	}
}

func seeds(f *testing.F) {
	f.Helper()
	files, err := filepath.Glob(filepath.Join("testdata", "golden", "*.json"))
	if err != nil || len(files) == 0 {
		f.Fatalf("no golden files: %v", err)
	}
	for _, name := range files {
		b, err := os.ReadFile(name)
		if err != nil {
			f.Fatal(err)
		}
		f.Add(b)
	}
	for _, s := range []string{
		``, `null`, `{}`, `[]`, `{"type":7}`, `{"type":"later","x":[{}]}`, `{"type":"hello"`,
		`{"type":"refuse","reason":"x"}`, `{"type":"sync","session":{}}`, "{\"type\":\"status\"}\n\n{\"type\":\"status\"}",
		`{"type":"hello","protocol":1e99,"version":"\ud800"}`,
	} {
		f.Add([]byte(s))
	}
}

// FuzzDecode checks that no line panics, that every failure is one of the
// four errors, and that whatever decodes can be encoded and read back.
func FuzzDecode(f *testing.F) {
	seeds(f)
	f.Fuzz(func(t *testing.T, line []byte) {
		m, err := protocol.Decode(line)
		if err != nil {
			if m != nil {
				t.Fatalf("message %#v with error %v", m, err)
			}
			if !errors.Is(err, protocol.ErrLineTooLong) && !errors.Is(err, protocol.ErrMalformed) &&
				!errors.Is(err, protocol.ErrMissingField) && !errors.Is(err, protocol.ErrInvalidValue) {
				t.Fatalf("unexpected error %v", err)
			}
			return
		}
		if m == (protocol.Unknown{}) {
			return
		}
		var buf bytes.Buffer
		if err := protocol.Encode(&buf, m); err != nil {
			t.Fatalf("Encode(%#v): %v", m, err)
		}
		if buf.Len() > protocol.MaxLineBytes {
			t.Fatalf("encoded to %d bytes", buf.Len())
		}
		again, err := protocol.Decode(buf.Bytes())
		if err != nil {
			t.Fatalf("Decode(%s): %v", buf.Bytes(), err)
		}
		if !reflect.DeepEqual(again, m) {
			t.Fatalf("round trip\n got %#v\nwant %#v", again, m)
		}
	})
}

// FuzzDecoder checks that no stream panics or makes the decoder loop.
func FuzzDecoder(f *testing.F) {
	seeds(f)
	f.Fuzz(func(t *testing.T, stream []byte) {
		d := protocol.NewDecoder(bytes.NewReader(stream))
		for range len(stream) + 2 {
			_, err := d.Next()
			if err == io.EOF || errors.Is(err, protocol.ErrLineTooLong) {
				return
			}
		}
		t.Fatal("the decoder did not reach the end of the stream")
	})
}
