package fakediscord_test

import (
	"bytes"
	"errors"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Zafnok/claude-rich-presence/internal/testutil/fakediscord"
)

const (
	setActivity   = `{"cmd":"SET_ACTIVITY","args":{"pid":4321,"activity":{"details":"Fixing a bug","state":"In a test"}},"nonce":"n-1"}`
	clearActivity = `{"cmd":"SET_ACTIVITY","args":{"pid":4321,"activity":null},"nonce":"n-2"}`
)

// The frame layout, written out in full once: opcode 0 and length 32, both
// little-endian, then the payload.
func TestHandshakeBytesOnTheWire(t *testing.T) {
	srv := fakediscord.Start(t, fakediscord.Options{})
	c := connect(t, srv.Addr())
	c.write(append([]byte{0x00, 0x00, 0x00, 0x00, 0x20, 0x00, 0x00, 0x00}, handshake...))

	m := c.readJSON(opFrame)
	if m["cmd"] != "DISPATCH" || m["evt"] != "READY" {
		t.Fatalf("answer = %v, want a READY dispatch", m)
	}
	data, _ := m["data"].(map[string]any)
	if data["v"] != float64(1) {
		t.Errorf("ready data = %v, want v 1", data)
	}
	if _, ok := data["user"].(map[string]any); !ok {
		t.Errorf("ready data = %v, want a user", data)
	}
	if _, ok := data["config"].(map[string]any); !ok {
		t.Errorf("ready data = %v, want a config", data)
	}
}

func TestDefaultConversationIsAnsweredAndRecorded(t *testing.T) {
	srv := fakediscord.Start(t, fakediscord.Options{Now: stepClock()})
	c := connect(t, srv.Addr())
	c.handshake()

	c.send(opFrame, setActivity)
	ack := c.readJSON(opFrame)
	if ack["cmd"] != "SET_ACTIVITY" || ack["nonce"] != "n-1" || ack["evt"] != nil {
		t.Errorf("ack = %v, want SET_ACTIVITY with nonce n-1 and no event", ack)
	}
	if data, _ := ack["data"].(map[string]any); data["details"] != "Fixing a bug" || data["state"] != "In a test" {
		t.Errorf("ack data = %v, want the activity", ack["data"])
	}

	c.send(opFrame, clearActivity)
	ack = c.readJSON(opFrame)
	if ack["nonce"] != "n-2" || ack["data"] != nil || ack["evt"] != nil {
		t.Errorf("ack of the clear = %v, want nonce n-2 and null data", ack)
	}

	// An absent activity is a clear too.
	c.send(opFrame, `{"cmd":"SET_ACTIVITY","args":{"pid":4321},"nonce":"n-3"}`)
	if ack = c.readJSON(opFrame); ack["nonce"] != "n-3" || ack["data"] != nil {
		t.Errorf("ack of the clear without an activity = %v", ack)
	}

	c.send(opPing, `{"beat":1}`)
	if pong := c.read(opPong); string(pong) != `{"beat":1}` {
		t.Errorf("pong payload = %s, want the ping's", pong)
	}

	// A pong is recorded and not answered; the close that follows is the
	// next thing the server acts on.
	c.send(opPong, `{"beat":2}`)
	c.send(opClose, `{"code":1000,"message":"bye"}`)
	c.expectClosed()
	srv.Await(fakediscord.KindDisconnect, 1)

	events := srv.Events()
	wantKinds := []fakediscord.Kind{
		fakediscord.KindHandshake, fakediscord.KindSetActivity, fakediscord.KindClearActivity,
		fakediscord.KindClearActivity, fakediscord.KindPing, fakediscord.KindPong,
		fakediscord.KindClose, fakediscord.KindDisconnect,
	}
	if len(events) != len(wantKinds) {
		t.Fatalf("recorded %d events, want %d: %+v", len(events), len(wantKinds), events)
	}
	start := time.Unix(1_700_000_000, 0)
	for i, ev := range events {
		if ev.Kind != wantKinds[i] {
			t.Errorf("event %d is %v, want %v", i, ev.Kind, wantKinds[i])
		}
		if want := start.Add(time.Duration(i+1) * time.Second); !ev.Time.Equal(want) {
			t.Errorf("event %d time = %v, want %v from the test's clock", i, ev.Time, want)
		}
		if ev.Conn != 1 {
			t.Errorf("event %d connection = %d, want 1", i, ev.Conn)
		}
	}

	if hs := events[0]; hs.Version != 1 || hs.ClientID != "1234567890" || hs.Opcode != opHandshake || string(hs.Payload) != handshake {
		t.Errorf("handshake event = %+v", hs)
	}
	set := events[1]
	if set.Command != "SET_ACTIVITY" || set.Nonce != "n-1" || set.PID != 4321 || set.Opcode != opFrame || string(set.Payload) != setActivity {
		t.Errorf("set-activity event = %+v", set)
	}
	if string(set.Activity) != `{"details":"Fixing a bug","state":"In a test"}` {
		t.Errorf("recorded activity = %s", set.Activity)
	}
	if clear := events[2]; clear.Nonce != "n-2" || clear.Activity != nil || clear.PID != 4321 {
		t.Errorf("clear event = %+v", clear)
	}
	if ping := events[4]; string(ping.Payload) != `{"beat":1}` || ping.Opcode != opPing {
		t.Errorf("ping event = %+v", ping)
	}
	if end := events[7]; end.Payload != nil || end.Opcode != 0 {
		t.Errorf("disconnect event = %+v", end)
	}
}

func TestNonceIsEchoedAsItArrived(t *testing.T) {
	srv := fakediscord.Start(t, fakediscord.Options{})
	c := connect(t, srv.Addr())
	c.handshake()

	c.send(opFrame, `{"cmd":"SET_ACTIVITY","args":{"pid":1,"activity":{"details":"ab"}},"nonce":42}`)
	if ack := c.readJSON(opFrame); ack["nonce"] != float64(42) {
		t.Errorf("ack = %v, want the numeric nonce back", ack)
	}
	c.send(opFrame, `{"cmd":"SET_ACTIVITY","args":{"pid":1,"activity":{"details":"ab"}}}`)
	if ack := c.readJSON(opFrame); ack["nonce"] != nil {
		t.Errorf("ack = %v, want a null nonce", ack)
	}
	for i, ev := range srv.Await(fakediscord.KindSetActivity, 2) {
		if ev.Nonce != "" {
			t.Errorf("event %d nonce = %q, want none recorded", i, ev.Nonce)
		}
	}
}

func TestUnknownCommandGetsAnErrorEventAndTheConnectionStays(t *testing.T) {
	srv := fakediscord.Start(t, fakediscord.Options{})
	c := connect(t, srv.Addr())
	c.handshake()

	c.send(opFrame, `{"cmd":"GET_GUILDS","args":{},"nonce":"n-9"}`)
	m := c.readJSON(opFrame)
	data, _ := m["data"].(map[string]any)
	if m["evt"] != "ERROR" || m["cmd"] != "GET_GUILDS" || m["nonce"] != "n-9" || data["code"] != float64(fakediscord.ErrorInvalidCommand) {
		t.Errorf("answer = %v, want an ERROR event with code 4002 and the nonce", m)
	}
	c.send(opPing, `"still here"`)
	c.read(opPong)

	if evs := srv.Await(fakediscord.KindCommand, 1); evs[0].Command != "GET_GUILDS" || evs[0].Nonce != "n-9" {
		t.Errorf("command event = %+v", evs[0])
	}
}

func TestWhatDiscordRefuses(t *testing.T) {
	tests := []struct {
		name           string
		afterHandshake bool
		send           []byte
		wantCode       int
		wantKind       fakediscord.Kind
	}{
		{"wrong version", false, raw(opHandshake, `{"v":2,"client_id":"1234567890"}`), fakediscord.CloseInvalidVersion, fakediscord.KindHandshake},
		{"missing version", false, raw(opHandshake, `{"client_id":"1234567890"}`), fakediscord.CloseInvalidVersion, fakediscord.KindHandshake},
		{"empty client id", false, raw(opHandshake, `{"v":1,"client_id":""}`), fakediscord.CloseInvalidClientID, fakediscord.KindHandshake},
		{"missing client id", false, raw(opHandshake, `{"v":1}`), fakediscord.CloseInvalidClientID, fakediscord.KindHandshake},
		{"handshake that is not JSON", false, raw(opHandshake, `{"v":`), fakediscord.CloseUnsupported, fakediscord.KindInvalid},
		{"handshake with a client id of the wrong type", false, raw(opHandshake, `{"v":1,"client_id":7}`), fakediscord.CloseUnsupported, fakediscord.KindInvalid},
		{"command before the handshake", false, raw(opFrame, setActivity), fakediscord.CloseUnsupported, fakediscord.KindSetActivity},
		{"second handshake", true, raw(opHandshake, handshake), fakediscord.CloseUnsupported, fakediscord.KindHandshake},
		{"command that is not JSON", true, raw(opFrame, `not json`), fakediscord.CloseUnsupported, fakediscord.KindInvalid},
		{"unknown opcode", true, raw(77, `{}`), fakediscord.CloseUnsupported, fakediscord.KindInvalid},
		{"declared length over the limit", true, []byte{1, 0, 0, 0, 0xF9, 0xFF, 0x00, 0x00}, fakediscord.CloseUnsupported, fakediscord.KindInvalid},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := fakediscord.Start(t, fakediscord.Options{})
			c := connect(t, srv.Addr())
			want := 1
			if tt.afterHandshake {
				c.handshake()
				if tt.wantKind == fakediscord.KindHandshake {
					want = 2
				}
			}
			c.write(tt.send)
			c.expectCloseFrame(tt.wantCode)
			srv.Await(tt.wantKind, want)
			srv.Await(fakediscord.KindDisconnect, 1)
		})
	}
}

func TestLargestFrameIsAccepted(t *testing.T) {
	srv := fakediscord.Start(t, fakediscord.Options{})
	c := connect(t, srv.Addr())
	c.handshake()

	// 0xFFF8 bytes of payload make a frame of exactly 64 KiB.
	payload := `"` + strings.Repeat("x", fakediscord.MaxFrameSize-8-2) + `"`
	c.send(opPing, payload)
	if pong := c.read(opPong); string(pong) != payload {
		t.Errorf("pong payload has %d bytes, want the ping's %d", len(pong), len(payload))
	}
}

// A pipe or socket is a byte stream: a frame may arrive in pieces, and two
// frames may arrive together.
func TestFramesSplitAndJoinedOnTheStream(t *testing.T) {
	srv := fakediscord.Start(t, fakediscord.Options{})
	c := connect(t, srv.Addr())

	for _, b := range raw(opHandshake, handshake) {
		c.write([]byte{b})
	}
	if m := c.readJSON(opFrame); m["evt"] != "READY" {
		t.Fatalf("answer to a handshake sent a byte at a time = %v", m)
	}

	c.write(append(raw(opFrame, setActivity), raw(opFrame, clearActivity)...))
	if ack := c.readJSON(opFrame); ack["nonce"] != "n-1" {
		t.Errorf("first ack = %v", ack)
	}
	if ack := c.readJSON(opFrame); ack["nonce"] != "n-2" {
		t.Errorf("second ack = %v", ack)
	}
}

func TestRejectHandshake(t *testing.T) {
	srv := fakediscord.Start(t, fakediscord.Options{Behavior: fakediscord.Behavior{RejectHandshake: 4000}})
	c := connect(t, srv.Addr())
	c.send(opHandshake, handshake)
	c.expectCloseFrame(4000)
	if evs := srv.Await(fakediscord.KindHandshake, 1); evs[0].ClientID != "1234567890" {
		t.Errorf("handshake event = %+v", evs[0])
	}
}

func TestActivityError(t *testing.T) {
	srv := fakediscord.Start(t, fakediscord.Options{})
	c := connect(t, srv.Addr())
	c.handshake()
	srv.Set(fakediscord.Behavior{ActivityError: 4000})

	for _, tt := range []struct{ payload, nonce string }{{setActivity, "n-1"}, {clearActivity, "n-2"}} {
		c.send(opFrame, tt.payload)
		m := c.readJSON(opFrame)
		data, _ := m["data"].(map[string]any)
		if m["evt"] != "ERROR" || m["cmd"] != "SET_ACTIVITY" || m["nonce"] != tt.nonce || data["code"] != float64(4000) {
			t.Errorf("answer = %v, want an ERROR event with code 4000 and nonce %s", m, tt.nonce)
		}
	}
	srv.Await(fakediscord.KindSetActivity, 1)
	srv.Await(fakediscord.KindClearActivity, 1)
}

func TestCloseOnConnect(t *testing.T) {
	srv := fakediscord.Start(t, fakediscord.Options{Behavior: fakediscord.Behavior{CloseOnConnect: true}})
	c := connect(t, srv.Addr())
	c.expectClosed()
	srv.Await(fakediscord.KindDisconnect, 1)
	if evs := srv.Events(); len(evs) != 1 {
		t.Errorf("recorded %+v, want only the disconnect", evs)
	}
}

func TestCloseAfterFrames(t *testing.T) {
	t.Run("after the handshake", func(t *testing.T) {
		srv := fakediscord.Start(t, fakediscord.Options{Behavior: fakediscord.Behavior{CloseAfterFrames: 1}})
		c := connect(t, srv.Addr())
		c.send(opHandshake, handshake)
		c.expectClosed()
		srv.Await(fakediscord.KindHandshake, 1)
	})
	t.Run("after the first command", func(t *testing.T) {
		srv := fakediscord.Start(t, fakediscord.Options{Behavior: fakediscord.Behavior{CloseAfterFrames: 2}})
		c := connect(t, srv.Addr())
		c.handshake()
		c.send(opFrame, setActivity)
		c.expectClosed()
		srv.Await(fakediscord.KindSetActivity, 1)
		srv.Await(fakediscord.KindDisconnect, 1)

		// The count is per connection.
		connect(t, srv.Addr()).handshake()
	})
}

func TestSilent(t *testing.T) {
	srv := fakediscord.Start(t, fakediscord.Options{Behavior: fakediscord.Behavior{Silent: true}})
	c := connect(t, srv.Addr())
	c.send(opHandshake, handshake)
	c.send(opFrame, setActivity)
	c.send(opPing, `"unanswered"`)
	c.send(77, `{}`)
	srv.Await(fakediscord.KindHandshake, 1)
	srv.Await(fakediscord.KindSetActivity, 1)
	srv.Await(fakediscord.KindPing, 1)
	srv.Await(fakediscord.KindInvalid, 1)

	// The first thing the client ever reads is the answer to a ping sent
	// after the silence was lifted: nothing was sent before, and the
	// connection was not closed.
	srv.Set(fakediscord.Behavior{})
	c.send(opPing, `"answered"`)
	if pong := c.read(opPong); string(pong) != `"answered"` {
		t.Errorf("first frame read = %s, want the pong for the later ping", pong)
	}
}

func TestSilentStillLetsTheClientHangUp(t *testing.T) {
	srv := fakediscord.Start(t, fakediscord.Options{Behavior: fakediscord.Behavior{Silent: true}})
	c := connect(t, srv.Addr())
	c.send(opHandshake, handshake)
	c.send(opClose, `{"code":1000,"message":"bye"}`)
	c.expectClosed()
	srv.Await(fakediscord.KindClose, 1)
}

func TestSilentAfterAnOversizedHeaderKeepsTheConnectionOpen(t *testing.T) {
	srv := fakediscord.Start(t, fakediscord.Options{Behavior: fakediscord.Behavior{Silent: true}})
	c := connect(t, srv.Addr())
	c.write([]byte{1, 0, 0, 0, 0xFF, 0xFF, 0xFF, 0xFF})
	c.write(raw(opPing, `"lost in the oversized frame"`))
	if evs := srv.Await(fakediscord.KindInvalid, 1); evs[0].Payload != nil || evs[0].Opcode != opFrame {
		t.Errorf("invalid event = %+v", evs[0])
	}
	_ = c.rw.Close()
	srv.Await(fakediscord.KindDisconnect, 1)
	if n := len(srv.Events()); n != 2 {
		t.Errorf("recorded %d events, want the invalid frame and the disconnect: %+v", n, srv.Events())
	}
}

func TestDelay(t *testing.T) {
	asked := make(chan time.Duration, 1)
	release := make(chan time.Time)
	srv := fakediscord.Start(t, fakediscord.Options{
		Behavior: fakediscord.Behavior{Delay: 5 * time.Second},
		After: func(d time.Duration) <-chan time.Time {
			asked <- d
			return release
		},
	})
	c := connect(t, srv.Addr())
	c.send(opHandshake, handshake)

	if d := within(t, "the server to start waiting", func() time.Duration { return <-asked }); d != 5*time.Second {
		t.Errorf("waited on %v, want 5s", d)
	}
	// The server is now holding the ready event. A ping sent meanwhile
	// reaches the client first.
	if err := srv.Ping([]byte(`"first"`)); err != nil {
		t.Fatal(err)
	}
	if ping := c.read(opPing); string(ping) != `"first"` {
		t.Errorf("first frame = %s, want the ping", ping)
	}
	close(release)
	if m := c.readJSON(opFrame); m["evt"] != "READY" {
		t.Errorf("after the delay got %v, want READY", m)
	}
}

func TestShutdownDuringADelayLeavesNothingRunning(t *testing.T) {
	asked := make(chan time.Duration, 1)
	srv := fakediscord.Start(t, fakediscord.Options{
		Behavior: fakediscord.Behavior{Delay: time.Hour},
		After: func(d time.Duration) <-chan time.Time {
			asked <- d
			return nil // never fires
		},
	})
	c := connect(t, srv.Addr())
	c.send(opHandshake, handshake)
	within(t, "the server to start waiting", func() time.Duration { return <-asked })

	// Close fails the test itself if the waiting goroutine does not end.
	srv.Close()
	c.expectClosed()
}

func TestDisconnectDuringADelayEndsTheConnection(t *testing.T) {
	asked := make(chan time.Duration, 1)
	srv := fakediscord.Start(t, fakediscord.Options{
		Behavior: fakediscord.Behavior{Delay: time.Hour},
		After: func(d time.Duration) <-chan time.Time {
			asked <- d
			return nil // never fires
		},
	})
	c := connect(t, srv.Addr())
	c.send(opHandshake, handshake)
	within(t, "the server to start waiting", func() time.Duration { return <-asked })

	if err := srv.Disconnect(); err != nil {
		t.Fatal(err)
	}
	c.expectClosed()
	srv.Await(fakediscord.KindDisconnect, 1)
}

func TestPing(t *testing.T) {
	srv := fakediscord.Start(t, fakediscord.Options{})
	c := connect(t, srv.Addr())
	c.handshake()
	if err := srv.Ping([]byte(`{"n":7}`)); err != nil {
		t.Fatal(err)
	}
	if got := c.read(opPing); string(got) != `{"n":7}` {
		t.Errorf("ping payload = %s", got)
	}
	c.send(opPong, `{"n":7}`)
	if evs := srv.Await(fakediscord.KindPong, 1); string(evs[0].Payload) != `{"n":7}` {
		t.Errorf("pong event = %+v", evs[0])
	}
}

func TestSendClose(t *testing.T) {
	srv := fakediscord.Start(t, fakediscord.Options{})
	c := connect(t, srv.Addr())
	c.handshake()
	if err := srv.SendClose(4002, "rate limited"); err != nil {
		t.Fatal(err)
	}
	if m := c.readJSON(opClose); m["code"] != float64(4002) || m["message"] != "rate limited" {
		t.Errorf("close frame = %v", m)
	}
	// The frame alone does not end the connection; Disconnect does.
	c.send(opPing, `1`)
	c.read(opPong)
	if err := srv.Disconnect(); err != nil {
		t.Fatal(err)
	}
	c.expectClosed()
}

func TestSendMalformed(t *testing.T) {
	oversize := []byte{1, 0, 0, 0, 0x00, 0x00, 0x01, 0x00}
	truncated := append([]byte{1, 0, 0, 0, 64, 0, 0, 0}, 0, 0, 0, 0, 0)
	tests := []struct {
		name string
		kind fakediscord.Malformed
		want []byte
	}{
		{"payload that is not JSON", fakediscord.MalformedJSON, raw(opFrame, `{"cmd":`)},
		{"less payload than declared", fakediscord.MalformedTruncated, truncated},
		{"declared length over the limit", fakediscord.MalformedOversize, oversize},
		{"unknown opcode", fakediscord.MalformedOpcode, raw(99, `{}`)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := fakediscord.Start(t, fakediscord.Options{})
			c := connect(t, srv.Addr())
			c.handshake()
			if err := srv.SendMalformed(tt.kind); err != nil {
				t.Fatal(err)
			}
			// Exactly these bytes arrive and nothing follows them.
			if err := srv.Disconnect(); err != nil {
				t.Fatal(err)
			}
			got := within(t, "the malformed bytes", func() []byte {
				var buf bytes.Buffer
				_, _ = buf.ReadFrom(c.rw)
				return buf.Bytes()
			})
			if !bytes.Equal(got, tt.want) {
				t.Errorf("received % x, want % x", got, tt.want)
			}
		})
	}

	srv := fakediscord.Start(t, fakediscord.Options{})
	connect(t, srv.Addr()).handshake()
	if err := srv.SendMalformed(0); err == nil || errors.Is(err, fakediscord.ErrNoConnection) {
		t.Errorf("SendMalformed(0) = %v, want an error about the kind", err)
	}
}

func TestSendRaw(t *testing.T) {
	srv := fakediscord.Start(t, fakediscord.Options{})
	c := connect(t, srv.Addr())
	c.handshake()
	if err := srv.SendRaw(raw(opFrame, `{"cmd":"DISPATCH","evt":"ERROR","data":{"code":1000,"message":"x"}}`)); err != nil {
		t.Fatal(err)
	}
	if m := c.readJSON(opFrame); m["evt"] != "ERROR" {
		t.Errorf("frame = %v", m)
	}
}

func TestSendingWithNoConnection(t *testing.T) {
	srv := fakediscord.Start(t, fakediscord.Options{})
	sends := map[string]func() error{
		"Ping":          func() error { return srv.Ping(nil) },
		"SendClose":     func() error { return srv.SendClose(1000, "x") },
		"SendMalformed": func() error { return srv.SendMalformed(fakediscord.MalformedJSON) },
		"SendRaw":       func() error { return srv.SendRaw([]byte{1}) },
		"Disconnect":    srv.Disconnect,
	}
	check := func(when string) {
		for name, send := range sends {
			if err := send(); !errors.Is(err, fakediscord.ErrNoConnection) {
				t.Errorf("%s %s = %v, want ErrNoConnection", name, when, err)
			}
		}
	}
	check("before any connection")

	c := connect(t, srv.Addr())
	c.handshake()
	_ = c.rw.Close()
	srv.Await(fakediscord.KindDisconnect, 1)
	check("after the client hung up")
}

func TestSendsGoToTheMostRecentConnection(t *testing.T) {
	srv := fakediscord.Start(t, fakediscord.Options{})
	first := connect(t, srv.Addr())
	first.handshake()
	second := connect(t, srv.Addr())
	second.handshake()

	if err := srv.Ping([]byte(`2`)); err != nil {
		t.Fatal(err)
	}
	second.read(opPing)
	if err := srv.Disconnect(); err != nil {
		t.Fatal(err)
	}
	second.expectClosed()
	srv.Await(fakediscord.KindDisconnect, 1)

	// With the second gone, the first is the most recent, and still served.
	if err := srv.Ping([]byte(`1`)); err != nil {
		t.Fatal(err)
	}
	if got := first.read(opPing); string(got) != `1` {
		t.Errorf("ping on the first connection = %s", got)
	}
	hs := srv.Await(fakediscord.KindHandshake, 2)
	if hs[0].Conn != 1 || hs[1].Conn != 2 {
		t.Errorf("handshakes arrived on connections %d and %d, want 1 and 2", hs[0].Conn, hs[1].Conn)
	}
}

func TestTwoServersAtDifferentIndicesDoNotInterfere(t *testing.T) {
	ns := fakediscord.NewNamespace(t)
	low := ns.Start(t, 0, fakediscord.Options{})
	high := ns.Start(t, 3, fakediscord.Options{Behavior: fakediscord.Behavior{RejectHandshake: 4000}})

	for _, srv := range []*fakediscord.Server{low, high} {
		if srv.Namespace() != ns {
			t.Errorf("server %d is in another namespace", srv.Index())
		}
		if want := ns.Prefix() + strconv.Itoa(srv.Index()); srv.Addr() != want || ns.Addr(srv.Index()) != want {
			t.Errorf("server %d listens at %q, want the prefix and the index, %q", srv.Index(), srv.Addr(), want)
		}
	}
	if !strings.HasSuffix(ns.Prefix(), "discord-ipc-") {
		t.Errorf("Prefix() = %q, want it to end with discord-ipc-", ns.Prefix())
	}

	a := connect(t, low.Addr())
	a.send(opHandshake, `{"v":1,"client_id":"for-low"}`)
	b := connect(t, high.Addr())
	b.send(opHandshake, `{"v":1,"client_id":"for-high"}`)
	if m := a.readJSON(opFrame); m["evt"] != "READY" {
		t.Errorf("index 0 answered %v, want READY", m)
	}
	b.expectCloseFrame(4000)

	if evs := low.Await(fakediscord.KindHandshake, 1); len(evs) != 1 || evs[0].ClientID != "for-low" {
		t.Errorf("index 0 recorded %+v", evs)
	}
	if evs := high.Await(fakediscord.KindHandshake, 1); len(evs) != 1 || evs[0].ClientID != "for-high" {
		t.Errorf("index 3 recorded %+v", evs)
	}

	// An index nobody listens at is not there to connect to.
	if rw, err := dial(ns.Addr(1)); err == nil {
		_ = rw.Close()
		t.Errorf("connected at index 1, where there is no server")
	}

	// Stopping one leaves the other serving.
	high.Close()
	a.send(opPing, `1`)
	a.read(opPong)
	connect(t, low.Addr()).handshake()
}

func TestTwoNamespacesDoNotShareAName(t *testing.T) {
	a := fakediscord.Start(t, fakediscord.Options{})
	b := fakediscord.Start(t, fakediscord.Options{})
	if a.Addr() == b.Addr() {
		t.Fatalf("two servers share %q", a.Addr())
	}
	connect(t, a.Addr()).handshake()
	connect(t, b.Addr()).handshake()
	if len(a.Await(fakediscord.KindHandshake, 1)) != 1 || len(b.Await(fakediscord.KindHandshake, 1)) != 1 {
		t.Errorf("each server should have recorded exactly one handshake")
	}
}

func TestStartFailures(t *testing.T) {
	t.Run("index already taken", func(t *testing.T) {
		ns := fakediscord.NewNamespace(t)
		ns.Start(t, 2, fakediscord.Options{})
		tb := &fakeTB{}
		message := fatalMessage(tb, func() { ns.Start(tb, 2, fakediscord.Options{}) })
		if !strings.Contains(message, "listening at "+ns.Addr(2)) {
			t.Errorf("fatal message = %q", message)
		}
	})
	for _, index := range []int{-1, 10} {
		t.Run("index "+strconv.Itoa(index), func(t *testing.T) {
			ns := fakediscord.NewNamespace(t)
			tb := &fakeTB{}
			message := fatalMessage(tb, func() { ns.Start(tb, index, fakediscord.Options{}) })
			if !strings.Contains(message, "not between 0 and 9") {
				t.Errorf("fatal message = %q", message)
			}
		})
	}
}

func TestShutdownRemovesTheEndpoint(t *testing.T) {
	var addr string
	t.Run("server", func(t *testing.T) {
		srv := fakediscord.Start(t, fakediscord.Options{})
		addr = srv.Addr()
		c := connect(t, addr)
		c.handshake()

		srv.Close()
		c.expectClosed()
		if rw, err := dial(addr); !errors.Is(err, fs.ErrNotExist) {
			if err == nil {
				_ = rw.Close()
			}
			t.Errorf("dialing after Close = %v, want a not-exist error", err)
		}
		// A second Close, and the one the test's cleanup makes, do nothing.
		srv.Close()
	})
	if rw, err := dial(addr); !errors.Is(err, fs.ErrNotExist) {
		if err == nil {
			_ = rw.Close()
		}
		t.Errorf("dialing after the test = %v, want a not-exist error", err)
	}
}

func TestShutdownIsCleanWithConnectionsInEveryState(t *testing.T) {
	srv := fakediscord.Start(t, fakediscord.Options{})
	idle := connect(t, srv.Addr())
	busy := connect(t, srv.Addr())
	busy.handshake()
	halfway := connect(t, srv.Addr())
	halfway.write([]byte{1, 0, 0}) // part of a header
	srv.Await(fakediscord.KindHandshake, 1)

	srv.Close()
	for _, c := range []*client{idle, busy, halfway} {
		c.expectClosed()
	}
}

func TestShutdownReportsAGoroutineLeftBehind(t *testing.T) {
	tb := &fakeTB{}
	srv := fakediscord.Start(tb, fakediscord.Options{})
	release := srv.LeakForTest()
	srv.ExpirePatienceForTest()
	srv.Close()
	errs := tb.reported()
	// How many are still running at that instant depends on how far the
	// server's own goroutines have got; that the one left behind is reported
	// does not.
	if len(errs) != 1 || !strings.Contains(errs[0], "goroutines still running") {
		t.Errorf("reported %q, want one error about goroutines left running", errs)
	}
	release()
	tb.finish()
	if errs := tb.reported(); len(errs) != 1 {
		t.Errorf("cleanup reported more: %q", errs)
	}
}

func TestAwaitFailsTheTestWhenTheEventNeverComes(t *testing.T) {
	tb := &fakeTB{}
	srv := fakediscord.Start(tb, fakediscord.Options{})
	t.Cleanup(tb.finish)
	connect(t, srv.Addr()).handshake()
	srv.Await(fakediscord.KindHandshake, 1)
	srv.ExpirePatienceForTest()

	message := fatalMessage(tb, func() { srv.Await(fakediscord.KindSetActivity, 1) })
	if !strings.Contains(message, "for 1 set-activity events, have 0") {
		t.Errorf("fatal message = %q", message)
	}
}

func TestKindNames(t *testing.T) {
	names := map[fakediscord.Kind]string{
		fakediscord.KindHandshake:     "handshake",
		fakediscord.KindSetActivity:   "set-activity",
		fakediscord.KindClearActivity: "clear-activity",
		fakediscord.KindCommand:       "command",
		fakediscord.KindPing:          "ping",
		fakediscord.KindPong:          "pong",
		fakediscord.KindClose:         "close",
		fakediscord.KindInvalid:       "invalid",
		fakediscord.KindDisconnect:    "disconnect",
		fakediscord.Kind(0):           "unknown",
	}
	for kind, want := range names {
		if got := kind.String(); got != want {
			t.Errorf("Kind(%d).String() = %q, want %q", int(kind), got, want)
		}
	}
}

// The fake is a second implementation of the framing. If it borrowed the
// production one, the two could no longer catch each other's mistakes.
func TestPackageDoesNotImportTheProductionDiscordCode(t *testing.T) {
	// Every Go file in the directory, whatever its build constraints.
	names, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	if len(names) == 0 {
		t.Fatal("found no source files to check")
	}
	for _, name := range names {
		file, err := parser.ParseFile(token.NewFileSet(), name, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		for _, imp := range file.Imports {
			if strings.Contains(imp.Path.Value, "/internal/discord") {
				t.Errorf("%s imports %s", name, imp.Path.Value)
			}
		}
	}
}
