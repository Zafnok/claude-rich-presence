package protocol_test

import (
	"go/parser"
	"go/token"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// TestPackageIsPure reads the package's own source and checks that it imports
// only the standard library and the domain, and nothing that touches the
// operating system.
func TestPackageIsPure(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	const domain = "github.com/Zafnok/claude-rich-presence/internal/domain"
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
			root, _, _ := strings.Cut(path, "/")
			switch {
			case path == domain:
			case strings.Contains(root, "."):
				t.Errorf("%s imports %s", name, path)
			case root == "os" || root == "net" || root == "log" || root == "syscall":
				t.Errorf("%s imports %s", name, path)
			}
		}
	}
	if checked < 4 {
		t.Errorf("checked %d source files, want at least 4", checked)
	}
}
