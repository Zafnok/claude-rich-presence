package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"io/fs"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Zafnok/claude-rich-presence/internal/domain"
)

// PlaceholderApplicationID stands in for the project's Discord application id
// until CRP-003 records the real one in ADR-0010. Discord rejects it. An
// application id is public, not a secret.
const PlaceholderApplicationID = "0"

// MinUpdateFloor is the shortest allowed interval between activity updates.
const MinUpdateFloor = 4 * time.Second

// EnvPrefix begins the name of every environment variable that sets a
// setting. The rest is the setting's name in upper case.
const EnvPrefix = "RICH_PRESENCE_"

// unsubstituted marks an environment value Claude Desktop passed without
// filling it in: the user left the extension setting empty, or this is the
// first start after install.
const unsubstituted = "${user_config."

// LogLevel is the least severe message that is logged.
type LogLevel string

// The log levels.
const (
	LogError LogLevel = "error"
	LogWarn  LogLevel = "warn"
	LogInfo  LogLevel = "info"
	LogDebug LogLevel = "debug"
)

// Valid reports whether l is a known log level.
func (l LogLevel) Valid() bool {
	return l == LogError || l == LogWarn || l == LogInfo || l == LogDebug
}

// Config is the validated configuration. Every field always holds a usable
// value.
type Config struct {
	Enabled              bool
	Privacy              domain.Privacy
	DiscordApplicationID string
	// IdleClearAfter is how long every session must be idle before the
	// activity is cleared. Zero means never.
	IdleClearAfter    time.Duration
	MinUpdateInterval time.Duration
	LogLevel          LogLevel
}

// Default returns the configuration used when nothing is set.
func Default() Config {
	return Config{
		Enabled:              true,
		Privacy:              domain.PrivacyStandard,
		DiscordApplicationID: PlaceholderApplicationID,
		IdleClearAfter:       15 * time.Minute,
		MinUpdateInterval:    15 * time.Second,
		LogLevel:             LogWarn,
	}
}

// The sources a warning can name.
const (
	SourceFile = "file"
	SourceEnv  = "environment"
)

// ProblemUnknownSetting is the Problem of a warning about a key in the file
// that is not a setting. Only in that warning is Setting text the user
// wrote, and not a name this package chose.
const ProblemUnknownSetting = "is not a known setting"

// Warning is one problem found while loading. It never carries the rejected
// value, which could be long or private.
type Warning struct {
	// Source is SourceFile or SourceEnv.
	Source string
	// Setting is the setting's name, or what else the problem is about.
	Setting string
	Problem string
}

func (w Warning) String() string {
	return w.Source + ": " + w.Setting + " " + w.Problem
}

// FileReader reads the configuration file. A missing file is reported with an
// error that matches fs.ErrNotExist.
type FileReader interface {
	ReadFile(name string) ([]byte, error)
}

// Load builds the configuration from defaults, then the file, then the
// environment. It cannot fail: each problem becomes a warning, and the
// setting keeps the value it had from the sources below.
func Load(goos string, getenv func(string) string, files FileReader) (Config, Dirs, []Warning) {
	cfg := Default()
	var warnings []Warning
	dirs, err := ResolveDirs(goos, getenv)
	if err != nil {
		warnings = append(warnings, Warning{SourceEnv, "home directory", "is not set, so the configuration file is not read"})
	} else {
		warnings = applyFile(&cfg, files, dirs.File)
	}
	return cfg, dirs, append(warnings, applyEnv(&cfg, getenv)...)
}

// setting is one row of the settings table: its name, the JSON type it has in
// the file, and how its text form is validated and stored. set returns the
// problem, or the empty string.
type setting struct {
	name     string
	jsonBool bool
	set      func(c *Config, value string) string
}

var settings = []setting{
	{"enabled", true, func(c *Config, v string) string {
		b, err := strconv.ParseBool(v)
		if err != nil {
			return "must be true or false"
		}
		c.Enabled = b
		return ""
	}},
	{"privacy", false, func(c *Config, v string) string {
		if !domain.Privacy(v).Valid() {
			return "must be minimal, standard or full"
		}
		c.Privacy = domain.Privacy(v)
		return ""
	}},
	{"discord_application_id", false, func(c *Config, v string) string {
		if !numeric(v) {
			return "must be 1 to 20 digits"
		}
		c.DiscordApplicationID = v
		return ""
	}},
	{"idle_clear_after", false, func(c *Config, v string) string {
		d, err := time.ParseDuration(v)
		if err != nil || d < 0 {
			return "must be a duration such as 15m, or 0 for never"
		}
		c.IdleClearAfter = d
		return ""
	}},
	{"min_update_interval", false, func(c *Config, v string) string {
		d, err := time.ParseDuration(v)
		if err != nil {
			return "must be a duration such as 15s"
		}
		if d < MinUpdateFloor {
			c.MinUpdateInterval = MinUpdateFloor
			return "is below the floor of " + MinUpdateFloor.String() + " and was raised to it"
		}
		c.MinUpdateInterval = d
		return ""
	}},
	{"log_level", false, func(c *Config, v string) string {
		if !LogLevel(v).Valid() {
			return "must be error, warn, info or debug"
		}
		c.LogLevel = LogLevel(v)
		return ""
	}},
}

// numeric reports whether v looks like a Discord id: decimal digits, no
// longer than an unsigned 64-bit number.
func numeric(v string) bool {
	if v == "" || len(v) > 20 {
		return false
	}
	for _, r := range v {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func applyFile(cfg *Config, files FileReader, path string) []Warning {
	data, err := files.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return []Warning{{SourceFile, FileName, "cannot be read"}}
	}
	// Some Windows editors begin a UTF-8 file with a byte order mark.
	data = bytes.TrimPrefix(data, []byte("ï»¿"))
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return []Warning{{SourceFile, FileName, "is not a JSON object"}}
	}
	var warnings []Warning
	for _, s := range settings {
		msg, ok := raw[s.name]
		if !ok {
			continue
		}
		delete(raw, s.name)
		if problem := s.setJSON(cfg, msg); problem != "" {
			warnings = append(warnings, Warning{SourceFile, s.name, problem})
		}
	}
	unknown := make([]string, 0, len(raw))
	for key := range raw {
		unknown = append(unknown, key)
	}
	sort.Strings(unknown)
	for _, key := range unknown {
		warnings = append(warnings, Warning{SourceFile, clip(key), ProblemUnknownSetting})
	}
	return warnings
}

func (s setting) setJSON(cfg *Config, msg json.RawMessage) string {
	if s.jsonBool {
		b, ok := decode[bool](msg)
		if !ok {
			return "must be true or false"
		}
		return s.set(cfg, strconv.FormatBool(b))
	}
	v, ok := decode[string](msg)
	if !ok {
		return "must be a string"
	}
	return s.set(cfg, v)
}

// decode reads a JSON value of exactly type T. JSON null is not one.
func decode[T any](msg json.RawMessage) (T, bool) {
	var p *T
	if err := json.Unmarshal(msg, &p); err != nil || p == nil {
		var zero T
		return zero, false
	}
	return *p, true
}

// clip shortens an unknown key so that a warning stays short.
func clip(key string) string {
	const limit = 40
	runes := []rune(key)
	if len(runes) <= limit {
		return key
	}
	return string(runes[:limit]) + "..."
}

func applyEnv(cfg *Config, getenv func(string) string) []Warning {
	var warnings []Warning
	for _, s := range settings {
		value := strings.TrimSpace(getenv(EnvPrefix + strings.ToUpper(s.name)))
		if value == "" || strings.Contains(value, unsubstituted) {
			continue
		}
		if problem := s.set(cfg, value); problem != "" {
			warnings = append(warnings, Warning{SourceEnv, s.name, problem})
		}
	}
	return warnings
}
