package presence

import "github.com/Zafnok/claude-rich-presence/internal/domain"

// Asset keys uploaded to the Discord application. Discord lower-cases keys.
const (
	assetLogo    = "logo"
	assetWorking = "working"
	assetWaiting = "waiting"
	assetIdle    = "idle"
)

// separator joins the parts of a text line.
const separator = " · "

// surfacePhrases is the fixed first line per surface. It is also the hover
// text of the large image.
var surfacePhrases = map[domain.Surface]string{
	domain.SurfaceCode:    "Claude Code",
	domain.SurfaceDesktop: "Claude Desktop",
}

// phraseKey names one row of the phrase table. Tool is domain.ToolNone
// unless the status is working.
type phraseKey struct {
	Surface domain.Surface
	Status  domain.Status
	Tool    domain.ToolKind
}

// statusPhrases is the status in words: one row per surface, status and tool
// kind. A working session with no tool is thinking. The phrase opens the
// second line and is the hover text of the small image.
var statusPhrases = map[phraseKey]string{
	{domain.SurfaceCode, domain.StatusWorking, domain.ToolNone}:       "Thinking",
	{domain.SurfaceCode, domain.StatusWorking, domain.ToolEditing}:    "Editing files",
	{domain.SurfaceCode, domain.StatusWorking, domain.ToolRunning}:    "Running commands",
	{domain.SurfaceCode, domain.StatusWorking, domain.ToolReading}:    "Reading files",
	{domain.SurfaceCode, domain.StatusWorking, domain.ToolSearching}:  "Searching",
	{domain.SurfaceCode, domain.StatusWorking, domain.ToolBrowsing}:   "Browsing the web",
	{domain.SurfaceCode, domain.StatusWorking, domain.ToolDelegating}: "Delegating to subagents",
	{domain.SurfaceCode, domain.StatusWorking, domain.ToolUsingTools}: "Using tools",
	{domain.SurfaceCode, domain.StatusWorking, domain.ToolGeneric}:    "Working",
	{domain.SurfaceCode, domain.StatusWaiting, domain.ToolNone}:       "Waiting for input",
	{domain.SurfaceCode, domain.StatusCompacting, domain.ToolNone}:    "Compacting context",
	{domain.SurfaceCode, domain.StatusIdle, domain.ToolNone}:          "Idle",

	{domain.SurfaceDesktop, domain.StatusWorking, domain.ToolNone}:       "Thinking",
	{domain.SurfaceDesktop, domain.StatusWorking, domain.ToolEditing}:    "Editing files",
	{domain.SurfaceDesktop, domain.StatusWorking, domain.ToolRunning}:    "Running commands",
	{domain.SurfaceDesktop, domain.StatusWorking, domain.ToolReading}:    "Reading files",
	{domain.SurfaceDesktop, domain.StatusWorking, domain.ToolSearching}:  "Searching",
	{domain.SurfaceDesktop, domain.StatusWorking, domain.ToolBrowsing}:   "Browsing the web",
	{domain.SurfaceDesktop, domain.StatusWorking, domain.ToolDelegating}: "Delegating to subagents",
	{domain.SurfaceDesktop, domain.StatusWorking, domain.ToolUsingTools}: "Using tools",
	{domain.SurfaceDesktop, domain.StatusWorking, domain.ToolGeneric}:    "Working",
	{domain.SurfaceDesktop, domain.StatusWaiting, domain.ToolNone}:       "Waiting for input",
	{domain.SurfaceDesktop, domain.StatusCompacting, domain.ToolNone}:    "Compacting context",
	{domain.SurfaceDesktop, domain.StatusIdle, domain.ToolNone}:          "Idle",
}

// statusImages is the small image per status. Compacting is work in
// progress and has no asset of its own.
var statusImages = map[domain.Status]string{
	domain.StatusWorking:    assetWorking,
	domain.StatusCompacting: assetWorking,
	domain.StatusWaiting:    assetWaiting,
	domain.StatusIdle:       assetIdle,
}
