package config_test

import (
	"errors"
	"go/parser"
	"go/token"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/Zafnok/claude-rich-presence/internal/config"
)

func TestResolveDirs(t *testing.T) {
	windows := map[string]string{
		"USERPROFILE":  `C:\Users\u`,
		"APPDATA":      `C:\Users\u\AppData\Roaming`,
		"LOCALAPPDATA": `C:\Users\u\AppData\Local`,
		"HOME":         "/c/Users/elsewhere",
	}
	cases := []struct {
		name string
		goos string
		env  map[string]string
		want config.Dirs
	}{
		{"windows", "windows", windows, config.Dirs{
			Config: `C:\Users\u\.rich-presence`,
			File:   `C:\Users\u\.rich-presence\config.json`,
			Logs:   `C:\Users\u\.rich-presence\logs`,
		}},
		{"windows, profile at a drive root", "windows", map[string]string{"USERPROFILE": `D:\`}, config.Dirs{
			Config: `D:\.rich-presence`,
			File:   `D:\.rich-presence\config.json`,
			Logs:   `D:\.rich-presence\logs`,
		}},
		{"macos", "darwin", map[string]string{"HOME": "/Users/u", "XDG_CONFIG_HOME": "/ignored"}, config.Dirs{
			Config: "/Users/u/Library/Application Support/rich-presence",
			File:   "/Users/u/Library/Application Support/rich-presence/config.json",
			Logs:   "/Users/u/Library/Application Support/rich-presence/logs",
		}},
		{"linux", "linux", map[string]string{"HOME": "/home/u"}, config.Dirs{
			Config: "/home/u/.config/rich-presence",
			File:   "/home/u/.config/rich-presence/config.json",
			Logs:   "/home/u/.config/rich-presence/logs",
		}},
		{"linux, trailing slash", "linux", map[string]string{"HOME": "/home/u/"}, config.Dirs{
			Config: "/home/u/.config/rich-presence",
			File:   "/home/u/.config/rich-presence/config.json",
			Logs:   "/home/u/.config/rich-presence/logs",
		}},
		{"linux, XDG", "linux", map[string]string{"HOME": "/home/u", "XDG_CONFIG_HOME": "/xdg"}, config.Dirs{
			Config: "/xdg/rich-presence",
			File:   "/xdg/rich-presence/config.json",
			Logs:   "/xdg/rich-presence/logs",
		}},
		{"linux, XDG without a home", "linux", map[string]string{"XDG_CONFIG_HOME": "/xdg"}, config.Dirs{
			Config: "/xdg/rich-presence",
			File:   "/xdg/rich-presence/config.json",
			Logs:   "/xdg/rich-presence/logs",
		}},
		{"linux, relative XDG is ignored", "linux", map[string]string{"HOME": "/home/u", "XDG_CONFIG_HOME": "xdg"}, config.Dirs{
			Config: "/home/u/.config/rich-presence",
			File:   "/home/u/.config/rich-presence/config.json",
			Logs:   "/home/u/.config/rich-presence/logs",
		}},
		{"another unix", "freebsd", map[string]string{"HOME": "/home/u"}, config.Dirs{
			Config: "/home/u/.config/rich-presence",
			File:   "/home/u/.config/rich-presence/config.json",
			Logs:   "/home/u/.config/rich-presence/logs",
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := config.ResolveDirs(tc.goos, func(key string) string { return tc.env[key] })
			if err != nil || got != tc.want {
				t.Errorf("got %+v, %v, want %+v", got, err, tc.want)
			}
		})
	}
}

func TestResolveDirsOnWindowsAvoidsAppData(t *testing.T) {
	env := map[string]string{
		"USERPROFILE":  `C:\Users\u`,
		"APPDATA":      `C:\Users\u\AppData\Roaming`,
		"LOCALAPPDATA": `C:\Users\u\AppData\Local`,
	}
	dirs, err := config.ResolveDirs("windows", func(key string) string { return env[key] })
	if err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{dirs.Config, dirs.File, dirs.Logs} {
		for _, key := range []string{"APPDATA", "LOCALAPPDATA"} {
			if strings.HasPrefix(strings.ToLower(dir), strings.ToLower(env[key])) {
				t.Errorf("%s is under %%%s%%", dir, key)
			}
		}
	}
}

func TestResolveDirsWithoutAHome(t *testing.T) {
	cases := []struct {
		goos string
		env  map[string]string
	}{
		{"windows", map[string]string{"HOME": "/home/u", "APPDATA": `C:\a`, "LOCALAPPDATA": `C:\l`}},
		{"darwin", map[string]string{"USERPROFILE": `C:\Users\u`, "XDG_CONFIG_HOME": "/xdg"}},
		{"linux", map[string]string{"USERPROFILE": `C:\Users\u`, "XDG_CONFIG_HOME": "relative"}},
	}
	for _, tc := range cases {
		got, err := config.ResolveDirs(tc.goos, func(key string) string { return tc.env[key] })
		if !errors.Is(err, config.ErrNoHome) || got != (config.Dirs{}) {
			t.Errorf("%s: got %+v, %v, want ErrNoHome", tc.goos, got, err)
		}
	}
}

// TestPackageImports holds the package to its doc comment: the file system
// and the environment reach it only through parameters.
func TestPackageImports(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	const module = "github.com/Zafnok/claude-rich-presence/"
	checked := 0
	fset := token.NewFileSet()
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		checked++
		file, err := parser.ParseFile(fset, name, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		for _, imp := range file.Imports {
			path, err := strconv.Unquote(imp.Path.Value)
			if err != nil {
				t.Fatal(err)
			}
			if path == "os" || strings.HasPrefix(path, "os/") || path == "path/filepath" {
				t.Errorf("%s imports %s", name, path)
			}
			if strings.HasPrefix(path, module) && path != module+"internal/domain" {
				t.Errorf("%s imports %s", name, path)
			}
		}
	}
	if checked < 3 {
		t.Errorf("checked %d files, want at least 3", checked)
	}
}
