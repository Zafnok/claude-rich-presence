// Throwaway prototype for CRP-001. Never merged.
//
// probe mcp   : minimal stdio MCP server that logs everything it observes.
// probe dump  : command-hook helper that logs the SHAPE of the hook input
//               (key paths and value types), and values only for an allowlist
//               of non-content scalar fields.
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const buildTag = "probe-v1" // overridden per build with -ldflags "-X main.version=..."

var version = buildTag

var (
	logMu   sync.Mutex
	logFile *os.File
	start   = time.Now()
)

func logDir() string {
	if d := os.Getenv("RP_SPIKE_DIR"); d != "" {
		return d
	}
	return filepath.Join(os.TempDir(), "rp-spike")
}

func openLog() {
	dir := logDir()
	_ = os.MkdirAll(dir, 0o755)
	f, err := os.OpenFile(filepath.Join(dir, "observations.jsonl"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err == nil {
		logFile = f
	}
}

func logEvent(kind string, fields map[string]any) {
	if logFile == nil {
		return
	}
	now := time.Now()
	rec := map[string]any{
		"ts":      now.Format("2006-01-02T15:04:05.000000Z07:00"),
		"mono_us": now.Sub(start).Microseconds(),
		"pid":     os.Getpid(),
		"ver":     version,
		"kind":    kind,
	}
	for k, v := range fields {
		rec[k] = v
	}
	b, _ := json.Marshal(rec)
	logMu.Lock()
	_, _ = logFile.Write(append(b, '\n'))
	logMu.Unlock()
}

// Values are logged only for these variables. Every other variable is logged by name only.
var envValueAllow = []string{
	"CLAUDE_PLUGIN_ROOT", "CLAUDE_PLUGIN_DATA", "CLAUDE_PROJECT_DIR", "CLAUDE_CODE_REMOTE",
	"CLAUDE_CODE_ENTRYPOINT", "CLAUDECODE", "CLAUDE_CODE_SESSION_ID", "CLAUDE_CODE_MCP_SERVER_NAME", "CLAUDE_EFFORT",
	"MCP_TIMEOUT", "MCP_TOOL_TIMEOUT", "TERM_PROGRAM",
}

func envSnapshot() (map[string]string, []string) {
	vals := map[string]string{}
	var names []string
	for _, kv := range os.Environ() {
		i := strings.IndexByte(kv, '=')
		if i <= 0 {
			continue
		}
		k, v := kv[:i], kv[i+1:]
		up := strings.ToUpper(k)
		allowed := strings.HasPrefix(up, "RP_") || strings.HasPrefix(up, "CLAUDE_PLUGIN_OPTION_")
		for _, a := range envValueAllow {
			if up == a {
				allowed = true
			}
		}
		if allowed {
			vals[k] = v
		}
		if strings.Contains(up, "CLAUDE") || strings.Contains(up, "MCP") || strings.Contains(up, "ANTHROPIC") || strings.HasPrefix(up, "RP_") {
			names = append(names, k)
		}
	}
	sort.Strings(names)
	return vals, names
}

func readMode() string {
	b, err := os.ReadFile(filepath.Join(logDir(), "mode.txt"))
	if err != nil {
		return "empty"
	}
	return strings.TrimSpace(string(b))
}

type rpcMsg struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
}

var outMu sync.Mutex

func reply(id json.RawMessage, result any) {
	b, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": id, "result": result})
	outMu.Lock()
	_, _ = os.Stdout.Write(append(b, '\n'))
	outMu.Unlock()
}

func replyErr(id json.RawMessage, code int, msg string) {
	b, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": id, "error": map[string]any{"code": code, "message": msg}})
	outMu.Lock()
	_, _ = os.Stdout.Write(append(b, '\n'))
	outMu.Unlock()
}

var tools = []map[string]any{
	{
		"name":        "presence_event",
		"description": "Internal. Called by hooks, not by the assistant.",
		"inputSchema": map[string]any{
			"type":                 "object",
			"properties":           map[string]any{"event": map[string]any{"type": "string"}},
			"additionalProperties": true,
		},
	},
	{
		"name":        "presence_status",
		"description": "Report whether Discord presence is working.",
		"inputSchema": map[string]any{"type": "object", "properties": map[string]any{}},
	},
}

func textResult(s string) map[string]any {
	return map[string]any{"content": []map[string]any{{"type": "text", "text": s}}}
}

func handleCall(m rpcMsg, recv time.Time) {
	var p struct {
		Name      string         `json:"name"`
		Arguments map[string]any `json:"arguments"`
		Meta      map[string]any `json:"_meta"`
	}
	_ = json.Unmarshal(m.Params, &p)
	mode := readMode()
	ev, _ := p.Arguments["event"].(string)
	logEvent("tools_call", map[string]any{"tool": p.Name, "args": p.Arguments, "meta": p.Meta, "mode": mode, "id": string(m.ID)})

	done := func(res map[string]any) {
		reply(m.ID, res)
		logEvent("tools_call_replied", map[string]any{"id": string(m.ID), "event": ev, "server_us": time.Since(recv).Microseconds()})
	}

	if p.Name == "presence_status" {
		done(textResult("probe " + version + " alive"))
		return
	}
	switch {
	case mode == "marker":
		done(textResult("RP-SPIKE-MARKER-7Q4Z for " + ev))
	case mode == "jsonctx":
		out, _ := json.Marshal(map[string]any{"hookSpecificOutput": map[string]any{"hookEventName": strings.SplitN(ev, "#", 2)[0], "additionalContext": "RP-SPIKE-JSONCTX-9K2M for " + ev}})
		done(textResult(string(out)))
	case mode == "suppress":
		done(textResult(`{"suppressOutput":true}`))
	case mode == "emptyjson":
		done(textResult(`{}`))
	case mode == "emptytext":
		done(textResult(""))
	case mode == "nocontent":
		done(map[string]any{})
	case mode == "error":
		res := textResult("RP-SPIKE-ERROR-3H8V for " + ev)
		res["isError"] = true
		done(res)
	case mode == "hang":
		logEvent("hanging", map[string]any{"id": string(m.ID), "event": ev})
	case mode == "exit":
		logEvent("exiting_on_call", map[string]any{"id": string(m.ID), "event": ev})
		os.Exit(3)
	case strings.HasPrefix(mode, "slow:"):
		var ms int
		_, _ = fmt.Sscanf(mode, "slow:%d", &ms)
		time.Sleep(time.Duration(ms) * time.Millisecond)
		done(map[string]any{"content": []any{}})
	default:
		done(map[string]any{"content": []any{}})
	}
}

func runMCP() {
	cwd, _ := os.Getwd()
	exe, _ := os.Executable()
	vals, names := envSnapshot()
	stdinInfo := ""
	if fi, err := os.Stdin.Stat(); err == nil {
		stdinInfo = fi.Mode().String()
	}
	logEvent("server_start", map[string]any{
		"args": os.Args, "cwd": cwd, "exe": exe, "ppid": os.Getppid(), "parent": parentInfo(),
		"env": vals, "env_names": names, "stdin_mode": stdinInfo, "console": consoleInfo(),
	})

	sig := make(chan os.Signal, 4)
	signal.Notify(sig)
	go func() {
		for s := range sig {
			logEvent("signal", map[string]any{"signal": s.String()})
			if s == os.Interrupt || s.String() == "terminated" {
				logEvent("server_exit", map[string]any{"why": "signal " + s.String()})
				os.Exit(0)
			}
		}
	}()
	go func() {
		for range time.Tick(5 * time.Second) {
			logEvent("heartbeat", map[string]any{"ppid": os.Getppid()})
		}
	}()

	sc := bufio.NewScanner(os.Stdin)
	sc.Buffer(make([]byte, 1<<20), 16<<20)
	for sc.Scan() {
		recv := time.Now()
		line := sc.Bytes()
		var m rpcMsg
		if err := json.Unmarshal(line, &m); err != nil {
			logEvent("bad_json", map[string]any{"len": len(line)})
			continue
		}
		switch m.Method {
		case "initialize":
			var p map[string]any
			_ = json.Unmarshal(m.Params, &p)
			logEvent("initialize", map[string]any{"params": p})
			pv, _ := p["protocolVersion"].(string)
			if pv == "" {
				pv = "2025-06-18"
			}
			reply(m.ID, map[string]any{
				"protocolVersion": pv,
				"capabilities":    map[string]any{"tools": map[string]any{}},
				"serverInfo":      map[string]any{"name": "rp-spike-probe", "version": version},
			})
		case "notifications/initialized":
			logEvent("initialized", nil)
		case "ping":
			logEvent("ping", nil)
			reply(m.ID, map[string]any{})
		case "tools/list":
			logEvent("tools_list", nil)
			reply(m.ID, map[string]any{"tools": tools})
		case "tools/call":
			go handleCall(m, recv)
		default:
			var p any
			_ = json.Unmarshal(m.Params, &p)
			logEvent("other_method", map[string]any{"method": m.Method, "params": p, "has_id": len(m.ID) > 0})
			if len(m.ID) > 0 && m.Method != "" {
				replyErr(m.ID, -32601, "method not found")
			}
		}
	}
	logEvent("server_exit", map[string]any{"why": "stdin closed", "scan_err": fmt.Sprint(sc.Err())})
}

// Values are logged only for these top-level hook input keys. Everything else: type only.
var dumpValueAllow = map[string]bool{
	"hook_event_name": true, "session_id": true, "source": true, "model": true, "notification_type": true,
	"reason": true, "tool_name": true, "agent_id": true, "agent_type": true, "permission_mode": true,
	"trigger": true, "from_model": true, "to_model": true, "stop_hook_active": true, "prompt_id": true,
	"error_type": true, "is_interrupt": true,
}

func shape(prefix string, v any, out map[string]string, depth int) {
	switch t := v.(type) {
	case map[string]any:
		if prefix != "" {
			out[prefix] = "object"
		}
		if depth >= 2 {
			return
		}
		for k, c := range t {
			p := k
			if prefix != "" {
				p = prefix + "." + k
			}
			shape(p, c, out, depth+1)
		}
	case []any:
		out[prefix] = "array"
	case string:
		out[prefix] = "string"
	case float64:
		out[prefix] = "number"
	case bool:
		out[prefix] = "boolean"
	case nil:
		out[prefix] = "null"
	}
}

func runDump() {
	var in map[string]any
	dec := json.NewDecoder(os.Stdin)
	if err := dec.Decode(&in); err != nil {
		logEvent("dump_bad_input", map[string]any{"err": err.Error()})
		return
	}
	shapes := map[string]string{}
	shape("", in, shapes, 0)
	vals := map[string]any{}
	for k, v := range in {
		if dumpValueAllow[k] {
			vals[k] = v
		}
	}
	// tool_input / tool_response are content: record only which sub-keys exist (done by shape).
	ev, _ := in["hook_event_name"].(string)
	env, _ := envSnapshot()
	logEvent("hook_dump", map[string]any{"event": ev, "shape": shapes, "values": vals, "env": env, "label": strings.Join(os.Args[2:], " ")})
}

func main() {
	openLog()
	mode := "mcp"
	if len(os.Args) > 1 {
		mode = os.Args[1]
	}
	switch mode {
	case "dump":
		runDump()
	case "version":
		fmt.Println(version)
	default:
		runMCP()
	}
}
