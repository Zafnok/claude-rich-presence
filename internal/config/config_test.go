package config_test

import (
	"errors"
	"fmt"
	"io/fs"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Zafnok/claude-rich-presence/internal/config"
	"github.com/Zafnok/claude-rich-presence/internal/domain"
)

// fakeFiles is the configuration file: its content, or the error reading it.
type fakeFiles struct {
	data string
	err  error
	read []string
}

func (f *fakeFiles) ReadFile(name string) ([]byte, error) {
	f.read = append(f.read, name)
	return []byte(f.data), f.err
}

func noFile() *fakeFiles { return &fakeFiles{err: fs.ErrNotExist} }

func env(pairs ...string) func(string) string {
	m := map[string]string{"HOME": "/home/u"}
	for i := 0; i < len(pairs); i += 2 {
		m[pairs[i]] = pairs[i+1]
	}
	return func(key string) string { return m[key] }
}

func load(t *testing.T, file string, pairs ...string) (config.Config, []config.Warning) {
	t.Helper()
	files := noFile()
	if file != "" {
		files = &fakeFiles{data: file}
	}
	cfg, _, warnings := config.Load("linux", env(pairs...), files)
	return cfg, warnings
}

func isDefault(cfg config.Config) bool {
	return reflect.DeepEqual(cfg, config.Default())
}

func TestDefault(t *testing.T) {
	want := config.Config{
		Enabled:              true,
		Privacy:              domain.PrivacyStandard,
		DiscordApplicationID: config.PlaceholderApplicationID,
		IdleClearAfter:       15 * time.Minute,
		MinUpdateInterval:    15 * time.Second,
		LogLevel:             config.LogWarn,
		LinkHosts:            []string{"github.com"},
	}
	if got := config.Default(); !reflect.DeepEqual(got, want) {
		t.Errorf("Default = %+v, want %+v", got, want)
	}
	cfg, warnings := load(t, "")
	if !isDefault(cfg) || len(warnings) != 0 {
		t.Errorf("Load with nothing set = %+v, %v", cfg, warnings)
	}
}

// settingCases gives, for each setting, a file value and a different
// environment value, both valid, and how to read the setting back.
var settingCases = []struct {
	name      string
	fileJSON  string
	fileWant  any
	envValue  string
	envWant   any
	get       func(config.Config) any
	invalid   string // as an environment value and as a JSON string
	problem   string
	wrongJSON string // a JSON value of the wrong type
	wrongType string
}{
	{
		"enabled", `false`, false, "0", false,
		func(c config.Config) any { return c.Enabled },
		"maybe", "must be true or false", `"false"`, "must be true or false",
	},
	{
		"privacy", `"minimal"`, domain.PrivacyMinimal, "full", domain.PrivacyFull,
		func(c config.Config) any { return c.Privacy },
		"summary", "must be minimal, standard or full", `1`, "must be a string",
	},
	{
		"discord_application_id", `"123"`, "123", "18446744073709551615", "18446744073709551615",
		func(c config.Config) any { return c.DiscordApplicationID },
		"12a", "must be 1 to 20 digits", `123`, "must be a string",
	},
	{
		"idle_clear_after", `"0"`, time.Duration(0), "1h", time.Hour,
		func(c config.Config) any { return c.IdleClearAfter },
		"-1s", "must be a duration such as 15m, or 0 for never", `null`, "must be a string",
	},
	{
		"min_update_interval", `"4s"`, 4 * time.Second, "1m", time.Minute,
		func(c config.Config) any { return c.MinUpdateInterval },
		"soon", "must be a duration such as 15s", `15`, "must be a string",
	},
	{
		"log_level", `"debug"`, config.LogDebug, "error", config.LogError,
		func(c config.Config) any { return c.LogLevel },
		"WARN", "must be error, warn, info or debug", `true`, "must be a string",
	},
}

func TestEachSettingFromEachSource(t *testing.T) {
	for _, tc := range settingCases {
		file := fmt.Sprintf(`{%q: %s}`, tc.name, tc.fileJSON)
		key := "RICH_PRESENCE_" + strings.ToUpper(tc.name)
		t.Run(tc.name+"/file", func(t *testing.T) {
			cfg, warnings := load(t, file)
			if got := tc.get(cfg); got != tc.fileWant || len(warnings) != 0 {
				t.Errorf("got %v, %v, want %v", got, warnings, tc.fileWant)
			}
		})
		t.Run(tc.name+"/environment", func(t *testing.T) {
			cfg, warnings := load(t, "", key, " "+tc.envValue+" ")
			if got := tc.get(cfg); got != tc.envWant || len(warnings) != 0 {
				t.Errorf("got %v, %v, want %v", got, warnings, tc.envWant)
			}
		})
		t.Run(tc.name+"/environment over file", func(t *testing.T) {
			cfg, warnings := load(t, file, key, tc.envValue)
			if got := tc.get(cfg); got != tc.envWant || len(warnings) != 0 {
				t.Errorf("got %v, %v, want %v", got, warnings, tc.envWant)
			}
		})
		t.Run(tc.name+"/leaves the others alone", func(t *testing.T) {
			cfg, _ := load(t, file, key, tc.envValue)
			for _, o := range settingCases {
				if o.name != tc.name && o.get(cfg) != o.get(config.Default()) {
					t.Errorf("%s changed to %v", o.name, o.get(cfg))
				}
			}
		})
	}
}

func TestInvalidValueFallsBackWithOneWarning(t *testing.T) {
	for _, tc := range settingCases {
		key := "RICH_PRESENCE_" + strings.ToUpper(tc.name)
		def := tc.get(config.Default())
		t.Run(tc.name+"/environment", func(t *testing.T) {
			cfg, warnings := load(t, "", key, tc.invalid)
			want := []config.Warning{{Source: config.SourceEnv, Setting: tc.name, Problem: tc.problem}}
			if got := tc.get(cfg); got != def || !reflect.DeepEqual(warnings, want) {
				t.Errorf("got %v, %v, want %v, %v", got, warnings, def, want)
			}
		})
		t.Run(tc.name+"/file", func(t *testing.T) {
			cfg, warnings := load(t, fmt.Sprintf(`{%q: %q}`, tc.name, tc.invalid))
			problem := tc.problem
			if tc.name == "enabled" {
				problem = tc.wrongType
			}
			want := []config.Warning{{Source: config.SourceFile, Setting: tc.name, Problem: problem}}
			if got := tc.get(cfg); got != def || !reflect.DeepEqual(warnings, want) {
				t.Errorf("got %v, %v, want %v, %v", got, warnings, def, want)
			}
		})
		t.Run(tc.name+"/file, wrong type", func(t *testing.T) {
			cfg, warnings := load(t, fmt.Sprintf(`{%q: %s}`, tc.name, tc.wrongJSON))
			want := []config.Warning{{Source: config.SourceFile, Setting: tc.name, Problem: tc.wrongType}}
			if got := tc.get(cfg); got != def || !reflect.DeepEqual(warnings, want) {
				t.Errorf("got %v, %v, want %v, %v", got, warnings, def, want)
			}
		})
		t.Run(tc.name+"/invalid environment keeps the file value", func(t *testing.T) {
			cfg, warnings := load(t, fmt.Sprintf(`{%q: %s}`, tc.name, tc.fileJSON), key, tc.invalid)
			if got := tc.get(cfg); got != tc.fileWant || len(warnings) != 1 || warnings[0].Source != config.SourceEnv {
				t.Errorf("got %v, %v, want %v and one environment warning", got, warnings, tc.fileWant)
			}
		})
	}
}

func TestWarningsNeverEchoTheValue(t *testing.T) {
	long := strings.Repeat("secret", 500)
	file := fmt.Sprintf(`{"privacy": %q, "log_level": %q, %q: 1}`, long, long, long)
	_, warnings := load(t, file,
		"RICH_PRESENCE_DISCORD_APPLICATION_ID", long,
		"RICH_PRESENCE_IDLE_CLEAR_AFTER", long,
		"RICH_PRESENCE_MIN_UPDATE_INTERVAL", long,
		"RICH_PRESENCE_ENABLED", long,
	)
	if len(warnings) != 7 {
		t.Fatalf("got %d warnings, want 7: %v", len(warnings), warnings)
	}
	for _, w := range warnings {
		if text := w.String(); len(text) > 120 {
			t.Errorf("warning is %d bytes long: %.80s", len(text), text)
		}
	}
}

func TestWarningString(t *testing.T) {
	w := config.Warning{Source: config.SourceFile, Setting: "privacy", Problem: "must be minimal, standard or full"}
	if got, want := w.String(), "file: privacy must be minimal, standard or full"; got != want {
		t.Errorf("String = %q, want %q", got, want)
	}
}

func TestApplicationIDLength(t *testing.T) {
	cases := []struct {
		value string
		valid bool
	}{
		{"1", true},
		{strings.Repeat("9", 20), true},
		{strings.Repeat("9", 21), false},
		{"١٢٣", false},
		{"-1", false},
		{"1.5", false},
	}
	for _, tc := range cases {
		cfg, warnings := load(t, "", "RICH_PRESENCE_DISCORD_APPLICATION_ID", tc.value)
		if tc.valid != (cfg.DiscordApplicationID == tc.value) || tc.valid != (len(warnings) == 0) {
			t.Errorf("%q: got %q, %v", tc.value, cfg.DiscordApplicationID, warnings)
		}
	}
	// An empty id in the file is a value, and an invalid one.
	cfg, warnings := load(t, `{"discord_application_id": ""}`)
	if cfg.DiscordApplicationID != config.PlaceholderApplicationID || len(warnings) != 1 {
		t.Errorf("empty id in the file: got %q, %v", cfg.DiscordApplicationID, warnings)
	}
}

func TestMinUpdateIntervalIsRaisedToTheFloor(t *testing.T) {
	want := "is below the floor of 4s and was raised to it"
	for _, value := range []string{"3999ms", "0", "-10s"} {
		cfg, warnings := load(t, "", "RICH_PRESENCE_MIN_UPDATE_INTERVAL", value)
		wantW := []config.Warning{{Source: config.SourceEnv, Setting: "min_update_interval", Problem: want}}
		if cfg.MinUpdateInterval != config.MinUpdateFloor || !reflect.DeepEqual(warnings, wantW) {
			t.Errorf("environment %q: got %v, %v", value, cfg.MinUpdateInterval, warnings)
		}
		cfg, warnings = load(t, fmt.Sprintf(`{"min_update_interval": %q}`, value))
		wantW[0].Source = config.SourceFile
		if cfg.MinUpdateInterval != config.MinUpdateFloor || !reflect.DeepEqual(warnings, wantW) {
			t.Errorf("file %q: got %v, %v", value, cfg.MinUpdateInterval, warnings)
		}
	}
	if config.MinUpdateFloor != 4*time.Second {
		t.Errorf("floor = %v", config.MinUpdateFloor)
	}
}

func TestUnsubstitutedEnvironmentValueIsNotSet(t *testing.T) {
	for _, tc := range settingCases {
		key := "RICH_PRESENCE_" + strings.ToUpper(tc.name)
		for _, value := range []string{"${user_config." + tc.name + "}", "", "   "} {
			cfg, warnings := load(t, "", key, value)
			if !isDefault(cfg) || len(warnings) != 0 {
				t.Errorf("%s=%q: got %+v, %v", key, value, cfg, warnings)
			}
		}
		// The file value still applies underneath.
		cfg, warnings := load(t, fmt.Sprintf(`{%q: %s}`, tc.name, tc.fileJSON), key, "${user_config."+tc.name+"}")
		if got := tc.get(cfg); got != tc.fileWant || len(warnings) != 0 {
			t.Errorf("%s over a file: got %v, %v", key, got, warnings)
		}
	}
}

func TestBadFileYieldsDefaultsAndOneWarning(t *testing.T) {
	cases := []struct {
		name    string
		files   *fakeFiles
		problem string
	}{
		{"unreadable", &fakeFiles{data: `{"privacy": "full"}`, err: fs.ErrPermission}, "cannot be read"},
		{"other read error", &fakeFiles{err: errors.New("disk on fire")}, "cannot be read"},
		{"empty", &fakeFiles{data: ""}, "is not a JSON object"},
		{"truncated", &fakeFiles{data: `{"privacy": "full"`}, "is not a JSON object"},
		{"array", &fakeFiles{data: `["privacy"]`}, "is not a JSON object"},
		{"string", &fakeFiles{data: `"privacy"`}, "is not a JSON object"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg, _, warnings := config.Load("linux", env(), tc.files)
			want := []config.Warning{{Source: config.SourceFile, Setting: "config.json", Problem: tc.problem}}
			if !isDefault(cfg) || !reflect.DeepEqual(warnings, want) {
				t.Errorf("got %+v, %v, want defaults and %v", cfg, warnings, want)
			}
		})
	}
	t.Run("the environment still applies", func(t *testing.T) {
		cfg, _, warnings := config.Load("linux", env("RICH_PRESENCE_PRIVACY", "minimal"), &fakeFiles{err: fs.ErrPermission})
		if cfg.Privacy != domain.PrivacyMinimal || len(warnings) != 1 {
			t.Errorf("got %v, %v", cfg.Privacy, warnings)
		}
	})
}

func TestMissingFileIsNormal(t *testing.T) {
	for _, err := range []error{fs.ErrNotExist, &fs.PathError{Op: "open", Path: "x", Err: fs.ErrNotExist}} {
		cfg, _, warnings := config.Load("linux", env(), &fakeFiles{err: err})
		if !isDefault(cfg) || len(warnings) != 0 {
			t.Errorf("%v: got %+v, %v", err, cfg, warnings)
		}
	}
	// A byte order mark before the JSON is skipped.
	cfg, warnings := load(t, "ï»¿"+`{"privacy": "full"}`)
	if cfg.Privacy != domain.PrivacyFull || len(warnings) != 0 {
		t.Errorf("byte order mark: got %+v, %v", cfg, warnings)
	}
	// JSON null holds no settings.
	cfg, warnings = load(t, "null")
	if !isDefault(cfg) || len(warnings) != 0 {
		t.Errorf("null: got %+v, %v", cfg, warnings)
	}
}

func TestUnknownKeysWarnEachAndAreIgnored(t *testing.T) {
	long := strings.Repeat("k", 41)
	cfg, warnings := load(t, `{"zeta": 1, "privacy": "full", "Privacy": "minimal", "alpha": {"x": []}, "`+long+`": null}`)
	if cfg.Privacy != domain.PrivacyFull {
		t.Errorf("privacy = %v", cfg.Privacy)
	}
	want := []config.Warning{
		{Source: config.SourceFile, Setting: "Privacy", Problem: "is not a known setting"},
		{Source: config.SourceFile, Setting: "alpha", Problem: "is not a known setting"},
		{Source: config.SourceFile, Setting: strings.Repeat("k", 40) + "...", Problem: "is not a known setting"},
		{Source: config.SourceFile, Setting: "zeta", Problem: "is not a known setting"},
	}
	if !reflect.DeepEqual(warnings, want) {
		t.Errorf("got %v, want %v", warnings, want)
	}
}

func TestEveryProblemIsReported(t *testing.T) {
	_, warnings := load(t, `{"privacy": "loud", "log_level": 3, "extra": 1}`,
		"RICH_PRESENCE_ENABLED", "perhaps", "RICH_PRESENCE_MIN_UPDATE_INTERVAL", "1s")
	want := []config.Warning{
		{Source: config.SourceFile, Setting: "privacy", Problem: "must be minimal, standard or full"},
		{Source: config.SourceFile, Setting: "log_level", Problem: "must be a string"},
		{Source: config.SourceFile, Setting: "extra", Problem: "is not a known setting"},
		{Source: config.SourceEnv, Setting: "enabled", Problem: "must be true or false"},
		{Source: config.SourceEnv, Setting: "min_update_interval", Problem: "is below the floor of 4s and was raised to it"},
	}
	if !reflect.DeepEqual(warnings, want) {
		t.Errorf("got %v, want %v", warnings, want)
	}
}

func TestLoadReadsTheResolvedFile(t *testing.T) {
	files := noFile()
	_, dirs, _ := config.Load("windows", env("USERPROFILE", `C:\Users\u`), files)
	want := `C:\Users\u\.rich-presence\config.json`
	if dirs.File != want || !reflect.DeepEqual(files.read, []string{want}) {
		t.Errorf("dirs.File = %q, read %q, want %q", dirs.File, files.read, want)
	}
}

func TestLoadWithoutAHome(t *testing.T) {
	files := &fakeFiles{data: `{"privacy": "full"}`}
	getenv := func(key string) string {
		if key == "RICH_PRESENCE_LOG_LEVEL" {
			return "info"
		}
		return ""
	}
	cfg, dirs, warnings := config.Load("linux", getenv, files)
	want := config.Default()
	want.LogLevel = config.LogInfo
	wantW := []config.Warning{{Source: config.SourceEnv, Setting: "home directory", Problem: "is not set, so the configuration file is not read"}}
	if !reflect.DeepEqual(cfg, want) || dirs != (config.Dirs{}) || !reflect.DeepEqual(warnings, wantW) || len(files.read) != 0 {
		t.Errorf("got %+v, %+v, %v, read %v", cfg, dirs, warnings, files.read)
	}
}

func TestLogLevelValid(t *testing.T) {
	for _, l := range []config.LogLevel{config.LogError, config.LogWarn, config.LogInfo, config.LogDebug} {
		if !l.Valid() {
			t.Errorf("%q is not valid", l)
		}
	}
	for _, l := range []config.LogLevel{"", "trace", "Warn"} {
		if l.Valid() {
			t.Errorf("%q is valid", l)
		}
	}
}

func FuzzLoad(f *testing.F) {
	f.Add(`{"enabled": false, "privacy": "full", "idle_clear_after": "15m"}`, "true", "${user_config.privacy}", "linux")
	f.Add(`{"min_update_interval": "1s", "unknown": [1, {"a": null}]}`, "", "9223372036854775807h", "windows")
	f.Add(`{"enabled": null, "log_level": 1e999}`, "\x00", "-0", "darwin")
	f.Add("\xff{", "1", "1ns", "")
	f.Add(`{"": "", "privacy": "\ud800"}`, " ", "١s", "plan9")
	f.Fuzz(func(t *testing.T, file, value, duration, goos string) {
		getenv := func(key string) string {
			switch key {
			case "HOME", "USERPROFILE", "XDG_CONFIG_HOME":
				return value
			case "RICH_PRESENCE_IDLE_CLEAR_AFTER", "RICH_PRESENCE_MIN_UPDATE_INTERVAL":
				return duration
			}
			return value
		}
		cfg, _, _ := config.Load(goos, getenv, &fakeFiles{data: file})
		if !cfg.Privacy.Valid() || !cfg.LogLevel.Valid() || cfg.IdleClearAfter < 0 ||
			cfg.MinUpdateInterval < config.MinUpdateFloor || cfg.DiscordApplicationID == "" {
			t.Errorf("Load returned an invalid configuration: %+v", cfg)
		}
	})
}
