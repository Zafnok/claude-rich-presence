package mcpclient

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os/exec"
	"sync"
	"time"
)

// TB is the part of testing.TB the package uses.
type TB interface {
	Helper()
	Errorf(format string, args ...any)
	Fatalf(format string, args ...any)
	Cleanup(func())
}

// ProtocolVersion is the revision of the protocol the client asks for.
const ProtocolVersion = "2025-11-25"

// ClaudeCode is the client name Claude Code gives in initialize.
const ClaudeCode = "claude-code"

// Options says what to start. Binary is required.
type Options struct {
	// Binary is the path of the executable.
	Binary string
	// Args are its arguments. The default is the one argument "mcp".
	Args []string
	// Env is the whole environment of the process, as "NAME=value" entries.
	// Nothing is inherited from the test, so that a test decides everything
	// the binary can see.
	Env []string
	// Patience bounds, in real time, how long the client waits for a response
	// and for the process to end. Running out fails the test. It is a
	// watchdog, not a delay. The default is twenty seconds.
	Patience time.Duration
}

// Info is what the server said of itself in its answer to initialize.
type Info struct {
	Name            string
	Version         string
	ProtocolVersion string
}

// Result is the result of a tool call: its first text item and the error flag.
type Result struct {
	Text    string
	IsError bool
}

// Error is a JSON-RPC error response.
type Error struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// Client is a running server process and the client at the other end of its
// standard streams. It is for one goroutine at a time, as a real client's
// requests to this server are: the server answers them in order.
type Client struct {
	t        TB
	cmd      *exec.Cmd
	stdin    io.WriteCloser
	lines    chan []byte // closed when standard output ends
	stderr   *buffer
	patience time.Duration
	nextID   int

	end  sync.Once
	code int
}

// buffer is a bytes.Buffer that the process's copier writes while a test
// reads.
type buffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (b *buffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.b.Write(p)
}

func (b *buffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.b.String()
}

// Start starts the binary. Nothing is sent until Initialize. If the test ends
// while the process still runs, the process is killed.
func Start(t TB, o Options) *Client {
	t.Helper()
	if len(o.Args) == 0 {
		o.Args = []string{"mcp"}
	}
	if o.Patience == 0 {
		o.Patience = 20 * time.Second
	}
	cmd := exec.Command(o.Binary, o.Args...)
	// An empty Env would mean "inherit", which is what a test must not get
	// by forgetting. A slice that is empty but not nil means "nothing".
	cmd.Env = append([]string{}, o.Env...)
	c := &Client{t: t, cmd: cmd, stderr: &buffer{}, patience: o.Patience, lines: make(chan []byte, 64)}
	cmd.Stderr = c.stderr
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatalf("mcpclient: opening standard input: %v", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("mcpclient: opening standard output: %v", err)
	}
	c.stdin = stdin
	if err := cmd.Start(); err != nil {
		t.Fatalf("mcpclient: starting %s: %v", o.Binary, err)
	}
	go func() {
		defer close(c.lines)
		r := bufio.NewReader(stdout)
		for {
			line, err := r.ReadBytes('\n')
			if len(line) > 0 {
				c.lines <- line
			}
			if err != nil {
				return
			}
		}
	}()
	t.Cleanup(func() { c.finish(true) })
	return c
}

// PID is the process id of the server.
func (c *Client) PID() int { return c.cmd.Process.Pid }

// Stderr is what the server has written to standard error so far.
func (c *Client) Stderr() string { return c.stderr.String() }

// Send writes one line to the server as it is, for what Request cannot say:
// a notification, or a line that is not a valid message.
func (c *Client) Send(line string) {
	c.t.Helper()
	if _, err := io.WriteString(c.stdin, line+"\n"); err != nil {
		c.t.Fatalf("mcpclient: writing to the server: %v", err)
	}
}

type response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  json.RawMessage `json:"result"`
	Error   *Error          `json:"error"`
}

// receive reads the next line of standard output, which must be a JSON-RPC
// response: standard output carries the protocol and nothing else.
func (c *Client) receive() response {
	c.t.Helper()
	timer := time.NewTimer(c.patience)
	defer timer.Stop()
	select {
	case line, ok := <-c.lines:
		if !ok {
			c.t.Fatalf("mcpclient: the server closed its output while a response was awaited; standard error: %q", c.Stderr())
		}
		var r response
		if err := json.Unmarshal(line, &r); err != nil || r.JSONRPC != "2.0" {
			c.t.Fatalf("mcpclient: standard output carried %q, which is not a protocol message", line)
		}
		return r
	case <-timer.C:
		c.t.Fatalf("mcpclient: no response within %v", c.patience)
	}
	return response{}
}

// Request sends a request and returns its result, or the error the server
// answered with. params is encoded as JSON; nil sends none.
func (c *Client) Request(method string, params any) (json.RawMessage, *Error) {
	c.t.Helper()
	c.nextID++
	msg := map[string]any{"jsonrpc": "2.0", "id": c.nextID, "method": method}
	if params != nil {
		msg["params"] = params
	}
	line, err := json.Marshal(msg)
	if err != nil {
		c.t.Fatalf("mcpclient: encoding the request: %v", err)
	}
	c.Send(string(line))
	r := c.receive()
	var id int
	if json.Unmarshal(r.ID, &id) != nil || id != c.nextID {
		c.t.Fatalf("mcpclient: response id %s, want %d", r.ID, c.nextID)
	}
	return r.Result, r.Error
}

// must is Request for a request that has to succeed.
func (c *Client) must(method string, params any, into any) {
	c.t.Helper()
	result, rpcErr := c.Request(method, params)
	if rpcErr != nil {
		c.t.Fatalf("mcpclient: %s failed: %d %s", method, rpcErr.Code, rpcErr.Message)
	}
	if err := json.Unmarshal(result, into); err != nil {
		c.t.Fatalf("mcpclient: the result of %s is not what the protocol says: %v", method, err)
	}
}

// Initialize performs the handshake under a client name, and returns what the
// server said of itself.
func (c *Client) Initialize(clientName string) Info {
	c.t.Helper()
	var result struct {
		ProtocolVersion string `json:"protocolVersion"`
		ServerInfo      struct {
			Name    string `json:"name"`
			Version string `json:"version"`
		} `json:"serverInfo"`
	}
	c.must("initialize", map[string]any{
		"protocolVersion": ProtocolVersion,
		"capabilities":    map[string]any{},
		"clientInfo":      map[string]any{"name": clientName, "version": "2.0.0"},
	}, &result)
	c.Send(`{"jsonrpc":"2.0","method":"notifications/initialized"}`)
	return Info{Name: result.ServerInfo.Name, Version: result.ServerInfo.Version, ProtocolVersion: result.ProtocolVersion}
}

// Tools lists the names of the tools the server offers, in its order.
func (c *Client) Tools() []string {
	c.t.Helper()
	var result struct {
		Tools []struct {
			Name string `json:"name"`
		} `json:"tools"`
	}
	c.must("tools/list", map[string]any{}, &result)
	names := make([]string, 0, len(result.Tools))
	for _, tool := range result.Tools {
		names = append(names, tool.Name)
	}
	return names
}

// Call calls a tool and returns when the server has answered. arguments is
// encoded as JSON; nil sends an empty object.
func (c *Client) Call(tool string, arguments any) Result {
	c.t.Helper()
	if arguments == nil {
		arguments = map[string]any{}
	}
	var result struct {
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
		IsError bool `json:"isError"`
	}
	c.must("tools/call", map[string]any{"name": tool, "arguments": arguments}, &result)
	if len(result.Content) == 0 {
		c.t.Fatalf("mcpclient: the result of %s has no content", tool)
	}
	return Result{Text: result.Content[0].Text, IsError: result.IsError}
}

// Close closes the server's input, which is how a client ends a session,
// waits for the process to end and returns its exit code. Anything the server
// wrote after its last response fails the test.
func (c *Client) Close() int {
	c.t.Helper()
	return c.finish(false)
}

// Kill ends the process at once, with no chance to clean up, as a crash or
// the operating system's task manager would, and waits until it is gone. It
// is the one place that knows how: the standard library maps it to SIGKILL
// on Linux and macOS and to TerminateProcess on Windows.
func (c *Client) Kill() {
	c.t.Helper()
	c.finish(true)
}

// finish ends the process, once, and returns its exit code. A second call
// returns the same code.
func (c *Client) finish(kill bool) int {
	c.t.Helper()
	c.end.Do(func() {
		if kill {
			_ = c.cmd.Process.Kill()
		}
		_ = c.stdin.Close()
		// Standard output must be read to its end before Wait, which closes
		// it.
		timer := time.NewTimer(c.patience)
		defer timer.Stop()
		for open := true; open; {
			select {
			case line, more := <-c.lines:
				open = more
				if more && !kill {
					c.t.Errorf("mcpclient: standard output carried %q after the last response", line)
				}
			case <-timer.C:
				_ = c.cmd.Process.Kill()
				for range c.lines {
				}
				_ = c.cmd.Wait()
				c.t.Fatalf("mcpclient: the server did not end within %v of its input closing; standard error: %q", c.patience, c.Stderr())
			}
		}
		err := c.cmd.Wait()
		var exit *exec.ExitError
		if err != nil && !errors.As(err, &exit) {
			c.t.Fatalf("mcpclient: waiting for the server: %v", err)
		}
		c.code = c.cmd.ProcessState.ExitCode()
	})
	return c.code
}
