package host_test

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/Zafnok/claude-rich-presence/internal/control/protocol"
	"github.com/Zafnok/claude-rich-presence/internal/diag"
	"github.com/Zafnok/claude-rich-presence/internal/domain"
	"github.com/Zafnok/claude-rich-presence/internal/host"
	"github.com/Zafnok/claude-rich-presence/internal/presence"
	"github.com/Zafnok/claude-rich-presence/internal/testutil/fakeclock"
)

func TestNewChecksTheConfiguration(t *testing.T) {
	valid := func() host.Config {
		return host.Config{
			Version:  "v0.0.0-20261003120000-0123456789ab+dirty",
			Acquire:  func() (host.Lock, error) { return nil, host.ErrLocked },
			Dial:     func(context.Context) (io.ReadWriteCloser, error) { return nil, errNoHost },
			Discord:  func() host.Discord { return nil },
			Render:   presence.Render,
			Clock:    fakeclock.New(start),
			Jitter:   func() float64 { return 0 },
			Counters: &diag.Counters{},
		}
	}
	if _, err := host.New(valid()); err != nil {
		t.Fatalf("a complete configuration was rejected: %v", err)
	}
	for name, spoil := range map[string]func(*host.Config){
		"no version":             func(c *host.Config) { c.Version = "" },
		"a version with a space": func(c *host.Config) { c.Version = "1.0 beta" },
		"no acquire":             func(c *host.Config) { c.Acquire = nil },
		"no dial":                func(c *host.Config) { c.Dial = nil },
		"no discord":             func(c *host.Config) { c.Discord = nil },
		"no render":              func(c *host.Config) { c.Render = nil },
		"no clock":               func(c *host.Config) { c.Clock = nil },
		"no jitter":              func(c *host.Config) { c.Jitter = nil },
		"no counters":            func(c *host.Config) { c.Counters = nil },
	} {
		t.Run(name, func(t *testing.T) {
			cfg := valid()
			spoil(&cfg)
			if node, err := host.New(cfg); err == nil || node != nil {
				t.Errorf("New returned %v, %v; want an error", node, err)
			}
		})
	}
}

func TestWhatIsPublishedBeforeRunIsKept(t *testing.T) {
	w := newWorld(t)
	a := w.spawn("a").open()
	a.publish(domain.KindTurnStarted, domain.KindToolStarted)

	want := host.Status{Role: host.RoleNone, Discord: protocol.DiscordUnknown}
	if got := a.node.Status(); got != want {
		t.Errorf("status before Run is %+v, want %+v", got, want)
	}
	a.run()
	w.eventually("a to hold the session as it was left", func() bool { return a.isHost() && a.holds(a) })
	if got := a.held()[0].Tool; got != string(domain.ToolEditing) {
		t.Errorf("the session's tool is %q, want editing", got)
	}
}

func TestAnInvalidEventIsDroppedAndCounted(t *testing.T) {
	w, a := hosting(t)
	renders := a.rendersSinceLock()

	a.node.Publish(domain.Event{SessionID: "", Surface: domain.SurfaceCode, At: w.clock.Now(), Kind: domain.KindTurnStarted})
	a.node.Publish(domain.Event{SessionID: a.session, Surface: "elsewhere", At: w.clock.Now(), Kind: domain.KindTurnStarted})
	if got := a.counters.Snapshot().EventsDropped; got != 2 {
		t.Errorf("%d events counted as dropped, want 2", got)
	}
	// The next event arrives, and is the only thing that changed anything.
	a.publish(domain.KindTurnStarted)
	w.eventually("the next event to reach the host", func() bool { return a.holds(a) && a.shows("Thinking") })
	if got := a.rendersSinceLock(); got != renders+1 {
		t.Errorf("rendered %d times, want once", got-renders)
	}
}

func TestStatusOfAHostCountsUptimeFromWhenItBecameHost(t *testing.T) {
	w := newWorld(t)
	w.clock.Advance(time.Hour)
	a := w.spawn("a", version("2.3.4")).open().run()
	w.eventually("a to hold its session", func() bool { return a.isHost() && a.holds(a) })

	w.clock.Advance(125 * time.Second)
	want := host.Status{Role: host.RoleHost, Discord: protocol.DiscordConnected, Sessions: 1, Version: "2.3.4", Uptime: 125 * time.Second}
	if got := a.node.Status(); got != want {
		t.Errorf("status is %+v, want %+v", got, want)
	}
}

func TestJitterPlacesAWaitInTheUpperHalfOfTheDelay(t *testing.T) {
	for _, tc := range []struct {
		jitter float64
		want   time.Duration
	}{
		{0, 4 * time.Second},
		{0.5, 6 * time.Second},
		{1, 8 * time.Second},
	} {
		if got := host.Jittered(8*time.Second, tc.jitter); got != tc.want {
			t.Errorf("jittered(8s, %v) is %v, want %v", tc.jitter, got, tc.want)
		}
	}
}

func TestTheStandDownDelayOutlastsTwoRetriesOfAFollower(t *testing.T) {
	// A follower that has just lost its host waits at most the base delay,
	// and then at most twice that. The host that stood down must still be
	// waiting after both.
	longest := host.Jittered(host.RetryBase, 1) + host.Jittered(2*host.RetryBase, 1)
	if host.StandDownDelay <= longest {
		t.Errorf("the stand-down delay is %v, want more than %v", host.StandDownDelay, longest)
	}
}

func TestANodeWaitsWithJitter(t *testing.T) {
	w := newWorld(t)
	w.jitter = func() float64 { return 0 }
	w.stubHost()
	w.spawn("b").run()
	w.sleeping(1)
	if got, want := w.lastWait(), host.RetryBase/2; got != want {
		t.Errorf("waiting %v with the least jitter, want %v", got, want)
	}
}
