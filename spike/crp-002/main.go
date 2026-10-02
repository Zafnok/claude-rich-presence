// Throwaway prototype for CRP-002. Never merged. See README.md.
//
// One binary, three roles:
//
//	crp002-spike mcp     the MCP server Claude Desktop starts from the bundle
//	crp002-spike peer    the same probes, run by hand from a terminal
//	crp002-spike mark    append a marker line to the log
//
// Everything observed is appended as one JSON object per line to
// ~/crp-002-spike/spike.log. Nothing is sent anywhere.
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"
)

const version = "0.0.1"

var (
	spikeDir string
	logPath  string
	role     string
	logMu    sync.Mutex
)

type fields map[string]any

func logEvent(ev string, f fields) {
	if f == nil {
		f = fields{}
	}
	f["t"] = time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	f["pid"] = os.Getpid()
	f["role"] = role
	f["ev"] = ev
	line, _ := json.Marshal(f)
	logMu.Lock()
	defer logMu.Unlock()
	fh, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer fh.Close()
	fh.Write(append(line, '\n'))
}

// configured returns an environment value, treating a placeholder the host
// did not substitute as unset.
func configured(name string) string {
	v := os.Getenv(name)
	if strings.HasPrefix(v, "${") {
		return ""
	}
	return v
}

func main() {
	home, _ := os.UserHomeDir()
	spikeDir = filepath.Join(home, "crp-002-spike")
	os.MkdirAll(filepath.Join(spikeDir, "triggers"), 0o700)
	logPath = filepath.Join(spikeDir, "spike.log")

	cmd := "mcp"
	if len(os.Args) > 1 {
		cmd = os.Args[1]
	}
	role = cmd
	switch cmd {
	case "pack":
		if err := pack(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	case "mark":
		logEvent("mark", fields{"text": strings.Join(os.Args[2:], " ")})
	case "peer":
		logStart()
		startProbes()
		go watchSignals()
		if len(os.Args) > 2 && os.Args[2] == "--once" {
			time.Sleep(4 * time.Second)
			exit("peer_once_done", 0)
		}
		heartbeat()
	case "mcp":
		logStart()
		startProbes()
		go watchSignals()
		go heartbeat()
		serveMCP()
	default:
		fmt.Fprintln(os.Stderr, "usage: crp002-spike mcp | peer [--once] | mark <text>")
		os.Exit(2)
	}
}

func exit(reason string, code int) {
	logEvent("exit", fields{"reason": reason, "code": code})
	os.Exit(code)
}

var secretName = regexp.MustCompile(`(?i)TOKEN|KEY|SECRET|PASSWORD|AUTH|CREDENTIAL|COOKIE|UUID|ACCOUNT|ORGANI[SZ]ATION|SESSION|SOCKET|EMAIL|USER`)
var shownName = regexp.MustCompile(`(?i)^(LOCALAPPDATA|APPDATA|TEMP|TMP|TMPDIR|USERPROFILE|HOME|XDG_RUNTIME_DIR|(SPIKE|CLAUDE|MCP|ELECTRON|MSIX|APPX)[A-Z0-9_]*)$`)

// logStart records what the process can see of how it was started. Environment
// variable names are all recorded. Values are recorded only for an allowlist,
// and never for a name that looks like a secret.
func logStart() {
	exe, _ := os.Executable()
	cwd, _ := os.Getwd()
	names := []string{}
	shown := fields{}
	for _, kv := range os.Environ() {
		name, value, _ := strings.Cut(kv, "=")
		if name == "" {
			continue
		}
		names = append(names, name)
		switch {
		case name == "SPIKE_SAMPLE_SECRET":
			shown[name] = fmt.Sprintf("<%d chars, placeholder=%v>", len(value), strings.HasPrefix(value, "${"))
		case name == "USERPROFILE", shownName.MatchString(name) && !secretName.MatchString(name):
			shown[name] = value
		}
	}
	sort.Strings(names)
	logEvent("start", fields{
		"version":  version,
		"args":     os.Args,
		"exe":      exe,
		"exeFinal": finalPath(exe),
		"cwd":      cwd,
		"cwdFinal": finalPath(cwd),
		"ppid":     os.Getppid(),
		"logPath":  logPath,
		"logFinal": finalPath(logPath),
		"envNames": names,
		"env":      shown,
		"sys":      sysFacts(),
	})
}

func startProbes() {
	for _, c := range candidates() {
		go runChannel(c)
	}
	go probeDiscord("start")
}

func watchSignals() {
	ch := make(chan os.Signal, 4)
	signal.Notify(ch, os.Interrupt, syscall.SIGTERM)
	s := <-ch
	exit("signal:"+s.String(), 0)
}

// heartbeat bounds the time of a death that leaves no other trace, and acts on
// trigger files dropped into ~/crp-002-spike/triggers by the runbook.
func heartbeat() {
	started := time.Now()
	handled := map[string]time.Time{}
	for tick := 1; ; tick++ {
		time.Sleep(time.Second)
		if tick%5 == 0 {
			logEvent("hb", nil)
		}
		entries, _ := os.ReadDir(filepath.Join(spikeDir, "triggers"))
		for _, e := range entries {
			info, err := e.Info()
			if err != nil || !info.ModTime().After(started) || !info.ModTime().After(handled[e.Name()]) {
				continue
			}
			handled[e.Name()] = info.ModTime()
			switch {
			case e.Name() == "discord":
				go probeDiscord("trigger")
			case e.Name() == "stop-peer" && role == "peer":
				exit("trigger:stop-peer", 0)
			case e.Name() == "exit-0" && role == "mcp":
				exit("trigger:exit-0", 0)
			case e.Name() == "exit-1" && role == "mcp":
				exit("trigger:exit-1", 1)
			}
		}
	}
}

type rpcMessage struct {
	ID     json.RawMessage `json:"id,omitempty"`
	Method string          `json:"method,omitempty"`
	Params json.RawMessage `json:"params,omitempty"`
}

func serveMCP() {
	in := bufio.NewReaderSize(os.Stdin, 1<<20)
	out := json.NewEncoder(os.Stdout)
	reply := func(id json.RawMessage, result any, rpcErr any) {
		m := map[string]any{"jsonrpc": "2.0", "id": id}
		if rpcErr != nil {
			m["error"] = rpcErr
		} else {
			m["result"] = result
		}
		out.Encode(m)
	}
	for {
		line, err := in.ReadBytes('\n')
		line = []byte(strings.TrimPrefix(string(line), string(rune(0xFEFF)))) // Windows PowerShell adds a byte order mark
		if len(strings.TrimSpace(string(line))) > 0 {
			var m rpcMessage
			if jerr := json.Unmarshal(line, &m); jerr != nil {
				logEvent("mcp_bad_json", fields{"bytes": len(line)})
			} else {
				handleMCP(m, reply)
			}
		}
		if err != nil {
			logEvent("stdin_closed", fields{"err": err.Error()})
			break
		}
	}
	if configured("SPIKE_LINGER") == "true" {
		// Stay alive to see whether something else ends the process.
		logEvent("linger_begin", nil)
		time.Sleep(90 * time.Second)
		exit("linger_survived_90s", 0)
	}
	exit("stdin_closed", 0)
}

func handleMCP(m rpcMessage, reply func(json.RawMessage, any, any)) {
	isRequest := len(m.ID) > 0
	switch m.Method {
	case "initialize":
		// Client name, version, protocol version and capabilities. No work content.
		var p struct {
			ProtocolVersion string          `json:"protocolVersion"`
			ClientInfo      json.RawMessage `json:"clientInfo"`
			Capabilities    json.RawMessage `json:"capabilities"`
		}
		json.Unmarshal(m.Params, &p)
		logEvent("mcp_initialize", fields{"protocolVersion": p.ProtocolVersion, "clientInfo": p.ClientInfo, "capabilities": p.Capabilities})
		reply(m.ID, map[string]any{
			"protocolVersion": p.ProtocolVersion,
			"capabilities":    map[string]any{"tools": map[string]any{}},
			"serverInfo":      map[string]any{"name": "crp002-spike", "version": version},
		}, nil)
	case "ping":
		logEvent("mcp_ping", nil)
		reply(m.ID, map[string]any{}, nil)
	case "tools/list":
		tools := []any{}
		if configured("SPIKE_EXPOSE_TOOL") != "false" {
			tools = append(tools, map[string]any{
				"name":        "spike_status",
				"description": "Diagnostic for the CRP-002 spike. Reports the server's process id.",
				"inputSchema": map[string]any{"type": "object", "properties": map[string]any{}},
			})
		}
		logEvent("mcp_tools_list", fields{"count": len(tools)})
		reply(m.ID, map[string]any{"tools": tools}, nil)
	case "tools/call":
		var p struct {
			Name string `json:"name"`
		}
		json.Unmarshal(m.Params, &p)
		logEvent("mcp_tools_call", fields{"name": p.Name})
		text := fmt.Sprintf("crp002-spike %s, process %d, log at %s", version, os.Getpid(), logPath)
		reply(m.ID, map[string]any{"content": []any{map[string]any{"type": "text", "text": text}}}, nil)
	default:
		// Method name only. Parameters are not read.
		logEvent("mcp_other", fields{"method": m.Method, "request": isRequest})
		if isRequest {
			reply(m.ID, nil, map[string]any{"code": -32601, "message": "method not found"})
		}
	}
}

// discordHandshake sends the handshake frame and reads one frame back. Only
// the opcode, command, event, close code and message are logged: a READY
// payload carries the Discord user's identity, which is not needed here.
func discordHandshake(rw io.ReadWriter, clientID string) fields {
	payload, _ := json.Marshal(map[string]any{"v": 1, "client_id": clientID})
	frame := make([]byte, 8, 8+len(payload))
	putU32(frame[0:], 0)
	putU32(frame[4:], uint32(len(payload)))
	if _, err := rw.Write(append(frame, payload...)); err != nil {
		return fields{"writeErr": err.Error()}
	}
	done := make(chan fields, 1)
	go func() {
		head := make([]byte, 8)
		if _, err := io.ReadFull(rw, head); err != nil {
			done <- fields{"readErr": err.Error()}
			return
		}
		n := getU32(head[4:])
		if n > 1<<16 {
			done <- fields{"op": getU32(head), "tooLong": n}
			return
		}
		body := make([]byte, n)
		if _, err := io.ReadFull(rw, body); err != nil {
			done <- fields{"op": getU32(head), "readErr": err.Error()}
			return
		}
		var msg struct {
			Cmd     string `json:"cmd"`
			Evt     string `json:"evt"`
			Code    int    `json:"code"`
			Message string `json:"message"`
		}
		json.Unmarshal(body, &msg)
		done <- fields{"op": getU32(head), "cmd": msg.Cmd, "evt": msg.Evt, "code": msg.Code, "message": msg.Message, "payloadBytes": n}
	}()
	select {
	case f := <-done:
		return f
	case <-time.After(5 * time.Second):
		return fields{"readErr": "no reply in 5s"}
	}
}

func putU32(b []byte, v uint32) {
	b[0], b[1], b[2], b[3] = byte(v), byte(v>>8), byte(v>>16), byte(v>>24)
}

func getU32(b []byte) uint32 {
	return uint32(b[0]) | uint32(b[1])<<8 | uint32(b[2])<<16 | uint32(b[3])<<24
}

func probeDiscord(why string) {
	rw, where, attempts := openDiscord()
	if rw == nil {
		logEvent("discord", fields{"why": why, "connected": false, "attempts": attempts})
		return
	}
	f := fields{"why": why, "connected": true, "endpoint": where}
	if id := configured("SPIKE_DISCORD_CLIENT_ID"); id != "" {
		f["handshake"] = discordHandshake(rw, id)
	} else {
		f["handshake"] = "skipped: no application id configured"
	}
	logEvent("discord", f)
	go rw.Close() // a close can block behind an abandoned read; do not wait for it
}
