package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/Zafnok/claude-rich-presence/internal/control/protocol"
	ctransport "github.com/Zafnok/claude-rich-presence/internal/control/transport"
)

// askTimeout is how long a host has to answer. The connection has no
// deadline of its own, so a timer closes it.
const askTimeout = 2 * time.Second

// unsafeDirMessage is all that status says of a runtime directory it will
// not connect in. The doctor names the directory and what to do.
const unsafeDirMessage = "the runtime directory is not safe to use, so no host was asked; run `" + BinaryName + " doctor`"

var (
	errNoWelcome = errors.New("the host did not welcome this binary")
	errNoAnswer  = errors.New("the host did not answer the status request")
)

// ask connects to the host's socket, introduces itself, asks for the host's
// status and closes. It is not a node: it sends no session. An error that
// matches ctransport.ErrNoHost means nothing is listening, and one that
// matches ctransport.ErrUnsafeDir that nothing was connected to.
func (s system) ask(socket string) (protocol.StatusResult, error) {
	conn, err := ctransport.Dial(context.Background(), socket, askTimeout)
	if err != nil {
		return protocol.StatusResult{}, err
	}
	defer func() { _ = conn.Close() }()
	stop := s.clock.AfterFunc(askTimeout, func() { _ = conn.Close() })
	defer stop()
	return converse(conn, s.version)
}

// converse says hello, expects a welcome, asks for the status and expects the
// answer. It sends nothing else, and in particular no session.
func converse(conn io.ReadWriter, binaryVersion string) (protocol.StatusResult, error) {
	if err := protocol.Encode(conn, protocol.Hello{Protocol: protocol.Version, Version: binaryVersion}); err != nil {
		return protocol.StatusResult{}, err
	}
	lines := protocol.NewDecoder(conn)
	if first, _ := lines.Next(); !isWelcome(first) {
		return protocol.StatusResult{}, errNoWelcome
	}
	if err := protocol.Encode(conn, protocol.Status{}); err != nil {
		return protocol.StatusResult{}, err
	}
	answer, _ := lines.Next()
	result, ok := answer.(protocol.StatusResult)
	if !ok {
		return protocol.StatusResult{}, errNoAnswer
	}
	return result, nil
}

func isWelcome(m protocol.Message) bool {
	_, ok := m.(protocol.Welcome)
	return ok
}

// runStatus prints what the running host says of itself. It exits 3 when no
// host is running, which is not a failure of this command. A runtime
// directory that another user could be listening in is a failure.
func (s system) runStatus(args []string, stdout, stderr io.Writer, getenv func(string) string) int {
	if code, ok := parseFlags("status", args, stderr); !ok {
		return code
	}
	paths, err := ctransport.Locate(getenv)
	if err != nil {
		fmt.Fprintf(stderr, "%s: status: %v\n", BinaryName, err)
		return exitFailure
	}
	result, err := s.ask(paths.Socket)
	switch {
	case errors.Is(err, ctransport.ErrNoHost):
		fmt.Fprintf(stdout, "No presence host is running.\n")
		return exitNoHost
	case errors.Is(err, ctransport.ErrUnsafeDir):
		fmt.Fprintf(stderr, "%s: status: %s\n", BinaryName, unsafeDirMessage)
		return exitFailure
	case err != nil:
		fmt.Fprintf(stderr, "%s: status: %v\n", BinaryName, err)
		return exitFailure
	}
	fmt.Fprintf(stdout, "Role: host\nDiscord: %s\nSessions: %d\nVersion: %s\nUptime: %s\nPaused: %s\n",
		result.Discord, result.Sessions, result.Version, time.Duration(result.UptimeSeconds)*time.Second,
		pausedWord(result.Pause, s.clock.Now()))
	return exitOK
}

// pausedWord says whether the host is paused, from what it said of itself.
func pausedWord(p *protocol.PauseState, now time.Time) string {
	switch {
	case p == nil:
		return "unavailable, the presence host is an older version"
	case !p.Paused || p.Until != 0 && now.UnixMilli() >= p.Until:
		return "no"
	case p.Until == 0:
		return "until resumed"
	}
	return "until " + time.UnixMilli(p.Until).UTC().Format("2006-01-02 15:04:05 UTC")
}
