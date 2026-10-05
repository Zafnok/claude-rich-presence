package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"time"

	"github.com/Zafnok/claude-rich-presence/internal/config"
	ctransport "github.com/Zafnok/claude-rich-presence/internal/control/transport"
	"github.com/Zafnok/claude-rich-presence/internal/diag"
)

// discordTimeout bounds the doctor's connection to Discord, in the dial and
// in every read after it.
const discordTimeout = 5 * time.Second

// runDoctor makes the checks of internal/diag and prints the report, which is
// safe to paste into a public issue. It exits 0 only when every check passes.
func (s system) runDoctor(args []string, stdout, stderr io.Writer, getenv func(string) string) int {
	if code, ok := parseFlags("doctor", args, stderr); !ok {
		return code
	}
	report := diag.Run(s.doctorPorts(getenv), s.version)
	fmt.Fprint(stdout, report.Format(homeDir(s.goos, getenv)))
	if report.Worst() != diag.Pass {
		return exitFailure
	}
	return exitOK
}

func homeDir(goos string, getenv func(string) string) string {
	if goos == "windows" {
		return getenv("USERPROFILE")
	}
	return getenv("HOME")
}

func (s system) doctorPorts(getenv func(string) string) diag.Ports {
	return diag.Ports{
		Config: func() (config.Config, config.Dirs, []config.Warning) {
			return config.Load(s.goos, getenv, s.files)
		},
		RuntimeDir: func() (string, diag.DirState) {
			paths, err := ctransport.Locate(getenv)
			if err != nil {
				return "", diag.DirUnresolved
			}
			// Prepare creates a directory that is missing, which a check must
			// not do, so it is only asked of one that is there.
			if _, err := os.Lstat(paths.Dir); errors.Is(err, fs.ErrNotExist) {
				return paths.Dir, diag.DirMissing
			}
			if ctransport.Prepare(paths.Dir) != nil {
				return paths.Dir, diag.DirUnsafe
			}
			return paths.Dir, diag.DirReady
		},
		DiscordEndpoints: func() (int, error) {
			_, candidates := s.discord(getenv)
			found := 0
			for _, c := range candidates {
				if s.exists(c) {
					found++
				}
			}
			return found, nil
		},
		DialDiscord: func() (io.ReadWriteCloser, error) {
			dialer, _ := s.discord(getenv)
			ctx, cancel := context.WithTimeout(context.Background(), discordTimeout)
			defer cancel()
			conn, err := dialer.Dial(ctx)
			if err != nil {
				return nil, err
			}
			// A deadline is a moment of real time, whatever clock the host runs on.
			if err := conn.SetDeadline(time.Now().Add(discordTimeout)); err != nil {
				_ = conn.Close()
				return nil, err
			}
			return conn, nil
		},
		Host: func() diag.HostAnswer {
			paths, err := ctransport.Locate(getenv)
			if err != nil {
				return diag.HostAnswer{State: diag.HostAbsent}
			}
			result, err := s.ask(paths.Socket)
			switch {
			case errors.Is(err, ctransport.ErrNoHost):
				return diag.HostAnswer{State: diag.HostAbsent}
			case err != nil:
				return diag.HostAnswer{State: diag.HostSilent}
			}
			return diag.HostAnswer{State: diag.HostAnswered, Version: result.Version, Sessions: result.Sessions}
		},
	}
}
