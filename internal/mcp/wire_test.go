package mcp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"
)

func TestDecode(t *testing.T) {
	cases := []struct {
		name string
		line string
		want message
	}{
		{"request", `{"jsonrpc":"2.0","id":1,"method":"ping"}`, message{id: json.RawMessage(`1`), method: "ping"}},
		{"string id", `{"jsonrpc":"2.0","id":"aA","method":"ping"}`, message{id: json.RawMessage(`"aA"`), method: "ping"}},
		{"params", `{"jsonrpc":"2.0","id":1,"method":"m","params": {"a": 1} }`, message{id: json.RawMessage(`1`), method: "m", params: json.RawMessage(`{"a": 1}`)}},
		{"null params", `{"jsonrpc":"2.0","id":1,"method":"m","params":null}`, message{id: json.RawMessage(`1`), method: "m"}},
		{"escaped strings", `{"jsonrpc":"2.0","id":1,"method":"ping"}`, message{id: json.RawMessage(`1`), method: "ping"}},
		{"notification", `{"jsonrpc":"2.0","method":"notifications/initialized"}`, message{method: "notifications/initialized"}},
		{"notification with params", `{"jsonrpc":"2.0","method":"n","params":{}}`, message{method: "n", params: json.RawMessage(`{}`)}},
		{"notification with bad params", `{"jsonrpc":"2.0","method":"n","params":[1]}`, message{}},
		{"result response", `{"jsonrpc":"2.0","id":1,"result":{}}`, message{}},
		{"error response", `{"jsonrpc":"2.0","id":1,"error":{"code":1,"message":"m"}}`, message{}},
		{"keys are case-sensitive", `{"jsonrpc":"2.0","ID":1,"method":"n"}`, message{method: "n"}},
		{"unknown members", `{"jsonrpc":"2.0","id":1,"method":"m","extra":true}`, message{id: json.RawMessage(`1`), method: "m"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, rejected := decode([]byte(c.line))
			if rejected != nil {
				t.Fatalf("rejected: %+v", rejected)
			}
			if string(got.id) != string(c.want.id) || (got.id == nil) != (c.want.id == nil) ||
				got.method != c.want.method || string(got.params) != string(c.want.params) {
				t.Errorf("got %+v, want %+v", got, c.want)
			}
		})
	}
}

// checkDecoded states what decode promises about any input at all.
func checkDecoded(t *testing.T, line []byte, msg message, rejected *rejection) {
	t.Helper()
	if rejected != nil {
		if !json.Valid(rejected.id) || !isID(rejected.id) && string(rejected.id) != "null" {
			t.Errorf("%q: rejection id %q cannot be sent", line, rejected.id)
		}
		if rejected.err.Code > -32600 || rejected.err.Code < -32700 || rejected.err.Message == "" {
			t.Errorf("%q: rejection %+v", line, rejected.err)
		}
		if msg.id != nil || msg.method != "" || msg.params != nil {
			t.Errorf("%q: both a message and a rejection", line)
		}
		return
	}
	if msg.id != nil && (!json.Valid(msg.id) || !isID(msg.id) || msg.method == "") {
		t.Errorf("%q: request %+v", line, msg)
	}
	if msg.params != nil && (!json.Valid(msg.params) || msg.params[0] != '{') {
		t.Errorf("%q: params %q", line, msg.params)
	}
}

func FuzzDecode(f *testing.F) {
	seeds := []string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"c","version":"1"}}}`,
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		`{"jsonrpc":"2.0","method":"notifications/cancelled","params":{"requestId":"1"}}`,
		`{"jsonrpc":"2.0","id":"s","method":"tools/call","params":{"name":"t","arguments":{}}}`,
		`{"jsonrpc":"2.0","id":-1,"method":"tools/list","params":null}`,
		`{"jsonrpc":"2.0","id":1.5,"method":"ping"}`,
		`{"jsonrpc":"2.0","id":null,"method":"ping"}`,
		`{"jsonrpc":"2.0","id":1,"result":{}}`,
		`{"jsonrpc":"1.0","id":[],"method":5,"params":[]}`,
		`[{"jsonrpc":"2.0","id":1,"method":"ping"}]`,
		`{"id":"\ud800"}`,
		"{\"jsonrpc\":\"2.0\",\"id\":\"\xff\"}",
		`null`, `0`, `""`, `{`, ``, ` `,
	}
	for _, s := range seeds {
		f.Add([]byte(s))
	}
	f.Fuzz(func(t *testing.T, line []byte) {
		msg, rejected := decode(line)
		checkDecoded(t, line, msg, rejected)
	})
}

// FuzzServe feeds arbitrary bytes to a whole server. Whatever arrives, the
// server finishes, and everything it wrote is a response on a line of its own.
func FuzzServe(f *testing.F) {
	const initialize = `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","clientInfo":{"name":"c","version":"1"}}}` + "\n"
	f.Add([]byte(initialize + `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"t","arguments":{"a":1}}}` + "\n"))
	f.Add([]byte(initialize + `{"jsonrpc":"2.0","id":2,"method":"tools/list"}` + "\nnot json\n\n"))
	f.Add([]byte(strings.Repeat("x", 300) + "\n" + `{"jsonrpc":"2.0","id":"p","method":"ping"}`))
	f.Fuzz(func(t *testing.T, input []byte) {
		tool, err := NewTool("t", "d", json.RawMessage(`{"type":"object"}`), func(arguments json.RawMessage) Result {
			return Result{Text: string(arguments)}
		})
		if err != nil {
			t.Fatal(err)
		}
		var out bytes.Buffer
		opts := Options{MaxLineBytes: 200, Initialize: func(ClientInfo) []Tool { return []Tool{tool} }}
		if err := Serve(bytes.NewReader(input), &out, opts); err != nil {
			t.Fatalf("Serve: %v", err)
		}
		for _, line := range strings.SplitAfter(out.String(), "\n") {
			if line == "" {
				continue
			}
			var r struct {
				JSONRPC string          `json:"jsonrpc"`
				ID      json.RawMessage `json:"id"`
			}
			if !strings.HasSuffix(line, "\n") || json.Unmarshal([]byte(line), &r) != nil || r.JSONRPC != "2.0" || r.ID == nil {
				t.Fatalf("output line is not a response: %q", line)
			}
		}
	})
}

// TestConcurrentResponsesAreWhole sends from many goroutines at once. The
// race detector fails this test if two sends can reach the stream together.
func TestConcurrentResponsesAreWhole(t *testing.T) {
	const senders, each = 16, 50
	var out bytes.Buffer
	w := &writer{out: &out}
	text := strings.Repeat("payload ", 200)
	var wg sync.WaitGroup
	for s := range senders {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range each {
				id := json.RawMessage(fmt.Sprintf(`"%d-%d"`, s, i))
				var err error
				if i%2 == 0 {
					err = w.result(id, callResult{Content: []textContent{{Type: "text", Text: text}}})
				} else {
					err = w.fail(id, rpcError{Code: codeInvalidParams, Message: text})
				}
				if err != nil {
					t.Error(err)
				}
			}
		}()
	}
	wg.Wait()

	seen := map[string]bool{}
	lines := strings.Split(strings.TrimSuffix(out.String(), "\n"), "\n")
	for _, line := range lines {
		var r struct {
			ID     string
			Result *callResult
			Error  *rpcError
		}
		if err := json.Unmarshal([]byte(line), &r); err != nil {
			t.Fatalf("a line is not whole: %v", err)
		}
		if (r.Result == nil) == (r.Error == nil) {
			t.Fatalf("line %q is not one response", r.ID)
		}
		if r.Result != nil && r.Result.Content[0].Text != text || r.Error != nil && r.Error.Message != text {
			t.Fatalf("line %q was damaged", r.ID)
		}
		seen[r.ID] = true
	}
	if len(lines) != senders*each || len(seen) != senders*each {
		t.Errorf("got %d lines with %d distinct ids, want %d", len(lines), len(seen), senders*each)
	}
}

func TestSendWritesNothingItCannotEncode(t *testing.T) {
	var out bytes.Buffer
	w := &writer{out: &out}
	if err := w.result(json.RawMessage(`1`), make(chan int)); err == nil {
		t.Error("no error for a result that cannot be encoded")
	}
	if err := w.result(json.RawMessage(`not an id`), struct{}{}); err == nil {
		t.Error("no error for an id that is not JSON")
	}
	if out.Len() != 0 {
		t.Errorf("wrote %q", out.String())
	}
}
