package cli

import (
	"bytes"
	"errors"
	"io"
	"net"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Zafnok/claude-rich-presence/internal/control/protocol"
	ctransport "github.com/Zafnok/claude-rich-presence/internal/control/transport"
)

// listenRaw listens on the world's control socket and hands each connection
// to handle, so that a test can play a host that does not behave.
func listenRaw(t *testing.T, w *world, handle func(net.Conn)) {
	t.Helper()
	paths, err := ctransport.Locate(w.env.get)
	if err != nil {
		t.Fatal(err)
	}
	if err := ctransport.Prepare(paths.Dir); err != nil {
		t.Fatal(err)
	}
	l, err := net.Listen("unix", paths.Socket)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = l.Close() })
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			go func() {
				defer func() { _ = c.Close() }()
				handle(c)
			}()
		}
	}()
}

// stayQuiet is a host that accepts and never answers, until the client hangs
// up.
func stayQuiet(c net.Conn) { _, _ = io.Copy(io.Discard, c) }

func TestStatusWithNoHost(t *testing.T) {
	w := newWorld(t)
	code, stdout, stderr := w.run("status")
	if code != 3 || stdout != "No presence host is running.\n" || stderr != "" {
		t.Errorf("got code %d, stdout %q, stderr %q; want 3 and the message", code, stdout, stderr)
	}
}

func TestStatusAsksTheHost(t *testing.T) {
	w := newWorld(t)
	w.discord(t)
	s := w.startSession(t)
	s.initialize()
	s.awaitStatus("Discord: connected")

	code, stdout, stderr := w.run("status")
	if code != 0 || stderr != "" {
		t.Fatalf("got code %d, stderr %q; want 0 and nothing", code, stderr)
	}
	want := []string{"Role: host", "Discord: connected", "Sessions: 1", "Version: 1.2.3", "Uptime: "}
	lines := strings.Split(strings.TrimSuffix(stdout, "\n"), "\n")
	if len(lines) != len(want) {
		t.Fatalf("stdout = %q, want %d lines", stdout, len(want))
	}
	for i, prefix := range want {
		if !strings.HasPrefix(lines[i], prefix) {
			t.Errorf("line %d = %q, want it to begin %q", i, lines[i], prefix)
		}
	}
	// The command is not a node: it holds no session of its own.
	if got := s.status(); !strings.Contains(got, "Sessions: 1") {
		t.Errorf("after the command, status = %q, want one session still", got)
	}
	if code := s.finish(); code != 0 {
		t.Errorf("exit code = %d, want 0", code)
	}
}

// The runtime directory is refused when it is not the user's alone. A file in
// its place is what every operating system refuses.
func TestStatusRefusesAnUnsafeRuntimeDirectory(t *testing.T) {
	w := newWorld(t)
	if err := os.WriteFile(w.runtime, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	code, stdout, stderr := w.run("status")
	want := "rich-presence: status: the runtime directory is not safe to use, so no host was asked; run `rich-presence doctor`\n"
	if code != 1 || stdout != "" || stderr != want {
		t.Errorf("got code %d, stdout %q, stderr %q; want 1, nothing, and %q", code, stdout, stderr, want)
	}
}

func TestStatusProblems(t *testing.T) {
	tests := []struct {
		name       string
		prepare    func(t *testing.T, w *world)
		advance    bool
		wantStderr string
	}{
		{
			name:       "no runtime directory can be resolved",
			prepare:    func(_ *testing.T, w *world) { w.env = w.env.with("RICH_PRESENCE_RUNTIME_DIR", "relative") },
			wantStderr: "status: ",
		},
		{
			name:    "a host that does not answer",
			prepare: func(t *testing.T, w *world) { listenRaw(t, w, stayQuiet) },
			advance: true,
			// The timer can end the connection before or after the hello is
			// written; either way the host has timed out.
			wantStderr: "status: ",
		},
		{
			name:       "a binary whose version the protocol cannot carry",
			prepare:    func(t *testing.T, w *world) { w.sys.version = "not a version"; listenRaw(t, w, stayQuiet) },
			wantStderr: "version",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := newWorld(t)
			tt.prepare(t, w)
			type result struct {
				code           int
				stdout, stderr string
			}
			done := make(chan result, 1)
			go func() {
				code, stdout, stderr := w.run("status")
				done <- result{code, stdout, stderr}
			}()
			if tt.advance {
				// The wait for an answer is a timer that closes the connection.
				w.clock.WaitForTimers(1)
				w.clock.Advance(askTimeout)
			}
			got := <-done
			if got.code != 1 || got.stdout != "" || !strings.Contains(got.stderr, tt.wantStderr) {
				t.Errorf("got code %d, stdout %q, stderr %q; want 1, nothing, and %q", got.code, got.stdout, got.stderr, tt.wantStderr)
			}
		})
	}
}

// scripted is a connection that reads what the host would say and records
// what is written, failing the write that is numbered failAt.
type scripted struct {
	in     io.Reader
	out    bytes.Buffer
	writes int
	failAt int
}

func (c *scripted) Read(p []byte) (int, error) { return c.in.Read(p) }

func (c *scripted) Write(p []byte) (int, error) {
	c.writes++
	if c.writes == c.failAt {
		return 0, errors.New("write failed")
	}
	return c.out.Write(p)
}

func TestConverse(t *testing.T) {
	welcome := `{"type":"welcome","protocol":1,"version":"2.0.0"}` + "\n"
	result := `{"type":"status_result","discord":"connected","sessions":3,"version":"2.0.0","uptime_seconds":61}` + "\n"
	tests := []struct {
		name       string
		host       string
		failAt     int
		version    string
		want       protocol.StatusResult
		wantErr    error
		wantErrAny bool
		wantSent   string
	}{
		{
			name:     "a host that answers",
			host:     welcome + result,
			version:  "1.0.0",
			want:     protocol.StatusResult{Discord: "connected", Sessions: 3, Version: "2.0.0", UptimeSeconds: 61},
			wantSent: `{"type":"hello","protocol":1,"version":"1.0.0"}` + "\n" + `{"type":"status"}` + "\n",
		},
		{name: "a refusal is no welcome", host: `{"type":"refuse","reason":"standing_down"}` + "\n", version: "1.0.0", wantErr: errNoWelcome},
		{name: "silence is no welcome", host: "", version: "1.0.0", wantErr: errNoWelcome},
		{name: "a line that is not a message is no welcome", host: "garbage\n", version: "1.0.0", wantErr: errNoWelcome},
		{name: "a host that welcomes and then says nothing", host: welcome, version: "1.0.0", wantErr: errNoAnswer},
		{name: "a host that answers with another message", host: welcome + welcome, version: "1.0.0", wantErr: errNoAnswer},
		{name: "a hello that cannot be sent", host: welcome + result, version: "1.0.0", failAt: 1, wantErrAny: true},
		{name: "a status request that cannot be sent", host: welcome + result, version: "1.0.0", failAt: 2, wantErrAny: true},
		{name: "a version the protocol cannot carry", host: welcome + result, version: "not a version", wantErrAny: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conn := &scripted{in: strings.NewReader(tt.host), failAt: tt.failAt}
			got, err := converse(conn, tt.version)
			switch {
			case tt.wantErrAny:
				if err == nil {
					t.Error("err = nil, want an error")
				}
			case !errors.Is(err, tt.wantErr):
				t.Errorf("err = %v, want %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("result = %+v, want %+v", got, tt.want)
			}
			if tt.wantSent != "" && conn.out.String() != tt.wantSent {
				t.Errorf("sent %q, want %q", conn.out.String(), tt.wantSent)
			}
		})
	}
}

func TestUptimeIsPrintedAsADuration(t *testing.T) {
	w := newWorld(t)
	listenRaw(t, w, func(c net.Conn) {
		lines := protocol.NewDecoder(c)
		for {
			m, err := lines.Next()
			if err != nil {
				return
			}
			switch m.(type) {
			case protocol.Hello:
				_ = protocol.Encode(c, protocol.Welcome{Protocol: 1, Version: "9.9.9"})
			case protocol.Status:
				_ = protocol.Encode(c, protocol.StatusResult{Discord: protocol.DiscordConnecting, Sessions: 2, Version: "9.9.9", UptimeSeconds: int64(90 * time.Minute / time.Second)})
			}
		}
	})
	code, stdout, _ := w.run("status")
	want := "Role: host\nDiscord: connecting\nSessions: 2\nVersion: 9.9.9\nUptime: 1h30m0s\n"
	if code != 0 || stdout != want {
		t.Errorf("got code %d, stdout %q; want 0 and %q", code, stdout, want)
	}
}
