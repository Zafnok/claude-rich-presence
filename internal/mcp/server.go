package mcp

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"io"
)

// LatestProtocolVersion is the newest revision of the specification this
// server speaks, and its answer to a client that asks for one it does not.
const LatestProtocolVersion = "2025-11-25"

// supportedVersions are the revisions whose handshake, tool listing and tool
// result are the ones implemented here. 2025-03-26 is left out because it
// requires JSON-RPC batches.
var supportedVersions = map[string]bool{
	LatestProtocolVersion: true,
	"2025-06-18":          true,
}

// DefaultMaxLineBytes is the longest input line accepted when Options does
// not set a limit.
const DefaultMaxLineBytes = 1 << 20

// ClientInfo identifies the client, as it described itself in initialize.
type ClientInfo struct {
	Name    string
	Version string
}

// Options configures Serve.
type Options struct {
	// Name and Version describe this server to the client.
	Name    string
	Version string
	// MaxLineBytes is the longest input line accepted, not counting the line
	// ending. Zero or less means DefaultMaxLineBytes; the smallest limit that
	// can be enforced is 15.
	MaxLineBytes int
	// Initialize is called once, when the client initializes, and returns the
	// tools that client is offered. Tool names must be unique. Nil means no
	// tools. Like a Handler, it must return at once.
	Initialize func(ClientInfo) []Tool
	// Shutdown is called exactly once, just before Serve returns. Nil means
	// nothing is called.
	Shutdown func()
}

// Serve reads messages from in and answers them on out until in ends. It
// returns nil when in ends cleanly, or the error that stopped it: a failed
// read, or a failed write, after which the client can no longer be answered.
func Serve(in io.Reader, out io.Writer, opts Options) error {
	s := &server{opts: opts, w: writer{out: out}}
	err := s.loop(in)
	if opts.Shutdown != nil {
		opts.Shutdown()
	}
	return err
}

type server struct {
	opts        Options
	w           writer
	initialized bool
	tools       []Tool
}

func (s *server) loop(in io.Reader) error {
	limit := s.opts.MaxLineBytes
	if limit <= 0 {
		limit = DefaultMaxLineBytes
	}
	// The buffer holds one line of the longest length and its line feed. A
	// line that fills it without ending is too long.
	r := bufio.NewReaderSize(in, limit+1)
	for {
		line, readErr := r.ReadSlice('\n')
		var writeErr error
		if errors.Is(readErr, bufio.ErrBufferFull) {
			for errors.Is(readErr, bufio.ErrBufferFull) {
				_, readErr = r.ReadSlice('\n')
			}
			writeErr = s.w.fail(nullID, rpcError{Code: codeInvalidRequest, Message: "message too long"})
		} else if len(bytes.TrimSpace(line)) > 0 {
			writeErr = s.handle(line)
		}
		if writeErr != nil {
			return writeErr
		}
		if readErr == io.EOF {
			return nil
		}
		if readErr != nil {
			return readErr
		}
	}
}

func (s *server) handle(line []byte) error {
	msg, rejected := decode(line)
	if rejected != nil {
		return s.w.fail(rejected.id, rejected.err)
	}
	if msg.id == nil {
		// A notification. None of them needs any action from a server that
		// runs one request at a time, and none may be answered.
		return nil
	}
	result, rpcErr := s.dispatch(msg)
	if rpcErr != nil {
		return s.w.fail(msg.id, *rpcErr)
	}
	return s.w.result(msg.id, result)
}

func (s *server) dispatch(msg message) (any, *rpcError) {
	switch msg.method {
	case "ping":
		return struct{}{}, nil
	case "initialize":
		return s.initialize(msg.params)
	case "tools/list":
		if !s.initialized {
			return nil, errNotInitialized
		}
		return s.listTools(msg.params)
	case "tools/call":
		if !s.initialized {
			return nil, errNotInitialized
		}
		return s.callTool(msg.params)
	}
	return nil, &rpcError{Code: codeMethodNotFound, Message: "method not found"}
}

var errNotInitialized = &rpcError{Code: codeInvalidRequest, Message: "server not initialized"}

func invalidParams(text string) *rpcError {
	return &rpcError{Code: codeInvalidParams, Message: text}
}

type implementation struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type initializeResult struct {
	ProtocolVersion string `json:"protocolVersion"`
	Capabilities    struct {
		Tools struct{} `json:"tools"`
	} `json:"capabilities"`
	ServerInfo implementation `json:"serverInfo"`
}

func (s *server) initialize(params json.RawMessage) (any, *rpcError) {
	if s.initialized {
		return nil, &rpcError{Code: codeInvalidRequest, Message: "already initialized"}
	}
	var p struct {
		ProtocolVersion string         `json:"protocolVersion"`
		ClientInfo      implementation `json:"clientInfo"`
	}
	if err := json.Unmarshal(params, &p); err != nil || p.ProtocolVersion == "" || p.ClientInfo.Name == "" || p.ClientInfo.Version == "" {
		return nil, invalidParams("initialize needs protocolVersion, clientInfo.name and clientInfo.version")
	}
	version := LatestProtocolVersion
	if supportedVersions[p.ProtocolVersion] {
		version = p.ProtocolVersion
	}
	if s.opts.Initialize != nil {
		s.tools = s.opts.Initialize(ClientInfo{Name: p.ClientInfo.Name, Version: p.ClientInfo.Version})
	}
	s.initialized = true
	return initializeResult{
		ProtocolVersion: version,
		ServerInfo:      implementation{Name: s.opts.Name, Version: s.opts.Version},
	}, nil
}

func (s *server) listTools(params json.RawMessage) (any, *rpcError) {
	if params != nil {
		var p struct {
			Cursor *string `json:"cursor"`
		}
		// The list is never paged, so no cursor was ever handed out.
		if err := json.Unmarshal(params, &p); err != nil || p.Cursor != nil {
			return nil, invalidParams("invalid cursor")
		}
	}
	definitions := make([]toolDefinition, 0, len(s.tools))
	for _, t := range s.tools {
		definitions = append(definitions, toolDefinition{Name: t.name, Description: t.description, InputSchema: t.inputSchema})
	}
	return struct {
		Tools []toolDefinition `json:"tools"`
	}{definitions}, nil
}

func (s *server) callTool(params json.RawMessage) (any, *rpcError) {
	var p struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
		Meta      json.RawMessage `json:"_meta"`
	}
	if err := json.Unmarshal(params, &p); err != nil || p.Name == "" {
		return nil, invalidParams("tools/call needs a tool name")
	}
	arguments := p.Arguments
	if string(arguments) == "null" {
		arguments = nil
	}
	if arguments != nil && arguments[0] != '{' {
		return nil, invalidParams("arguments must be an object")
	}
	meta := p.Meta
	if string(meta) == "null" {
		meta = nil
	}
	if meta != nil && meta[0] != '{' {
		return nil, invalidParams("_meta must be an object")
	}
	for _, t := range s.tools {
		if t.name == p.Name {
			result := t.call(arguments, meta)
			return callResult{Content: []textContent{{Type: "text", Text: result.Text}}, IsError: result.IsError}, nil
		}
	}
	return nil, invalidParams("unknown tool")
}
