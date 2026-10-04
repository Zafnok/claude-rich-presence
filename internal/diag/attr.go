package diag

import (
	"log/slog"
	"time"
)

// text is a string that code outside this package can supply only as a
// constant. The type is not exported, so a string variable cannot be
// converted to it there, while an untyped constant such as a literal is
// converted by the compiler. It is what makes logging a string taken from an
// event a compile error.
type text string

// Attr is one attribute of a log line. Its content is not exported, so the
// functions of this package are the only way to make one that holds anything.
// The zero Attr is ignored.
type Attr struct {
	attr slog.Attr
}

// State names a state, such as that of the Discord connection.
func State(key, name text) Attr {
	return Attr{slog.String(string(key), string(name))}
}

// ErrorClass names a class of error. An error's own text is never logged: it
// may hold a path.
func ErrorClass(class text) Attr {
	return Attr{slog.String("error", string(class))}
}

// Count is a number of things.
func Count(key text, n int64) Attr {
	return Attr{slog.Int64(string(key), n)}
}

// Duration is a length of time.
func Duration(key text, d time.Duration) Attr {
	return Attr{slog.Duration(string(key), d)}
}

// Version is the version of a binary. It is the one attribute whose value is
// not a constant, so it is passed through CleanVersion.
func Version(key text, v string) Attr {
	return Attr{slog.String(string(key), CleanVersion(v))}
}

// InvalidVersion is what CleanVersion returns for a string that is not
// shaped like a version.
const InvalidVersion = "invalid"

// maxVersionLen is the longest version, in bytes. It equals the limit of the
// control protocol.
const maxVersionLen = 64

// CleanVersion returns v if it is shaped like a version, and InvalidVersion
// otherwise. The alphabet is that of the control protocol: it has no path
// separator and no space, so a version cannot carry a path or a sentence.
func CleanVersion(v string) string {
	if v == "" || len(v) > maxVersionLen {
		return InvalidVersion
	}
	for i := 0; i < len(v); i++ {
		c := v[i]
		letter := c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
		digit := c >= '0' && c <= '9'
		if !letter && !digit && c != '.' && c != '-' && c != '+' && c != '_' && c != '(' && c != ')' {
			return InvalidVersion
		}
	}
	return v
}
