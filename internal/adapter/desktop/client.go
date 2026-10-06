package desktop

import "strings"

// Client is what started the server, as far as the adapters care.
type Client int

// The clients. ClientCode is the zero value: a client that is not recognised
// is treated as Claude Code, which shows nothing without hook events.
const (
	// ClientCode is Claude Code, in any of its forms.
	ClientCode Client = iota
	// ClientDesktop is the copy of the server that reports Claude Desktop.
	ClientDesktop
	// ClientPassive is Claude Desktop's second copy, which reports nothing.
	ClientPassive
)

// clients is every client name that is recognised. The names were recorded by
// CRP-002 on Windows with Claude Desktop 2.9939.4 and are not documented.
// Supporting another host is one more row.
var clients = []struct {
	name string
	// prefix is set when the client's name only begins with name.
	prefix bool
	client Client
}{
	{name: "claude-ai", client: ClientDesktop},
	// Followed by the extension's display name.
	{name: "local-agent-mode-", prefix: true, client: ClientPassive},
	{name: "claude-code", client: ClientCode},
}

// Recognise tells which client gave name in MCP initialize. The match is
// exact, and case matters.
func Recognise(name string) Client {
	for _, row := range clients {
		if name == row.name || row.prefix && strings.HasPrefix(name, row.name) {
			return row.client
		}
	}
	return ClientCode
}
