package code

import (
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/Zafnok/claude-rich-presence/internal/domain"
)

// toolKinds maps the names of built-in tools to tool kinds.
var toolKinds = map[string]domain.ToolKind{
	"Edit":         domain.ToolEditing,
	"Write":        domain.ToolEditing,
	"MultiEdit":    domain.ToolEditing,
	"NotebookEdit": domain.ToolEditing,
	"Bash":         domain.ToolRunning,
	"PowerShell":   domain.ToolRunning,
	"Read":         domain.ToolReading,
	"Grep":         domain.ToolSearching,
	"Glob":         domain.ToolSearching,
	"WebFetch":     domain.ToolBrowsing,
	"WebSearch":    domain.ToolBrowsing,
	"Agent":        domain.ToolDelegating,
	"Task":         domain.ToolDelegating,
}

// mcpPrefix begins the name of every tool an MCP server provides. The
// server's name follows it, up to the next double underscore.
const mcpPrefix = "mcp__"

// browserWords mark an MCP server as a browser when its name contains one.
var browserWords = []string{"browser", "chrome", "playwright", "puppeteer"}

// toolKind maps a tool name to a tool kind. The name goes no further.
func toolKind(name string) domain.ToolKind {
	if kind, ok := toolKinds[name]; ok {
		return kind
	}
	rest, ok := strings.CutPrefix(name, mcpPrefix)
	if !ok {
		return domain.ToolGeneric
	}
	server, _, _ := strings.Cut(strings.ToLower(rest), "__")
	for _, word := range browserWords {
		if strings.Contains(server, word) {
			return domain.ToolBrowsing
		}
	}
	return domain.ToolUsingTools
}

// families are the model families a label may name. The vocabulary is closed,
// as tool kinds are: a model id can be set by the user, to the name of a
// private deployment for one, so no word of it is ever published. A family
// that is not listed gives no label until it is added here.
var families = []string{"opus", "sonnet", "haiku", "fable"}

// modelLabel turns a model id into a short family label: the family name and
// version, as "Opus 5.5" for "claude-opus-5-5". An id it cannot read gives
// the empty string.
//
// Model ids change often, so the version is derived from the id's structure
// and not from a list of ids. After the vendor's name come a family word and
// one or two version numbers of one or two digits, in either order. Whatever
// follows them, such as a release date or a variant, is not read.
func modelLabel(id string) string {
	tokens := strings.FieldsFunc(strings.ToLower(id), func(r rune) bool {
		return !isLetter(r) && !isDigit(r)
	})
	vendor := slices.Index(tokens, "claude")
	if vendor < 0 {
		return ""
	}
	family := ""
	var version []string
scan:
	for _, token := range tokens[vendor+1:] {
		switch {
		case family == "" && slices.Contains(families, token):
			family = token
		case len(version) < 2 && len(token) <= 2 && all(token, isDigit):
			version = append(version, token)
		default:
			break scan
		}
	}
	if family == "" || len(version) == 0 {
		return ""
	}
	return strings.ToUpper(family[:1]) + family[1:] + " " + strings.Join(version, ".")
}

// printable reports whether s is free of control characters and of bytes
// that are not text.
func printable(s string) bool {
	for _, r := range s {
		if r < 0x20 || r == 0x7f || r == utf8.RuneError {
			return false
		}
	}
	return true
}

func isLetter(r rune) bool { return r >= 'a' && r <= 'z' }

func isDigit(r rune) bool { return r >= '0' && r <= '9' }

func all(s string, is func(rune) bool) bool {
	for _, r := range s {
		if !is(r) {
			return false
		}
	}
	return true
}

// projectName reduces a working directory to its last element, for either
// slash style and with or without a trailing separator, and cleans it as a
// display name from a profile is cleaned, because a directory can be named
// anything and Claude can create one. It returns the empty string when there
// is no usable last element: a root, a bare drive, a dot entry, or a name
// with nothing left once it is clean.
func projectName(cwd string) string {
	const separators = `/\`
	trimmed := strings.TrimRight(cwd, separators)
	name := trimmed[strings.LastIndexAny(trimmed, separators)+1:]
	if name == "" || name == "." || name == ".." || strings.HasSuffix(name, ":") {
		return ""
	}
	return domain.CleanName(name)
}
