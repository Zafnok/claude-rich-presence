package domain

// Surface is a place Claude runs.
type Surface string

// The surfaces.
const (
	SurfaceCode    Surface = "code"
	SurfaceDesktop Surface = "desktop"
)

// Valid reports whether s is a known surface.
func (s Surface) Valid() bool {
	return s == SurfaceCode || s == SurfaceDesktop
}

// Status is what a session is doing.
type Status string

// The statuses.
const (
	StatusIdle       Status = "idle"
	StatusWorking    Status = "working"
	StatusWaiting    Status = "waiting"
	StatusCompacting Status = "compacting"
)

// Statuses lists every status, in a fixed order.
func Statuses() []Status {
	return []Status{StatusIdle, StatusWorking, StatusWaiting, StatusCompacting}
}

// Valid reports whether s is a known status.
func (s Status) Valid() bool {
	return s == StatusIdle || s == StatusWorking || s == StatusWaiting || s == StatusCompacting
}

// ToolKind is the kind of tool a session is using. The vocabulary is closed:
// an adapter maps every tool to one of these and never forwards a tool name.
type ToolKind string

// The tool kinds. ToolNone means no tool is in use. ToolGeneric is a tool the
// adapter could not place.
const (
	ToolNone       ToolKind = ""
	ToolEditing    ToolKind = "editing"
	ToolRunning    ToolKind = "running"
	ToolReading    ToolKind = "reading"
	ToolSearching  ToolKind = "searching"
	ToolBrowsing   ToolKind = "browsing"
	ToolDelegating ToolKind = "delegating"
	ToolUsingTools ToolKind = "tools"
	ToolGeneric    ToolKind = "generic"
)

// ToolKinds lists every tool kind except ToolNone, in a fixed order.
func ToolKinds() []ToolKind {
	return []ToolKind{
		ToolEditing, ToolRunning, ToolReading, ToolSearching,
		ToolBrowsing, ToolDelegating, ToolUsingTools, ToolGeneric,
	}
}

// Valid reports whether k names a tool kind. ToolNone is not one.
func (k ToolKind) Valid() bool {
	for _, known := range ToolKinds() {
		if k == known {
			return true
		}
	}
	return false
}

// Privacy is how much of a session may be shown. The adapter enforces it
// before an event leaves its process; the domain only carries the level.
type Privacy string

// The privacy levels.
const (
	PrivacyMinimal  Privacy = "minimal"
	PrivacyStandard Privacy = "standard"
	PrivacyFull     Privacy = "full"
)

// Valid reports whether p is a known privacy level.
func (p Privacy) Valid() bool {
	return p == PrivacyMinimal || p == PrivacyStandard || p == PrivacyFull
}
