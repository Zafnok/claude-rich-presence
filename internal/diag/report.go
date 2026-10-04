package diag

import (
	"fmt"
	"strings"
)

// Report is the outcome of the doctor checks.
type Report struct {
	// Version is that of this binary.
	Version  string
	Findings []Finding
	// LogFile is the log file, or empty if there is no log directory.
	LogFile string
}

// Worst returns the worst result among the findings.
func (r Report) Worst() Result {
	worst := Pass
	for _, f := range r.Findings {
		worst = max(worst, f.Result)
	}
	return worst
}

// Format returns the report as text that is safe to paste into a public
// issue. home is the user's home directory; every path is passed through
// Redact.
func (r Report) Format(home string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "rich-presence doctor, version %s\n\n", r.Version)
	for _, f := range r.Findings {
		fmt.Fprintf(&b, "[%s] %s: %s\n", f.Result, f.Check, f.Detail)
		if f.Path != "" {
			fmt.Fprintf(&b, "       At:  %s\n", Redact(f.Path, home))
		}
		if f.Action != "" {
			fmt.Fprintf(&b, "       Try: %s\n", f.Action)
		}
	}
	b.WriteString("\n")
	if r.LogFile != "" {
		fmt.Fprintf(&b, "Log file: %s\n", Redact(r.LogFile, home))
	}
	fmt.Fprintf(&b, "Result: %s\n", r.Worst())
	return b.String()
}

// redactedUser stands in for the user's name in a path outside their home.
const redactedUser = "<user>"

// Redact removes the user's name from a path. A path inside home has home
// replaced with ~. In any other path, an element equal to the last element
// of home, which is the user's name, is replaced with a placeholder.
//
// Both separators count on every operating system and letter case is
// ignored, so that a path is never left alone for being spelled differently
// from home.
func Redact(path, home string) string {
	home = strings.TrimRight(home, `/\`)
	if home == "" {
		return path
	}
	if n := len(home); len(path) >= n && strings.EqualFold(path[:n], home) && (len(path) == n || isSeparator(path[n])) {
		return "~" + path[n:]
	}
	user := home[strings.LastIndexAny(home, `/\`)+1:]
	var b strings.Builder
	start := 0
	for i := 0; i <= len(path); i++ {
		if i < len(path) && !isSeparator(path[i]) {
			continue
		}
		if element := path[start:i]; strings.EqualFold(element, user) {
			b.WriteString(redactedUser)
		} else {
			b.WriteString(element)
		}
		if i < len(path) {
			b.WriteByte(path[i])
		}
		start = i + 1
	}
	return b.String()
}

func isSeparator(c byte) bool {
	return c == '/' || c == '\\'
}
