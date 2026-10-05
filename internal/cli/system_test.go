package cli

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Zafnok/claude-rich-presence/internal/control/protocol"
	"github.com/Zafnok/claude-rich-presence/internal/discord/session"
	"github.com/Zafnok/claude-rich-presence/internal/host"
)

func TestRealSystem(t *testing.T) {
	s := realSystem()

	t.Run("the clock is the operating system's", func(t *testing.T) {
		if d := time.Since(s.clock.Now()); d < 0 || d > time.Minute {
			t.Errorf("Now() is %v from the real time", d)
		}
		fired := make(chan struct{})
		s.clock.AfterFunc(time.Millisecond, func() { close(fired) })
		select {
		case <-fired:
		case <-time.After(10 * time.Second):
			t.Fatal("the timer did not fire")
		}
		if stop := s.clock.AfterFunc(time.Hour, func() {}); !stop() {
			t.Error("a timer that has not fired could not be stopped")
		}
	})

	t.Run("files are read from the file system", func(t *testing.T) {
		name := filepath.Join(t.TempDir(), "config.json")
		if err := os.WriteFile(name, []byte("{}"), 0o600); err != nil {
			t.Fatal(err)
		}
		got, err := s.files.ReadFile(name)
		if err != nil || string(got) != "{}" {
			t.Errorf("ReadFile() = %q, %v", got, err)
		}
	})

	t.Run("an endpoint exists when its file does", func(t *testing.T) {
		name := filepath.Join(t.TempDir(), "discord-ipc-0")
		if s.exists(name) {
			t.Error("a missing file exists")
		}
		if err := os.WriteFile(name, nil, 0o600); err != nil {
			t.Fatal(err)
		}
		if !s.exists(name) {
			t.Error("a file that is there does not exist")
		}
	})

	t.Run("Discord is looked for in the places the operating system has", func(t *testing.T) {
		dialer, candidates := s.discord(noEnv)
		if dialer == nil || len(candidates) == 0 {
			t.Errorf("discord() = %v, %v, want a dialer and candidates", dialer, candidates)
		}
	})

	t.Run("a session is named for the process", func(t *testing.T) {
		if got, prefix := s.sessionID(), "pending-"+strconv.Itoa(os.Getpid())+"-"; !strings.HasPrefix(got, prefix) {
			t.Errorf("session id %q does not begin %q", got, prefix)
		}
	})

	t.Run("notification ends with the stop function", func(t *testing.T) {
		ctx, stop := s.notify(t.Context())
		stop()
		select {
		case <-ctx.Done():
		case <-time.After(10 * time.Second):
			t.Fatal("the context did not end")
		}
	})

	t.Run("the version is meaningful", func(t *testing.T) {
		if s.version == "" {
			t.Error("the version is empty")
		}
	})

	t.Run("jitter is between zero and one", func(t *testing.T) {
		for range 100 {
			if j := jitter(); j < 0 || j >= 1 {
				t.Fatalf("jitter() = %v", j)
			}
		}
	})
}

func TestDiscordState(t *testing.T) {
	tests := []struct {
		state session.State
		want  protocol.DiscordState
	}{
		{session.Ready, protocol.DiscordConnected},
		{session.Connecting, protocol.DiscordConnecting},
		{session.Disconnected, protocol.DiscordDisconnected},
		{session.Stopped, protocol.DiscordUnknown},
		{session.State(99), protocol.DiscordUnknown},
	}
	for _, tt := range tests {
		if got := discordState(tt.state); got != tt.want {
			t.Errorf("discordState(%d) = %q, want %q", tt.state, got, tt.want)
		}
	}
}

func TestAdapterWords(t *testing.T) {
	roles := map[host.Role]string{host.RoleHost: "host", host.RoleFollower: "follower", host.RoleNone: ""}
	for in, want := range roles {
		if got := string(adapterRole(in)); got != want {
			t.Errorf("adapterRole(%d) = %q, want %q", in, got, want)
		}
	}
	states := map[protocol.DiscordState]string{
		protocol.DiscordConnected:    "connected",
		protocol.DiscordConnecting:   "connecting",
		protocol.DiscordDisconnected: "disconnected",
		protocol.DiscordUnknown:      "",
	}
	for in, want := range states {
		if got := string(adapterDiscord(in)); got != want {
			t.Errorf("adapterDiscord(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestOffPresence(t *testing.T) {
	line := off()
	if st := line.status.Status(); st.Role != "off" || st.Sessions != 0 {
		t.Errorf("Status() = %+v, want off with no sessions", st)
	}
	line.publisher.Publish(testEvent())
	line.stop()
}
