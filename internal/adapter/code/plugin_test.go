package code_test

import (
	"encoding/json"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Zafnok/claude-rich-presence/internal/adapter/code"
	"github.com/Zafnok/claude-rich-presence/internal/domain"
)

// The files of the Claude Code plugin and what they must agree with, from
// this package's directory.
const (
	repoRoot        = "../../.."
	pluginDir       = repoRoot + "/plugin"
	pluginManifest  = pluginDir + "/.claude-plugin/plugin.json"
	hookFile        = pluginDir + "/hooks/hooks.json"
	marketplaceFile = repoRoot + "/.claude-plugin/marketplace.json"
	bundleManifest  = repoRoot + "/extension/manifest.json"

	// minimumClaudeCode is the oldest Claude Code that loads the plugin: the
	// first with fixed-choice options in userConfig.
	minimumClaudeCode = "Claude Code 2.1.271 or later"
)

func readText(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.FromSlash(name))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// readJSON decodes a file strictly, so that a field the test does not know
// fails it.
func readJSON(t *testing.T, name string, into any) {
	t.Helper()
	dec := json.NewDecoder(strings.NewReader(readText(t, name)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(into); err != nil {
		t.Fatalf("%s: %v", name, err)
	}
}

type pluginOption struct {
	Type        string   `json:"type"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Options     []string `json:"options"`
	Default     string   `json:"default"`
}

type plugin struct {
	Name        string `json:"name"`
	DisplayName string `json:"displayName"`
	Version     string `json:"version"`
	Description string `json:"description"`
	Author      struct {
		Name string `json:"name"`
	} `json:"author"`
	Homepage   string                  `json:"homepage"`
	Repository string                  `json:"repository"`
	License    string                  `json:"license"`
	Keywords   []string                `json:"keywords"`
	McpServers string                  `json:"mcpServers"`
	UserConfig map[string]pluginOption `json:"userConfig"`
}

// bundle is the part of the bundle's manifest the plugin depends on.
type bundle struct {
	Name       string `json:"name"`
	Repository struct {
		URL string `json:"url"`
	} `json:"repository"`
	UserConfig map[string]json.RawMessage `json:"user_config"`
}

func readPlugin(t *testing.T) plugin {
	t.Helper()
	var p plugin
	readJSON(t, pluginManifest, &p)
	return p
}

func readBundle(t *testing.T) bundle {
	t.Helper()
	var b bundle
	if err := json.Unmarshal([]byte(readText(t, bundleManifest)), &b); err != nil {
		t.Fatal(err)
	}
	return b
}

type hookHandler struct {
	Type    string            `json:"type"`
	Server  string            `json:"server"`
	Tool    string            `json:"tool"`
	Input   map[string]string `json:"input"`
	Timeout *int              `json:"timeout"`
}

type hookGroup struct {
	Matcher string        `json:"matcher"`
	Hooks   []hookHandler `json:"hooks"`
}

// TestHookFileMatchesTheAllowlist proves the plugin's hook file and the
// adapter's allowlist identical: the same events, and for each the literal
// event name and exactly the allowlisted fields. It also holds the hook file
// to rules 3 and 6 to 9 of ADR-0007.
func TestHookFileMatchesTheAllowlist(t *testing.T) {
	var file struct {
		Description string                 `json:"description"`
		Hooks       map[string][]hookGroup `json:"hooks"`
	}
	readJSON(t, hookFile, &file)
	server := "plugin:" + readPlugin(t).Name + ":" + readBundle(t).Name

	var want []string
	for _, event := range code.HookEvents() {
		want = append(want, event.Name)
		groups := file.Hooks[event.Name]
		if len(groups) != 1 || len(groups[0].Hooks) != 1 {
			t.Errorf("%s: want exactly one handler, got %+v", event.Name, groups)
			continue
		}
		matcher := ""
		if event.Name == "SessionStart" {
			// So that it does not match at launch, where the hook is skipped
			// and the skip is shown as an error.
			matcher = "clear|compact"
		}
		if groups[0].Matcher != matcher {
			t.Errorf("%s: matcher %q, want %q", event.Name, groups[0].Matcher, matcher)
		}
		handler := groups[0].Hooks[0]
		if handler.Type != "mcp_tool" || handler.Tool != "presence_event" {
			t.Errorf("%s: calls %s %q", event.Name, handler.Type, handler.Tool)
		}
		if handler.Server != server {
			t.Errorf("%s: server %q, want %q", event.Name, handler.Server, server)
		}
		if handler.Timeout == nil || *handler.Timeout != 2 {
			t.Errorf("%s: the timeout must be set to 2 seconds", event.Name)
		}
		input := map[string]string{"event": event.Name}
		for _, field := range event.Fields {
			input[field] = "${" + field + "}"
		}
		if !maps.Equal(handler.Input, input) {
			t.Errorf("%s: input %v, want %v", event.Name, handler.Input, input)
		}
	}
	got := slices.Sorted(maps.Keys(file.Hooks))
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Errorf("hooked events %v, want %v", got, want)
	}
	if _, ok := file.Hooks["SessionEnd"]; ok {
		t.Error("SessionEnd must not be hooked: the server is stopped before it runs")
	}
}

func TestPluginManifest(t *testing.T) {
	p := readPlugin(t)
	b := readBundle(t)
	version := strings.TrimSpace(readText(t, repoRoot+"/VERSION"))
	if p.Name != "rich-presence" {
		t.Errorf("name %q", p.Name)
	}
	if p.Version != version {
		t.Errorf("version %q, want %q from the VERSION file", p.Version, version)
	}
	if p.Repository != b.Repository.URL || p.License != "MIT" {
		t.Errorf("repository %q, license %q", p.Repository, p.License)
	}
	// A new bundle URL takes effect only together with a new version.
	if want := p.Repository + "/releases/download/v" + version + "/rich-presence.mcpb"; p.McpServers != want {
		t.Errorf("mcpServers %q, want %q", p.McpServers, want)
	}
	for _, text := range []string{"not affiliated with", minimumClaudeCode} {
		if !strings.Contains(p.Description, text) {
			t.Errorf("the description lacks %q", text)
		}
	}
	if !strings.Contains(readText(t, repoRoot+"/README.md"), minimumClaudeCode) {
		t.Errorf("README.md lacks %q", minimumClaudeCode)
	}

	// A plugin option reaches the server through the bundle's reference of
	// the same name, so the two sets of names must be one.
	if got, want := slices.Sorted(maps.Keys(p.UserConfig)), slices.Sorted(maps.Keys(b.UserConfig)); !slices.Equal(got, want) {
		t.Errorf("userConfig has %v, the bundle has %v", got, want)
	}
	for key, option := range p.UserConfig {
		if option.Type != "string" || option.Title == "" || option.Description == "" {
			t.Errorf("userConfig.%s: %+v", key, option)
		}
	}
	privacy := p.UserConfig["privacy"]
	for _, level := range privacy.Options {
		if !domain.Privacy(level).Valid() {
			t.Errorf("privacy option %q is not a privacy level", level)
		}
	}
	if len(privacy.Options) != 3 || privacy.Default != string(domain.PrivacyStandard) {
		t.Errorf("privacy: options %v, default %q", privacy.Options, privacy.Default)
	}
}

func TestMarketplaceListsThePlugin(t *testing.T) {
	var m struct {
		Name  string `json:"name"`
		Owner struct {
			Name string `json:"name"`
		} `json:"owner"`
		Metadata struct {
			Description string `json:"description"`
		} `json:"metadata"`
		Plugins []struct {
			Name        string `json:"name"`
			Source      string `json:"source"`
			Description string `json:"description"`
		} `json:"plugins"`
	}
	readJSON(t, marketplaceFile, &m)
	if len(m.Plugins) != 1 || m.Plugins[0].Source != "./plugin" || m.Plugins[0].Name != readPlugin(t).Name {
		t.Errorf("plugins: %+v", m.Plugins)
	}
	// Strict validation refuses a marketplace with no description.
	if m.Metadata.Description == "" || m.Owner.Name == "" {
		t.Errorf("metadata.description and owner.name must be set")
	}
}

// TestPluginIsJSONAndMarkdownOnly checks that the plugin directory holds no
// executable, no script and no top-level bin directory (ADR-0007, rules 1
// and 5), and that the cache Claude Code writes there is ignored (rule 11).
func TestPluginIsJSONAndMarkdownOnly(t *testing.T) {
	root := filepath.FromSlash(pluginDir)
	files := 0
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		switch {
		case entry.IsDir() && entry.Name() == ".mcpb-cache":
			return filepath.SkipDir
		case entry.IsDir() && rel == "bin":
			t.Error("the plugin has a top-level bin directory")
		case entry.IsDir():
		case filepath.Ext(path) == ".json" || filepath.Ext(path) == ".md":
			files++
		default:
			t.Errorf("%s is neither JSON nor Markdown", rel)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	// The manifest, the hook file and the three skills.
	if files != 5 {
		t.Errorf("found %d files, want 5", files)
	}
	if !slices.Contains(strings.Split(readText(t, repoRoot+"/.gitignore"), "\n"), "plugin/.mcpb-cache/") {
		t.Error(".gitignore does not ignore plugin/.mcpb-cache/")
	}
}
