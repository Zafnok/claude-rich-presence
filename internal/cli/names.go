package cli

// Every name the product publishes is defined here and nowhere else in Go
// code, so a rename is a one-line change (ADR-0010). None may carry a vendor
// mark.
const (
	// ProductName is the human-readable name. It is provisional until the
	// owner settles naming in CRP-003.
	ProductName = "Rich Presence"

	// BinaryName is the name of the executable.
	BinaryName = "rich-presence"

	// PluginName is the name in the plugin and extension manifests.
	PluginName = "rich-presence"

	// The MCP tools the server exposes.
	ToolEvent   = "presence_event"
	ToolSummary = "presence_summary"
	ToolStatus  = "presence_status"
	ToolPause   = "presence_pause"
)
