package domain_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// TestPackageIsPure reads the package's own source and checks that it imports
// only the standard library, nothing that touches the operating system, and
// never reads the real clock.
func TestPackageIsPure(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
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
			case strings.Contains(root, "."):
				t.Errorf("%s imports %s, which is not in the standard library", name, path)
			case root == "os" || root == "net" || root == "log" || root == "syscall":
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
	if checked < 5 {
		t.Errorf("checked %d source files, want at least 5", checked)
	}
}
