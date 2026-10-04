package config

import (
	"encoding/json"
	"net/url"
	"path"
	"slices"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/Zafnok/claude-rich-presence/internal/domain"
)

// DefaultLinkHost is the one host a repository link may name unless the
// link_hosts setting adds others.
const DefaultLinkHost = "github.com"

// Limits on project profiles. Lengths are in bytes.
const (
	// MaxProfiles is how many profiles are read from the file.
	MaxProfiles = 64
	// MaxLinkHosts is how many link_hosts entries are read from the file.
	MaxLinkHosts = 16
	// MaxNameLen is the longest display name. It fits where the directory
	// name would go.
	MaxNameLen = domain.MaxProjectLen
	// MaxAreas is how many areas a profile keeps.
	MaxAreas = 8
	// MaxAreaLen is the longest area.
	MaxAreaLen = 32
	// MaxLinkLen is Discord's limit on the URL of a button.
	MaxLinkLen = 512

	maxPathLen    = 4096
	maxHostLen    = 253
	maxLabelLen   = 63
	maxSegmentLen = 100
)

// The settings that only the file can hold, and the keys of a profile.
const (
	keyLinkHosts = "link_hosts"
	keyProjects  = "projects"
	keyPath      = "path"
	keyPrivacy   = "privacy"
	keyName      = "name"
	keyAreas     = "areas"
	keyLink      = "link"
)

const problemPrivacy = "must be minimal, standard or full"

// markup is removed from display names and areas: the characters Discord
// reads as formatting, as a mention or as a link.
const markup = "*`~|<>[]\\@"

// Profile is what the user set for one project. Profiles come from the
// user's configuration file and from nowhere else (ADR-0012).
type Profile struct {
	// Path is the project's directory, in the form paths are compared in for
	// the operating system the configuration was loaded for. It is not the
	// text the user wrote and is not for display.
	Path string
	// Privacy is the level for this project, or empty for the global one.
	Privacy domain.Privacy
	// Name is shown in place of the directory name. It may be empty.
	Name string
	// Areas are the project's main parts. It may be empty.
	Areas []string
	// Link is a validated repository link, or empty.
	Link string
}

// Settings are what applies to one session.
type Settings struct {
	Privacy domain.Privacy
	// Name is the project's display name. When it is empty, the name of the
	// working directory is the project's name.
	Name string
	// Areas belongs to the configuration. Do not modify it.
	Areas []string
	Link  string
}

// Effective returns the settings for a session whose working directory is
// cwd: those of the profile that matches, over the global ones. goos must be
// the runtime.GOOS value the configuration was loaded with.
//
// A profile matches when cwd is its path or inside it, and of several the
// longest path wins. Paths are compared as text, cleaned, without regard to
// case on Windows and macOS and to the kind of separator on Windows. Nothing
// is read from the file system, so symbolic links are not resolved.
func (c Config) Effective(goos, cwd string) Settings {
	s := Settings{Privacy: c.Privacy}
	dir, ok := normalize(goos, cwd)
	if !ok {
		return s
	}
	best := -1
	for i := range c.Projects {
		root := c.Projects[i].Path
		if within(dir, root) && (best < 0 || len(root) > len(c.Projects[best].Path)) {
			best = i
		}
	}
	if best < 0 {
		return s
	}
	p := c.Projects[best]
	if p.Privacy != "" {
		s.Privacy = p.Privacy
	}
	s.Name, s.Areas, s.Link = p.Name, p.Areas, p.Link
	return s
}

// within reports whether dir is root or inside it. Both are normalized.
func within(dir, root string) bool {
	return dir == root || strings.HasPrefix(dir, strings.TrimSuffix(root, "/")+"/")
}

// normalize puts an absolute path into the form paths are compared in:
// cleaned, with forward slashes, and in lower case where the operating
// system's file names ignore case. It reports false for a path that is not
// absolute or is too long.
func normalize(goos, p string) (string, bool) {
	if len(p) > maxPathLen {
		return "", false
	}
	volume := ""
	if goos == "windows" {
		p = strings.TrimPrefix(strings.ReplaceAll(p, `\`, "/"), "//?/")
		switch {
		case len(p) >= 3 && asciiLetter(p[0]) && p[1] == ':' && p[2] == '/':
			volume, p = p[:2], p[2:]
		case strings.HasPrefix(p, "//"):
			// A network share. Cleaning removes one of its two slashes.
			volume = "/"
		default:
			return "", false
		}
	}
	if !strings.HasPrefix(p, "/") {
		return "", false
	}
	key := volume + path.Clean(p)
	if goos == "windows" || goos == "darwin" {
		key = strings.ToLower(key)
	}
	return key, true
}

func asciiLetter(b byte) bool {
	return b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z'
}

func asciiDigit(b byte) bool { return b >= '0' && b <= '9' }

// ValidateLink checks a repository link against the rules of ADR-0012 and
// returns it in its published form, without a trailing ".git". A valid result
// validates to itself. The link must
// be https, name a host in hosts exactly, and have a path of an owner and a
// repository. A user name, password, port, query, fragment, escape or any
// further path segment makes it invalid.
func ValidateLink(raw string, hosts []string) (string, bool) {
	if len(raw) > MaxLinkLen {
		return "", false
	}
	for i := 0; i < len(raw); i++ {
		if b := raw[i]; !segmentByte(b) && b != ':' && b != '/' {
			return "", false
		}
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Opaque != "" {
		return "", false
	}
	host := strings.ToLower(u.Host)
	if !slices.Contains(hosts, host) {
		return "", false
	}
	segments := strings.Split(u.Path, "/")
	if len(segments) != 3 {
		return "", false
	}
	owner, repo := segments[1], strings.TrimSuffix(segments[2], ".git")
	// A name that still ends in ".git" would lose that ending the next time
	// the link is validated, and the host validates it again.
	if !segment(owner) || !segment(repo) || strings.HasSuffix(repo, ".git") {
		return "", false
	}
	return "https://" + host + "/" + owner + "/" + repo, true
}

// segment reports whether s can be the owner or the repository in a link.
func segment(s string) bool {
	if s == "" || s == "." || s == ".." || len(s) > maxSegmentLen {
		return false
	}
	// Every byte is one of these or a colon: ValidateLink checked.
	return !strings.Contains(s, ":")
}

func segmentByte(b byte) bool {
	return asciiLetter(b) || asciiDigit(b) || b == '-' || b == '.' || b == '_'
}

// validHost reports whether h is a host name in lower case, with no port.
func validHost(h string) bool {
	if len(h) > maxHostLen {
		return false
	}
	for _, label := range strings.Split(h, ".") {
		if label == "" || len(label) > maxLabelLen || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for i := 0; i < len(label); i++ {
			if b := label[i]; !asciiDigit(b) && b != '-' && (b < 'a' || b > 'z') {
				return false
			}
		}
	}
	return true
}

// CleanName makes a display name safe to show: one line, with control
// characters, invisible characters and markup removed, runs of spaces
// collapsed, and cut to MaxNameLen. The result is empty if nothing is left.
func CleanName(name string) string {
	return clean(name, MaxNameLen)
}

// CleanAreas cleans each area as CleanName does, to MaxAreaLen, drops those
// that are empty and those that repeat an earlier one but for case and
// spacing, and keeps the first MaxAreas. It reports whether any were left
// out for being over that number.
func CleanAreas(areas []string) (kept []string, over bool) {
	var seen []string
	for _, area := range areas {
		area = clean(area, MaxAreaLen)
		key := strings.ToLower(area)
		if area == "" || slices.Contains(seen, key) {
			continue
		}
		if len(kept) == MaxAreas {
			return kept, true
		}
		seen = append(seen, key)
		kept = append(kept, area)
	}
	return kept, false
}

// clean is the cleaning CleanName describes, to a limit in bytes.
func clean(s string, limit int) string {
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

// applyProfiles reads the settings that only the file can hold, link_hosts
// and projects, and removes them from raw. A profile that cannot be used is
// skipped, a field that cannot be used is left unset, and each gives one
// warning that names it by position and never by what the user wrote.
func applyProfiles(cfg *Config, goos string, raw map[string]json.RawMessage) []Warning {
	var warnings []Warning
	warn := func(setting, problem string) {
		warnings = append(warnings, Warning{SourceFile, setting, problem})
	}
	for i, host := range list[string](raw, keyLinkHosts, MaxLinkHosts, "must be a list of host names", warn) {
		host = strings.ToLower(host)
		switch {
		case !validHost(host):
			warn(index(keyLinkHosts, i), "must be a host name such as gitlab.com")
		case !slices.Contains(cfg.LinkHosts, host):
			cfg.LinkHosts = append(cfg.LinkHosts, host)
		}
	}
	for i, msg := range list[json.RawMessage](raw, keyProjects, MaxProfiles, "must be a list of profiles", warn) {
		label := index(keyProjects, i)
		p, ok := parseProfile(goos, msg, cfg.LinkHosts, label, warn)
		switch {
		case !ok:
		case slices.ContainsFunc(cfg.Projects, func(o Profile) bool { return o.Path == p.Path }):
			warn(label, "repeats the path of an earlier profile and was skipped")
		default:
			cfg.Projects = append(cfg.Projects, p)
		}
	}
	return warnings
}

// index names one entry of a list setting, counting from zero as JSON does.
func index(setting string, i int) string {
	return setting + "[" + strconv.Itoa(i) + "]"
}

// list takes the list setting named key out of raw, cut to limit entries.
func list[T any](raw map[string]json.RawMessage, key string, limit int, problem string, warn func(setting, problem string)) []T {
	msg, ok := raw[key]
	if !ok {
		return nil
	}
	delete(raw, key)
	items, ok := decode[[]T](msg)
	if !ok {
		warn(key, problem)
		return nil
	}
	if len(items) > limit {
		warn(key, "has more than "+strconv.Itoa(limit)+" entries, and the rest are ignored")
		items = items[:limit]
	}
	return items
}

// parseProfile reads one entry of projects. It reports false, after one
// warning, for an entry that is not an object or has no usable path.
func parseProfile(goos string, msg json.RawMessage, hosts []string, label string, warn func(setting, problem string)) (Profile, bool) {
	fields, ok := decode[map[string]json.RawMessage](msg)
	if !ok {
		warn(label, "must be an object and was skipped")
		return Profile{}, false
	}
	written, _ := decode[string](fields[keyPath])
	key, ok := normalize(goos, written)
	if !ok {
		warn(label, "must have a path that is an absolute directory and was skipped")
		return Profile{}, false
	}
	p := Profile{Path: key}
	delete(fields, keyPath)
	field := func(name string) (json.RawMessage, bool) {
		value, ok := fields[name]
		delete(fields, name)
		return value, ok
	}
	if value, ok := field(keyPrivacy); ok {
		level, _ := decode[string](value)
		if p.Privacy = domain.Privacy(level); !p.Privacy.Valid() {
			p.Privacy = ""
			warn(label+"."+keyPrivacy, problemPrivacy)
		}
	}
	if value, ok := field(keyName); ok {
		name, _ := decode[string](value)
		if p.Name = CleanName(name); p.Name == "" {
			warn(label+"."+keyName, "must be text with something to show")
		}
	}
	if value, ok := field(keyAreas); ok {
		areas, ok := decode[[]string](value)
		if !ok {
			warn(label+"."+keyAreas, "must be a list of strings")
		}
		var over bool
		if p.Areas, over = CleanAreas(areas); over {
			warn(label+"."+keyAreas, "has more than "+strconv.Itoa(MaxAreas)+" entries, and the rest are ignored")
		}
	}
	if value, ok := field(keyLink); ok {
		link, _ := decode[string](value)
		if p.Link, ok = ValidateLink(link, hosts); !ok {
			warn(label+"."+keyLink, "is not an accepted repository link and is not published")
		}
	}
	if len(fields) > 0 {
		warn(label, "has a setting that is not known")
	}
	return p, true
}
