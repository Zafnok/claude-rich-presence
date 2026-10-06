package code

import (
	"encoding/json"
	"strconv"
	"time"

	"github.com/Zafnok/claude-rich-presence/internal/domain"
	"github.com/Zafnok/claude-rich-presence/internal/mcp"
)

const statusDescription = "Reports whether Discord Rich Presence is working: this session's role, the Discord connection, the number of open sessions, and whether presence is paused or hidden. Read-only. With preview set to true it returns a private preview instead: every part of the Discord card as it is shown now, which can name the project and is for the user alone."

const statusSchema = `{"type":"object","properties":{"preview":{"type":"boolean","description":"Return the private preview of the Discord card instead of the diagnostic summary."}}}`

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
// vocabularies, a count and the state of a pause, and no text, so that no
// project name or path can travel through it.
type Status struct {
	Role    Role
	Discord Discord
	// Sessions is the number of open sessions the host knows.
	Sessions int
	// Paused says presence is paused for every session. PausedUntil is when
	// the pause ends by itself, or the zero time for one that lasts until it
	// is resumed.
	Paused      bool
	PausedUntil time.Time
	// NoPause says the host is from before pausing and cannot pause.
	NoPause bool
}

// StatusSource supplies the summary. Status is called from a tool call that
// Claude waits on, so it must return at once from what is already known and
// must not perform I/O (ADR-0008).
type StatusSource interface {
	Status() Status
}

// Preview is the card as Discord was last told to show it. It can name a
// project, so the adapter returns it through the tool result and nowhere
// else, and never logs it.
type Preview struct {
	// Shown is false when nothing is shown.
	Shown    bool
	Activity domain.Activity
}

// PreviewSource supplies the card, and false when it is not known. The same
// rules apply as for a StatusSource.
type PreviewSource interface {
	Preview() (Preview, bool)
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

// handleStatus is the status tool. It reads one flag from its arguments,
// which chooses between the diagnostic summary and the private preview, and
// changes nothing.
func (a *Adapter) handleStatus(arguments json.RawMessage) (result mcp.Result) {
	defer func() {
		if recover() != nil {
			result = mcp.Result{Text: statusUnavailable, IsError: true}
		}
	}()
	var asked struct {
		Preview bool `json:"preview"`
	}
	// Arguments that cannot be read ask for the summary.
	_ = json.Unmarshal(arguments, &asked)
	s := a.status.Status()
	a.mu.Lock()
	level := a.session.privacy
	a.mu.Unlock()
	if asked.Preview {
		return mcp.Result{Text: a.previewText(s, level)}
	}
	return mcp.Result{Text: "Role: " + word(roleText, s.Role) + "\n" +
		"Discord: " + word(discordText, s.Discord) + "\n" +
		"Sessions: " + strconv.Itoa(s.Sessions) + "\n" +
		"Privacy: " + string(level) + "\n" +
		"Paused: " + a.pausedText(s) + "\n" +
		"Events ignored: " + strconv.FormatInt(a.ignored.Load(), 10) + "\n" +
		"Events dropped: " + strconv.FormatInt(a.dropped.Load(), 10)}
}

// pausedText says whether presence is paused, and for how much longer.
func (a *Adapter) pausedText(s Status) string {
	switch {
	case s.NoPause:
		return "unavailable, the presence host is an older version"
	case !s.Paused:
		return "no"
	case s.PausedUntil.IsZero():
		return "until resumed"
	}
	return "for another " + remaining(s.PausedUntil.Sub(a.clock.Now()))
}

// remaining is a length of time for a person to read: whole seconds, rounded
// up, and never less than one.
func remaining(d time.Duration) string {
	whole := d.Truncate(time.Second)
	if whole < d {
		whole += time.Second
	}
	return max(whole, time.Second).String()
}

// previewHeading opens every preview. It says what the text is, because it
// is unlike the summary, which is safe to paste anywhere.
const previewHeading = "PRIVATE PREVIEW of your Discord card. It can name your project, so do not paste it anywhere public.\n"

// previewText is the private preview: whether presence is on, whether this
// session is part of it, and every slot of the card as Discord was last told
// to show it.
func (a *Adapter) previewText(s Status, level domain.Privacy) string {
	text := previewHeading
	if level == domain.PrivacyOff {
		text += "This session: hidden. Its privacy level is off, so it is not published and not counted.\n"
	} else {
		text += "This session: published at the privacy level " + string(level) + ".\n"
	}
	switch {
	case s.Role == RoleOff || a.preview == nil:
		return text + "Presence: off. Nothing is shown."
	case s.NoPause:
		return text + "Presence: the card cannot be read, because the presence host is an older version. Restart your other sessions so that they update."
	case s.Paused:
		return text + "Presence: paused " + a.pausedText(s) + ". Nothing is shown."
	}
	card, known := a.preview.Preview()
	switch {
	case !known:
		return text + "Presence: the card is not known yet. Ask again in a moment."
	case !card.Shown:
		return text + "Presence: nothing is shown. No session is published, or every session has been idle for long enough to clear the card."
	}
	c := card.Activity
	timer := ""
	if !c.Start.IsZero() {
		timer = "counting up from " + c.Start.UTC().Format("2006-01-02 15:04:05 UTC")
	}
	return text + "Presence: shown.\n" +
		slot("Line 1", c.Details) +
		slot("Line 2", c.State) +
		slot("Timer", timer) +
		slot("Large image", c.LargeImage) +
		slot("Large image hover text", c.LargeText) +
		slot("Small image", c.SmallImage) +
		slot("Small image hover text", c.SmallText) +
		slot("Button", c.Button.Label) +
		"Button link: " + filled(c.Button.URL)
}

// slot is one line of the preview.
func slot(name, value string) string {
	return name + ": " + filled(value) + "\n"
}

// filled is a slot's value, or a word for one that is empty.
func filled(value string) string {
	if value == "" {
		return "(empty)"
	}
	return value
}
