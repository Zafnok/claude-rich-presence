package cli

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	dtransport "github.com/Zafnok/claude-rich-presence/internal/discord/transport"
	"github.com/Zafnok/claude-rich-presence/internal/domain"
	"github.com/Zafnok/claude-rich-presence/internal/testutil/fakeclock"
	"github.com/Zafnok/claude-rich-presence/internal/testutil/fakediscord"
)

// env is a process environment for a test.
type env map[string]string

func (e env) get(name string) string { return e[name] }

// with returns a copy of e with the settings added.
func (e env) with(pairs ...string) env {
	out := env{}
	for k, v := range e {
		out[k] = v
	}
	for i := 0; i+1 < len(pairs); i += 2 {
		out[pairs[i]] = pairs[i+1]
	}
	return out
}

// world is a system and an environment that touch nothing outside the test:
// a runtime directory and a home of its own, a fake Discord, a fake clock.
type world struct {
	sys     system
	env     env
	clock   *fakeclock.Clock
	ns      *fakediscord.Namespace
	runtime string
	// signal ends the context the system's notify returned, as a termination
	// signal would.
	signal context.CancelFunc
	mu     sync.Mutex
}

// newWorld makes a world with no Discord listening. start puts one up.
func newWorld(t *testing.T) *world {
	t.Helper()
	// The runtime directory is short, so that the socket path fits.
	runtimeDir, err := os.MkdirTemp("", "rp")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(runtimeDir) })
	home := t.TempDir()
	w := &world{
		clock:   fakeclock.New(time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)),
		ns:      fakediscord.NewNamespace(t),
		runtime: runtimeDir + string(os.PathSeparator) + "run",
		env: env{
			"HOME":                      home,
			"USERPROFILE":               home,
			"RICH_PRESENCE_RUNTIME_DIR": runtimeDir + string(os.PathSeparator) + "run",
		},
	}
	w.sys = realSystem()
	w.sys.version = "1.2.3"
	w.sys.clock = w.clock
	w.sys.sessionID = func() string { return "test-session" }
	w.sys.discord = func(func(string) string) (dtransport.Dialer, []string) {
		return dtransport.NewAt(w.ns.Prefix()), dtransport.Candidates([]string{w.ns.Prefix()})
	}
	w.sys.exists = func(path string) bool { return path == w.ns.Addr(0) }
	w.sys.notify = func(ctx context.Context) (context.Context, context.CancelFunc) {
		ctx, cancel := context.WithCancel(ctx)
		w.mu.Lock()
		w.signal = cancel
		w.mu.Unlock()
		return ctx, cancel
	}
	return w
}

// discord starts the fake Discord.
func (w *world) discord(t *testing.T) *fakediscord.Server {
	t.Helper()
	return w.ns.Start(t, 0, fakediscord.Options{})
}

// raise sends the termination signal.
func (w *world) raise() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.signal()
}

// run runs a command with empty input and returns what it printed.
func (w *world) run(args ...string) (code int, stdout, stderr string) {
	var out, errs bytes.Buffer
	code = w.sys.run(args, strings.NewReader(""), &out, &errs, w.env.get)
	return code, out.String(), errs.String()
}

// eventually polls cond until it holds, and fails the test if it does not
// within a generous deadline. The deadline is a watchdog, not a delay.
func eventually(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

const (
	initializeLine = `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"claude-code","version":"2.0.0"}}}`
	initializedMsg = `{"jsonrpc":"2.0","method":"notifications/initialized"}`
)

// client is a running mcp command and the client at the other end of it.
type client struct {
	t      *testing.T
	in     *io.PipeWriter
	out    *bufio.Reader
	outEnd *io.PipeReader
	done   chan int
	stderr *bytes.Buffer
	nextID int
}

// startSession runs the mcp command in the world and returns its client. The
// test must end it, with finish or by raising the signal.
func (w *world) startSession(t *testing.T) *client {
	t.Helper()
	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	s := &client{t: t, in: inW, outEnd: outR, out: bufio.NewReader(outR), done: make(chan int, 1), stderr: &bytes.Buffer{}, nextID: 1}
	go func() {
		code := w.sys.run([]string{"mcp"}, inR, outW, s.stderr, w.env.get)
		_ = outW.Close()
		s.done <- code
	}()
	t.Cleanup(func() { _ = inW.Close(); _ = outR.Close() })
	return s
}

// send writes one line to the server.
func (s *client) send(line string) {
	s.t.Helper()
	if _, err := io.WriteString(s.in, line+"\n"); err != nil {
		s.t.Fatalf("writing to the server: %v", err)
	}
}

// receive reads one line from the server and requires it to be a JSON-RPC
// message: standard output carries the protocol and nothing else.
func (s *client) receive() map[string]any {
	s.t.Helper()
	line, err := s.out.ReadString('\n')
	if err != nil {
		s.t.Fatalf("reading from the server: %v", err)
	}
	var msg map[string]any
	if err := json.Unmarshal([]byte(line), &msg); err != nil || msg["jsonrpc"] != "2.0" {
		s.t.Fatalf("standard output carried %q, which is not a protocol message", line)
	}
	return msg
}

// request sends a request and returns its response.
func (s *client) request(method, params string) map[string]any {
	s.t.Helper()
	s.nextID++
	s.send(`{"jsonrpc":"2.0","id":` + itoa(s.nextID) + `,"method":"` + method + `","params":` + params + `}`)
	return s.receive()
}

func itoa(n int) string {
	b, _ := json.Marshal(n)
	return string(b)
}

// initialize performs the handshake as Claude Code.
func (s *client) initialize() {
	s.t.Helper()
	s.initializeAs("claude-code")
}

// initializeAs performs the handshake under a client name.
func (s *client) initializeAs(name string) {
	s.t.Helper()
	s.send(strings.Replace(initializeLine, "claude-code", name, 1))
	if s.receive()["error"] != nil {
		s.t.Fatal("initialize failed")
	}
	s.send(initializedMsg)
}

// toolNames lists the tools the server offers.
func (s *client) toolNames() []string {
	s.t.Helper()
	resp := s.request("tools/list", `{}`)
	var names []string
	for _, tool := range resp["result"].(map[string]any)["tools"].([]any) {
		names = append(names, tool.(map[string]any)["name"].(string))
	}
	return names
}

// status calls the status tool and returns its text.
func (s *client) status() string {
	s.t.Helper()
	resp := s.request("tools/call", `{"name":"presence_status","arguments":{}}`)
	content := resp["result"].(map[string]any)["content"].([]any)
	return content[0].(map[string]any)["text"].(string)
}

// awaitStatus polls the status tool until its text contains want.
func (s *client) awaitStatus(want string) {
	s.t.Helper()
	var got string
	eventually(s.t, "status "+want, func() bool {
		got = s.status()
		return strings.Contains(got, want)
	})
}

// finish closes the input and returns the exit code, which must come within
// a second.
func (s *client) finish() int {
	s.t.Helper()
	_ = s.in.Close()
	return s.exit()
}

// exit waits for the command to end, which it must do within a second.
func (s *client) exit() int {
	s.t.Helper()
	select {
	case code := <-s.done:
		rest, _ := io.ReadAll(s.out)
		if len(rest) != 0 {
			s.t.Errorf("standard output carried %q after the last response", rest)
		}
		return code
	case <-time.After(time.Second):
		s.t.Fatal("the command did not end within a second")
		return -1
	}
}

func testEvent() domain.Event {
	return domain.Event{SessionID: "s", Surface: domain.SurfaceCode, At: time.Unix(1, 0), Kind: domain.KindSessionOpened}
}
