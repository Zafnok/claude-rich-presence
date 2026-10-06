package main

import (
	"archive/zip"
	"bytes"
	"debug/elf"
	"debug/macho"
	"debug/pe"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"regexp"
	"slices"
	"strings"
	"time"
)

// versionToken stands for the version in the manifest in the repository. The
// version has one source, the VERSION file, and build writes it in.
const versionToken = "@VERSION@"

// dirnamePrefix begins a path in the manifest that is relative to the
// extracted bundle.
const dirnamePrefix = "${__dirname}/"

// serverName is the name ADR-0007 fixes. It is the last segment of the
// address the plugin's hooks use, and ADR-0010 keeps the brand out of it.
const serverName = "presence"

// manifestVersion is the MCPB manifest format this tool writes and checks.
const manifestVersion = "0.3"

// The files of the bundle that are not binaries.
const (
	manifestPath = "manifest.json"
	iconPath     = "icon.png"
	licensePath  = "LICENSE.md"
	noticesPath  = "THIRD-PARTY-NOTICES.md"
)

// serverBinary describes one server binary of the bundle.
type serverBinary struct {
	// platform is the manifest's name for the operating system.
	platform string
	// path is where the binary sits in the archive.
	path string
	// source is the build flag that names the file.
	source string
}

// binaries are the three servers. The manifest selects among them by
// operating system and not by CPU architecture (ADR-0007), so the Mac binary
// is universal and Linux ships amd64 only.
var binaries = []serverBinary{
	{"win32", "server/rich-presence.exe", "windows"},
	{"darwin", "server/rich-presence-darwin", "darwin"},
	{"linux", "server/rich-presence-linux", "linux"},
}

// sourceNames are the build flags that name an input file, in the order they
// are reported.
var sourceNames = []string{"manifest", "icon", "license", "notices", "windows", "darwin", "linux"}

// serverArgs are the arguments the manifest gives every binary.
var serverArgs = []string{"mcp"}

// fixedTime is the modification time of every entry. The earliest time a zip
// archive can hold, so that the archive does not depend on the clock.
var fixedTime = time.Date(1980, 1, 1, 0, 0, 0, 0, time.UTC)

// semver is the shape the manifest's version must have.
var semver = regexp.MustCompile(`^\d+\.\d+\.\d+(-[0-9A-Za-z.-]+)?$`)

// placeholder finds a user setting reference in an environment value.
var placeholder = regexp.MustCompile(`\$\{user_config\.([^}]*)\}`)

// entry is one file of the archive.
type entry struct {
	name string
	data []byte
	mode fs.FileMode
}

// assemble builds the archive from the input files, keyed by the build flag
// that names each. It returns either the archive or the problems found: what
// it built is checked before it is returned.
func assemble(version string, files map[string][]byte) ([]byte, []string) {
	manifest, problem := substituteVersion(files["manifest"], version)
	if problem != "" {
		return nil, []string{problem}
	}
	entries := []entry{
		{manifestPath, manifest, 0o644},
		{iconPath, files["icon"], 0o644},
		{licensePath, files["license"], 0o644},
		{noticesPath, files["notices"], 0o644},
	}
	for _, b := range binaries {
		entries = append(entries, entry{b.path, files[b.source], 0o755})
	}
	var archive bytes.Buffer
	// Writing to memory cannot fail. Anything wrong with the result shows
	// as a problem below.
	_ = writeZip(&archive, entries)
	if problems := check(archive.Bytes(), version); len(problems) > 0 {
		return nil, problems
	}
	return archive.Bytes(), nil
}

// writeZip writes the entries in the order given. Each has the fixed time and
// its mode set explicitly, because archives do not otherwise keep Unix
// permissions.
func writeZip(w io.Writer, entries []entry) error {
	var first error
	note := func(err error) {
		if first == nil {
			first = err
		}
	}
	zw := zip.NewWriter(w)
	for _, e := range entries {
		header := &zip.FileHeader{Name: e.name, Method: zip.Deflate, Modified: fixedTime}
		header.SetMode(e.mode)
		fw, err := zw.CreateHeader(header)
		note(err)
		if err == nil {
			_, err = fw.Write(e.data)
			note(err)
		}
	}
	note(zw.Close())
	return first
}

// check reports every way the archive falls short of what ADR-0007 and the
// MCPB manifest specification require. An empty result means it passes.
func check(data []byte, version string) []string {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return []string{"not a zip archive: " + err.Error()}
	}
	c := checker{files: map[string]*zip.File{}}
	for _, f := range zr.File {
		c.files[f.Name] = f
	}
	c.layout()
	c.manifest(version)
	c.binaries()
	return c.problems
}

// checker accumulates the problems found in one archive.
type checker struct {
	files    map[string]*zip.File
	problems []string
	// referenced are the archive paths the manifest names.
	referenced []string
}

func (c *checker) problem(format string, args ...any) {
	c.problems = append(c.problems, fmt.Sprintf(format, args...))
}

// read returns the contents of an entry, recording a problem if it cannot be
// read. An entry that is missing is reported by the caller.
func (c *checker) read(name string) ([]byte, bool) {
	f, ok := c.files[name]
	if !ok {
		return nil, false
	}
	rc, err := f.Open()
	if err == nil {
		var data []byte
		data, err = io.ReadAll(rc)
		rc.Close()
		if err == nil {
			return data, true
		}
	}
	c.problem("%s cannot be read: %v", name, err)
	return nil, false
}

// layout checks that the archive holds the fixed set of files and no other.
func (c *checker) layout() {
	want := []string{manifestPath, iconPath, licensePath, noticesPath}
	for _, b := range binaries {
		want = append(want, b.path)
	}
	for _, name := range want {
		if _, ok := c.files[name]; !ok {
			c.problem("%s is missing", name)
		}
	}
	var extra []string
	for name := range c.files {
		if !slices.Contains(want, name) {
			extra = append(extra, name)
		}
	}
	slices.Sort(extra)
	for _, name := range extra {
		c.problem("%s is not part of the bundle layout", name)
	}
}

// override is a platform's replacement for the server's launch settings.
type override struct {
	Command string            `json:"command"`
	Args    []string          `json:"args"`
	Env     map[string]string `json:"env"`
}

// manifest is the part of the MCPB manifest, version 0.3, that this project
// uses. Fields outside it are rejected, which catches a misspelt key.
type manifest struct {
	ManifestVersion string `json:"manifest_version"`
	Name            string `json:"name"`
	DisplayName     string `json:"display_name"`
	Version         string `json:"version"`
	Description     string `json:"description"`
	Author          struct {
		Name string `json:"name"`
	} `json:"author"`
	License    string `json:"license"`
	Repository struct {
		Type string `json:"type"`
		URL  string `json:"url"`
	} `json:"repository"`
	Icon   string `json:"icon"`
	Server struct {
		Type       string `json:"type"`
		EntryPoint string `json:"entry_point"`
		McpConfig  struct {
			override
			PlatformOverrides map[string]override `json:"platform_overrides"`
		} `json:"mcp_config"`
	} `json:"server"`
	UserConfig map[string]struct {
		Type        string `json:"type"`
		Title       string `json:"title"`
		Description string `json:"description"`
		Default     any    `json:"default"`
		Required    bool   `json:"required"`
	} `json:"user_config"`
	Tools          json.RawMessage `json:"tools"`
	ToolsGenerated bool            `json:"tools_generated"`
	Compatibility  struct {
		Platforms []string `json:"platforms"`
	} `json:"compatibility"`
}

// userConfigTypes are the types the specification gives a setting.
var userConfigTypes = []string{"string", "number", "boolean", "directory", "file"}

// manifest checks manifest.json against the specification and against the
// decisions of ADR-0007 and ADR-0010.
func (c *checker) manifest(version string) {
	data, ok := c.read(manifestPath)
	if !ok {
		return
	}
	var m manifest
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&m); err != nil {
		c.problem("manifest.json is not valid: %v", err)
		return
	}
	c.identity(&m, version)
	c.server(&m)
	c.userConfig(&m)
	if !m.ToolsGenerated {
		c.problem("manifest.json must set tools_generated, so that tools need not be listed")
	}
	if m.Tools != nil {
		c.problem("manifest.json must not list tools")
	}
	if want := []string{"darwin", "linux", "win32"}; !slices.Equal(m.Compatibility.Platforms, want) {
		c.problem("compatibility.platforms must be %v, not %v", want, m.Compatibility.Platforms)
	}
	c.referenced = append(c.referenced, m.Icon)
	for _, name := range c.referenced {
		if _, ok := c.files[name]; !ok {
			c.problem("manifest.json names %s, which is not in the bundle", name)
		}
	}
}

// identity checks the fields that say what the bundle is.
func (c *checker) identity(m *manifest, version string) {
	if m.ManifestVersion != manifestVersion {
		c.problem("manifest_version must be %s, not %q", manifestVersion, m.ManifestVersion)
	}
	if m.Name != serverName {
		c.problem("name must be %q, the last segment of the hooks' server address, not %q", serverName, m.Name)
	}
	for _, name := range []string{m.Name, m.DisplayName} {
		lower := strings.ToLower(name)
		if strings.Contains(lower, "claude") || strings.Contains(lower, "anthropic") {
			c.problem("name %q must not contain the brand names of ADR-0010", name)
		}
	}
	if m.DisplayName == "" || m.Author.Name == "" || m.Repository.URL == "" || m.Icon == "" {
		c.problem("display_name, author.name, repository.url and icon are required")
	}
	if m.License != "MIT" {
		c.problem("license must be MIT, not %q", m.License)
	}
	if !semver.MatchString(m.Version) {
		c.problem("version %q is not a semantic version", m.Version)
	}
	if m.Version != version {
		c.problem("version is %q, want %q", m.Version, version)
	}
	if !strings.Contains(strings.ToLower(m.Description), "not affiliated") {
		c.problem("description must carry the unaffiliated notice")
	}
}

// server checks how the manifest starts the binaries.
func (c *checker) server(m *manifest) {
	s := &m.Server
	if s.Type != "binary" {
		c.problem("server.type must be binary, not %q", s.Type)
	}
	base := s.McpConfig.override
	if !slices.Equal(base.Args, serverArgs) {
		c.problem("mcp_config.args must be %v", serverArgs)
	}
	c.referenced = append(c.referenced, s.EntryPoint, strings.TrimPrefix(base.Command, dirnamePrefix))
	if s.EntryPoint != strings.TrimPrefix(base.Command, dirnamePrefix) {
		c.problem("entry_point must be the path of mcp_config.command")
	}
	for platform := range s.McpConfig.PlatformOverrides {
		if !slices.ContainsFunc(binaries, func(b serverBinary) bool { return b.platform == platform }) {
			c.problem("platform_overrides names %q, which is not a supported platform", platform)
		}
	}
	// Each override repeats the arguments and environment, so that the
	// result is the same whether a host merges overrides over the base or
	// replaces it.
	for _, b := range binaries {
		o, ok := s.McpConfig.PlatformOverrides[b.platform]
		switch {
		case !ok:
			c.problem("platform_overrides has no entry for %s", b.platform)
		case o.Command != dirnamePrefix+b.path:
			c.problem("platform_overrides.%s.command must be %s%s", b.platform, dirnamePrefix, b.path)
		case !slices.Equal(o.Args, base.Args) || !mapsEqual(o.Env, base.Env):
			c.problem("platform_overrides.%s must repeat the arguments and environment", b.platform)
		}
		c.referenced = append(c.referenced, strings.TrimPrefix(o.Command, dirnamePrefix))
	}
}

func mapsEqual(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if w, ok := b[k]; !ok || v != w {
			return false
		}
	}
	return true
}

// userConfig checks the settings and that each one reaches the server.
func (c *checker) userConfig(m *manifest) {
	env := m.Server.McpConfig.Env
	reached := map[string]bool{}
	for _, value := range env {
		for _, match := range placeholder.FindAllStringSubmatch(value, -1) {
			reached[match[1]] = true
			if _, ok := m.UserConfig[match[1]]; !ok {
				c.problem("the environment refers to user_config.%s, which is not defined", match[1])
			}
		}
	}
	for key, option := range m.UserConfig {
		if !slices.Contains(userConfigTypes, option.Type) {
			c.problem("user_config.%s has type %q", key, option.Type)
		}
		if option.Title == "" || option.Description == "" {
			c.problem("user_config.%s needs a title and a description", key)
		}
		if option.Required {
			// Claude Desktop starts the server before the form is saved.
			c.problem("user_config.%s must not be required: the server starts before the user fills it in", key)
		}
		if option.Default != nil {
			// Claude Code lets a default here win over the value the user
			// chose in the plugin's settings (CRP-042). The server has its
			// own defaults.
			c.problem("user_config.%s must have no default: it would override the plugin's setting", key)
		}
		if !reached[key] {
			c.problem("user_config.%s is not passed to the server", key)
		}
	}
}

// binaries checks the mode and the architecture of each server binary.
func (c *checker) binaries() {
	for _, b := range binaries {
		f, ok := c.files[b.path]
		if !ok {
			continue
		}
		if b.platform != "win32" && f.Mode()&0o111 != 0o111 {
			c.problem("%s is not executable: mode %v", b.path, f.Mode())
		}
		data, ok := c.read(b.path)
		if !ok {
			continue
		}
		if why := architecture(b.platform, data); why != "" {
			c.problem("%s: %s", b.path, why)
		}
	}
}

// architecture returns why data is not the binary the platform needs, or the
// empty string. Windows and Linux ship amd64 only. The Mac binary must hold
// both architectures.
func architecture(platform string, data []byte) string {
	r := bytes.NewReader(data)
	switch platform {
	case "win32":
		f, err := pe.NewFile(r)
		if err != nil {
			return "not a Windows executable: " + err.Error()
		}
		if f.Machine != pe.IMAGE_FILE_MACHINE_AMD64 {
			return fmt.Sprintf("Windows machine type %#x, want amd64", f.Machine)
		}
	case "darwin":
		f, err := macho.NewFatFile(r)
		if err != nil {
			return "not a universal Mac binary: " + err.Error()
		}
		var got []macho.Cpu
		for _, a := range f.Arches {
			got = append(got, a.Cpu)
		}
		slices.Sort(got)
		if want := []macho.Cpu{macho.CpuAmd64, macho.CpuArm64}; !slices.Equal(got, want) {
			return fmt.Sprintf("Mac architectures %v, want amd64 and arm64", got)
		}
	default:
		f, err := elf.NewFile(r)
		if err != nil {
			return "not a Linux executable: " + err.Error()
		}
		if f.Machine != elf.EM_X86_64 {
			return fmt.Sprintf("Linux machine %v, want amd64", f.Machine)
		}
	}
	return ""
}
