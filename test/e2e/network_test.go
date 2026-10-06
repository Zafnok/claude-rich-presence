package e2e

import (
	"bytes"
	"encoding/json"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// releaseTargets are the operating systems and architectures a release is
// built for (CONTRIBUTING.md). Each links different packages.
var releaseTargets = []string{"windows/amd64", "darwin/amd64", "darwin/arm64", "linux/amd64"}

const (
	modulePath = "github.com/Zafnok/claude-rich-presence"
	// resolverPackage is the one DNS package in the list. The standard
	// library's net package imports it on every operating system, so it
	// cannot be kept out of a binary that opens a Unix socket. The test
	// checks that nothing else imports it.
	resolverPackage = "vendor/golang.org/x/net/dns/dnsmessage"
)

// networkClients are the packages that speak a network protocol, or exist to
// support one that does. An entry that ends in a slash is a prefix.
var networkClients = []string{
	"net/http", "net/http/", "net/rpc", "net/rpc/", "net/smtp", "net/mail",
	"net/textproto", "crypto/tls", "crypto/x509", "crypto/x509/", "mime",
	"mime/", "golang.org/x/net/", "vendor/golang.org/x/net/",
	"golang.org/x/crypto/", "vendor/golang.org/x/crypto/", "google.golang.org/",
}

// thirdParty are the modules the binary may link besides its own and the
// standard library (ADR-0003, ADR-0017). An entry is a prefix.
var thirdParty = []string{"golang.org/x/sys/", "github.com/Microsoft/go-winio", "github.com/Microsoft/go-winio/"}

// socketPackages are the packages outside the standard library that may
// import net: the two transports of this module, the pipe library with the
// package of its own it needs, and the system call package, which converts
// socket addresses.
var socketPackages = []string{
	"golang.org/x/sys/windows",
	modulePath + "/internal/control/transport",
	modulePath + "/internal/discord/transport",
	"github.com/Microsoft/go-winio",
	"github.com/Microsoft/go-winio/internal/socket",
}

// The names this module may use from net and from the pipe library. None of
// them resolves a name, and the only two that open a connection are checked
// for the network they open it on.
var (
	netNames  = []string{"Conn", "Listener", "Addr", "UnixAddr", "ListenUnix", "Dialer", "ErrClosed"}
	pipeNames = []string{"DialPipeContext"}
)

// linked is one package of the build, as go list describes it.
type linked struct {
	ImportPath string
	Dir        string
	Standard   bool
	GoFiles    []string
	Imports    []string
}

// TestTheBinaryLinksNoNetworkClient is the check ADR-0008 promises: the
// program opens the local Discord pipe and the local control socket, and
// cannot make a network request because the code for one is not in it.
//
// For every release target it reads the list of packages the binary is built
// from and fails if a package that speaks HTTP, TLS or DNS is there, or a
// module nobody approved. The net package has to be there for Unix sockets,
// and it holds a resolver and a TCP dialer of its own, so the test also reads
// this module's source: net is imported only by the two transports, only the
// names that cannot reach a network are used from it, and every connection is
// opened on the network "unix".
func TestTheBinaryLinksNoNetworkClient(t *testing.T) {
	t.Parallel()
	for _, target := range releaseTargets {
		t.Run(target, func(t *testing.T) {
			t.Parallel()
			packages := linkedPackages(t, target)
			own := 0
			for _, p := range packages {
				for _, problem := range packageProblems(p) {
					t.Errorf("%s: %s", p.ImportPath, problem)
				}
				if !strings.HasPrefix(p.ImportPath, modulePath+"/") {
					continue
				}
				own++
				for _, name := range p.GoFiles {
					for _, problem := range sourceProblems(t, filepath.Join(p.Dir, name)) {
						t.Errorf("%s: %s", filepath.Join(p.ImportPath, name), problem)
					}
				}
			}
			// A list that is too short was not the binary's.
			if own < 10 {
				t.Errorf("the list holds %d packages of this module, want at least 10", own)
			}
			if !slices.ContainsFunc(packages, func(p linked) bool { return p.ImportPath == "net" }) {
				t.Error("net is not in the list: the checks on it have nothing to check, so this test needs another look")
			}
		})
	}
}

// TestTheNetworkCheckCatchesWhatItShould gives the check packages and source
// that break each rule, so that a pass above is known to mean something.
func TestTheNetworkCheckCatchesWhatItShould(t *testing.T) {
	t.Parallel()
	packages := map[string]linked{
		"an HTTP client":             {ImportPath: "net/http", Standard: true},
		"a package under net/http":   {ImportPath: "net/http/httptrace", Standard: true},
		"TLS":                        {ImportPath: "crypto/tls", Standard: true},
		"a DNS package":              {ImportPath: "github.com/miekg/dns"},
		"the networking module":      {ImportPath: "golang.org/x/net/proxy"},
		"an unapproved module":       {ImportPath: "example.com/telemetry"},
		"net outside the transports": {ImportPath: modulePath + "/internal/host", Imports: []string{"net"}},
		"the resolver outside net":   {ImportPath: modulePath + "/internal/cli", Imports: []string{resolverPackage}},
	}
	for name, p := range packages {
		if len(packageProblems(p)) == 0 {
			t.Errorf("%s was not caught", name)
		}
	}
	for _, p := range []linked{
		{ImportPath: "net", Standard: true, Imports: []string{resolverPackage}},
		{ImportPath: resolverPackage, Standard: true},
		{ImportPath: "net/url", Standard: true},
		{ImportPath: "net/netip", Standard: true},
		{ImportPath: modulePath + "/internal/control/transport", Imports: []string{"net"}},
		{ImportPath: "golang.org/x/sys/windows"},
		{ImportPath: "github.com/Microsoft/go-winio", Imports: []string{"net"}},
	} {
		if problems := packageProblems(p); len(problems) > 0 {
			t.Errorf("%s was refused: %v", p.ImportPath, problems)
		}
	}

	sources := map[string]string{
		"a TCP dial":            `import "net"; func f() { var d net.Dialer; d.DialContext(nil, "tcp", "example.com:443") }`,
		"a network not written": `import "net"; func f(n string) { var d net.Dialer; d.DialContext(nil, n, "x") }`,
		"a plain dial":          `import "net"; func f() { net.Dial("unix", "x") }`,
		"a lookup":              `import "net"; func f() { net.LookupHost("example.com") }`,
		"a renamed import":      `import n "net"; func f() { n.Dial("tcp", "x") }`,
		"a dot import":          `import . "net"; func f() { Dial("tcp", "x") }`,
		"a datagram listener":   `import "net"; func f() { net.ListenUnix("unixgram", nil) }`,
		"a pipe listener":       `import "github.com/Microsoft/go-winio"; func f() { winio.ListenPipe("x", nil) }`,
	}
	for name, body := range sources {
		if len(sourceProblems(t, writeSource(t, body))) == 0 {
			t.Errorf("%s was not caught", name)
		}
	}
	allowed := `import ("net"; "github.com/Microsoft/go-winio")
		func f() (net.Conn, error) {
			var d net.Dialer
			net.ListenUnix("unix", &net.UnixAddr{})
			winio.DialPipeContext(nil, "x")
			return d.DialContext(nil, "unix", "x")
		}`
	if problems := sourceProblems(t, writeSource(t, allowed)); len(problems) > 0 {
		t.Errorf("the allowed uses were refused: %v", problems)
	}
}

// linkedPackages lists every package cmd/rich-presence is built from for a
// target, as a release builds it.
func linkedPackages(t *testing.T, target string) []linked {
	t.Helper()
	goos, goarch, _ := strings.Cut(target, "/")
	cmd := exec.Command("go", "list", "-deps", "-json=ImportPath,Dir,Standard,GoFiles,Imports", "../../cmd/rich-presence")
	cmd.Env = append(os.Environ(), "GOOS="+goos, "GOARCH="+goarch, "CGO_ENABLED=0", "GOFLAGS=-mod=readonly")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("go list: %v\n%s", err, stderr.String())
	}
	var packages []linked
	for decoder := json.NewDecoder(bytes.NewReader(out)); ; {
		var p linked
		if err := decoder.Decode(&p); errors.Is(err, io.EOF) {
			return packages
		} else if err != nil {
			t.Fatalf("reading the package list: %v", err)
		}
		packages = append(packages, p)
	}
}

// packageProblems says what is wrong with one package being in the build.
func packageProblems(p linked) []string {
	var problems []string
	path := p.ImportPath
	own := strings.HasPrefix(path, modulePath+"/")
	switch {
	case path == resolverPackage:
	case matches(networkClients, path):
		problems = append(problems, "is a network client")
	case hasElement(path, "dns"):
		problems = append(problems, "is a DNS package")
	case !p.Standard && !own && !matches(thirdParty, path):
		problems = append(problems, "is from a module that is not approved")
	}
	for _, imported := range p.Imports {
		switch {
		case imported == "net" && path != "net" && !slices.Contains(socketPackages, path) && !p.Standard:
			problems = append(problems, "imports net, which only the transports may")
		case imported == resolverPackage && path != "net":
			problems = append(problems, "imports the DNS resolver's package, which only net may")
		}
	}
	return problems
}

// matches reports whether path is one of the entries, or begins with one
// that ends in a slash.
func matches(entries []string, path string) bool {
	return slices.ContainsFunc(entries, func(entry string) bool {
		return path == entry || strings.HasSuffix(entry, "/") && strings.HasPrefix(path, entry)
	})
}

// hasElement reports whether an element of an import path is the word, or
// begins with it.
func hasElement(path, word string) bool {
	for _, element := range strings.Split(path, "/") {
		if strings.HasPrefix(element, word) {
			return true
		}
	}
	return false
}

// sourceProblems reads one source file of this module and says where it uses
// net or the pipe library for something that could reach a network.
func sourceProblems(t *testing.T, name string) []string {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, name, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	var problems []string
	// The name each of the two packages has in this file.
	local := map[string][]string{}
	for _, imp := range file.Imports {
		path, _ := strconv.Unquote(imp.Path.Value)
		var names []string
		switch path {
		case "net":
			names = netNames
		case "github.com/Microsoft/go-winio":
			names = pipeNames
		default:
			continue
		}
		as := map[string]string{"net": "net", "github.com/Microsoft/go-winio": "winio"}[path]
		if imp.Name != nil {
			as = imp.Name.Name
		}
		if as == "." || as == "_" {
			problems = append(problems, "imports "+path+" without a name")
			continue
		}
		local[as] = names
	}
	ast.Inspect(file, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.SelectorExpr:
			pkg, ok := n.X.(*ast.Ident)
			if !ok {
				return true
			}
			if names, ok := local[pkg.Name]; ok && !slices.Contains(names, n.Sel.Name) {
				problems = append(problems, "uses "+pkg.Name+"."+n.Sel.Name)
			}
		case *ast.CallExpr:
			sel, ok := n.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			at := -1
			switch sel.Sel.Name {
			case "DialContext":
				at = 1
			case "ListenUnix":
				at = 0
			}
			// A pipe is dialled by name and has no network to choose.
			if pkg, ok := sel.X.(*ast.Ident); ok && slices.Equal(local[pkg.Name], pipeNames) {
				at = -1
			}
			if at < 0 {
				return true
			}
			if at >= len(n.Args) || !isLiteral(n.Args[at], `"unix"`) {
				problems = append(problems, "calls "+sel.Sel.Name+" on a network that is not written as \"unix\"")
			}
		}
		return true
	})
	return problems
}

func isLiteral(e ast.Expr, value string) bool {
	lit, ok := e.(*ast.BasicLit)
	return ok && lit.Kind == token.STRING && lit.Value == value
}

// writeSource writes a source file made of body and returns its name.
func writeSource(t *testing.T, body string) string {
	t.Helper()
	name := filepath.Join(t.TempDir(), "source.go")
	if err := os.WriteFile(name, []byte("package p\n"+body+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return name
}
