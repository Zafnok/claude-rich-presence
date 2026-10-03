package mcp

import (
	"bytes"
	"encoding/json"
	"io"
	"sync"
	"unicode/utf8"
)

// JSON-RPC 2.0 error codes.
const (
	codeParseError     = -32700
	codeInvalidRequest = -32600
	codeMethodNotFound = -32601
	codeInvalidParams  = -32602
)

// nullID is the id of an error response to a message whose id could not be
// read.
var nullID = json.RawMessage("null")

// rpcError is the error member of a response.
type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// message is a decoded request or notification.
type message struct {
	// id is the request id exactly as the client wrote it. It is nil for a
	// notification, which must never be answered.
	id     json.RawMessage
	method string
	// params is nil or a JSON object.
	params json.RawMessage
}

// rejection is the error response owed to a line that is not a valid message.
type rejection struct {
	id  json.RawMessage
	err rpcError
}

func reject(id json.RawMessage, code int, text string) (message, *rejection) {
	return message{}, &rejection{id: id, err: rpcError{Code: code, Message: text}}
}

// decode reads one line. It returns either a message or the rejection to send
// back. A response from the client, which nothing here ever asks for, and a
// notification with unusable params both come back as a message with no id, so
// that they are dropped without a reply.
func decode(line []byte) (message, *rejection) {
	if !utf8.Valid(line) || !json.Valid(line) {
		return reject(nullID, codeParseError, "parse error")
	}
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(line, &envelope); err != nil {
		return reject(nullID, codeInvalidRequest, "message must be a JSON object")
	}

	id, hasID := envelope["id"]
	replyID := nullID
	if hasID && isID(id) {
		replyID = id
	}
	if version, _ := asString(envelope["jsonrpc"]); version != "2.0" {
		return reject(replyID, codeInvalidRequest, `jsonrpc must be "2.0"`)
	}
	if hasID && !isID(id) {
		return reject(nullID, codeInvalidRequest, "id must be a string or an integer")
	}

	rawMethod, hasMethod := envelope["method"]
	if !hasMethod {
		_, hasResult := envelope["result"]
		_, hasError := envelope["error"]
		if hasResult || hasError {
			return message{}, nil
		}
		return reject(replyID, codeInvalidRequest, "method is required")
	}
	method, ok := asString(rawMethod)
	if !ok || method == "" {
		return reject(replyID, codeInvalidRequest, "method must be a non-empty string")
	}

	params := envelope["params"]
	if string(params) == "null" {
		params = nil
	}
	if params != nil && params[0] != '{' {
		if !hasID {
			return message{}, nil
		}
		return reject(id, codeInvalidParams, "params must be an object")
	}
	// Without an id this is a notification, and id is nil.
	return message{id: id, method: method, params: params}, nil
}

// isID reports whether raw, a valid JSON value, is a string or an integer.
// Null, fractions and exponents are not ids in MCP.
func isID(raw json.RawMessage) bool {
	if raw[0] == '"' {
		return true
	}
	for _, c := range raw {
		if c != '-' && (c < '0' || c > '9') {
			return false
		}
	}
	return true
}

// asString returns the string in raw, a valid JSON value or nothing.
func asString(raw json.RawMessage) (string, bool) {
	if len(raw) == 0 || raw[0] != '"' {
		return "", false
	}
	var s string
	_ = json.Unmarshal(raw, &s)
	return s, true
}

// response is a result or an error sent to the client.
type response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

// writer sends responses. Each one reaches the stream whole, in a single
// write, whoever else is sending.
type writer struct {
	mu  sync.Mutex
	out io.Writer
}

func (w *writer) result(id json.RawMessage, result any) error {
	return w.send(response{JSONRPC: "2.0", ID: id, Result: result})
}

func (w *writer) fail(id json.RawMessage, e rpcError) error {
	return w.send(response{JSONRPC: "2.0", ID: id, Error: &e})
}

func (w *writer) send(r response) error {
	var line bytes.Buffer
	enc := json.NewEncoder(&line)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(r); err != nil {
		return err
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	_, err := w.out.Write(line.Bytes())
	return err
}
