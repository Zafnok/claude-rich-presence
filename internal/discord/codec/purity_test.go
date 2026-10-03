package codec_test

import (
	"go/parser"
	"go/token"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// TestPackageIsPure reads the package's own source and checks that it imports
// only what is listed here: nothing that opens a file, a socket or a pipe, and
// nothing of this module but the domain package.
func TestPackageIsPure(t *testing.T) {
	allowed := map[string]bool{
		"bytes":           true,
		"encoding/binary": true,
		"encoding/json":   true,
		"errors":          true,
		"fmt":             true,
		"io":              true,
		"github.com/Zafnok/claude-rich-presence/internal/domain": true,
	}
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
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
			if !allowed[path] {
				t.Errorf("%s imports %s, which is not on the list", name, path)
			}
		}
	}
	if checked < 3 {
		t.Errorf("checked %d source files, want at least 3", checked)
	}
}
