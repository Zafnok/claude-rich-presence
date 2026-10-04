package mcp

import (
	"bytes"
	"encoding/json"
	"errors"
)

// Handler runs a tool. arguments is the JSON object the client sent, or nil
// when it sent none. A handler is called on the goroutine that reads the
// input, so it must return at once and must not perform I/O (ADR-0008).
type Handler func(arguments json.RawMessage) Result

// MetaHandler is a Handler that is also given the "_meta" object of the call,
// or nil when the call had none. A client may use _meta to say where a call
// came from. The same rules apply as for a Handler.
type MetaHandler func(arguments, meta json.RawMessage) Result

// Result is what a tool call returns: one item of text.
type Result struct {
	Text string
	// IsError marks a failure of the tool itself, which the client may show
	// to the model.
	IsError bool
}

// Tool is a tool the server exposes. Make one with NewTool.
type Tool struct {
	name        string
	description string
	inputSchema json.RawMessage
	handler     MetaHandler
}

// NewTool checks and builds a tool. The name is 1 to 128 characters from
// A-Z, a-z, 0-9, underscore, hyphen and dot. inputSchema is a JSON Schema
// object; the server publishes it and does not validate arguments against it.
func NewTool(name, description string, inputSchema json.RawMessage, handler Handler) (Tool, error) {
	if handler == nil {
		return Tool{}, errNilHandler
	}
	return NewMetaTool(name, description, inputSchema, func(arguments, _ json.RawMessage) Result {
		return handler(arguments)
	})
}

var errNilHandler = errors.New("mcp: tool handler is nil")

// NewMetaTool is NewTool for a handler that reads the call's _meta.
func NewMetaTool(name, description string, inputSchema json.RawMessage, handler MetaHandler) (Tool, error) {
	if !validToolName(name) {
		return Tool{}, errors.New("mcp: tool name must be 1 to 128 characters of A-Z, a-z, 0-9, '_', '-' and '.'")
	}
	var schema bytes.Buffer
	if err := json.Compact(&schema, inputSchema); err != nil || schema.Len() == 0 || schema.Bytes()[0] != '{' {
		return Tool{}, errors.New("mcp: tool input schema must be a JSON object")
	}
	if handler == nil {
		return Tool{}, errNilHandler
	}
	return Tool{name: name, description: description, inputSchema: schema.Bytes(), handler: handler}, nil
}

func validToolName(name string) bool {
	if name == "" || len(name) > 128 {
		return false
	}
	for _, c := range []byte(name) {
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9', c == '_', c == '-', c == '.':
		default:
			return false
		}
	}
	return true
}

// call runs the handler. A panic becomes a tool error. What the handler
// panicked with is dropped, because it may hold the arguments.
func (t Tool) call(arguments, meta json.RawMessage) (result Result) {
	defer func() {
		if recover() != nil {
			result = Result{Text: "tool failed", IsError: true}
		}
	}()
	return t.handler(arguments, meta)
}

// toolDefinition is a tool as tools/list describes it.
type toolDefinition struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"inputSchema"`
}

type textContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// callResult is a Result as tools/call returns it.
type callResult struct {
	Content []textContent `json:"content"`
	IsError bool          `json:"isError,omitempty"`
}
