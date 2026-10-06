package codec_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Zafnok/claude-rich-presence/internal/discord/codec"
	"github.com/Zafnok/claude-rich-presence/internal/domain"
)

func fixedNonce() string { return fixtureNonce }

// wire is the bytes WriteFrame sends for f.
func wire(t *testing.T, f codec.Frame) string {
	t.Helper()
	var buf bytes.Buffer
	if err := codec.WriteFrame(&buf, f); err != nil {
		t.Fatalf("WriteFrame: %v", err)
	}
	return buf.String()
}

func TestHandshakeMatchesFixture(t *testing.T) {
	if got := wire(t, codec.Handshake("123456789012345678")); got != fixtureHandshake {
		t.Errorf("got  %q\nwant %q", got, fixtureHandshake)
	}
}

func TestSetActivityMatchesFixture(t *testing.T) {
	a := &domain.Activity{
		Details:    "Competitive | In a Match",
		State:      "In a Group",
		Start:      time.Unix(1507665886, 0),
		LargeImage: "numbani_map",
		LargeText:  "Numbani",
		SmallImage: "pharah_profile",
		SmallText:  "Pharah",
	}
	f, nonce := codec.SetActivity(9999, fixedNonce, a)
	if got := wire(t, f); got != fixtureSetActivity {
		t.Errorf("got  %q\nwant %q", got, fixtureSetActivity)
	}
	if nonce != fixtureNonce {
		t.Errorf("returned nonce %q, want %q", nonce, fixtureNonce)
	}
}

func TestSetActivityWithAButtonMatchesFixture(t *testing.T) {
	a := &domain.Activity{
		Details:    "Competitive | In a Match",
		State:      "In a Group",
		Start:      time.Unix(1507665886, 0),
		LargeImage: "numbani_map",
		LargeText:  "Numbani",
		SmallImage: "pharah_profile",
		SmallText:  "Pharah",
		Button:     domain.Button{Label: "View on GitHub", URL: "https://github.com/me/visions"},
	}
	f, _ := codec.SetActivity(9999, fixedNonce, a)
	if got := wire(t, f); got != fixtureSetActivityButton {
		t.Errorf("got  %q\nwant %q", got, fixtureSetActivityButton)
	}
}

func TestSetActivityNilClears(t *testing.T) {
	f, nonce := codec.SetActivity(9999, fixedNonce, nil)
	if got := wire(t, f); got != fixtureClearActivity {
		t.Errorf("got  %q\nwant %q", got, fixtureClearActivity)
	}
	if nonce != fixtureNonce {
		t.Errorf("returned nonce %q, want %q", nonce, fixtureNonce)
	}
}

func TestSetActivityTakesOneNonce(t *testing.T) {
	calls := 0
	codec.SetActivity(1, func() string { calls++; return "n" }, nil)
	if calls != 1 {
		t.Errorf("generator called %d times, want 1", calls)
	}
}

// TestActivityFields checks the activity object alone: which fields appear
// and which are left out.
func TestActivityFields(t *testing.T) {
	tests := []struct {
		name string
		in   domain.Activity
		want string
	}{
		{"nothing set", domain.Activity{}, `{}`},
		{"details only", domain.Activity{Details: "Refactoring"}, `{"details":"Refactoring"}`},
		{"state only", domain.Activity{State: "3 sessions"}, `{"state":"3 sessions"}`},
		{"start only", domain.Activity{Start: time.Unix(1700000000, 999).UTC()}, `{"timestamps":{"start":1700000000}}`},
		{"large image only", domain.Activity{LargeImage: "logo"}, `{"assets":{"large_image":"logo"}}`},
		{"large text only", domain.Activity{LargeText: "Hover"}, `{"assets":{"large_text":"Hover"}}`},
		{"small image only", domain.Activity{SmallImage: "badge"}, `{"assets":{"small_image":"badge"}}`},
		{"small text only", domain.Activity{SmallText: "Busy"}, `{"assets":{"small_text":"Busy"}}`},
		{"playing is the default", domain.Activity{Type: domain.ActivityPlaying}, `{}`},
		{"listening", domain.Activity{Type: domain.ActivityListening}, `{"type":2}`},
		{"watching", domain.Activity{Type: domain.ActivityWatching}, `{"type":3}`},
		{"competing", domain.Activity{Type: domain.ActivityCompeting}, `{"type":5}`},
		{"type above the range", domain.Activity{Type: 4}, `{}`},
		{"type below the range", domain.Activity{Type: -1}, `{}`},
		{"text is not escaped for HTML", domain.Activity{Details: `a <b> & "c"`}, `{"details":"a <b> & \"c\""}`},
		{"text beyond ASCII", domain.Activity{State: "naïve ✓"}, `{"state":"naïve ✓"}`},
		{"a button", domain.Activity{Button: domain.Button{Label: "View repository", URL: "https://example.org/a/b"}}, `{"buttons":[{"label":"View repository","url":"https://example.org/a/b"}]}`},
		{"a button is not escaped for HTML", domain.Activity{Button: domain.Button{Label: "A & B", URL: "https://example.org/a/b?c&d"}}, `{"buttons":[{"label":"A & B","url":"https://example.org/a/b?c&d"}]}`},
		{"a label of 32 characters", domain.Activity{Button: domain.Button{Label: strings.Repeat("é", 32), URL: "https://e.org"}}, `{"buttons":[{"label":"` + strings.Repeat("é", 32) + `","url":"https://e.org"}]}`},
		{"a label of 33 characters", domain.Activity{Button: domain.Button{Label: strings.Repeat("a", 33), URL: "https://e.org"}}, `{}`},
		{"a button without a label", domain.Activity{Button: domain.Button{URL: "https://e.org"}}, `{}`},
		{"a button without a link", domain.Activity{Button: domain.Button{Label: "View"}}, `{}`},
		{"a link of 512 bytes", domain.Activity{Button: domain.Button{Label: "View", URL: strings.Repeat("a", 512)}}, `{"buttons":[{"label":"View","url":"` + strings.Repeat("a", 512) + `"}]}`},
		{"a link of 513 bytes", domain.Activity{Button: domain.Button{Label: "View", URL: strings.Repeat("a", 513)}}, `{}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, _ := codec.SetActivity(7, fixedNonce, &tt.in)
			if f.Op != codec.OpFrame {
				t.Errorf("opcode %d, want %d", f.Op, codec.OpFrame)
			}
			var got struct {
				Args struct {
					Activity json.RawMessage `json:"activity"`
				} `json:"args"`
			}
			if err := json.Unmarshal(f.Payload, &got); err != nil {
				t.Fatalf("payload is not JSON: %v", err)
			}
			if string(got.Args.Activity) != tt.want {
				t.Errorf("got  %s\nwant %s", got.Args.Activity, tt.want)
			}
		})
	}
}

func TestPongCarriesThePingPayload(t *testing.T) {
	ping, err := codec.Decode(codec.Frame{Op: codec.OpPing, Payload: []byte(fixturePing)})
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if got, want := wire(t, codec.Pong(ping.Payload)), "\x04\x00\x00\x00\x0a\x00\x00\x00"+fixturePing; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestDecode(t *testing.T) {
	tests := []struct {
		name    string
		op      codec.Opcode
		payload string
		want    codec.Message
	}{
		{"ready", codec.OpFrame, fixtureReady, codec.Message{Kind: codec.KindReady}},
		{"error", codec.OpFrame, fixtureError, codec.Message{Kind: codec.KindError, Nonce: fixtureNonce, Code: 4000, Text: "Invalid payload"}},
		{"error without data", codec.OpFrame, `{"cmd":"SET_ACTIVITY","evt":"ERROR","nonce":"n"}`, codec.Message{Kind: codec.KindError, Nonce: "n"}},
		{"error with null data", codec.OpFrame, `{"evt":"ERROR","data":null}`, codec.Message{Kind: codec.KindError}},
		{"error with unknown fields", codec.OpFrame, `{"evt":"ERROR","data":{"code":1,"extra":[1]},"more":{}}`, codec.Message{Kind: codec.KindError, Code: 1}},
		{"acknowledgement", codec.OpFrame, fixtureAck, codec.Message{Kind: codec.KindAck, Nonce: fixtureNonce}},
		{"acknowledgement of a clear", codec.OpFrame, `{"cmd":"SET_ACTIVITY","data":null,"nonce":"n"}`, codec.Message{Kind: codec.KindAck, Nonce: "n"}},
		{"another dispatch", codec.OpFrame, `{"cmd":"DISPATCH","evt":"ACTIVITY_JOIN","data":{"secret":"s"}}`, codec.Message{}},
		{"ready event on another command", codec.OpFrame, `{"cmd":"SUBSCRIBE","evt":"READY"}`, codec.Message{}},
		{"reply to another command", codec.OpFrame, `{"cmd":"SUBSCRIBE","evt":null,"nonce":"n"}`, codec.Message{}},
		{"set-activity with another event", codec.OpFrame, `{"cmd":"SET_ACTIVITY","evt":"OTHER","nonce":"n"}`, codec.Message{}},
		{"empty object", codec.OpFrame, `{}`, codec.Message{}},
		{"close", codec.OpClose, fixtureClose, codec.Message{Kind: codec.KindClose, Code: 4000, Text: "Invalid Client ID"}},
		{"close with unknown fields", codec.OpClose, `{"code":1000,"why":true}`, codec.Message{Kind: codec.KindClose, Code: 1000}},
		{"ping", codec.OpPing, fixturePing, codec.Message{Kind: codec.KindPing, Payload: []byte(fixturePing)}},
		{"ping that is not JSON", codec.OpPing, "\xff", codec.Message{Kind: codec.KindPing, Payload: []byte("\xff")}},
		{"pong", codec.OpPong, fixturePing, codec.Message{}},
		{"handshake", codec.OpHandshake, fixtureHandshake[8:], codec.Message{}},
		{"unknown opcode", 99, "not json", codec.Message{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := codec.Decode(codec.Frame{Op: tt.op, Payload: []byte(tt.payload)})
			if err != nil {
				t.Fatalf("Decode: %v", err)
			}
			if got.Kind != tt.want.Kind || got.Nonce != tt.want.Nonce || got.Code != tt.want.Code ||
				got.Text != tt.want.Text || !bytes.Equal(got.Payload, tt.want.Payload) {
				t.Errorf("got %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestDecodeMalformed(t *testing.T) {
	tests := []struct {
		name    string
		op      codec.Opcode
		payload string
	}{
		{"frame that is empty", codec.OpFrame, ""},
		{"frame that is not JSON", codec.OpFrame, "{"},
		{"frame that is not an object", codec.OpFrame, `[1]`},
		{"frame with a numeric nonce", codec.OpFrame, `{"cmd":"SET_ACTIVITY","nonce":1}`},
		{"error whose data is not an object", codec.OpFrame, `{"evt":"ERROR","data":"no"}`},
		{"error whose code is not a number", codec.OpFrame, `{"evt":"ERROR","data":{"code":"4000"}}`},
		{"close that is empty", codec.OpClose, ""},
		{"close that is not an object", codec.OpClose, `"bye"`},
		{"close whose code overflows", codec.OpClose, `{"code":1e99}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := codec.Decode(codec.Frame{Op: tt.op, Payload: []byte(tt.payload)})
			if !errors.Is(err, codec.ErrMalformed) {
				t.Errorf("got %v, want ErrMalformed", err)
			}
			if got.Kind != codec.KindUnknown {
				t.Errorf("got kind %d with an error, want unknown", got.Kind)
			}
		})
	}
}
