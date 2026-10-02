//go:build !windows

package main

import (
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

func candidates() []candidate {
	const leaf = "rich-presence-crp002"
	home, _ := os.UserHomeDir()
	list := []candidate{
		{"tmpdir", filepath.Join(os.TempDir(), leaf)}, // ADR-0006 on macOS
		{"home", filepath.Join(home, "."+leaf)},
	}
	if d := os.Getenv("XDG_RUNTIME_DIR"); d != "" {
		list = append(list, candidate{"xdg", filepath.Join(d, leaf)})
	}
	return list
}

// tryLock takes the flock of ADR-0005. The descriptor is deliberately never closed.
func tryLock(path string) (string, error) {
	fd, err := syscall.Open(path, syscall.O_RDWR|syscall.O_CREAT, 0o600)
	if err != nil {
		return "", err
	}
	if err := syscall.Flock(fd, syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		syscall.Close(fd)
		return "", err
	}
	return finalPath(path), nil
}

func finalPath(path string) string {
	p, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "error: " + err.Error()
	}
	return p
}

func run(name string, args ...string) string {
	out, err := exec.Command(name, args...).CombinedOutput()
	if err != nil {
		return strings.TrimSpace(string(out)) + " error: " + err.Error()
	}
	return strings.TrimSpace(string(out))
}

func sysFacts() fields {
	f := fields{}
	exe, _ := os.Executable()
	// Extended attributes of our own binary: is the quarantine attribute present?
	f["xattr"] = run("xattr", "-l", exe)
	f["codesign"] = run("codesign", "-dv", exe)
	chain := []string{}
	pid := os.Getpid()
	for depth := 0; depth < 10 && pid > 1; depth++ {
		line := run("ps", "-o", "ppid=,comm=", "-p", strconv.Itoa(pid))
		chain = append(chain, fmt.Sprintf("%d %s", pid, line))
		parent, err := strconv.Atoi(strings.Fields(line + " x")[0])
		if err != nil {
			break
		}
		pid = parent
	}
	f["ancestry"] = chain
	return f
}

func openDiscord() (io.ReadWriteCloser, string, []string) {
	attempts := []string{}
	dir := "/tmp"
	for _, name := range []string{"XDG_RUNTIME_DIR", "TMPDIR", "TMP", "TEMP"} {
		if v := os.Getenv(name); v != "" {
			dir = v
			break
		}
	}
	for n := 0; n < 10; n++ {
		path := filepath.Join(dir, fmt.Sprintf("discord-ipc-%d", n))
		conn, err := net.Dial("unix", path)
		if err == nil {
			return conn, path, attempts
		}
		attempts = append(attempts, fmt.Sprintf("%d: %v", n, err))
	}
	return nil, "", attempts
}
