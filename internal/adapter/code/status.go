package code

import (
	"encoding/json"
	"strconv"

	"github.com/Zafnok/claude-rich-presence/internal/mcp"
)

const statusDescription = "Reports whether Discord Rich Presence is working: this session's role, the Discord connection and the number of open sessions. Read-only."

// statusUnavailable is the status tool's answer when the source fails.
const statusUnavailable = "Presence status is unavailable."

// Role is the part this process plays in presence.
type Role string

// The roles. Any other value is shown as unknown.
const (
	// RoleHost: this process holds the Discord connection.
	RoleHost Role = "host"
	// RoleFollower: this process forwards its events to the host.
	RoleFollower Role = "follower"
	// RoleOff: presence is switched off, and nothing is published.
	RoleOff Role = "off"
)

// Discord is the state of the host's connection to Discord.
type Discord string

// The connection states. Any other value is shown as unknown.
const (
	DiscordConnected    Discord = "connected"
	DiscordConnecting   Discord = "connecting"
	DiscordDisconnected Discord = "disconnected"
)

// Status is the diagnostic summary the status tool reports. It holds closed
// vocabularies and a count, and no text, so that no project name or path can
// travel through it.
type Status struct {
	Role    Role
	Discord Discord
	// Sessions is the number of open sessions the host knows.
	Sessions int
}

// StatusSource supplies the summary. Status is called from a tool call that
// Claude waits on, so it must return at once from what is already known and
// must not perform I/O (ADR-0008).
type StatusSource interface {
	Status() Status
}

var roleText = map[Role]string{
	RoleHost:     "host",
	RoleFollower: "follower",
	RoleOff:      "off",
}

var discordText = map[Discord]string{
	DiscordConnected:    "connected",
	DiscordConnecting:   "connecting",
	DiscordDisconnected: "disconnected",
}

// word is the fixed text for a value, or "unknown". The value itself is
// never printed.
func word[K comparable](words map[K]string, value K) string {
	if text, ok := words[value]; ok {
		return text
	}
	return "unknown"
}

// handleStatus is the status tool. It reads nothing from its arguments and
// changes nothing.
func (a *Adapter) handleStatus(json.RawMessage) (result mcp.Result) {
	defer func() {
		if recover() != nil {
			result = mcp.Result{Text: statusUnavailable, IsError: true}
		}
	}()
	s := a.status.Status()
	return mcp.Result{Text: "Role: " + word(roleText, s.Role) + "\n" +
		"Discord: " + word(discordText, s.Discord) + "\n" +
		"Sessions: " + strconv.Itoa(s.Sessions) + "\n" +
		"Privacy: " + string(a.privacy) + "\n" +
		"Events ignored: " + strconv.FormatInt(a.ignored.Load(), 10) + "\n" +
		"Events dropped: " + strconv.FormatInt(a.dropped.Load(), 10)}
}
