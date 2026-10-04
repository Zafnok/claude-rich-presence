package diag

import (
	"fmt"
	"io"
	"strings"

	"github.com/Zafnok/claude-rich-presence/internal/config"
	"github.com/Zafnok/claude-rich-presence/internal/discord/codec"
)

// Result is the outcome of one doctor check.
type Result int

// The results, from best to worst.
const (
	Pass Result = iota
	Warn
	Fail
)

func (r Result) String() string {
	switch r {
	case Pass:
		return "pass"
	case Warn:
		return "warn"
	}
	return "fail"
}

// The names of the checks, in the order Run makes them.
const (
	CheckConfig    = "Configuration"
	CheckRuntime   = "Runtime directory"
	CheckEndpoint  = "Discord endpoint"
	CheckHandshake = "Discord handshake"
	CheckHost      = "Host"
	CheckVersion   = "Version"
)

// Finding is what one check found.
type Finding struct {
	Check  string
	Result Result
	// Detail is a one-line explanation. It never holds a path.
	Detail string
	// Action is what the user can try. It is empty on a pass.
	Action string
	// Path is the file or directory the finding is about, if any. It is the
	// only field a path goes in, and Format redacts it.
	Path string
}

// DirState is what was found at the runtime directory. The zero value is the
// worst, so a port that forgets to set it does not pass.
type DirState int

// The states of the runtime directory.
const (
	// DirUnresolved means no directory gives a socket path that fits.
	DirUnresolved DirState = iota
	// DirUnsafe means it is not a directory, or not the user's alone.
	DirUnsafe
	// DirMissing means it has not been created yet.
	DirMissing
	// DirReady means it exists, is the user's, and the socket path fits.
	DirReady
)

// HostState is what asking a host for its status came to. The zero value is
// the worst.
type HostState int

// The states of the host.
const (
	// HostSilent means something holds the control socket and did not
	// answer.
	HostSilent HostState = iota
	// HostAbsent means nothing is listening, as when no session is open.
	HostAbsent
	// HostAnswered means a host answered.
	HostAnswered
)

// HostAnswer is what a host said of itself. Only State is set unless it is
// HostAnswered.
type HostAnswer struct {
	State    HostState
	Version  string
	Sessions int
}

// Ports are what the checks look at. Every field must be set.
type Ports struct {
	// Config loads the configuration, as config.Load does.
	Config func() (config.Config, config.Dirs, []config.Warning)
	// RuntimeDir resolves and inspects the runtime directory of the control
	// channel. The path is empty when it is DirUnresolved.
	RuntimeDir func() (string, DirState)
	// DiscordEndpoints counts the candidate pipes or sockets that exist.
	DiscordEndpoints func() (int, error)
	// DialDiscord connects to Discord. Reads on the connection must give up
	// after a short time, so that a Discord which does not answer cannot
	// hang the check.
	DialDiscord func() (io.ReadWriteCloser, error)
	// Host asks a running host for its status.
	Host func() HostAnswer
}

// Run makes every check. version is that of this binary.
func Run(p Ports, version string) Report {
	cfg, dirs, warnings := p.Config()
	host := p.Host()
	report := Report{
		Version: CleanVersion(version),
		Findings: []Finding{
			checkConfig(dirs, warnings),
			checkRuntimeDir(p.RuntimeDir()),
			checkEndpoint(p.DiscordEndpoints()),
			checkHandshake(p.DialDiscord, cfg.DiscordApplicationID),
			checkHost(host),
			checkVersion(host, version),
		},
	}
	if dirs.Logs != "" {
		report.LogFile = LogPath(dirs.Logs)
	}
	return report
}

func checkConfig(dirs config.Dirs, warnings []config.Warning) Finding {
	f := Finding{Check: CheckConfig, Path: dirs.File}
	switch {
	case dirs.File == "":
		f.Result = Fail
		f.Detail = "the home directory is not set, so there is no configuration file and no log"
		f.Action = "Set HOME, or USERPROFILE on Windows, where Claude is started"
	case len(warnings) > 0:
		problems := make([]string, len(warnings))
		for i, w := range warnings {
			// The key of an unknown setting is text the user wrote, and
			// is left out.
			if w.Problem == config.ProblemUnknownSetting {
				w.Setting = "a key"
			}
			problems[i] = w.String()
		}
		f.Result = Warn
		f.Detail = fmt.Sprintf("%d ignored: %s", len(warnings), strings.Join(problems, "; "))
		f.Action = "Correct these; until then each keeps its default"
	default:
		f.Detail = "loaded without warnings"
	}
	return f
}

func checkRuntimeDir(path string, state DirState) Finding {
	f := Finding{Check: CheckRuntime, Path: path}
	switch state {
	case DirReady:
		f.Detail = "exists, is yours alone, and the socket path fits"
	case DirMissing:
		f.Result = Warn
		f.Detail = "has not been created yet"
		f.Action = "Start a Claude session; the directory is made when presence first runs"
	case DirUnsafe:
		f.Result = Fail
		f.Detail = "is not a directory that only you can access"
		f.Action = "Remove it, or set RICH_PRESENCE_RUNTIME_DIR to a directory of your own"
	default:
		f.Result = Fail
		f.Detail = "no directory gives a socket path short enough for this system"
		f.Action = "Set RICH_PRESENCE_RUNTIME_DIR to a short absolute path"
	}
	return f
}

func checkEndpoint(found int, err error) Finding {
	f := Finding{Check: CheckEndpoint}
	switch {
	case err != nil:
		f.Result = Warn
		f.Detail = "could not look for Discord's pipe or socket"
		f.Action = "Run doctor again; if this stays, report it"
	case found == 0:
		f.Result = Fail
		f.Detail = "no Discord pipe or socket was found"
		f.Action = "Start the Discord desktop app. The web and mobile apps cannot show presence"
	default:
		f.Detail = fmt.Sprintf("found %d", found)
	}
	return f
}

// closeInvalidClientID is the code of the close frame Discord answers a
// handshake with when it does not know the application id.
const closeInvalidClientID = 4000

// checkHandshake connects, sends a handshake and reads one frame. It sets no
// activity, and the connection is closed before it returns.
func checkHandshake(dial func() (io.ReadWriteCloser, error), applicationID string) Finding {
	f := Finding{Check: CheckHandshake, Result: Fail, Action: "Restart Discord, then run doctor again"}
	conn, err := dial()
	if err != nil {
		f.Result = Warn
		f.Detail = "could not connect to Discord, so no handshake was tried"
		f.Action = "Start the Discord desktop app"
		return f
	}
	defer func() { _ = conn.Close() }()
	if codec.WriteFrame(conn, codec.Handshake(applicationID)) != nil {
		f.Detail = "the handshake could not be sent"
		return f
	}
	frame, err := codec.ReadFrame(conn)
	if err != nil {
		f.Detail = "Discord did not answer the handshake"
		return f
	}
	msg, err := codec.Decode(frame)
	switch {
	case err != nil:
		f.Detail = "Discord's answer to the handshake could not be read"
	case msg.Kind == codec.KindReady:
		f.Result, f.Action = Pass, ""
		f.Detail = "Discord accepted the application id"
	case msg.Kind == codec.KindClose && msg.Code == closeInvalidClientID:
		f.Detail = "Discord does not know the application id"
		f.Action = "Check discord_application_id in the configuration"
	case msg.Kind == codec.KindClose:
		f.Detail = fmt.Sprintf("Discord closed the connection with code %d", msg.Code)
	default:
		f.Detail = "Discord answered the handshake with something other than ready"
	}
	return f
}

func checkHost(host HostAnswer) Finding {
	f := Finding{Check: CheckHost}
	switch host.State {
	case HostAnswered:
		f.Detail = fmt.Sprintf("a host answered, with %d sessions", host.Sessions)
	case HostAbsent:
		f.Result = Warn
		f.Detail = "no host is running"
		f.Action = "Start a Claude session; the first one becomes the host"
	default:
		f.Result = Fail
		f.Detail = "something holds the control socket and did not answer"
		f.Action = "Close every Claude session so the host exits, then start one again"
	}
	return f
}

func checkVersion(host HostAnswer, version string) Finding {
	f := Finding{Check: CheckVersion}
	self, theirs := CleanVersion(version), CleanVersion(host.Version)
	switch {
	case host.State != HostAnswered:
		f.Result = Warn
		f.Detail = "no host answered, so there is nothing to compare " + self + " with"
		f.Action = "Start a Claude session, then run doctor again"
	case host.Version != version:
		f.Result = Fail
		f.Detail = "the host is " + theirs + " and this binary is " + self
		f.Action = "Close every Claude session so the old host exits, then start one again"
	default:
		f.Detail = "the host and this binary are both " + self
	}
	return f
}
