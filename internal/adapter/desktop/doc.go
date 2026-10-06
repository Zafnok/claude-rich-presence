// Package desktop translates the process lifetime of the desktop extension
// into domain events. It reports lifecycle only: that Claude Desktop is open,
// and since when. Claude Desktop gives an extension no signal about
// conversations, and none is looked for (ADR-0008).
//
// Claude Desktop runs two copies of the server from launch to quit, told
// apart by the client name in MCP initialize (CRP-002). Recognise reads that
// name. The copy initialized by ClientDesktop reports the app, through an
// adapter made with New. The other, ClientPassive, reports nothing, through
// an adapter made with NewPassive: two adapters for one app would show two
// sessions.
//
// Both offer the status tool and no other. There are no hooks in Claude
// Desktop to call an event tool.
//
// It may import internal/domain and internal/mcp. It must not import
// internal/host, internal/cli, any transport or any Discord package.
package desktop
