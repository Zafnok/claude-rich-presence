package cli

import (
	"context"
	"errors"
	"io"
	"time"

	"github.com/Zafnok/claude-rich-presence/internal/config"
	"github.com/Zafnok/claude-rich-presence/internal/control/protocol"
	ctransport "github.com/Zafnok/claude-rich-presence/internal/control/transport"
	"github.com/Zafnok/claude-rich-presence/internal/diag"
	"github.com/Zafnok/claude-rich-presence/internal/discord/session"
	dtransport "github.com/Zafnok/claude-rich-presence/internal/discord/transport"
	"github.com/Zafnok/claude-rich-presence/internal/host"
	"github.com/Zafnok/claude-rich-presence/internal/presence"
)

// dialTimeout is how long a node waits to connect to a host's socket.
const dialTimeout = 2 * time.Second

// hostConfig supplies the real ports of a host node (CRP-032).
func (s system) hostConfig(cfg config.Config, paths ctransport.Paths, getenv func(string) string, log *diag.Logger) host.Config {
	dialer, _ := s.discord(getenv)
	return host.Config{
		Version: s.version,
		Acquire: func() (host.Lock, error) {
			held, err := ctransport.Acquire(paths)
			if errors.Is(err, ctransport.ErrLocked) {
				return nil, host.ErrLocked
			}
			if err != nil {
				return nil, err
			}
			return lock{held}, nil
		},
		Dial: func(ctx context.Context) (io.ReadWriteCloser, error) {
			return ctransport.Dial(ctx, paths.Socket, dialTimeout)
		},
		Discord: func() host.Discord {
			return discordLink{session.New(session.Config{
				ApplicationID: cfg.DiscordApplicationID,
				Dial: func(ctx context.Context) (io.ReadWriteCloser, error) {
					conn, err := dialer.Dial(ctx)
					if errors.Is(err, dtransport.ErrNotRunning) {
						return nil, session.ErrNotRunning
					}
					return conn, err
				},
				Clock:    s.clock,
				Interval: cfg.MinUpdateInterval,
				Jitter:   jitter,
				Logger:   log,
			})}
		},
		Render:   presence.Render,
		Settings: presence.Settings{IdleClear: cfg.IdleClearAfter},
		Link:     func(raw string) (string, bool) { return config.ValidateLink(raw, cfg.LinkHosts) },
		Clock:    s.clock,
		Jitter:   jitter,
		Counters: &diag.Counters{},
		Logger:   log,
	}
}

// lock is a held host lock as the node sees it.
type lock struct{ held *ctransport.HostLock }

// Listen opens the control socket. Its listener's Accept returns what the
// node reads and writes lines on.
func (l lock) Listen() (host.Listener, error) {
	listener, err := l.held.Listen()
	if err != nil {
		return nil, err
	}
	return socket{listener}, nil
}

func (l lock) Release() error { return l.held.Release() }

// socket is a control listener as the node sees it. Close is the listener's.
type socket struct{ *ctransport.Listener }

func (l socket) Accept() (io.ReadWriteCloser, error) { return l.Listener.Accept() }

// discordLink is the Discord session manager as the node sees it.
type discordLink struct{ *session.Manager }

// State maps the manager's state to the control protocol's.
func (d discordLink) State() protocol.DiscordState { return discordState(d.Status().State) }

func discordState(s session.State) protocol.DiscordState {
	switch s {
	case session.Ready:
		return protocol.DiscordConnected
	case session.Connecting:
		return protocol.DiscordConnecting
	case session.Disconnected:
		return protocol.DiscordDisconnected
	}
	return protocol.DiscordUnknown
}
