package desktop

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Zafnok/claude-rich-presence/internal/domain"
	"github.com/Zafnok/claude-rich-presence/internal/mcp"
)

var noon = time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)

// stepClock tells a time that moves on a minute each time it is read.
type stepClock struct{ now time.Time }

func (c *stepClock) Now() time.Time {
	t := c.now
	c.now = c.now.Add(time.Minute)
	return t
}

// recorder is a publisher that keeps what it was given.
type recorder struct{ events []domain.Event }

func (r *recorder) Publish(e domain.Event) { r.events = append(r.events, e) }

// panicker is a publisher that fails.
type panicker struct{ calls int }

func (p *panicker) Publish(domain.Event) {
	p.calls++
	panic("the publisher failed")
}

type fixedStatus Status

func (s fixedStatus) Status() Status { return Status(s) }

func options(pub Publisher) Options {
	return Options{
		Privacy:   domain.PrivacyStandard,
		ID:        "desktop-1",
		Publisher: pub,
		Status:    fixedStatus{},
		Clock:     &stepClock{now: noon},
	}
}

func TestRecognise(t *testing.T) {
	tests := []struct {
		name string
		want Client
	}{
		{"claude-ai", ClientDesktop},
		{"claude-ai ", ClientCode},
		{"Claude-AI", ClientCode},
		{"claude-ai-next", ClientCode},
		{"local-agent-mode-Rich Presence", ClientPassive},
		{"local-agent-mode-", ClientPassive},
		{"local-agent-mode", ClientCode},
		{"Local-Agent-Mode-Rich Presence", ClientCode},
		{"x-local-agent-mode-Rich Presence", ClientCode},
		{"claude-code", ClientCode},
		{"some-other-client", ClientCode},
		{"", ClientCode},
	}
	for _, tt := range tests {
		if got := Recognise(tt.name); got != tt.want {
			t.Errorf("Recognise(%q) = %d, want %d", tt.name, got, tt.want)
		}
	}
}

func TestNewChecksItsOptions(t *testing.T) {
	tests := []struct {
		name   string
		change func(*Options)
		want   string
	}{
		{"an unknown privacy level", func(o *Options) { o.Privacy = "loud" }, "privacy"},
		{"no session id", func(o *Options) { o.ID = "" }, "session id"},
		{"a session id that is too long", func(o *Options) { o.ID = strings.Repeat("x", domain.MaxIDLen+1) }, "session id"},
		{"no publisher", func(o *Options) { o.Publisher = nil }, "publisher"},
		{"no status source", func(o *Options) { o.Status = nil }, "status"},
		{"no clock", func(o *Options) { o.Clock = nil }, "clock"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := options(&recorder{})
			tt.change(&opts)
			a, err := New(opts)
			if a != nil || err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("New = %v, %v; want no adapter and an error naming the %s", a, err, tt.want)
			}
		})
	}
	opts := options(&recorder{})
	opts.ID = strings.Repeat("x", domain.MaxIDLen)
	if _, err := New(opts); err != nil {
		t.Errorf("New with the longest session id: %v", err)
	}
}

func TestNewPassiveChecksItsArguments(t *testing.T) {
	if a, err := NewPassive("loud", fixedStatus{}); a != nil || err == nil || !strings.Contains(err.Error(), "privacy") {
		t.Errorf("NewPassive with an unknown level = %v, %v; want an error naming the privacy", a, err)
	}
	if a, err := NewPassive(domain.PrivacyFull, nil); a != nil || err == nil || !strings.Contains(err.Error(), "status") {
		t.Errorf("NewPassive without a status source = %v, %v; want an error naming the status", a, err)
	}
}

func TestSessionOpensAndEnds(t *testing.T) {
	for _, level := range []domain.Privacy{domain.PrivacyMinimal, domain.PrivacyStandard, domain.PrivacyFull} {
		t.Run(string(level), func(t *testing.T) {
			pub := &recorder{}
			opts := options(pub)
			opts.Privacy = level
			a, err := New(opts)
			if err != nil {
				t.Fatal(err)
			}
			if len(pub.events) != 0 {
				t.Fatalf("published %v before the session opened", pub.events)
			}
			a.Open()
			a.Open()
			opened := domain.Event{SessionID: "desktop-1", Surface: domain.SurfaceDesktop, At: noon, Kind: domain.KindSessionOpened, Privacy: level}
			if want := []domain.Event{opened}; !reflect.DeepEqual(pub.events, want) {
				t.Fatalf("after opening twice, published %+v, want %+v", pub.events, want)
			}
			a.Close()
			a.Close()
			a.Open()
			ended := domain.Event{SessionID: "desktop-1", Surface: domain.SurfaceDesktop, At: noon.Add(time.Minute), Kind: domain.KindSessionEnded}
			if want := []domain.Event{opened, ended}; !reflect.DeepEqual(pub.events, want) {
				t.Fatalf("after closing, published %+v, want %+v", pub.events, want)
			}
			for _, e := range pub.events {
				if err := e.Validate(); err != nil {
					t.Errorf("%s is not a valid event: %v", e.Kind, err)
				}
			}
		})
	}
}

func TestCloseWithoutOpenPublishesNothing(t *testing.T) {
	pub := &recorder{}
	a, err := New(options(pub))
	if err != nil {
		t.Fatal(err)
	}
	a.Close()
	a.Open()
	if len(pub.events) != 0 {
		t.Errorf("published %+v, want nothing for a session that never opened", pub.events)
	}
}

func TestAPublisherThatPanicsCostsOnlyTheEvent(t *testing.T) {
	pub := &panicker{}
	a, err := New(options(pub))
	if err != nil {
		t.Fatal(err)
	}
	a.Open()
	a.Close()
	if pub.calls != 2 {
		t.Errorf("the publisher was called %d times, want once to open and once to end", pub.calls)
	}
}

func TestPassiveCopyPublishesNothing(t *testing.T) {
	a, err := NewPassive(domain.PrivacyFull, fixedStatus{Role: RolePassive})
	if err != nil {
		t.Fatal(err)
	}
	// There is no publisher and no clock to reach.
	a.Open()
	a.Close()
	if got := call(t, a); !strings.HasPrefix(got, "Role: passive\n") {
		t.Errorf("status = %q, want the passive role", got)
	}
}

// serve runs requests through a real server offering the adapter's tools,
// and returns the result of the last.
func serve(t *testing.T, a *Adapter, requests ...string) json.RawMessage {
	t.Helper()
	in := `{"jsonrpc":"2.0","id":0,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"claude-ai","version":"0.1.0"}}}` + "\n"
	for _, r := range requests {
		in += r + "\n"
	}
	var out bytes.Buffer
	if err := mcp.Serve(strings.NewReader(in), &out, mcp.Options{Initialize: func(mcp.ClientInfo) []mcp.Tool { return a.Tools() }}); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	var last struct {
		Result json.RawMessage `json:"result"`
	}
	if err := json.Unmarshal([]byte(lines[len(lines)-1]), &last); err != nil || last.Result == nil {
		t.Fatalf("the server answered %q, want a result", lines[len(lines)-1])
	}
	return last.Result
}

// call is the text the status tool answers with.
func call(t *testing.T, a *Adapter) string {
	t.Helper()
	var result struct {
		Content []struct{ Text string }
		IsError bool
	}
	raw := serve(t, a, `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"presence_status","arguments":{"ignored":"text"}}}`)
	if err := json.Unmarshal(raw, &result); err != nil || len(result.Content) != 1 || result.IsError {
		t.Fatalf("the status tool answered %s, want one item of text and no error", raw)
	}
	return result.Content[0].Text
}

func TestOnlyTheStatusToolIsOffered(t *testing.T) {
	reporting, err := New(options(&recorder{}))
	if err != nil {
		t.Fatal(err)
	}
	passive, err := NewPassive(domain.PrivacyMinimal, fixedStatus{})
	if err != nil {
		t.Fatal(err)
	}
	for name, a := range map[string]*Adapter{"reporting": reporting, "passive": passive} {
		var list struct {
			Tools []struct {
				Name        string
				Description string
				InputSchema map[string]any
			}
		}
		if err := json.Unmarshal(serve(t, a, `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`), &list); err != nil {
			t.Fatal(err)
		}
		if len(list.Tools) != 1 || list.Tools[0].Name != StatusToolName || list.Tools[0].Description == "" || list.Tools[0].InputSchema["type"] != "object" {
			t.Errorf("%s: tools = %+v, want the status tool alone, described, taking an object", name, list.Tools)
		}
	}
}

func TestStatusText(t *testing.T) {
	tests := []struct {
		name   string
		status Status
		want   string
	}{
		{"a host", Status{Role: RoleHost, Discord: DiscordConnected, Sessions: 2}, "Role: host\nDiscord: connected\nSessions: 2\nPrivacy: standard"},
		{"a follower", Status{Role: RoleFollower, Discord: DiscordConnecting, Sessions: 3}, "Role: follower\nDiscord: connecting\nSessions: 3\nPrivacy: standard"},
		{"presence off", Status{Role: RoleOff, Discord: DiscordDisconnected}, "Role: off\nDiscord: disconnected\nSessions: 0\nPrivacy: standard"},
		{"the passive copy", Status{Role: RolePassive, Discord: DiscordConnected, Sessions: 1}, "Role: passive\nDiscord: connected\nSessions: 1\nPrivacy: standard"},
		{"values outside the vocabularies", Status{Role: "C:\\secret", Discord: "secret"}, "Role: unknown\nDiscord: unknown\nSessions: 0\nPrivacy: standard"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := options(&recorder{})
			opts.Status = fixedStatus(tt.status)
			a, err := New(opts)
			if err != nil {
				t.Fatal(err)
			}
			if got := call(t, a); got != tt.want {
				t.Errorf("status = %q, want %q", got, tt.want)
			}
		})
	}
}
