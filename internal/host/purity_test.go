package host_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// TestPackageReachesTheOutsideOnlyThroughPorts reads the package's own
// source and checks the rule in its documentation: it imports the core
// packages and the control codec, no transport, adapter or command-line
// package, and nothing of the operating system. It never reads the real
// clock or starts a real timer, so every wait in it is one a test can move.
func TestPackageReachesTheOutsideOnlyThroughPorts(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	const module = "github.com/Zafnok/claude-rich-presence/internal/"
	allowed := map[string]bool{
		module + "domain":           true,
		module + "presence":         true,
		module + "control/protocol": true,
		module + "diag":             true,
	}
	forbidden := map[string]bool{
		"os": true, "net": true, "log": true, "syscall": true, "bufio": true,
		"path": true, "runtime": true, "math": true, "crypto": true,
	}
	clock := map[string]bool{
		"Now": true, "Since": true, "Until": true, "Sleep": true, "After": true,
		"AfterFunc": true, "Tick": true, "NewTimer": true, "NewTicker": true,
	}
	checked := 0
	fset := token.NewFileSet()
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		checked++
		file, err := parser.ParseFile(fset, name, nil, 0)
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
			case allowed[path]:
			case strings.Contains(root, "."):
				t.Errorf("%s imports %s", name, path)
			case forbidden[root]:
				t.Errorf("%s imports %s", name, path)
			}
		}
		ast.Inspect(file, func(n ast.Node) bool {
			sel, ok := n.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			if pkg, ok := sel.X.(*ast.Ident); ok && pkg.Name == "time" && clock[sel.Sel.Name] {
				t.Errorf("%s uses time.%s", name, sel.Sel.Name)
			}
			return true
		})
	}
	if checked < 6 {
		t.Errorf("checked %d source files, want at least 6", checked)
	}
}
