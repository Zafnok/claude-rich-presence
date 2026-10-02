package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"time"
)

// A candidate is one place the lock file and control socket of ADR-0006 could
// live. Each is probed independently, so one run shows which locations are
// shared between a process started by Claude Desktop and one started from a
// terminal.
type candidate struct {
	name string
	dir  string
}

// runChannel plays ADR-0005 in miniature for one candidate directory: whoever
// takes the lock listens on the socket, everyone else connects to it. A
// follower that loses the host tries the lock again.
func runChannel(c candidate) {
	lockPath := filepath.Join(c.dir, "host.lock")
	sockPath := filepath.Join(c.dir, "ctl.sock")
	base := func() fields {
		return fields{"candidate": c.name, "dir": c.dir}
	}
	if err := os.MkdirAll(c.dir, 0o700); err != nil {
		f := base()
		f["err"] = err.Error()
		logEvent("channel_mkdir_failed", f)
		return
	}
	last := ""
	for {
		lockFinal, lockErr := tryLock(lockPath)
		if lockErr == nil {
			os.Remove(sockPath)
			ln, err := net.Listen("unix", sockPath)
			f := base()
			f["dirFinal"] = finalPath(c.dir)
			f["lockFinal"] = lockFinal
			f["dirList"] = listDir(c.dir)
			if err != nil {
				f["listenErr"] = err.Error()
				logEvent("channel_host", f)
				select {} // keep the lock; there is nothing else to try
			}
			logEvent("channel_host", f)
			serveChannel(c, ln)
			return
		}
		conn, err := net.DialTimeout("unix", sockPath, 2*time.Second)
		if err != nil {
			state := "nodial:" + lockErr.Error() + "|" + err.Error()
			if state != last {
				f := base()
				f["lockErr"] = lockErr.Error()
				f["dialErr"] = err.Error()
				f["dirFinal"] = finalPath(c.dir)
				f["dirList"] = listDir(c.dir)
				logEvent("channel_follower_cannot_connect", f)
				last = state
			}
			time.Sleep(2 * time.Second)
			continue
		}
		fmt.Fprintf(conn, "{\"pid\":%d,\"role\":%q}\n", os.Getpid(), role)
		reply, _ := bufio.NewReader(conn).ReadString('\n')
		var host struct {
			Pid  int    `json:"pid"`
			Role string `json:"role"`
		}
		json.Unmarshal([]byte(reply), &host)
		f := base()
		f["lockErr"] = lockErr.Error()
		f["hostPid"] = host.Pid
		f["hostRole"] = host.Role
		f["dirFinal"] = finalPath(c.dir)
		logEvent("channel_follower_connected", f)
		io.Copy(io.Discard, conn) // the open connection is the liveness signal
		conn.Close()
		f = base()
		f["hostPid"] = host.Pid
		logEvent("channel_host_lost", f)
		last = ""
		time.Sleep(300 * time.Millisecond)
	}
}

func serveChannel(c candidate, ln net.Listener) {
	for {
		conn, err := ln.Accept()
		if err != nil {
			logEvent("channel_accept_failed", fields{"candidate": c.name, "err": err.Error()})
			return
		}
		go func() {
			defer conn.Close()
			r := bufio.NewReader(conn)
			hello, _ := r.ReadString('\n')
			var peer struct {
				Pid  int    `json:"pid"`
				Role string `json:"role"`
			}
			json.Unmarshal([]byte(hello), &peer)
			logEvent("channel_host_accepted", fields{"candidate": c.name, "peerPid": peer.Pid, "peerRole": peer.Role})
			fmt.Fprintf(conn, "{\"pid\":%d,\"role\":%q}\n", os.Getpid(), role)
			io.Copy(io.Discard, r)
			logEvent("channel_host_peer_gone", fields{"candidate": c.name, "peerPid": peer.Pid})
		}()
	}
}

func listDir(dir string) []string {
	entries, _ := os.ReadDir(dir)
	names := []string{}
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names
}
