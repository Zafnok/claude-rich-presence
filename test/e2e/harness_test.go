package e2e

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Zafnok/claude-rich-presence/internal/cli"
	"github.com/Zafnok/claude-rich-presence/internal/config"
	ctransport "github.com/Zafnok/claude-rich-presence/internal/control/transport"
	"github.com/Zafnok/claude-rich-presence/internal/diag"
	dtransport "github.com/Zafnok/claude-rich-presence/internal/discord/transport"
	"github.com/Zafnok/claude-rich-presence/internal/testutil/fakediscord"
	"github.com/Zafnok/claude-rich-presence/internal/testutil/mcpclient"
)

// EnvCoverDir names the environment variable that says where the processes
// write their coverage data. It must be an absolute path.
const EnvCoverDir = "RICH_PRESENCE_E2E_COVERDIR"

// interval is the least time between two Discord updates in every scenario:
// the floor of the setting, so that the suite is as quick as the product
// allows.
const interval = config.MinUpdateFloor

// patience bounds every wait for a state. It is a watchdog, not a delay: a
// wait ends as soon as its condition holds.
const patience = 45 * time.Second

// The versions of the two builds. Most scenarios run the current one. The
// newer one is the same source under a higher version, for the scenario in
// which an upgraded binary takes over (E12).
const (
	versionCurrent = "1.0.0"
	versionNewer   = "1.1.0"
)

// built is what TestMain made: the directory of the binaries, the current
// binary, and the directory the processes write coverage data to.
var built struct {
	dir      string
	current  string
	coverDir string
}

func TestMain(m *testing.M) {
	os.Exit(runTests(m))
}

func runTests(m *testing.M) int {
	// Listing the tests, as the fuzz step of CI does for every package, runs
	// none of them and needs no binary.
	flag.Parse()
	if list := flag.Lookup("test.list"); list != nil && list.Value.String() != "" {
		return m.Run()
	}
	dir, err := os.MkdirTemp("", "rp-e2e-")
	if err != nil {
		fmt.Fprintln(os.Stderr, "e2e:", err)
		return 1
	}
	built.dir = dir
	built.coverDir = os.Getenv(EnvCoverDir)
	if built.coverDir == "" {
		built.coverDir = filepath.Join(dir, "cover")
	}
	if !filepath.IsAbs(built.coverDir) {
		fmt.Fprintf(os.Stderr, "e2e: %s is %q, which is not an absolute path\n", EnvCoverDir, built.coverDir)
		return 1
	}
	if err := os.MkdirAll(built.coverDir, 0o755); err != nil {
		fmt.Fprintln(os.Stderr, "e2e:", err)
		return 1
	}
	// The second build is started now, so that it is made while the first
	// is, and not while the scenario that needs it waits. It is waited for
	// before the directory it writes to is removed.
	go func() { _, _ = newerBinary() }()
	defer func() {
		_, _ = newerBinary()
		_ = os.RemoveAll(dir)
	}()
	if built.current, err = build(versionCurrent); err != nil {
		fmt.Fprintln(os.Stderr, "e2e:", err)
		return 1
	}
	code := m.Run()
	if err := mergeCoverage(); err != nil {
		fmt.Fprintln(os.Stderr, "e2e:", err)
		return 1
	}
	return code
}

// processCoverDir makes the directory one process writes its coverage data
// to. Each process has its own: on Windows the Go runtime fails to write the
// data, and says so on standard error, when two processes of one binary end
// at the same moment with the same directory.
func processCoverDir() (string, error) {
	return os.MkdirTemp(built.coverDir, processCoverPrefix)
}

const processCoverPrefix = "process-"

// mergeCoverage folds what each process wrote into the one directory the
// gate reads, and removes the directories of the processes.
func mergeCoverage() error {
	dirs, err := filepath.Glob(filepath.Join(built.coverDir, processCoverPrefix+"*"))
	if err != nil {
		return err
	}
	var written []string
	for _, dir := range dirs {
		// A process that was killed wrote nothing.
		if entries, err := os.ReadDir(dir); err == nil && len(entries) > 0 {
			written = append(written, dir)
		}
	}
	if len(written) > 0 {
		merged, err := os.MkdirTemp(built.dir, "merged-")
		if err != nil {
			return err
		}
		cmd := exec.Command("go", "tool", "covdata", "merge", "-i="+strings.Join(written, ","), "-o="+merged)
		if output, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("merging the coverage data: %v\n%s", err, output)
		}
		// A file that is already there, from an earlier run into the same
		// directory, is replaced: the same name means the same build.
		files, err := os.ReadDir(merged)
		if err != nil {
			return err
		}
		for _, file := range files {
			data, err := os.ReadFile(filepath.Join(merged, file.Name()))
			if err == nil {
				err = os.WriteFile(filepath.Join(built.coverDir, file.Name()), data, 0o644)
			}
			if err != nil {
				return fmt.Errorf("merging the coverage data: %v", err)
			}
		}
	}
	for _, dir := range dirs {
		if err := os.RemoveAll(dir); err != nil {
			return err
		}
	}
	return nil
}

// build compiles the product as a release is compiled, without cgo and with
// the version set through the linker, and with coverage instrumentation, which
// is how running it covers the main function.
func build(version string) (string, error) {
	out := filepath.Join(built.dir, cli.BinaryName+"-"+version)
	if runtime.GOOS == "windows" {
		out += ".exe"
	}
	cmd := exec.Command("go", "build", "-cover", "-covermode=atomic",
		"-ldflags", "-X github.com/Zafnok/claude-rich-presence/internal/cli.version="+version,
		"-o", out, "../../cmd/rich-presence")
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	if output, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("building version %s: %v\n%s", version, err, output)
	}
	return out, nil
}

// newerBinary builds the newer version the first time it is asked for.
var newerBinary = sync.OnceValues(func() (string, error) { return build(versionNewer) })

// scene is everything one scenario touches outside its own process: a home
// directory, a runtime directory and a Discord endpoint name that nothing
// else uses.
type scene struct {
	t  *testing.T
	ns *fakediscord.Namespace
	// home holds the configuration directory and the log.
	home string
	// runtime is the runtime directory, which the product makes when it
	// needs it.
	runtime string
	root    string
	// extra is added to the environment of every process started from now on.
	extra []string

	mu       sync.Mutex
	servers  []*fakediscord.Server
	sessions []*session
}

// newScene makes a scene with no Discord listening. When the scenario ends it
// checks that nothing is left running, and if the scenario failed it prints
// the fake Discord's record, the product's log and what each process wrote to
// standard error.
func newScene(t *testing.T) *scene {
	t.Helper()
	// The directory is short, so that the socket path fits the limit.
	root, err := os.MkdirTemp("", "rp")
	if err != nil {
		t.Fatal(err)
	}
	s := &scene{t: t, root: root, home: filepath.Join(root, "home"), runtime: filepath.Join(root, "run")}
	if err := os.Mkdir(s.home, 0o700); err != nil {
		t.Fatal(err)
	}
	// Registered first, so it runs last: after every process has been ended
	// and every fake Discord closed.
	t.Cleanup(func() {
		if t.Failed() {
			t.Log(s.report())
		}
		// On Windows a file that a process still has open cannot be removed,
		// so this also shows that no process outlived the scenario.
		if err := os.RemoveAll(root); err != nil {
			t.Errorf("something was left behind in %s: %v", root, err)
		}
	})
	s.ns = fakediscord.NewNamespace(t)
	return s
}

// environ is the whole environment of a process in the scene. Nothing is
// inherited from the test's own, apart from what Windows needs to open a
// socket at all.
func (s *scene) environ() []string {
	env := []string{
		"HOME=" + s.home,
		"USERPROFILE=" + s.home,
		ctransport.EnvRuntimeDir + "=" + s.runtime,
		dtransport.EnvEndpoint + "=" + s.ns.Prefix(),
		config.EnvPrefix + "MIN_UPDATE_INTERVAL=" + interval.String(),
		config.EnvPrefix + "LOG_LEVEL=debug",
	}
	if root := os.Getenv("SystemRoot"); root != "" {
		env = append(env, "SystemRoot="+root)
	}
	return append(env, s.extra...)
}

// processEnviron is environ for one process that is about to start, with a
// place of its own for its coverage data.
func (s *scene) processEnviron() []string {
	s.t.Helper()
	dir, err := processCoverDir()
	if err != nil {
		s.t.Fatalf("making a coverage directory: %v", err)
	}
	return append(s.environ(), "GOCOVERDIR="+dir)
}

// getenv reads the scene's environment as a process in it would.
func (s *scene) getenv(name string) string {
	value := ""
	for _, entry := range s.environ() {
		if k, v, _ := strings.Cut(entry, "="); k == name {
			value = v
		}
	}
	return value
}

// paths are the runtime files of the scene.
func (s *scene) paths() ctransport.Paths {
	s.t.Helper()
	paths, err := ctransport.Locate(s.getenv)
	if err != nil {
		s.t.Fatalf("locating the runtime directory: %v", err)
	}
	if paths.Dir != s.runtime {
		s.t.Fatalf("the runtime directory is %s, want the scene's own, %s", paths.Dir, s.runtime)
	}
	return paths
}

// logText is the product's log, with the predecessor it rotated out, or the
// empty string if nothing was logged.
func (s *scene) logText() string {
	dirs, err := config.ResolveDirs(runtime.GOOS, s.getenv)
	if err != nil {
		s.t.Fatalf("resolving the log directory: %v", err)
	}
	var text strings.Builder
	for _, name := range []string{diag.LogPath(dirs.Logs) + ".1", diag.LogPath(dirs.Logs)} {
		data, err := os.ReadFile(name)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			s.t.Fatalf("reading the log: %v", err)
		}
		text.Write(data)
	}
	return text.String()
}

// discord starts the fake Discord, at the first of the ten endpoints.
func (s *scene) discord(b fakediscord.Behavior) *fakediscord.Server {
	s.t.Helper()
	srv := s.ns.Start(s.t, 0, fakediscord.Options{Behavior: b, Patience: patience})
	s.mu.Lock()
	s.servers = append(s.servers, srv)
	s.mu.Unlock()
	return srv
}

// run runs one command of a binary to its end, with nothing on standard
// input.
func (s *scene) run(binary string, args ...string) (code int, stdout, stderr string) {
	s.t.Helper()
	var out, errs bytes.Buffer
	cmd := exec.Command(binary, args...)
	cmd.Env = s.processEnviron()
	cmd.Stdout, cmd.Stderr = &out, &errs
	err := cmd.Run()
	var exit *exec.ExitError
	if err != nil && !errors.As(err, &exit) {
		s.t.Fatalf("running %v: %v", args, err)
	}
	return cmd.ProcessState.ExitCode(), out.String(), errs.String()
}

// report is what a failed scenario prints.
func (s *scene) report() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	var b strings.Builder
	for i, srv := range s.servers {
		fmt.Fprintf(&b, "fake Discord %d at %s recorded:\n", i+1, srv.Addr())
		events := srv.Events()
		for _, ev := range events {
			fmt.Fprintf(&b, "  %s conn %d %-14s %s\n", ev.Time.Format("15:04:05.000"), ev.Conn, ev.Kind, ev.Payload)
		}
		if len(events) == 0 {
			b.WriteString("  nothing\n")
		}
	}
	if len(s.servers) == 0 {
		b.WriteString("no fake Discord was started\n")
	}
	for _, c := range s.sessions {
		fmt.Fprintf(&b, "standard error of %s (process %d): %q\n", c.id, c.PID(), c.Stderr())
	}
	fmt.Fprintf(&b, "the log:\n%s", s.logText())
	return b.String()
}

// session is one running mcp command and the scripted Claude Code that
// drives it.
type session struct {
	*mcpclient.Client
	t *testing.T
	// id is the session id its hooks carry, and cwd their working directory.
	id  string
	cwd string
}

// start starts the current binary as a Claude Code session and initializes
// it. name becomes the session id its hooks carry.
func (s *scene) start(name string) *session {
	s.t.Helper()
	return s.startBinary(built.current, name)
}

func (s *scene) startBinary(binary, name string) *session {
	s.t.Helper()
	c := s.launch(binary, name)
	c.Initialize(mcpclient.ClaudeCode)
	return c
}

// startAs starts the current binary and initializes it under a client name
// that is not the one Claude Code gives.
func (s *scene) startAs(client, name string) *session {
	s.t.Helper()
	c := s.launch(built.current, name)
	c.Initialize(client)
	return c
}

// launch starts a binary and leaves it uninitialized.
func (s *scene) launch(binary, name string) *session {
	s.t.Helper()
	c := &session{
		Client: mcpclient.Start(s.t, mcpclient.Options{Binary: binary, Env: s.processEnviron(), Patience: patience}),
		t:      s.t,
		id:     name,
		cwd:    filepath.Join(s.home, "work", "project-of-"+name),
	}
	s.mu.Lock()
	s.sessions = append(s.sessions, c)
	s.mu.Unlock()
	return c
}

// hook calls the event tool as a hook of Claude Code does, with the session
// id and the working directory every hook carries, and requires the one
// result the tool ever gives. fields are pairs of a name and a value.
func (c *session) hook(event string, fields ...string) {
	c.t.Helper()
	arguments := map[string]any{"event": event, "session_id": c.id, "cwd": c.cwd}
	for i := 0; i+1 < len(fields); i += 2 {
		arguments[fields[i]] = fields[i+1]
	}
	c.event(arguments)
}

// event calls the event tool with exactly these arguments.
func (c *session) event(arguments map[string]any) {
	c.t.Helper()
	if got := c.Call(cli.ToolEvent, arguments); got.Text != "{}" || got.IsError {
		c.t.Fatalf("the event tool answered %+v, want the constant {}", got)
	}
}

// status is the text of the status tool.
func (c *session) status() string {
	c.t.Helper()
	return c.Call(cli.ToolStatus, nil).Text
}

// awaitStatus polls the status tool until its text contains every one of
// want.
func (c *session) awaitStatus(want ...string) {
	c.t.Helper()
	var got string
	eventually(c.t, fmt.Sprintf("the status of %s to show %q", c.id, want), func() bool {
		got = c.status()
		for _, w := range want {
			if !strings.Contains(got, w) {
				return false
			}
		}
		return true
	}, func() string { return "last status: " + got })
}

// finish ends the session as Claude Code does, by closing its input, and
// requires a clean exit with nothing on standard error.
func (c *session) finish() {
	c.t.Helper()
	if code := c.Close(); code != 0 {
		c.t.Errorf("%s exited with code %d, want 0", c.id, code)
	}
	if stderr := c.Stderr(); stderr != "" {
		c.t.Errorf("%s wrote to standard error: %q", c.id, stderr)
	}
}

// eventually polls cond until it holds, and fails the test, with what detail
// returns, if it does not within the patience.
func eventually(t *testing.T, what string, cond func() bool, detail ...func() string) {
	t.Helper()
	deadline := time.Now().Add(patience)
	for !cond() {
		if time.Now().After(deadline) {
			extra := ""
			for _, d := range detail {
				extra += "; " + d()
			}
			t.Fatalf("timed out after %v waiting for %s%s", patience, what, extra)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// shown is one thing the fake Discord was told to show: an activity, or
// nothing.
type shown struct {
	// Conn numbers the connection it arrived on, from 1.
	Conn int
	At   time.Time
	// Clear is set when Discord was told to show nothing.
	Clear   bool
	Details string
	State   string
	// Start is the start of the elapsed timer, in Unix seconds.
	Start int64
	// Raw is the whole activity as it arrived.
	Raw string
}

func (v shown) String() string {
	if v.Clear {
		return fmt.Sprintf("conn %d: clear", v.Conn)
	}
	return fmt.Sprintf("conn %d: %q / %q, start %d", v.Conn, v.Details, v.State, v.Start)
}

// showing lists, in order, everything the fake Discord was told to show.
func showing(t *testing.T, srv *fakediscord.Server) []shown {
	t.Helper()
	var out []shown
	for _, ev := range srv.Events() {
		switch ev.Kind {
		case fakediscord.KindClearActivity:
			out = append(out, shown{Conn: ev.Conn, At: ev.Time, Clear: true})
		case fakediscord.KindSetActivity:
			var a struct {
				Details    string `json:"details"`
				State      string `json:"state"`
				Timestamps struct {
					Start int64 `json:"start"`
				} `json:"timestamps"`
			}
			if err := json.Unmarshal(ev.Activity, &a); err != nil {
				t.Fatalf("the activity %s is not what Discord documents: %v", ev.Activity, err)
			}
			out = append(out, shown{Conn: ev.Conn, At: ev.Time, Details: a.Details, State: a.State, Start: a.Timestamps.Start, Raw: string(ev.Activity)})
		}
	}
	return out
}

// awaitShown waits until the last thing the fake Discord was told to show
// satisfies want, and returns it. It is the last that counts: that is what a
// Discord would be displaying.
func awaitShown(t *testing.T, srv *fakediscord.Server, what string, want func(shown) bool) shown {
	t.Helper()
	var all []shown
	eventually(t, "Discord to show "+what, func() bool {
		all = showing(t, srv)
		return len(all) > 0 && want(all[len(all)-1])
	}, func() string { return fmt.Sprintf("it was told: %v", all) })
	return all[len(all)-1]
}

// lines is a predicate for an activity with exactly these two text lines.
func lines(details, state string) func(shown) bool {
	return func(v shown) bool { return !v.Clear && v.Details == details && v.State == state }
}

// count is how many events of a kind the fake Discord has recorded.
func count(srv *fakediscord.Server, kind fakediscord.Kind) int {
	n := 0
	for _, ev := range srv.Events() {
		if ev.Kind == kind {
			n++
		}
	}
	return n
}
