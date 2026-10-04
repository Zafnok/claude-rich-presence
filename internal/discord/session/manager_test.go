package session_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Zafnok/claude-rich-presence/internal/discord/session"
	"github.com/Zafnok/claude-rich-presence/internal/schedule"
	"github.com/Zafnok/claude-rich-presence/internal/testutil/fakediscord"
)

const interval = schedule.DiscordInterval

func TestShowsTheActivityOnceDiscordAppears(t *testing.T) {
	h := newHarness(t)
	h.start()
	h.inBackoff(1, 0)
	h.wantStatus(session.Disconnected, session.ErrorNotRunning)

	h.m.Set(activity("a"))
	eventually(t, "the update to be dropped", func() bool { return h.logs.count("activity update dropped") == 1 })

	srv := h.serve(fakediscord.Behavior{})
	h.clock.Advance(time.Second)
	h.ready(1)

	// The dropped update counted towards the rate limit, so the activity is
	// due one interval after it was set.
	h.timers(1)
	h.clock.Advance(interval - 2*time.Second)
	if n := count(srv, fakediscord.KindSetActivity); n != 0 {
		t.Fatalf("%d activities sent before the interval had passed", n)
	}
	h.clock.Advance(time.Second)
	ev := srv.Await(fakediscord.KindSetActivity, 1)[0]
	if !strings.Contains(string(ev.Activity), "details-a") {
		t.Errorf("activity sent is %s, want the one that was set", ev.Activity)
	}
	if ev.PID != int64(os.Getpid()) {
		t.Errorf("process id sent is %d, want this process's, %d", ev.PID, os.Getpid())
	}
	if got := srv.Await(fakediscord.KindHandshake, 1)[0].ClientID; got != applicationID {
		t.Errorf("handshake carried application id %q, want %q", got, applicationID)
	}
	want := start.Add(interval)
	eventually(t, "the acknowledgement", func() bool { return h.m.Status().LastUpdate.Equal(want) })
	h.wantStatus(session.Ready, session.ErrorNotRunning)
}

func TestReconnectsAndResendsNoFasterThanTheSchedulerAllows(t *testing.T) {
	h := newHarness(t)
	srv := h.serve(fakediscord.Behavior{})
	h.start()
	h.ready(1)

	h.m.Set(activity("a"))
	srv.Await(fakediscord.KindSetActivity, 1)
	eventually(t, "the acknowledgement", func() bool { return h.m.Status().LastUpdate.Equal(start) })

	if err := srv.Disconnect(); err != nil {
		t.Fatal(err)
	}
	h.inBackoff(1, 0)
	h.wantStatus(session.Disconnected, session.ErrorConnectionLost)

	h.clock.Advance(time.Second)
	h.ready(2)
	h.timers(1)
	h.clock.Advance(interval - 2*time.Second)
	if n := count(srv, fakediscord.KindSetActivity); n != 1 {
		t.Fatalf("%d activities sent before the interval had passed, want 1", n)
	}
	h.clock.Advance(time.Second)

	sent := srv.Await(fakediscord.KindSetActivity, 2)
	if sent[1].Conn != 2 {
		t.Errorf("the activity was resent on connection %d, want 2", sent[1].Conn)
	}
	if !bytes.Equal(sent[0].Activity, sent[1].Activity) {
		t.Errorf("resent %s, want %s", sent[1].Activity, sent[0].Activity)
	}
	if gap := sent[1].Time.Sub(sent[0].Time); gap != interval {
		t.Errorf("activities were sent %v apart, want %v", gap, interval)
	}
}

func TestAnUnansweredHandshakeTimesOutAndIsRetried(t *testing.T) {
	h := newHarness(t)
	srv := h.serve(fakediscord.Behavior{Silent: true})
	h.start()
	srv.Await(fakediscord.KindHandshake, 1)
	h.wantStatus(session.Connecting, "")

	h.timers(1)
	h.clock.Advance(session.HandshakeTimeout)
	h.inBackoff(1, 0)
	h.wantStatus(session.Disconnected, session.ErrorHandshakeTimeout)

	srv.Set(fakediscord.Behavior{})
	h.clock.Advance(time.Second)
	h.ready(1)
	if n := count(srv, fakediscord.KindHandshake); n != 2 {
		t.Errorf("%d handshakes, want 2", n)
	}
}

func TestBackoffGrowsToTheCapAndResetsWhenReady(t *testing.T) {
	h := newHarness(t)
	h.start()
	var srv *fakediscord.Server
	waits := []time.Duration{1, 2, 4, 8, 16, 32, 60, 60}
	for i, want := range waits {
		want *= time.Second
		h.inBackoff(i+1, 0)
		if got := h.clock.lastWait(); got != want {
			t.Fatalf("wait %d is %v, want %v", i+1, got, want)
		}
		if i == len(waits)-1 {
			srv = h.serve(fakediscord.Behavior{})
		}
		h.clock.Advance(want)
	}
	h.ready(1)

	if err := srv.Disconnect(); err != nil {
		t.Fatal(err)
	}
	h.inBackoff(len(waits)+1, 0)
	if got := h.clock.lastWait(); got != time.Second {
		t.Errorf("the wait after being ready is %v, want 1s", got)
	}
}

func TestJitterPlacesTheWaitInTheUpperHalfOfTheDelay(t *testing.T) {
	for _, c := range []struct {
		name   string
		delay  time.Duration
		jitter float64
		want   time.Duration
	}{
		{"none", time.Second, 0, 500 * time.Millisecond},
		{"half", time.Second, 0.5, 750 * time.Millisecond},
		{"all", time.Second, 1, time.Second},
		{"at the cap", 60 * time.Second, 0.25, 37500 * time.Millisecond},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := session.Jittered(c.delay, c.jitter); got != c.want {
				t.Errorf("Jittered(%v, %v) = %v, want %v", c.delay, c.jitter, got, c.want)
			}
		})
	}
}

func TestAnInvalidApplicationIDIsRetriedAtTheCapAndReportedOnce(t *testing.T) {
	h := newHarness(t)
	reject := fakediscord.Behavior{RejectHandshake: fakediscord.CloseInvalidClientID}
	srv := h.serve(reject)
	h.start()
	for attempt := 1; attempt <= 3; attempt++ {
		if attempt > 1 {
			h.clock.Advance(60 * time.Second)
		}
		h.inBackoff(attempt, 0)
		h.wantStatus(session.Disconnected, session.ErrorInvalidApplicationID)
		if got := h.clock.lastWait(); got != 60*time.Second {
			t.Fatalf("wait after rejection %d is %v, want 60s", attempt, got)
		}
	}
	if n := count(srv, fakediscord.KindHandshake); n != 3 {
		t.Errorf("%d handshakes, want 3", n)
	}
	if n := h.logs.count("level=WARN"); n != 1 {
		t.Fatalf("%d warnings after three rejections, want 1:\n%s", n, h.logs)
	}
	if !strings.Contains(h.logs.String(), "error=invalid_application_id") {
		t.Errorf("the warning does not name the error class:\n%s", h.logs)
	}

	// Once the state has changed, the same fault is worth saying again.
	srv.Set(fakediscord.Behavior{})
	h.clock.Advance(60 * time.Second)
	h.ready(1)
	srv.Set(reject)
	if err := srv.Disconnect(); err != nil {
		t.Fatal(err)
	}
	h.inBackoff(4, 0)
	h.clock.Advance(time.Second)
	h.inBackoff(5, 0)
	if n := h.logs.count("level=WARN"); n != 2 {
		t.Errorf("%d warnings, want a second after being ready in between:\n%s", n, h.logs)
	}
}

func TestAPingIsAnsweredWithAPong(t *testing.T) {
	h := newHarness(t)
	srv := h.serve(fakediscord.Behavior{})
	h.start()
	h.ready(1)

	// Frames the manager has no use for come first, and change nothing: an
	// acknowledgement of a command it did not send, and another event.
	for _, payload := range []string{
		`{"cmd":"SET_ACTIVITY","nonce":"not-ours","data":null}`,
		`{"cmd":"DISPATCH","evt":"SOMETHING_ELSE","data":{}}`,
	} {
		if err := srv.SendRaw(rawFrame(1, payload)); err != nil {
			t.Fatal(err)
		}
	}
	payload := []byte(`{"n":7}`)
	if err := srv.Ping(payload); err != nil {
		t.Fatal(err)
	}
	if got := srv.Await(fakediscord.KindPong, 1)[0].Payload; !bytes.Equal(got, payload) {
		t.Errorf("pong carried %s, want %s", got, payload)
	}
	h.wantStatus(session.Ready, "")
	if got := h.m.Status().LastUpdate; !got.IsZero() {
		t.Errorf("last update is %v after an acknowledgement that was not ours, want none", got)
	}
}

func TestAnErrorResponseIsLoggedAndTheConnectionKept(t *testing.T) {
	h := newHarness(t)
	srv := h.serve(fakediscord.Behavior{ActivityError: 4000})
	h.start()
	h.ready(1)

	h.m.Set(activity("a"))
	eventually(t, "the warning", func() bool { return h.logs.count("level=WARN") == 1 })
	if log := h.logs.String(); !strings.Contains(log, "error=activity_rejected") || !strings.Contains(log, "code=4000") {
		t.Errorf("the warning does not carry the class and the code:\n%s", log)
	}
	h.wantStatus(session.Ready, session.ErrorActivityRejected)
	if got := h.m.Status().LastUpdate; !got.IsZero() {
		t.Errorf("last update is %v after a rejected activity, want none", got)
	}
	if n := count(srv, fakediscord.KindDisconnect); n != 0 {
		t.Errorf("the connection was dropped")
	}
}

func TestClearIsSentLikeAnyOtherUpdate(t *testing.T) {
	h := newHarness(t)
	srv := h.serve(fakediscord.Behavior{})
	h.start()
	h.ready(1)

	h.m.Set(activity("a"))
	srv.Await(fakediscord.KindSetActivity, 1)
	h.m.Clear()
	h.timers(1)
	h.clock.Advance(interval)
	srv.Await(fakediscord.KindClearActivity, 1)
	want := start.Add(interval)
	eventually(t, "the acknowledgement", func() bool { return h.m.Status().LastUpdate.Equal(want) })
}

func TestFramesThatEndTheConnection(t *testing.T) {
	for _, c := range []struct {
		name string
		send func(*fakediscord.Server) error
		want string
	}{
		{"close frame", func(s *fakediscord.Server) error { return s.SendClose(4002, "slow down") }, session.ErrorClosedByDiscord},
		{"malformed frame", func(s *fakediscord.Server) error { return s.SendMalformed(fakediscord.MalformedJSON) }, session.ErrorProtocol},
	} {
		t.Run(c.name, func(t *testing.T) {
			h := newHarness(t)
			srv := h.serve(fakediscord.Behavior{})
			h.start()
			h.ready(1)
			if err := c.send(srv); err != nil {
				t.Fatal(err)
			}
			h.inBackoff(1, 0)
			h.wantStatus(session.Disconnected, c.want)
			h.clock.Advance(time.Second)
			h.ready(2)
		})
	}
}

func TestSetReturnsAtOnceWhileAWriteIsBlockedAndTheWriteTimesOut(t *testing.T) {
	h := newHarness(t)
	srv := h.serve(fakediscord.Behavior{})
	h.start()
	h.ready(1)

	g := h.conn()
	g.hold()
	h.m.Set(activity("a"))
	g.awaitBlocked(t)
	within(t, "Set, Clear and Status while a write is blocked", func() {
		h.m.Set(activity("b"))
		h.m.Clear()
		_ = h.m.Status()
	})

	// The scheduler holds the clear for its interval; the write gives up
	// first.
	h.timers(2)
	h.clock.Advance(session.WriteTimeout)
	h.inBackoff(1, 1)
	h.wantStatus(session.Disconnected, session.ErrorWrite)

	h.clock.Advance(time.Second)
	h.ready(2)
	h.timers(1)
	h.clock.Advance(interval)
	srv.Await(fakediscord.KindClearActivity, 1)
	if n := count(srv, fakediscord.KindSetActivity); n != 0 {
		t.Errorf("%d activities reached Discord, want none: the first write never finished", n)
	}
}

func TestSetReturnsAtOnceBeforeRunAndAfterIt(t *testing.T) {
	h := newHarness(t)
	h.wantStatus(session.Disconnected, "")
	within(t, "Set before Run", func() { h.m.Set(activity("a")) })
	h.start()
	h.inBackoff(1, 0)
	h.cancel()
	h.wait()
	h.wantStatus(session.Stopped, session.ErrorNotRunning)
	within(t, "Set and Clear after Run", func() {
		h.m.Set(activity("b"))
		h.m.Clear()
	})
}

func TestStopWhileReadyClearsBeforeClosing(t *testing.T) {
	h := newHarness(t)
	srv := h.serve(fakediscord.Behavior{})
	h.start()
	h.ready(1)
	h.m.Set(activity("a"))
	srv.Await(fakediscord.KindSetActivity, 1)

	h.cancel()
	h.wait()
	h.wantStatus(session.Stopped, "")

	srv.Await(fakediscord.KindDisconnect, 1)
	var kinds []fakediscord.Kind
	for _, ev := range srv.Events() {
		kinds = append(kinds, ev.Kind)
	}
	want := []fakediscord.Kind{fakediscord.KindHandshake, fakediscord.KindSetActivity, fakediscord.KindClearActivity, fakediscord.KindDisconnect}
	if len(kinds) != len(want) {
		t.Fatalf("the server recorded %v, want %v", kinds, want)
	}
	for i := range want {
		if kinds[i] != want[i] {
			t.Fatalf("the server recorded %v, want %v", kinds, want)
		}
	}
	if pid := srv.Events()[2].PID; pid != int64(os.Getpid()) {
		t.Errorf("the clear named process %d, want this one", pid)
	}
}

func TestStopWhileDisconnectedReturnsPromptly(t *testing.T) {
	h := newHarness(t)
	h.start()
	h.inBackoff(1, 0)
	h.cancel()
	h.wait()
	h.wantStatus(session.Stopped, session.ErrorNotRunning)
}

func TestStopWhileADialIsInProgress(t *testing.T) {
	h := newHarness(t)
	dialling := make(chan struct{})
	h.dialer = func(ctx context.Context) (io.ReadWriteCloser, error) {
		close(dialling)
		<-ctx.Done()
		return nil, ctx.Err()
	}
	h.start()
	<-dialling
	h.wantStatus(session.Connecting, "")
	h.cancel()
	h.wait()
	h.wantStatus(session.Stopped, "")
}

func TestStopWhileTheHandshakeIsPending(t *testing.T) {
	h := newHarness(t)
	srv := h.serve(fakediscord.Behavior{Silent: true})
	h.start()
	srv.Await(fakediscord.KindHandshake, 1)
	h.cancel()
	h.wait()
	h.wantStatus(session.Stopped, "")
	srv.Await(fakediscord.KindDisconnect, 1)
	if n := count(srv, fakediscord.KindClearActivity); n != 0 {
		t.Errorf("a clear was sent on a connection that was never ready")
	}
}

func TestStopWhileAWriteIsBlocked(t *testing.T) {
	h := newHarness(t)
	h.serve(fakediscord.Behavior{})
	h.start()
	h.ready(1)
	g := h.conn()
	g.hold()
	h.m.Set(activity("a"))
	g.awaitBlocked(t)

	h.cancel()
	h.wait()
	h.wantStatus(session.Stopped, "")
}

func TestStopDoesNotWaitLongForAnUnacknowledgedClear(t *testing.T) {
	h := newHarness(t)
	srv := h.serve(fakediscord.Behavior{})
	h.start()
	h.ready(1)
	srv.Set(fakediscord.Behavior{Silent: true})

	h.cancel()
	srv.Await(fakediscord.KindClearActivity, 1)
	h.timers(1)
	h.clock.Advance(session.ClearTimeout)
	h.wait()
}

func TestStopDoesNotWaitLongForAClearThatCannotBeWritten(t *testing.T) {
	h := newHarness(t)
	srv := h.serve(fakediscord.Behavior{})
	h.start()
	h.ready(1)
	g := h.conn()
	g.hold()

	h.cancel()
	g.awaitBlocked(t)
	h.timers(1)
	h.clock.Advance(session.ClearTimeout)
	h.wait()
	srv.Await(fakediscord.KindDisconnect, 1)
	if n := count(srv, fakediscord.KindClearActivity); n != 0 {
		t.Errorf("the held clear reached Discord")
	}
}

func TestAnyDialFailureIsRetried(t *testing.T) {
	h := newHarness(t)
	fail := true
	h.dialer = func(ctx context.Context) (io.ReadWriteCloser, error) {
		if fail {
			fail = false
			return nil, errors.New("permission denied")
		}
		return h.dial(ctx)
	}
	h.serve(fakediscord.Behavior{})
	h.start()
	h.inBackoff(1, 0)
	h.wantStatus(session.Disconnected, session.ErrorDial)
	h.clock.Advance(time.Second)
	h.ready(1)
}

func TestAHandshakeThatCannotBeWrittenIsRetried(t *testing.T) {
	h := newHarness(t)
	broken := true
	h.dialer = func(ctx context.Context) (io.ReadWriteCloser, error) {
		conn, err := h.dial(ctx)
		if broken && err == nil {
			broken = false
			_ = conn.Close()
		}
		return conn, err
	}
	h.serve(fakediscord.Behavior{})
	h.start()
	h.inBackoff(1, 0)
	h.wantStatus(session.Disconnected, session.ErrorWrite)
	h.clock.Advance(time.Second)
	h.ready(1)
}

func TestStateNames(t *testing.T) {
	for state, want := range map[session.State]string{
		session.Disconnected: "disconnected",
		session.Connecting:   "connecting",
		session.Ready:        "ready",
		session.Stopped:      "stopped",
	} {
		if got := state.String(); got != want {
			t.Errorf("State(%d) is %q, want %q", int(state), got, want)
		}
	}
}
