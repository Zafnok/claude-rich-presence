package mcpclient_test

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Zafnok/claude-rich-presence/internal/testutil/mcpclient"
)

// envServer makes this test binary act as a server instead of running the
// tests. Its value chooses how the server behaves.
const envServer = "MCPCLIENT_TEST_SERVER"

func TestMain(m *testing.M) {
	if mode := os.Getenv(envServer); mode != "" {
		os.Exit(serve(mode))
	}
	os.Exit(m.Run())
}

// serve is a server just large enough to answer the client: it names itself,
// lists one tool and echoes the arguments of a call.
func serve(mode string) int {
	in := bufio.NewScanner(os.Stdin)
	for in.Scan() {
		var req struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
			Params struct {
				ClientInfo struct {
					Name string `json:"name"`
				} `json:"clientInfo"`
				Name      string          `json:"name"`
				Arguments json.RawMessage `json:"arguments"`
			} `json:"params"`
		}
		if json.Unmarshal(in.Bytes(), &req) != nil || req.ID == nil {
			continue
		}
		var result any
		switch req.Method {
		case "initialize":
			result = map[string]any{"protocolVersion": "2025-11-25", "serverInfo": map[string]any{"name": "echo for " + req.Params.ClientInfo.Name, "version": "9.9.9"}}
		case "tools/list":
			result = map[string]any{"tools": []any{map[string]any{"name": "echo"}}}
		case "tools/call":
			if mode == "chatty" {
				fmt.Println("this line is not a protocol message")
				continue
			}
			result = map[string]any{"content": []any{map[string]any{"type": "text", "text": req.Params.Name + " " + string(req.Params.Arguments)}}, "isError": req.Params.Name == "fail"}
		default:
			fmt.Printf(`{"jsonrpc":"2.0","id":%s,"error":{"code":-32601,"message":"no such method"}}`+"\n", req.ID)
			continue
		}
		line, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": result})
		fmt.Println(string(line))
	}
	if mode == "stubborn" {
		// Never ends of its own accord.
		for {
			time.Sleep(time.Hour)
		}
	}
	fmt.Fprintln(os.Stderr, "goodbye")
	return 7
}

func start(t mcpclient.TB, mode string, patience time.Duration) *mcpclient.Client {
	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("finding the test binary: %v", err)
	}
	env := []string{envServer + "=" + mode}
	if root := os.Getenv("SystemRoot"); root != "" {
		env = append(env, "SystemRoot="+root)
	}
	return mcpclient.Start(t, mcpclient.Options{Binary: exe, Args: []string{"-test.run=^$"}, Env: env, Patience: patience})
}

func TestASessionFromInitializeToClose(t *testing.T) {
	c := start(t, "echo", 0)
	if c.PID() <= 0 {
		t.Errorf("PID() = %d", c.PID())
	}
	info := c.Initialize("some-client")
	if info.Name != "echo for some-client" || info.Version != "9.9.9" || info.ProtocolVersion != mcpclient.ProtocolVersion {
		t.Errorf("Initialize() = %+v", info)
	}
	if tools := c.Tools(); len(tools) != 1 || tools[0] != "echo" {
		t.Errorf("Tools() = %q", tools)
	}
	if got := c.Call("echo", map[string]any{"a": 1}); got.Text != `echo {"a":1}` || got.IsError {
		t.Errorf("Call() = %+v", got)
	}
	if got := c.Call("fail", nil); got.Text != "fail {}" || !got.IsError {
		t.Errorf("Call() with no arguments = %+v, want an empty object sent and the error flag read", got)
	}
	if _, rpcErr := c.Request("no/such", nil); rpcErr == nil || rpcErr.Code != -32601 {
		t.Errorf("Request() of an unknown method gave the error %+v", rpcErr)
	}
	if code := c.Close(); code != 7 {
		t.Errorf("Close() = %d, want the exit code 7", code)
	}
	if code := c.Close(); code != 7 {
		t.Errorf("a second Close() = %d, want the same code", code)
	}
	if got := c.Stderr(); !strings.Contains(got, "goodbye") {
		t.Errorf("Stderr() = %q", got)
	}
}

func TestKillEndsAProcessThatWouldNotEnd(t *testing.T) {
	c := start(t, "stubborn", 0)
	c.Initialize(mcpclient.ClaudeCode)
	c.Kill()
	if got := c.Stderr(); strings.Contains(got, "goodbye") {
		t.Errorf("the process ended in its own time: %q", got)
	}
}

func TestTheTestEndingKillsTheProcess(t *testing.T) {
	tb := &fakeTB{}
	c := start(tb, "stubborn", 0)
	c.Initialize(mcpclient.ClaudeCode)
	tb.cleanup()
	if tb.fatal != "" || len(tb.errors) != 0 {
		t.Errorf("ending the test reported %q %q", tb.fatal, tb.errors)
	}
	c.Kill() // returns at once: the process is gone
}

func TestFailures(t *testing.T) {
	tests := []struct {
		name string
		mode string
		do   func(c *mcpclient.Client)
		want string
	}{
		{"a line that is not a protocol message", "chatty", func(c *mcpclient.Client) { c.Call("echo", nil) }, "not a protocol message"},
		{"a request that fails", "echo", func(c *mcpclient.Client) {
			c.Call("echo", nil)
			c.Initialize("x")
			c.Tools()
			c.Request("x", nil)
			c.Send("{}")
		}, ""},
		{"a server that does not end when its input closes", "stubborn", func(c *mcpclient.Client) { c.Close() }, "did not end"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tb := &fakeTB{}
			defer tb.cleanup()
			tb.run(func() {
				c := start(tb, tt.mode, 500*time.Millisecond)
				c.Initialize(mcpclient.ClaudeCode)
				tt.do(c)
			})
			if !strings.Contains(tb.fatal, tt.want) {
				t.Errorf("the test was failed with %q, want it to mention %q", tb.fatal, tt.want)
			}
		})
	}
}

// fakeTB stands in for the test when the thing under test is what the client
// reports to it.
type fakeTB struct {
	mu       sync.Mutex
	errors   []string
	fatal    string
	cleanups []func()
}

type fatalSignal struct{}

func (f *fakeTB) Helper() {}

func (f *fakeTB) Errorf(format string, args ...any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.errors = append(f.errors, fmt.Sprintf(format, args...))
}

func (f *fakeTB) Fatalf(format string, args ...any) {
	f.mu.Lock()
	f.fatal = fmt.Sprintf(format, args...)
	f.mu.Unlock()
	panic(fatalSignal{})
}

func (f *fakeTB) Cleanup(fn func()) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.cleanups = append(f.cleanups, fn)
}

// run calls fn and stops where it fails the test, as a real test would.
func (f *fakeTB) run(fn func()) {
	defer func() {
		if r := recover(); r != nil {
			if _, ok := r.(fatalSignal); !ok {
				panic(r)
			}
		}
	}()
	fn()
}

// cleanup runs what was registered, last first.
func (f *fakeTB) cleanup() {
	f.mu.Lock()
	fns := f.cleanups
	f.cleanups = nil
	f.mu.Unlock()
	for i := len(fns) - 1; i >= 0; i-- {
		f.run(fns[i])
	}
}
