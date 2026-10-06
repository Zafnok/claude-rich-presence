package diag_test

import (
	"bytes"
	"errors"
	"io"
	"io/fs"
	"strings"
	"testing"

	"github.com/Zafnok/claude-rich-presence/internal/config"
	"github.com/Zafnok/claude-rich-presence/internal/diag"
	"github.com/Zafnok/claude-rich-presence/internal/discord/codec"
)

const version = "v1.2.3"

// fakeConn is a Discord connection: what Discord will send, what was sent to
// it, and whether it was closed.
type fakeConn struct {
	answer   bytes.Buffer
	sent     bytes.Buffer
	writeErr error
	closed   int
}

func (c *fakeConn) Read(p []byte) (int, error) { return c.answer.Read(p) }

func (c *fakeConn) Write(p []byte) (int, error) {
	if c.writeErr != nil {
		return 0, c.writeErr
	}
	return c.sent.Write(p)
}

func (c *fakeConn) Close() error {
	c.closed++
	return errInjected
}

// answering returns a connection on which Discord sends the frames.
func answering(t *testing.T, frames ...codec.Frame) *fakeConn {
	t.Helper()
	conn := &fakeConn{}
	for _, f := range frames {
		if err := codec.WriteFrame(&conn.answer, f); err != nil {
			t.Fatal(err)
		}
	}
	return conn
}

var ready = codec.Frame{Op: codec.OpFrame, Payload: []byte(`{"cmd":"DISPATCH","evt":"READY","data":{"v":1}}`)}

// configFiles is the configuration file of healthyPorts.
type configFiles struct{ data string }

func (c configFiles) ReadFile(string) ([]byte, error) {
	if c.data == "" {
		return nil, fs.ErrNotExist
	}
	return []byte(c.data), nil
}

func loader(home, file string) func() (config.Config, config.Dirs, []config.Warning) {
	return func() (config.Config, config.Dirs, []config.Warning) {
		getenv := func(key string) string {
			if key == "HOME" {
				return home
			}
			return ""
		}
		return config.Load("linux", getenv, configFiles{file})
	}
}

// healthyPorts are ports on which every check passes.
func healthyPorts(t *testing.T) (diag.Ports, *fakeConn) {
	conn := answering(t, ready)
	return diag.Ports{
		Config:           loader("/home/zed", `{"discord_application_id":"1234"}`),
		RuntimeDir:       func() (string, diag.DirState) { return "/run/user/1000/rich-presence", diag.DirReady },
		DiscordEndpoints: func() (int, error) { return 1, nil },
		DialDiscord:      func() (io.ReadWriteCloser, error) { return conn, nil },
		Host: func() diag.HostAnswer {
			return diag.HostAnswer{State: diag.HostAnswered, Version: version, Sessions: 2}
		},
	}, conn
}

func finding(t *testing.T, r diag.Report, check string) diag.Finding {
	t.Helper()
	for _, f := range r.Findings {
		if f.Check == check {
			return f
		}
	}
	t.Fatalf("no finding for %q", check)
	return diag.Finding{}
}

func TestEveryCheckPasses(t *testing.T) {
	ports, conn := healthyPorts(t)
	report := diag.Run(ports, version)

	want := []diag.Finding{
		{Check: diag.CheckConfig, Detail: "loaded without warnings", Path: "/home/zed/.config/rich-presence/config.json"},
		{Check: diag.CheckRuntime, Detail: "exists, is yours alone, and the socket path fits", Path: "/run/user/1000/rich-presence"},
		{Check: diag.CheckEndpoint, Detail: "found 1"},
		{Check: diag.CheckHandshake, Detail: "Discord accepted the application id"},
		{Check: diag.CheckHost, Detail: "a host answered, with 2 sessions"},
		{Check: diag.CheckVersion, Detail: "the host and this binary are both v1.2.3"},
	}
	if len(report.Findings) != len(want) {
		t.Fatalf("got %d findings, want %d", len(report.Findings), len(want))
	}
	for i, w := range want {
		if got := report.Findings[i]; got != w {
			t.Errorf("finding %d = %+v\nwant %+v", i, got, w)
		}
	}
	if report.Worst() != diag.Pass {
		t.Errorf("Worst = %v, want pass", report.Worst())
	}
	if report.Version != version {
		t.Errorf("Version = %q", report.Version)
	}
	if want := diag.LogPath("/home/zed/.config/rich-presence/logs"); report.LogFile != want {
		t.Errorf("LogFile = %q, want %q", report.LogFile, want)
	}
	if conn.closed != 1 {
		t.Errorf("the Discord connection was closed %d times, want once", conn.closed)
	}
}

// TestChecks changes one port of a healthy set and looks at the finding of
// the check that port feeds.
func TestChecks(t *testing.T) {
	host := func(a diag.HostAnswer) func(*diag.Ports) {
		return func(p *diag.Ports) { p.Host = func() diag.HostAnswer { return a } }
	}
	dir := func(path string, state diag.DirState) func(*diag.Ports) {
		return func(p *diag.Ports) { p.RuntimeDir = func() (string, diag.DirState) { return path, state } }
	}
	endpoints := func(n int, err error) func(*diag.Ports) {
		return func(p *diag.Ports) { p.DiscordEndpoints = func() (int, error) { return n, err } }
	}
	cfg := func(home, file string) func(*diag.Ports) {
		return func(p *diag.Ports) { p.Config = loader(home, file) }
	}
	tests := []struct {
		name   string
		change func(*diag.Ports)
		check  string
		result diag.Result
		detail string
		action string
	}{
		{"configuration with warnings", cfg("/home/zed", `{"privacy":"loud","log_level":7}`), diag.CheckConfig, diag.Warn,
			"2 ignored: file: privacy must be off, minimal, standard or full; file: log_level must be a string",
			"Correct these; until then each keeps its default"},
		{"configuration that is not JSON", cfg("/home/zed", `{`), diag.CheckConfig, diag.Warn,
			"1 ignored: file: config.json is not a JSON object",
			"Correct these; until then each keeps its default"},
		{"no home directory", cfg("", ""), diag.CheckConfig, diag.Fail,
			"the home directory is not set, so there is no configuration file and no log",
			"Set HOME, or USERPROFILE on Windows, where Claude is started"},

		{"runtime directory missing", dir("/run/user/1000/rich-presence", diag.DirMissing), diag.CheckRuntime, diag.Warn,
			"has not been created yet",
			"Start a Claude session; the directory is made when presence first runs"},
		{"runtime directory unsafe", dir("/tmp/rich-presence-1000", diag.DirUnsafe), diag.CheckRuntime, diag.Fail,
			"is not a directory that only you can access",
			"Remove it, or set RICH_PRESENCE_RUNTIME_DIR to a directory of your own"},
		{"runtime directory unresolved", dir("", diag.DirUnresolved), diag.CheckRuntime, diag.Fail,
			"no directory gives a socket path short enough for this system",
			"Set RICH_PRESENCE_RUNTIME_DIR to a short absolute path"},
		{"runtime directory state unknown", dir("", diag.DirState(99)), diag.CheckRuntime, diag.Fail,
			"no directory gives a socket path short enough for this system",
			"Set RICH_PRESENCE_RUNTIME_DIR to a short absolute path"},

		{"several endpoints", endpoints(3, nil), diag.CheckEndpoint, diag.Pass, "found 3", ""},
		{"endpoints cannot be listed", endpoints(0, errInjected), diag.CheckEndpoint, diag.Warn,
			"could not look for Discord's pipe or socket",
			"Run doctor again; if this stays, report it"},
		{"no endpoint", endpoints(0, nil), diag.CheckEndpoint, diag.Fail,
			"no Discord pipe or socket was found",
			"Start the Discord desktop app. The web and mobile apps cannot show presence"},

		{"no host", host(diag.HostAnswer{State: diag.HostAbsent}), diag.CheckHost, diag.Warn,
			"no host is running",
			"Start a Claude session; the first one becomes the host"},
		{"silent host", host(diag.HostAnswer{}), diag.CheckHost, diag.Fail,
			"something holds the control socket and did not answer",
			"Close every Claude session so the host exits, then start one again"},
		{"host state unknown", host(diag.HostAnswer{State: diag.HostState(99)}), diag.CheckHost, diag.Fail,
			"something holds the control socket and did not answer",
			"Close every Claude session so the host exits, then start one again"},

		{"no host to compare with", host(diag.HostAnswer{State: diag.HostAbsent}), diag.CheckVersion, diag.Warn,
			"no host answered, so there is nothing to compare v1.2.3 with",
			"Start a Claude session, then run doctor again"},
		{"silent host to compare with", host(diag.HostAnswer{State: diag.HostSilent, Version: version}), diag.CheckVersion, diag.Warn,
			"no host answered, so there is nothing to compare v1.2.3 with",
			"Start a Claude session, then run doctor again"},
		{"older host", host(diag.HostAnswer{State: diag.HostAnswered, Version: "v1.0.0"}), diag.CheckVersion, diag.Fail,
			"the host is v1.0.0 and this binary is v1.2.3",
			"Close every Claude session so the old host exits, then start one again"},
		{"host with a version that is not one", host(diag.HostAnswer{State: diag.HostAnswered, Version: "/home/zed/x"}), diag.CheckVersion, diag.Fail,
			"the host is invalid and this binary is v1.2.3",
			"Close every Claude session so the old host exits, then start one again"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ports, _ := healthyPorts(t)
			tt.change(&ports)
			report := diag.Run(ports, version)
			got := finding(t, report, tt.check)
			if got.Result != tt.result || got.Detail != tt.detail || got.Action != tt.action {
				t.Errorf("finding = %+v\nwant %v, %q, %q", got, tt.result, tt.detail, tt.action)
			}
			if tt.result != diag.Pass && report.Worst() < tt.result {
				t.Errorf("Worst = %v, below %v", report.Worst(), tt.result)
			}
		})
	}
}

func TestUnknownConfigurationKeyIsNotRepeated(t *testing.T) {
	ports, _ := healthyPorts(t)
	ports.Config = loader("/home/zed", `{"secret-project":true}`)
	got := finding(t, diag.Run(ports, version), diag.CheckConfig)
	if want := "1 ignored: file: a key is not a known setting"; got.Result != diag.Warn || got.Detail != want {
		t.Errorf("finding = %+v, want a warning with %q", got, want)
	}
}

func TestNoHomeMeansNoLogFile(t *testing.T) {
	ports, _ := healthyPorts(t)
	ports.Config = loader("", "")
	report := diag.Run(ports, version)
	if report.LogFile != "" {
		t.Errorf("LogFile = %q, want none", report.LogFile)
	}
	if strings.Contains(report.Format(""), "Log file") {
		t.Error("the report names a log file")
	}
}

func TestRunCleansItsOwnVersion(t *testing.T) {
	ports, _ := healthyPorts(t)
	if got := diag.Run(ports, "not a version").Version; got != diag.InvalidVersion {
		t.Errorf("Version = %q", got)
	}
}

func TestHandshakeCheck(t *testing.T) {
	const restart = "Restart Discord, then run doctor again"
	closeFrame := func(payload string) codec.Frame {
		return codec.Frame{Op: codec.OpClose, Payload: []byte(payload)}
	}
	tests := []struct {
		name     string
		conn     func(t *testing.T) *fakeConn
		result   diag.Result
		detail   string
		action   string
		wantSent bool
	}{
		{"ready", func(t *testing.T) *fakeConn { return answering(t, ready) },
			diag.Pass, "Discord accepted the application id", "", true},
		{"unknown application id", func(t *testing.T) *fakeConn {
			return answering(t, closeFrame(`{"code":4000,"message":"Invalid Client ID"}`))
		}, diag.Fail, "Discord does not know the application id", "Check discord_application_id in the configuration", true},
		{"another close", func(t *testing.T) *fakeConn {
			return answering(t, closeFrame(`{"code":4004,"message":"/home/zed"}`))
		}, diag.Fail, "Discord closed the connection with code 4004", restart, true},
		{"no answer", func(t *testing.T) *fakeConn { return answering(t) },
			diag.Fail, "Discord did not answer the handshake", restart, true},
		{"an answer that is not JSON", func(t *testing.T) *fakeConn {
			return answering(t, codec.Frame{Op: codec.OpFrame, Payload: []byte("{")})
		}, diag.Fail, "Discord's answer to the handshake could not be read", restart, true},
		{"an answer that is not ready", func(t *testing.T) *fakeConn {
			return answering(t, codec.Frame{Op: codec.OpPing, Payload: []byte("{}")}, ready)
		}, diag.Fail, "Discord answered the handshake with something other than ready", restart, true},
		{"the handshake cannot be sent", func(t *testing.T) *fakeConn {
			conn := answering(t, ready)
			conn.writeErr = errInjected
			return conn
		}, diag.Fail, "the handshake could not be sent", restart, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ports, _ := healthyPorts(t)
			conn := tt.conn(t)
			ports.DialDiscord = func() (io.ReadWriteCloser, error) { return conn, nil }
			got := finding(t, diag.Run(ports, version), diag.CheckHandshake)
			if got.Result != tt.result || got.Detail != tt.detail || got.Action != tt.action {
				t.Errorf("finding = %+v\nwant %v, %q, %q", got, tt.result, tt.detail, tt.action)
			}
			if conn.closed != 1 {
				t.Errorf("the connection was closed %d times, want once", conn.closed)
			}
			if !tt.wantSent {
				if conn.sent.Len() != 0 {
					t.Errorf("sent %d bytes", conn.sent.Len())
				}
				return
			}
			// Exactly one frame went to Discord: the handshake, with the
			// configured application id. No activity was set.
			frame, err := codec.ReadFrame(&conn.sent)
			if err != nil {
				t.Fatal(err)
			}
			if want := codec.Handshake("1234"); frame.Op != codec.OpHandshake || !bytes.Equal(frame.Payload, want.Payload) {
				t.Errorf("sent opcode %d with %s, want the handshake %s", frame.Op, frame.Payload, want.Payload)
			}
			if _, err := codec.ReadFrame(&conn.sent); !errors.Is(err, io.EOF) {
				t.Errorf("more than the handshake was sent: %v", err)
			}
		})
	}
}

func TestHandshakeCheckWhenDiscordCannotBeReached(t *testing.T) {
	ports, _ := healthyPorts(t)
	ports.DialDiscord = func() (io.ReadWriteCloser, error) { return nil, errInjected }
	got := finding(t, diag.Run(ports, version), diag.CheckHandshake)
	want := diag.Finding{
		Check:  diag.CheckHandshake,
		Result: diag.Warn,
		Detail: "could not connect to Discord, so no handshake was tried",
		Action: "Start the Discord desktop app",
	}
	if got != want {
		t.Errorf("finding = %+v\nwant %+v", got, want)
	}
}

func TestResultString(t *testing.T) {
	for r, want := range map[diag.Result]string{diag.Pass: "pass", diag.Warn: "warn", diag.Fail: "fail", diag.Result(9): "fail"} {
		if got := r.String(); got != want {
			t.Errorf("Result(%d) = %q, want %q", int(r), got, want)
		}
	}
}

func closeWith(payload string) codec.Frame {
	return codec.Frame{Op: codec.OpClose, Payload: []byte(payload)}
}
