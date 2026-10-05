package cli

import (
	"context"
	"errors"
	"net"
	"os"
	"strings"
	"testing"
	"time"

	dtransport "github.com/Zafnok/claude-rich-presence/internal/discord/transport"
)

// deadlineFailing is a Discord connection whose deadline cannot be set.
type deadlineFailing struct{ net.Conn }

func (deadlineFailing) SetDeadline(time.Time) error      { return errors.New("no deadline") }
func (deadlineFailing) SetReadDeadline(time.Time) error  { return nil }
func (deadlineFailing) SetWriteDeadline(time.Time) error { return nil }
func (deadlineFailing) Read([]byte) (int, error)         { return 0, errors.New("closed") }
func (deadlineFailing) Write([]byte) (int, error)        { return 0, errors.New("closed") }
func (deadlineFailing) Close() error                     { return nil }

type fixedDialer struct{ conn dtransport.Conn }

func (d fixedDialer) Dial(context.Context) (dtransport.Conn, error) { return d.conn, nil }

func TestDoctorPasses(t *testing.T) {
	w := newWorld(t)
	w.discord(t)
	s := w.startSession(t)
	s.initialize()
	s.awaitStatus("Discord: connected")

	code, stdout, stderr := w.run("doctor")
	if code != 0 || stderr != "" {
		t.Fatalf("got code %d, stderr %q; want 0 and nothing\n%s", code, stderr, stdout)
	}
	for _, want := range []string{
		"[pass] Configuration", "[pass] Runtime directory", "[pass] Discord endpoint",
		"[pass] Discord handshake", "[pass] Host", "[pass] Version", "Result: pass",
	} {
		if !strings.Contains(stdout, want) {
			t.Errorf("report lacks %q:\n%s", want, stdout)
		}
	}
	if strings.Contains(stdout, w.env["HOME"]) {
		t.Errorf("the report names the user's home directory:\n%s", stdout)
	}
	if code := s.finish(); code != 0 {
		t.Errorf("session exit code = %d, want 0", code)
	}
}

func TestDoctorFindsProblems(t *testing.T) {
	tests := []struct {
		name    string
		prepare func(t *testing.T, w *world)
		advance bool
		want    []string
	}{
		{
			name:    "nothing is running",
			prepare: func(_ *testing.T, w *world) { w.sys.exists = func(string) bool { return false } },
			want: []string{
				"[warn] Runtime directory", "[fail] Discord endpoint", "[warn] Discord handshake",
				"[warn] Host: no host is running", "Result: fail",
			},
		},
		{
			name:    "no runtime directory can be resolved",
			prepare: func(_ *testing.T, w *world) { w.env = w.env.with("RICH_PRESENCE_RUNTIME_DIR", "relative") },
			want:    []string{"[fail] Runtime directory", "[warn] Host: no host is running"},
		},
		{
			name: "the runtime directory is not a directory",
			prepare: func(t *testing.T, w *world) {
				if err := os.WriteFile(strings.TrimSuffix(w.runtime, ""), nil, 0o600); err != nil {
					t.Fatal(err)
				}
			},
			want: []string{"[fail] Runtime directory"},
		},
		{
			name:    "something holds the socket and does not answer",
			prepare: func(t *testing.T, w *world) { listenRaw(t, w, stayQuiet) },
			advance: true,
			want:    []string{"[pass] Runtime directory", "[fail] Host: something holds the control socket"},
		},
		{
			name: "a deadline cannot be set on the Discord connection",
			prepare: func(_ *testing.T, w *world) {
				w.sys.discord = func(func(string) string) (dtransport.Dialer, []string) {
					return fixedDialer{deadlineFailing{}}, nil
				}
			},
			want: []string{"[warn] Discord handshake: could not connect"},
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
				code, stdout, stderr := w.run("doctor")
				done <- result{code, stdout, stderr}
			}()
			if tt.advance {
				w.clock.WaitForTimers(1)
				w.clock.Advance(askTimeout)
			}
			got := <-done
			if got.code != 1 || got.stderr != "" {
				t.Errorf("got code %d, stderr %q; want 1 and nothing", got.code, got.stderr)
			}
			for _, want := range tt.want {
				if !strings.Contains(got.stdout, want) {
					t.Errorf("report lacks %q:\n%s", want, got.stdout)
				}
			}
		})
	}
}

func TestHomeDir(t *testing.T) {
	e := env{"HOME": "/home/ada", "USERPROFILE": `C:\Users\ada`}
	if got := homeDir("windows", e.get); got != `C:\Users\ada` {
		t.Errorf("homeDir(windows) = %q", got)
	}
	for _, goos := range []string{"linux", "darwin"} {
		if got := homeDir(goos, e.get); got != "/home/ada" {
			t.Errorf("homeDir(%s) = %q", goos, got)
		}
	}
}
