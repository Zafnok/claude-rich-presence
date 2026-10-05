package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"

	"github.com/Zafnok/claude-rich-presence/internal/adapter/code"
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

// startPresence starts a host node unless presence is off, in which case it
// starts nothing: no lock file, no socket. Whatever stops the node from
// starting also turns presence off and not the server, because presence must
// never impair Claude (ADR-0008).
func (s system) startPresence(cfg config.Config, getenv func(string) string, log *diag.Logger) presenceLine {
	if !cfg.Enabled || getenv(EnvRemote) == "true" {
		return off()
	}
	paths, err := ctransport.Locate(getenv)
	if err != nil {
		log.Error("presence is off: there is no usable runtime directory", diag.ErrorClass("runtime_directory"))
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

// mcpSession is one run of the mcp command.
type mcpSession struct {
	sys      system
	cfg      config.Config
	log      *diag.Logger
	presence presenceLine

	mu      sync.Mutex
	adapter *code.Adapter
}

// initialize runs when the client initializes. The adapter is chosen by the
// client's name once there is more than one; until then every client is
// Claude Code (CRP-050 adds the choice here).
func (m *mcpSession) initialize(mcp.ClientInfo) []mcp.Tool {
	goos := m.sys.goos
	adapter, err := code.New(code.Options{
		Privacy: m.cfg.Privacy,
		Resolve: func(cwd string) code.Settings {
			s := m.cfg.Effective(goos, cwd)
			return code.Settings{Privacy: s.Privacy, Name: s.Name}
		},
		ProvisionalID: m.sys.sessionID(),
		Publisher:     m.presence.publisher,
		Status:        m.presence.status,
		Clock:         m.sys.clock,
	})
	if err != nil {
		m.log.Error("no tools are offered: the adapter could not be built", diag.ErrorClass("adapter_config"))
		return nil
	}
	adapter.Open()
	m.mu.Lock()
	m.adapter = adapter
	m.mu.Unlock()
	return adapter.Tools()
}

// shutdown ends the session's events and then the node: the adapter's last
// event reaches the node before the node gives up its role, which clears the
// presence it holds.
func (m *mcpSession) shutdown() {
	m.mu.Lock()
	adapter := m.adapter
	m.mu.Unlock()
	if adapter != nil {
		adapter.Close()
	}
	m.presence.stop()
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

	m := &mcpSession{sys: s, cfg: cfg, log: log, presence: s.startPresence(cfg, getenv, log)}
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
