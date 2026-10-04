package mcp_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"strings"
	"testing"
	"testing/iotest"

	"github.com/Zafnok/claude-rich-presence/internal/mcp"
)

const initLine = `{"jsonrpc":"2.0","id":"init","method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"test-client","version":"1.2.3"}}}` + "\n"

const pingLine = `{"jsonrpc":"2.0","id":"alive","method":"ping"}` + "\n"

// reply is a response as a client reads it.
type reply struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  json.RawMessage `json:"result"`
	Error   *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

// parse checks that output is nothing but whole response lines and returns
// them.
func parse(t *testing.T, output string) []reply {
	t.Helper()
	if output == "" {
		return nil
	}
	if !strings.HasSuffix(output, "\n") {
		t.Fatalf("output does not end with a line feed: %q", output)
	}
	var replies []reply
	for _, line := range strings.Split(strings.TrimSuffix(output, "\n"), "\n") {
		var fields map[string]json.RawMessage
		if err := json.Unmarshal([]byte(line), &fields); err != nil {
			t.Fatalf("output line is not a JSON object: %q", line)
		}
		_, hasResult := fields["result"]
		_, hasError := fields["error"]
		if string(fields["jsonrpc"]) != `"2.0"` || fields["id"] == nil || hasResult == hasError || len(fields) != 3 {
			t.Fatalf("output line is not a response: %q", line)
		}
		var r reply
		if err := json.Unmarshal([]byte(line), &r); err != nil {
			t.Fatalf("output line %q: %v", line, err)
		}
		replies = append(replies, r)
	}
	return replies
}

func serve(t *testing.T, input string, opts mcp.Options) []reply {
	t.Helper()
	var out bytes.Buffer
	if err := mcp.Serve(strings.NewReader(input), &out, opts); err != nil {
		t.Fatalf("Serve: %v", err)
	}
	return parse(t, out.String())
}

func newTool(t *testing.T, name string, handler mcp.Handler) mcp.Tool {
	t.Helper()
	tool, err := mcp.NewTool(name, "A test tool.", json.RawMessage(`{"type":"object"}`), handler)
	if err != nil {
		t.Fatal(err)
	}
	return tool
}

func withTools(tools ...mcp.Tool) mcp.Options {
	return mcp.Options{Initialize: func(mcp.ClientInfo) []mcp.Tool { return tools }}
}

func wantError(t *testing.T, r reply, id string, code int) {
	t.Helper()
	if r.Error == nil || r.Error.Code != code || string(r.ID) != id || r.Error.Message == "" {
		t.Errorf("got id %s, error %+v; want id %s, code %d and a message", r.ID, r.Error, id, code)
	}
}

func wantResult(t *testing.T, r reply, id, result string) {
	t.Helper()
	if r.Error != nil || string(r.ID) != id || string(r.Result) != result {
		t.Errorf("got id %s, result %s, error %+v; want id %s, result %s", r.ID, r.Result, r.Error, id, result)
	}
}

func TestGoldenSession(t *testing.T) {
	input, err := os.ReadFile("testdata/session.input")
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile("testdata/session.golden")
	if err != nil {
		t.Fatal(err)
	}
	echo, err := mcp.NewTool("echo", "Returns its arguments.",
		json.RawMessage(`{ "type": "object", "properties": { "text": { "type": "string" } }, "required": ["text"] }`),
		func(arguments json.RawMessage) mcp.Result { return mcp.Result{Text: string(arguments)} })
	if err != nil {
		t.Fatal(err)
	}
	fail, err := mcp.NewTool("fail", "Always fails.", json.RawMessage(`{"type":"object","additionalProperties":false}`),
		func(json.RawMessage) mcp.Result { return mcp.Result{Text: "it failed", IsError: true} })
	if err != nil {
		t.Fatal(err)
	}
	var client mcp.ClientInfo
	shutdowns := 0
	var out bytes.Buffer
	err = mcp.Serve(bytes.NewReader(input), &out, mcp.Options{
		Name:    "golden-server",
		Version: "0.0.1",
		Initialize: func(c mcp.ClientInfo) []mcp.Tool {
			client = c
			return []mcp.Tool{echo, fail}
		},
		Shutdown: func() { shutdowns++ },
	})
	if err != nil {
		t.Fatalf("Serve: %v", err)
	}
	if out.String() != string(want) {
		t.Errorf("output differs from testdata/session.golden\ngot:\n%s\nwant:\n%s", out.String(), want)
	}
	if client != (mcp.ClientInfo{Name: "golden-client", Version: "1.2.3"}) {
		t.Errorf("client = %+v", client)
	}
	if shutdowns != 1 {
		t.Errorf("shutdown ran %d times, want 1", shutdowns)
	}
}

func TestBeforeInitialize(t *testing.T) {
	calls := 0
	opts := withTools(newTool(t, "event", func(json.RawMessage) mcp.Result {
		calls++
		return mcp.Result{}
	}))
	input := `{"jsonrpc":"2.0","id":1,"method":"tools/list"}
{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"event"}}
{"jsonrpc":"2.0","id":3,"method":"ping"}
{"jsonrpc":"2.0","id":4,"method":"resources/list"}
{"jsonrpc":"2.0","method":"notifications/initialized"}
{"jsonrpc":"2.0","id":5,"method":"tools/list"}
` + initLine + `{"jsonrpc":"2.0","id":6,"method":"tools/call","params":{"name":"event"}}
`
	got := serve(t, input, opts)
	if len(got) != 7 {
		t.Fatalf("got %d replies, want 7", len(got))
	}
	wantError(t, got[0], "1", -32600)
	wantError(t, got[1], "2", -32600)
	wantResult(t, got[2], "3", "{}")
	wantError(t, got[3], "4", -32601)
	wantError(t, got[4], "5", -32600)
	if got[5].Error != nil {
		t.Errorf("initialize failed: %+v", got[5].Error)
	}
	wantResult(t, got[6], "6", `{"content":[{"type":"text","text":""}]}`)
	if calls != 1 {
		t.Errorf("the tool ran %d times, want once, after initialize", calls)
	}
}

func TestErrors(t *testing.T) {
	cases := []struct {
		name string
		line string
		id   string
		code int
	}{
		{"not JSON", `{"jsonrpc":"2.0","id":1,`, "null", -32700},
		{"not JSON at all", `hello`, "null", -32700},
		{"two values", `{"jsonrpc":"2.0","id":1,"method":"ping"} {}`, "null", -32700},
		{"not UTF-8", "{\"jsonrpc\":\"2.0\",\"id\":\"\xff\",\"method\":\"ping\"}", "null", -32700},
		{"batch", `[{"jsonrpc":"2.0","id":1,"method":"ping"}]`, "null", -32600},
		{"scalar", `7`, "null", -32600},
		{"null", `null`, "null", -32600},
		{"no jsonrpc", `{"id":1,"method":"ping"}`, "1", -32600},
		{"wrong jsonrpc", `{"jsonrpc":"1.0","id":"a","method":"ping"}`, `"a"`, -32600},
		{"jsonrpc not a string", `{"jsonrpc":2.0,"id":1,"method":"ping"}`, "1", -32600},
		{"wrong jsonrpc and bad id", `{"jsonrpc":"1.0","id":{},"method":"ping"}`, "null", -32600},
		{"wrong jsonrpc and no id", `{"jsonrpc":"1.0","method":"ping"}`, "null", -32600},
		{"null id", `{"jsonrpc":"2.0","id":null,"method":"ping"}`, "null", -32600},
		{"fractional id", `{"jsonrpc":"2.0","id":1.5,"method":"ping"}`, "null", -32600},
		{"exponent id", `{"jsonrpc":"2.0","id":1e3,"method":"ping"}`, "null", -32600},
		{"boolean id", `{"jsonrpc":"2.0","id":true,"method":"ping"}`, "null", -32600},
		{"array id", `{"jsonrpc":"2.0","id":[1],"method":"ping"}`, "null", -32600},
		{"no method", `{"jsonrpc":"2.0","id":1}`, "1", -32600},
		{"no method and no id", `{"jsonrpc":"2.0"}`, "null", -32600},
		{"method not a string", `{"jsonrpc":"2.0","id":1,"method":5}`, "1", -32600},
		{"null method", `{"jsonrpc":"2.0","id":1,"method":null}`, "1", -32600},
		{"empty method", `{"jsonrpc":"2.0","id":1,"method":""}`, "1", -32600},
		{"second initialize", strings.TrimSuffix(initLine, "\n"), `"init"`, -32600},
		{"unknown method", `{"jsonrpc":"2.0","id":1,"method":"resources/list"}`, "1", -32601},
		{"notification name as a request", `{"jsonrpc":"2.0","id":1,"method":"notifications/initialized"}`, "1", -32601},
		{"method names are case-sensitive", `{"jsonrpc":"2.0","id":1,"method":"Ping"}`, "1", -32601},
		{"array params", `{"jsonrpc":"2.0","id":1,"method":"ping","params":[]}`, "1", -32602},
		{"string params", `{"jsonrpc":"2.0","id":1,"method":"tools/list","params":"x"}`, "1", -32602},
		{"cursor", `{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{"cursor":"abc"}}`, "1", -32602},
		{"cursor of the wrong type", `{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{"cursor":5}}`, "1", -32602},
		{"call without params", `{"jsonrpc":"2.0","id":1,"method":"tools/call"}`, "1", -32602},
		{"call with null params", `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":null}`, "1", -32602},
		{"call without a name", `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{}}`, "1", -32602},
		{"call with a name of the wrong type", `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":5}}`, "1", -32602},
		{"call with an unknown tool", `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"nope"}}`, "1", -32602},
		{"call with array arguments", `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"event","arguments":[1]}}`, "1", -32602},
		{"call with string arguments", `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"event","arguments":"x"}}`, "1", -32602},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			calls := 0
			opts := withTools(newTool(t, "event", func(json.RawMessage) mcp.Result {
				calls++
				return mcp.Result{Text: "{}"}
			}))
			got := serve(t, initLine+c.line+"\n"+pingLine, opts)
			if len(got) != 3 {
				t.Fatalf("got %d replies, want 3", len(got))
			}
			wantError(t, got[1], c.id, c.code)
			wantResult(t, got[2], `"alive"`, "{}")
			if calls != 0 {
				t.Errorf("the tool ran %d times", calls)
			}
		})
	}
}

func TestInitializeErrors(t *testing.T) {
	cases := map[string]string{
		"no params":              `{"jsonrpc":"2.0","id":1,"method":"initialize"}`,
		"no protocol version":    `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"clientInfo":{"name":"c","version":"1"}}}`,
		"version of wrong type":  `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":20251125,"clientInfo":{"name":"c","version":"1"}}}`,
		"no client info":         `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25"}}`,
		"no client name":         `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","clientInfo":{"version":"1"}}}`,
		"no client version":      `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","clientInfo":{"name":"c"}}}`,
		"client info wrong type": `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","clientInfo":"c"}}`,
	}
	for name, line := range cases {
		t.Run(name, func(t *testing.T) {
			initialized := 0
			opts := mcp.Options{Initialize: func(mcp.ClientInfo) []mcp.Tool {
				initialized++
				return nil
			}}
			// A failed initialize leaves the server waiting for a good one.
			got := serve(t, line+"\n"+`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`+"\n"+initLine, opts)
			if len(got) != 3 {
				t.Fatalf("got %d replies, want 3", len(got))
			}
			wantError(t, got[0], "1", -32602)
			wantError(t, got[1], "2", -32600)
			if got[2].Error != nil || initialized != 1 {
				t.Errorf("later initialize: error %+v, callback ran %d times", got[2].Error, initialized)
			}
		})
	}
}

func TestNotificationsAreNeverAnswered(t *testing.T) {
	calls := 0
	opts := withTools(newTool(t, "event", func(json.RawMessage) mcp.Result {
		calls++
		return mcp.Result{}
	}))
	lines := []string{
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		`{"jsonrpc":"2.0","method":"notifications/cancelled","params":{"requestId":"init","reason":"changed my mind"}}`,
		`{"jsonrpc":"2.0","method":"notifications/cancelled","params":"malformed"}`,
		`{"jsonrpc":"2.0","method":"notifications/roots/list_changed"}`,
		`{"jsonrpc":"2.0","method":"notifications/unheard-of","params":{"a":1}}`,
		`{"jsonrpc":"2.0","method":"ping"}`,
		`{"jsonrpc":"2.0","method":"tools/list"}`,
		`{"jsonrpc":"2.0","method":"tools/call","params":{"name":"event"}}`,
		`{"jsonrpc":"2.0","method":"tools/call","params":{"name":"nope"}}`,
		`{"jsonrpc":"2.0","method":"initialize"}`,
		// Responses, which this server never asks for.
		`{"jsonrpc":"2.0","id":9,"result":{}}`,
		`{"jsonrpc":"2.0","id":9,"error":{"code":-32601,"message":"no"}}`,
	}
	for _, prefix := range []string{"", initLine} {
		input := prefix + strings.Join(lines, "\n") + "\n"
		got := serve(t, input, opts)
		if want := strings.Count(prefix, "\n"); len(got) != want {
			t.Errorf("got %d replies, want %d", len(got), want)
		}
	}
	if calls != 0 {
		t.Errorf("a notification ran the tool %d times", calls)
	}
}

func TestIDsAreEchoedExactly(t *testing.T) {
	ids := []string{
		`0`, `1`, `-7`, `123456789012345678901234567890`,
		`"abc"`, `""`, `"1"`, `"with \"quotes\" and \\ and é and é"`, `"null"`,
	}
	for _, id := range ids {
		input := initLine +
			`{"jsonrpc":"2.0", "id" : ` + id + ` ,"method":"ping"}` + "\n" +
			`{"jsonrpc":"2.0","id":` + id + `,"method":"nope"}` + "\n"
		got := serve(t, input, mcp.Options{})
		if len(got) != 3 {
			t.Fatalf("id %s: got %d replies, want 3", id, len(got))
		}
		wantResult(t, got[1], id, "{}")
		wantError(t, got[2], id, -32601)
	}
}

func TestVersionNegotiation(t *testing.T) {
	cases := map[string]string{
		"2025-11-25": "2025-11-25",
		"2025-06-18": "2025-06-18",
		"2025-03-26": "2025-11-25",
		"2024-11-05": "2025-11-25",
		"2026-07-28": "2025-11-25",
		"1.0.0":      "2025-11-25",
	}
	for requested, want := range cases {
		input := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"` + requested + `","clientInfo":{"name":"c","version":"1"}}}` + "\n"
		got := serve(t, input, mcp.Options{Name: "rich-presence", Version: "9.9.9"})
		if len(got) != 1 {
			t.Fatalf("%s: got %d replies, want 1", requested, len(got))
		}
		wantResult(t, got[0], "1", `{"protocolVersion":"`+want+`","capabilities":{"tools":{}},"serverInfo":{"name":"rich-presence","version":"9.9.9"}}`)
	}
	if mcp.LatestProtocolVersion != "2025-11-25" {
		t.Errorf("LatestProtocolVersion = %s", mcp.LatestProtocolVersion)
	}
}

func TestToolsDependOnTheClient(t *testing.T) {
	opts := mcp.Options{Initialize: func(c mcp.ClientInfo) []mcp.Tool {
		if c.Name == "claude-code" {
			return []mcp.Tool{newTool(t, "event", func(json.RawMessage) mcp.Result { return mcp.Result{} })}
		}
		return nil
	}}
	for client, want := range map[string]string{
		"claude-code": `{"tools":[{"name":"event","description":"A test tool.","inputSchema":{"type":"object"}}]}`,
		"claude-ai":   `{"tools":[]}`,
	} {
		input := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","clientInfo":{"name":"` + client + `","version":"1"}}}` + "\n" +
			`{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}` + "\n"
		got := serve(t, input, opts)
		if len(got) != 2 {
			t.Fatalf("%s: got %d replies, want 2", client, len(got))
		}
		wantResult(t, got[1], "2", want)
	}
}

func TestNoInitializeCallbackMeansNoTools(t *testing.T) {
	got := serve(t, initLine+
		`{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{"cursor":null}}`+"\n"+
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"event"}}`+"\n", mcp.Options{})
	if len(got) != 3 {
		t.Fatalf("got %d replies, want 3", len(got))
	}
	wantResult(t, got[1], "2", `{"tools":[]}`)
	wantError(t, got[2], "3", -32602)
}

func TestToolCallArguments(t *testing.T) {
	cases := []struct {
		name   string
		params string
		want   string
	}{
		{"object", `{"name":"event","arguments":{"a":[1,2],"b":"x"}}`, `{"a":[1,2],"b":"x"}`},
		{"empty object", `{"name":"event","arguments":{}}`, `{}`},
		{"absent", `{"name":"event"}`, `absent`},
		{"null", `{"name":"event","arguments":null}`, `absent`},
		{"with meta", `{"_meta":{"claudecode/toolUseId":"t1"},"name":"event","arguments":{"a":1}}`, `{"a":1}`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var seen []string
			opts := withTools(
				newTool(t, "other", func(json.RawMessage) mcp.Result { return mcp.Result{Text: "wrong tool"} }),
				newTool(t, "event", func(arguments json.RawMessage) mcp.Result {
					if arguments == nil {
						seen = append(seen, "absent")
					} else {
						seen = append(seen, string(arguments))
					}
					return mcp.Result{Text: "{}"}
				}),
			)
			got := serve(t, initLine+`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":`+c.params+`}`+"\n", opts)
			if len(got) != 2 {
				t.Fatalf("got %d replies, want 2", len(got))
			}
			wantResult(t, got[1], "1", `{"content":[{"type":"text","text":"{}"}]}`)
			if len(seen) != 1 || seen[0] != c.want {
				t.Errorf("handler saw %q, want [%q]", seen, c.want)
			}
		})
	}
}

func TestToolCallMeta(t *testing.T) {
	cases := []struct {
		name   string
		params string
		want   string
	}{
		{"object", `{"name":"event","_meta":{"claudecode/toolUseId":"t1"},"arguments":{"a":1}}`, `{"claudecode/toolUseId":"t1"}`},
		{"empty object", `{"name":"event","_meta":{}}`, `{}`},
		{"absent", `{"name":"event","arguments":{"a":1}}`, `absent`},
		{"null", `{"name":"event","_meta":null}`, `absent`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var seen []string
			tool, err := mcp.NewMetaTool("event", "d", json.RawMessage(`{"type":"object"}`), func(_, meta json.RawMessage) mcp.Result {
				if meta == nil {
					seen = append(seen, "absent")
				} else {
					seen = append(seen, string(meta))
				}
				return mcp.Result{Text: "{}"}
			})
			if err != nil {
				t.Fatal(err)
			}
			got := serve(t, initLine+`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":`+c.params+`}`+"\n", withTools(tool))
			if len(got) != 2 {
				t.Fatalf("got %d replies, want 2", len(got))
			}
			wantResult(t, got[1], "1", `{"content":[{"type":"text","text":"{}"}]}`)
			if len(seen) != 1 || seen[0] != c.want {
				t.Errorf("handler saw %q, want [%q]", seen, c.want)
			}
		})
	}
}

func TestToolCallMetaMustBeAnObject(t *testing.T) {
	called := false
	opts := withTools(newTool(t, "event", func(json.RawMessage) mcp.Result {
		called = true
		return mcp.Result{}
	}))
	got := serve(t, initLine+`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"event","_meta":"x"}}`+"\n", opts)
	if len(got) != 2 {
		t.Fatalf("got %d replies, want 2", len(got))
	}
	wantError(t, got[1], "1", -32602)
	if called {
		t.Error("the handler ran")
	}
}

func TestNewMetaToolNeedsAHandler(t *testing.T) {
	if _, err := mcp.NewMetaTool("t", "d", json.RawMessage(`{"type":"object"}`), nil); err == nil {
		t.Error("NewMetaTool accepted a nil handler")
	}
}

func TestHandlerPanicIsAToolError(t *testing.T) {
	opts := withTools(
		newTool(t, "explode", func(arguments json.RawMessage) mcp.Result { panic("secret " + string(arguments)) }),
		newTool(t, "out-of-range", func(arguments json.RawMessage) mcp.Result {
			return mcp.Result{Text: string(arguments[:1])}
		}),
	)
	input := initLine +
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"explode","arguments":{"k":"private"}}}` + "\n" +
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"out-of-range"}}` + "\n" +
		pingLine
	got := serve(t, input, opts)
	if len(got) != 4 {
		t.Fatalf("got %d replies, want 4", len(got))
	}
	const failed = `{"content":[{"type":"text","text":"tool failed"}],"isError":true}`
	wantResult(t, got[1], "1", failed)
	wantResult(t, got[2], "2", failed)
	wantResult(t, got[3], `"alive"`, "{}")
}

func TestLineLengthLimit(t *testing.T) {
	const limit = 64
	pad := func(n int) string {
		const head, tail = `{"jsonrpc":"2.0","id":1,"method":"ping"`, `}`
		return head + strings.Repeat(" ", n-len(head)-len(tail)) + tail
	}
	cases := []struct {
		name  string
		input string
		want  []int // 0 for a result, otherwise the error code
	}{
		{"at the limit", pad(limit) + "\n" + pingLine, []int{0, 0}},
		{"at the limit with a carriage return", pad(limit-1) + "\r\n" + pingLine, []int{0, 0}},
		{"at the limit at end of input", pad(limit), []int{0}},
		{"one over", pad(limit+1) + "\n" + pingLine, []int{-32600, 0}},
		{"one over at end of input", pingLine + pad(limit+1), []int{0, -32600}},
		{"many times over", pad(limit*10+3) + "\n" + pingLine, []int{-32600, 0}},
		{"exactly two buffers", pad(2*(limit+1)-1) + "\n" + pingLine, []int{-32600, 0}},
		{"two long lines", pad(200) + "\n" + pad(300) + "\n" + pingLine, []int{-32600, -32600, 0}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := serve(t, c.input, mcp.Options{MaxLineBytes: limit})
			if len(got) != len(c.want) {
				t.Fatalf("got %d replies, want %d", len(got), len(c.want))
			}
			for i, code := range c.want {
				if code == 0 {
					if got[i].Error != nil {
						t.Errorf("reply %d: unexpected error %+v", i, got[i].Error)
					}
					continue
				}
				wantError(t, got[i], "null", code)
			}
		})
	}
}

func TestDefaultLineLengthLimit(t *testing.T) {
	long := `{"jsonrpc":"2.0","id":1,"method":"ping","params":{"pad":"` + strings.Repeat("x", mcp.DefaultMaxLineBytes) + `"}}`
	fits := `{"jsonrpc":"2.0","id":2,"method":"ping","params":{"pad":"` + strings.Repeat("x", mcp.DefaultMaxLineBytes-100) + `"}}`
	for _, limit := range []int{0, -1} {
		got := serve(t, long+"\n"+fits+"\n", mcp.Options{MaxLineBytes: limit})
		if len(got) != 2 {
			t.Fatalf("got %d replies, want 2", len(got))
		}
		wantError(t, got[0], "null", -32600)
		wantResult(t, got[1], "2", "{}")
	}
}

func TestLineEndingsAndBlankLines(t *testing.T) {
	input := "\n  \n\r\n" +
		`{"jsonrpc":"2.0","id":1,"method":"ping"}` + "\r\n" +
		"\n" +
		"\t" + `{"jsonrpc":"2.0","id":2,"method":"ping"} ` + "\n" +
		`{"jsonrpc":"2.0","id":3,"method":"ping"}`
	got := serve(t, input, mcp.Options{})
	if len(got) != 3 {
		t.Fatalf("got %d replies, want 3", len(got))
	}
	for i, id := range []string{"1", "2", "3"} {
		wantResult(t, got[i], id, "{}")
	}
}

func TestEmptyInput(t *testing.T) {
	shutdowns := 0
	var out bytes.Buffer
	if err := mcp.Serve(strings.NewReader(""), &out, mcp.Options{Shutdown: func() { shutdowns++ }}); err != nil {
		t.Fatal(err)
	}
	if out.Len() != 0 || shutdowns != 1 {
		t.Errorf("wrote %q, shutdown ran %d times", out.String(), shutdowns)
	}
}

func TestReadError(t *testing.T) {
	broken := errors.New("pipe broke")
	in := io.MultiReader(strings.NewReader(pingLine+`{"jsonrpc":"2.0","id":"partial","method":"ping"}`), iotest.ErrReader(broken))
	shutdowns := 0
	var out bytes.Buffer
	err := mcp.Serve(in, &out, mcp.Options{Shutdown: func() { shutdowns++ }})
	if !errors.Is(err, broken) {
		t.Errorf("Serve returned %v, want the read error", err)
	}
	if shutdowns != 1 {
		t.Errorf("shutdown ran %d times, want 1", shutdowns)
	}
	// The line that was cut short is still answered: it was a whole message.
	got := parse(t, out.String())
	if len(got) != 2 {
		t.Fatalf("got %d replies, want 2", len(got))
	}
	wantResult(t, got[1], `"partial"`, "{}")
}

type failingWriter struct {
	err    error
	writes int
}

func (w *failingWriter) Write([]byte) (int, error) {
	w.writes++
	return 0, w.err
}

func TestWriteErrorStopsTheServer(t *testing.T) {
	gone := errors.New("client gone")
	inputs := map[string]string{
		"result":         pingLine + pingLine,
		"error":          `{"jsonrpc":"2.0","id":1,"method":"nope"}` + "\n" + pingLine,
		"rejected line":  "garbage\n" + pingLine,
		"line too long":  strings.Repeat("x", 5000) + "\n" + pingLine,
		"at end of file": strings.TrimSuffix(pingLine, "\n"),
	}
	for name, input := range inputs {
		t.Run(name, func(t *testing.T) {
			out := &failingWriter{err: gone}
			shutdowns := 0
			err := mcp.Serve(strings.NewReader(input), out, mcp.Options{MaxLineBytes: 100, Shutdown: func() { shutdowns++ }})
			if !errors.Is(err, gone) {
				t.Errorf("Serve returned %v, want the write error", err)
			}
			if out.writes != 1 || shutdowns != 1 {
				t.Errorf("%d writes and %d shutdowns, want 1 and 1", out.writes, shutdowns)
			}
		})
	}
}

// countingWriter records each call to Write.
type countingWriter struct{ writes []string }

func (w *countingWriter) Write(p []byte) (int, error) {
	w.writes = append(w.writes, string(p))
	return len(p), nil
}

func TestEachResponseIsOneWrite(t *testing.T) {
	out := &countingWriter{}
	input := initLine + pingLine + "garbage\n" + `{"jsonrpc":"2.0","id":1,"method":"tools/list"}` + "\n"
	if err := mcp.Serve(strings.NewReader(input), out, mcp.Options{}); err != nil {
		t.Fatal(err)
	}
	if len(out.writes) != 4 {
		t.Fatalf("got %d writes, want 4", len(out.writes))
	}
	for _, w := range out.writes {
		if strings.Count(w, "\n") != 1 || !strings.HasSuffix(w, "\n") {
			t.Errorf("write is not one whole line: %q", w)
		}
		parse(t, w)
	}
}

func TestResultTextNeverBreaksTheLine(t *testing.T) {
	const text = "line one\nline two\r\n <b>&amp;</b>\x00 \"quoted\" \\ é"
	opts := withTools(newTool(t, "event", func(json.RawMessage) mcp.Result { return mcp.Result{Text: text} }))
	got := serve(t, initLine+`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"event"}}`+"\n", opts)
	if len(got) != 2 {
		t.Fatalf("got %d replies, want 2", len(got))
	}
	var result struct {
		Content []struct{ Type, Text string }
	}
	if err := json.Unmarshal(got[1].Result, &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Content) != 1 || result.Content[0].Type != "text" || result.Content[0].Text != text {
		t.Errorf("content = %+v", result.Content)
	}
}

func TestNewTool(t *testing.T) {
	handler := func(json.RawMessage) mcp.Result { return mcp.Result{} }
	schema := json.RawMessage(`{"type":"object"}`)
	cases := []struct {
		name    string
		tool    string
		schema  json.RawMessage
		handler mcp.Handler
		ok      bool
	}{
		{"plain", "presence_event", schema, handler, true},
		{"every allowed character", "aZ09_-.", schema, handler, true},
		{"longest name", strings.Repeat("n", 128), schema, handler, true},
		{"schema with spaces", "t", json.RawMessage(" {\n \"type\": \"object\" } "), handler, true},
		{"empty name", "", schema, handler, false},
		{"name too long", strings.Repeat("n", 129), schema, handler, false},
		{"name with a space", "a b", schema, handler, false},
		{"name with a slash", "a/b", schema, handler, false},
		{"name with a non-ASCII letter", "é", schema, handler, false},
		{"name below each range", "a`", schema, handler, false},
		{"name with @", "a@", schema, handler, false},
		{"name with a brace", "a{", schema, handler, false},
		{"name with a colon", "a:", schema, handler, false},
		{"nil schema", "t", nil, handler, false},
		{"blank schema", "t", json.RawMessage("  "), handler, false},
		{"schema not JSON", "t", json.RawMessage(`{"type":`), handler, false},
		{"schema null", "t", json.RawMessage(`null`), handler, false},
		{"schema an array", "t", json.RawMessage(`[]`), handler, false},
		{"schema true", "t", json.RawMessage(`true`), handler, false},
		{"nil handler", "t", schema, nil, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := mcp.NewTool(c.tool, "d", c.schema, c.handler)
			if (err == nil) != c.ok {
				t.Errorf("NewTool error = %v, want ok %v", err, c.ok)
			}
		})
	}
}
