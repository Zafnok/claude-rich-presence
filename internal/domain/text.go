package domain

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// markup is removed from text that will be shown: the characters Discord
// reads as formatting, as a mention or as a link.
const markup = "*`~|<>[]\\@"

// CleanName makes a project name safe to show, whether it is a directory
// name or a display name the user set: one line, with control characters,
// invisible characters and markup removed, runs of spaces collapsed, and cut
// to MaxProjectLen. The result is empty if nothing is left.
//
// Cleaning a clean name changes nothing, which is how a name is checked: the
// adapters clean what they publish, and Validate refuses a project that
// CleanName would change.
func CleanName(name string) string {
	return CleanText(name, MaxProjectLen)
}

// CleanText is the cleaning CleanName describes, to a limit in bytes.
func CleanText(s string, limit int) string {
	var b strings.Builder
	space := false
	for _, r := range s {
		switch {
		case unicode.IsSpace(r) || unicode.IsControl(r):
			space = true
			continue
		case r == utf8.RuneError || unicode.Is(unicode.Cf, r) || strings.ContainsRune(markup, r):
			continue
		}
		need := utf8.RuneLen(r)
		if space && b.Len() > 0 {
			need++
		}
		if b.Len()+need > limit {
			break
		}
		if need > utf8.RuneLen(r) {
			b.WriteByte(' ')
		}
		space = false
		b.WriteRune(r)
	}
	return b.String()
}

// modelLabel reports whether s can be a model label. Labels come from a
// closed table in the adapters, such as "Opus 5.5", so the check is stricter
// than cleaning: ASCII letters, digits and dots, in words with one space
// between them.
func modelLabel(s string) bool {
	for i := 0; i < len(s); i++ {
		c := s[i]
		letter := c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
		digit := c >= '0' && c <= '9'
		if !letter && !digit && c != '.' && c != ' ' {
			return false
		}
	}
	return CleanText(s, MaxModelLen) == s
}
