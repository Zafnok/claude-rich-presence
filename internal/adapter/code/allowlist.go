package code

import (
	"encoding/json"
	"slices"
	"strconv"
	"strings"

	"github.com/Zafnok/claude-rich-presence/internal/domain"
)

// The fields a hook may pass. No other field of a hook event is ever read.
const (
	fieldEvent            = "event"
	fieldSessionID        = "session_id"
	fieldCwd              = "cwd"
	fieldToolName         = "tool_name"
	fieldModel            = "model"
	fieldToModel          = "to_model"
	fieldNotificationType = "notification_type"
	fieldAgentID          = "agent_id"
)

// Length limits, in bytes, on the values of fields. A longer value makes the
// whole call malformed.
const (
	maxFieldLen = 256
	maxCwdLen   = 4096
)

// HookEvent is one hook event the adapter accepts and the fields it reads
// from it, besides the event name.
type HookEvent struct {
	Name   string
	Fields []string
}

// hookEvent is a row of the allowlist.
type hookEvent struct {
	name string
	// fields are the fields read from this event. Any other is not parsed.
	fields []string
	// required are the fields without which the event is malformed, besides
	// the session id, which every event needs.
	required []string
	// kind is the presence event this hook event becomes. It is empty for
	// Notification, where the notification type decides.
	kind domain.Kind
}

// allowlist is every hook event the adapter accepts, with the fields it reads
// from each. It is the one definition: the tool's input schema is generated
// from it, the parser reads through it, and the plugin's hook file must match
// it.
//
// The rows follow the event table in docs/architecture/README.md and the
// fields recorded in docs/research/crp-001-claude-code-adapter.md.
var allowlist = []hookEvent{
	{name: "SessionStart", fields: []string{fieldSessionID, fieldCwd, fieldModel}, kind: domain.KindSessionRefreshed},
	{name: "UserPromptSubmit", fields: []string{fieldSessionID, fieldCwd}, kind: domain.KindTurnStarted},
	{name: "PreToolUse", fields: []string{fieldSessionID, fieldCwd, fieldToolName}, required: []string{fieldToolName}, kind: domain.KindToolStarted},
	{name: "PostToolUse", fields: []string{fieldSessionID, fieldCwd}, kind: domain.KindToolFinished},
	{name: "PostToolUseFailure", fields: []string{fieldSessionID, fieldCwd}, kind: domain.KindToolFinished},
	{name: "Notification", fields: []string{fieldSessionID, fieldCwd, fieldNotificationType}, required: []string{fieldNotificationType}},
	{name: "Stop", fields: []string{fieldSessionID, fieldCwd}, kind: domain.KindTurnFinished},
	{name: "StopFailure", fields: []string{fieldSessionID, fieldCwd}, kind: domain.KindTurnFinished},
	{name: "PreCompact", fields: []string{fieldSessionID, fieldCwd}, kind: domain.KindCompactionStarted},
	{name: "PostCompact", fields: []string{fieldSessionID, fieldCwd}, kind: domain.KindCompactionFinished},
	{name: "PostModelSwitch", fields: []string{fieldSessionID, fieldCwd, fieldToModel}, required: []string{fieldToModel}, kind: domain.KindModelChanged},
	{name: "SubagentStart", fields: []string{fieldSessionID, fieldCwd, fieldAgentID}, required: []string{fieldAgentID}, kind: domain.KindSubagentStarted},
	{name: "SubagentStop", fields: []string{fieldSessionID, fieldCwd, fieldAgentID}, required: []string{fieldAgentID}, kind: domain.KindSubagentStopped},
}

// notificationKinds maps a notification type to its presence event. A type
// that is not listed produces none.
var notificationKinds = map[string]domain.Kind{
	"permission_prompt":  domain.KindAttentionNeeded,
	"agent_needs_input":  domain.KindAttentionNeeded,
	"elicitation_dialog": domain.KindAttentionNeeded,
	"idle_prompt":        domain.KindIdle,
}

// HookEvents returns the allowlist: each hook event the adapter accepts and
// the fields a hook may pass with it. The caller owns the result.
func HookEvents() []HookEvent {
	out := make([]HookEvent, 0, len(allowlist))
	for _, row := range allowlist {
		out = append(out, HookEvent{Name: row.name, Fields: slices.Clone(row.fields)})
	}
	return out
}

// eventSchema builds the input schema of the event tool from the allowlist:
// the event name, limited to the names in the table, and each field any row
// reads. Nothing else is declared.
func eventSchema() json.RawMessage {
	var names, properties []string
	var declared []string
	for _, row := range allowlist {
		names = append(names, strconv.Quote(row.name))
		for _, field := range row.fields {
			if !slices.Contains(declared, field) {
				declared = append(declared, field)
				properties = append(properties, strconv.Quote(field)+`:{"type":"string"}`)
			}
		}
	}
	return json.RawMessage(`{"type":"object","properties":{` +
		strconv.Quote(fieldEvent) + `:{"type":"string","enum":[` + strings.Join(names, ",") + `]},` +
		strings.Join(properties, ",") +
		`},"required":[` + strconv.Quote(fieldEvent) + `,` + strconv.Quote(fieldSessionID) + `]}`)
}

// hook is one call to the event tool, reduced to its allowlisted fields.
type hook struct {
	row    hookEvent
	values map[string]string
}

func (h hook) get(field string) string {
	return h.values[field]
}

// parseHook reads the arguments of a call to the event tool. It reports false
// when they are malformed: not an object, an unknown event name, a field of
// the wrong type or too long, or a missing required field.
//
// Only the fields the event's row names are decoded. The working directory is
// decoded only when readCwd is set, so that below the full privacy level it
// is discarded on receipt (ADR-0008). An empty string is an absent field,
// because that is what Claude Code substitutes for one.
func parseHook(arguments json.RawMessage, readCwd bool) (hook, bool) {
	var raw map[string]json.RawMessage
	if json.Unmarshal(arguments, &raw) != nil {
		return hook{}, false
	}
	name, ok := text(raw[fieldEvent], maxFieldLen)
	if !ok {
		return hook{}, false
	}
	at := slices.IndexFunc(allowlist, func(row hookEvent) bool { return row.name == name })
	if at < 0 {
		return hook{}, false
	}
	h := hook{row: allowlist[at], values: map[string]string{}}
	for _, field := range h.row.fields {
		limit := maxFieldLen
		switch field {
		case fieldCwd:
			if !readCwd {
				continue
			}
			limit = maxCwdLen
		case fieldSessionID:
			limit = domain.MaxIDLen
		}
		value, ok := text(raw[field], limit)
		if !ok {
			return hook{}, false
		}
		h.values[field] = value
	}
	if h.get(fieldSessionID) == "" {
		return hook{}, false
	}
	for _, field := range h.row.required {
		if h.get(field) == "" {
			return hook{}, false
		}
	}
	return h, true
}

// text decodes a JSON string of at most limit bytes. An absent value and a
// null are the empty string. Anything else that is not a string is refused.
func text(raw json.RawMessage, limit int) (string, bool) {
	if raw == nil {
		return "", true
	}
	var s string
	if json.Unmarshal(raw, &s) != nil || len(s) > limit {
		return "", false
	}
	return s, true
}
