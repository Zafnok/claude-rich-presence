package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"

	"github.com/Zafnok/claude-rich-presence/internal/adapter/code"
	"github.com/Zafnok/claude-rich-presence/internal/adapter/desktop"
	"github.com/Zafnok/claude-rich-presence/internal/config"
	"github.com/Zafnok/claude-rich-presence/internal/control/protocol"
	ctransport "github.com/Zafnok/claude-rich-presence/internal/control/transport"
	"github.com/Zafnok/claude-rich-presence/internal/diag"
	"github.com/Zafnok/claude-rich-presence/internal/domain"
	"github.com/Zafnok/claude-rich-presence/internal/host"
	"github.com/Zafnok/claude-rich-presence/internal/mcp"
)

// EnvRemote is set to "true" where Claude Code runs on a remote machine, which
// has no Discord to show presence in.
const EnvRemote = "CLAUDE_CODE_REMOTE"

// presenceLine is where the events of the session go, the status the session
// can report, and how to end it.
type presenceLine struct {
	publisher code.Publisher
	status    code.StatusSource
	// stop gives up the node's role and waits for it to finish.
	stop func()
}

// off is a presence that publishes nothing and holds nothing. The server
// still answers every tool, so that the client sees the same tools whether
// presence is on or not.
func off() presenceLine {
	return presenceLine{publisher: discard{}, status: offStatus{}, stop: func() {}}
}

type discard struct{}

func (discard) Publish(domain.Event) {}

type offStatus struct{}

func (offStatus) Status() code.Status {
	return code.Status{Role: code.RoleOff, Discord: code.DiscordDisconnected}
}

// nodeStatus is what the status tool reports, from the node's memory.
type nodeStatus struct{ node *host.Node }

func (n nodeStatus) Status() code.Status {
	st := n.node.Status()
	return code.Status{Role: adapterRole(st.Role), Discord: adapterDiscord(st.Discord), Sessions: st.Sessions}
}

// adapterRole maps the node's role to the adapter's word. A node that has no
// role yet has none, and is reported as unknown.
func adapterRole(r host.Role) code.Role {
	switch r {
	case host.RoleHost:
		return code.RoleHost
	case host.RoleFollower:
		return code.RoleFollower
	}
	return ""
}

func adapterDiscord(d protocol.DiscordState) code.Discord {
	switch d {
	case protocol.DiscordConnected:
		return code.DiscordConnected
	case protocol.DiscordConnecting:
		return code.DiscordConnecting
	case protocol.DiscordDisconnected:
		return code.DiscordDisconnected
	}
	return ""
}

// runtimePaths is where the lock and the socket of the host are, or false
// when presence is off: switched off, running remotely, or with no usable
// runtime directory. Whatever stops presence from starting turns presence off
// and not the server, because presence must never impair Claude (ADR-0008).
func runtimePaths(cfg config.Config, getenv func(string) string, log *diag.Logger) (ctransport.Paths, bool) {
	if !cfg.Enabled || getenv(EnvRemote) == "true" {
		return ctransport.Paths{}, false
	}
	paths, err := ctransport.Locate(getenv)
	if err != nil {
		log.Error("presence is off: there is no usable runtime directory", diag.ErrorClass("runtime_directory"))
		return ctransport.Paths{}, false
	}
	return paths, true
}

// startPresence starts a host node unless presence is off, in which case it
// starts nothing: no lock file, no socket.
func (s system) startPresence(cfg config.Config, getenv func(string) string, log *diag.Logger) presenceLine {
	paths, on := runtimePaths(cfg, getenv, log)
	if !on {
		return off()
	}
	node, err := host.New(s.hostConfig(cfg, paths, getenv, log))
	if err != nil {
		log.Error("presence is off: the host node could not be built", diag.ErrorClass("host_config"))
		return off()
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		node.Run(ctx)
	}()
	return presenceLine{publisher: node, status: nodeStatus{node}, stop: func() {
		cancel()
		<-done
	}}
}

// adapter is what stands between the client and presence: the tools the
// client is offered, and the session they report.
type adapter interface {
	Tools() []mcp.Tool
	// Open publishes the session, and Close ends it.
	Open()
	Close()
}

// mcpSession is one run of the mcp command.
type mcpSession struct {
	sys    system
	cfg    config.Config
	log    *diag.Logger
	getenv func(string) string

	mu sync.Mutex
	// ended is set by shutdown, after which nothing is started.
	ended bool
	// end ends the session of the adapter, and stop gives up what presence
	// holds. Each is nil until initialize has set it.
	end  func()
	stop func()
}

// initialize runs when the client initializes, and chooses the adapter by
// the name the client gives. Until then the process has taken no lock, opened
// no connection and published nothing: Claude Desktop starts a copy at launch
// that it never initializes (CRP-002).
func (m *mcpSession) initialize(client mcp.ClientInfo) []mcp.Tool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.ended {
		return nil
	}
	switch desktop.Recognise(client.Name) {
	case desktop.ClientDesktop:
		return m.open(m.desktopAdapter())
	case desktop.ClientPassive:
		return m.open(m.passiveAdapter())
	}
	return m.open(m.codeAdapter())
}

// open opens the session of an adapter and returns its tools.
func (m *mcpSession) open(a adapter, err error) []mcp.Tool {
	if err != nil {
		m.log.Error("no tools are offered: the adapter could not be built", diag.ErrorClass("adapter_config"))
		return nil
	}
	a.Open()
	m.end = a.Close
	return a.Tools()
}

// present starts presence for a client that reports a session.
func (m *mcpSession) present() presenceLine {
	line := m.sys.startPresence(m.cfg, m.getenv, m.log)
	m.stop = line.stop
	return line
}

// codeAdapter is the adapter of a Claude Code session.
func (m *mcpSession) codeAdapter() (adapter, error) {
	line := m.present()
	goos := m.sys.goos
	return code.New(code.Options{
		Privacy: m.cfg.Privacy,
		Resolve: func(cwd string) code.Settings {
			s := m.cfg.Effective(goos, cwd)
			return code.Settings{Privacy: s.Privacy, Name: s.Name, Link: s.Link}
		},
		ProvisionalID: m.sys.sessionID(),
		Publisher:     line.publisher,
		Status:        line.status,
		Clock:         m.sys.clock,
	})
}

// desktopAdapter is the adapter of the copy of the server that reports
// Claude Desktop. Its session lasts as long as the process, under one id.
func (m *mcpSession) desktopAdapter() (adapter, error) {
	line := m.present()
	return desktop.New(desktop.Options{
		Privacy:   m.cfg.Privacy,
		ID:        m.sys.sessionID(),
		Publisher: line.publisher,
		Status:    desktopStatus{line.status},
		Clock:     m.sys.clock,
	})
}

// passiveAdapter is the adapter of the second copy Claude Desktop runs. It
// is not a node: it never tries the host lock and never follows a host. Its
// status tool reports what the host says of itself.
func (m *mcpSession) passiveAdapter() (adapter, error) {
	status := desktop.StatusSource(desktopStatus{offStatus{}})
	if paths, on := runtimePaths(m.cfg, m.getenv, m.log); on {
		asker := &hostAsker{ask: func() (protocol.StatusResult, error) { return m.sys.ask(paths.Socket) }}
		m.stop = asker.stop
		status = asker
	}
	return desktop.NewPassive(m.cfg.Privacy, status)
}

// desktopStatus is a status in the words of the Desktop adapter, which are
// the same words.
type desktopStatus struct{ source code.StatusSource }

func (d desktopStatus) Status() desktop.Status {
	s := d.source.Status()
	return desktop.Status{Role: desktop.Role(s.Role), Discord: desktop.Discord(s.Discord), Sessions: s.Sessions}
}

// hostAsker is the status of a process that is not a node. A status tool may
// not wait for the host (ADR-0008), so Status answers with what the host said
// last and asks again on another goroutine, as the status of a follower does.
// The first answer is therefore unknown.
type hostAsker struct {
	// ask is the question the status command puts to the host.
	ask func() (protocol.StatusResult, error)

	mu      sync.Mutex
	last    desktop.Status
	asking  bool
	stopped bool
	// pending counts the goroutine that is asking, if there is one.
	pending sync.WaitGroup
}

func (h *hostAsker) Status() desktop.Status {
	h.mu.Lock()
	defer h.mu.Unlock()
	if !h.asking && !h.stopped {
		h.asking = true
		h.pending.Add(1)
		go h.refresh()
	}
	last := h.last
	last.Role = desktop.RolePassive
	return last
}

// refresh asks the host once. A host that does not answer, or is not there,
// leaves the connection unknown and the count at zero.
func (h *hostAsker) refresh() {
	defer h.pending.Done()
	var next desktop.Status
	defer func() {
		// A panic costs the answer and nothing else.
		_ = recover()
		h.mu.Lock()
		h.last, h.asking = next, false
		h.mu.Unlock()
	}()
	if result, err := h.ask(); err == nil {
		next = desktop.Status{Discord: desktop.Discord(adapterDiscord(result.Discord)), Sessions: result.Sessions}
	}
}

// stop waits for a question that is on its way, and lets no other start.
func (h *hostAsker) stop() {
	h.mu.Lock()
	h.stopped = true
	h.mu.Unlock()
	h.pending.Wait()
}

// shutdown ends the events of the session and then the node: the last event
// of the adapter reaches the node before the node gives up its role, which
// clears the presence it holds.
func (m *mcpSession) shutdown() {
	m.mu.Lock()
	m.ended = true
	end, stop := m.end, m.stop
	m.mu.Unlock()
	if end != nil {
		end()
	}
	if stop != nil {
		stop()
	}
}

// runMCP serves the tools over standard input and output until the input
// closes or the process is told to stop. Standard output carries the protocol
// and nothing else; the log is a file.
func (s system) runMCP(args []string, stdin io.Reader, stdout, stderr io.Writer, getenv func(string) string) int {
	if code, ok := parseFlags("mcp", args, stderr); !ok {
		return code
	}
	cfg, dirs, warnings := config.Load(s.goos, getenv, s.files)
	log := diag.Open(s.fs, dirs.Logs, cfg.LogLevel, s.clock.Now)
	if len(warnings) > 0 {
		log.Warn("the configuration has problems; run doctor", diag.Count("problems", int64(len(warnings))))
	}
	ctx, stopSignals := s.notify(context.Background())
	defer stopSignals()

	m := &mcpSession{sys: s, cfg: cfg, log: log, getenv: getenv}
	var once sync.Once
	shutdown := func() { once.Do(m.shutdown) }

	served := make(chan error, 1)
	go func() {
		// A panic in the server ends the session, not the process's output.
		defer func() {
			if recover() != nil {
				log.Error("the server stopped on a panic", diag.ErrorClass("panic"))
				served <- errPanic
			}
		}()
		served <- mcp.Serve(stdin, stdout, mcp.Options{
			Name:       BinaryName,
			Version:    s.version,
			Initialize: m.initialize,
			Shutdown:   shutdown,
		})
	}()

	select {
	case <-ctx.Done():
		// Serve is blocked reading and is left to end with the process.
		shutdown()
		return exitOK
	case err := <-served:
		shutdown()
		if err != nil {
			fmt.Fprintf(stderr, "%s: mcp: %v\n", BinaryName, err)
			return exitFailure
		}
		return exitOK
	}
}

// errPanic is what a panic in the server is reported as.
var errPanic = errors.New("internal error")
