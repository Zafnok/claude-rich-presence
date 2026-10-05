package main

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

const usageText = "usage: mcpb build -version V -manifest F -icon F -license F -notices F -windows F -darwin F -linux F -out F\n" +
	"       mcpb check -version V bundle\n"

// disk is a fake file system: a name it does not hold cannot be read, and
// writes are kept, unless failWrite is set.
type disk struct {
	files     map[string][]byte
	failWrite bool
}

func (d *disk) read(name string) ([]byte, error) {
	data, ok := d.files[name]
	if !ok {
		return nil, errors.New("no such file")
	}
	return data, nil
}

func (d *disk) write(name string, data []byte) error {
	if d.failWrite {
		return errors.New("read-only")
	}
	d.files[name] = data
	return nil
}

// goodDisk holds the inputs of a good build under the names f-<flag>.
func goodDisk(t *testing.T) *disk {
	d := &disk{files: map[string][]byte{}}
	for name, data := range goodFiles(t) {
		d.files["f-"+name] = data
	}
	return d
}

func buildArgs(skip string) []string {
	args := []string{"build", "-version", testVersion, "-out", "out.mcpb"}
	for _, name := range sourceNames {
		if name != skip {
			args = append(args, "-"+name, "f-"+name)
		}
	}
	return args
}

func runWith(d *disk, args ...string) (code int, stdout, stderr string) {
	var out, errOut bytes.Buffer
	code = run(args, &out, &errOut, d.read, d.write)
	return code, out.String(), errOut.String()
}

func TestRun(t *testing.T) {
	good := func(t *testing.T) *disk { return goodDisk(t) }
	tests := []struct {
		name       string
		prepare    func(*testing.T) *disk
		args       []string
		wantCode   int
		wantStdout string
		wantStderr string
	}{
		{name: "no command", args: nil, wantCode: 2, wantStderr: "mcpb: no command given\n" + usageText},
		{name: "unknown command", args: []string{"pack"}, wantCode: 2, wantStderr: "mcpb: unknown command \"pack\"\n" + usageText},
		{name: "build writes the bundle", prepare: good, args: buildArgs(""), wantCode: 0, wantStdout: "wrote out.mcpb ("},
		{name: "build with a bad flag", prepare: good, args: []string{"build", "-nope"}, wantCode: 2, wantStderr: "flag provided but not defined: -nope"},
		{name: "build with a positional argument", prepare: good, args: append(buildArgs(""), "extra"), wantCode: 2, wantStderr: "mcpb: build takes flags only\n"},
		{name: "build without -out", prepare: good, args: []string{"build", "-version", "1.0.0"}, wantCode: 2, wantStderr: "mcpb: build needs -version and -out\n"},
		{name: "build without a source", prepare: good, args: buildArgs("icon"), wantCode: 2, wantStderr: "mcpb: build needs -icon\n"},
		{name: "build with an unreadable source", prepare: func(t *testing.T) *disk {
			d := goodDisk(t)
			delete(d.files, "f-linux")
			return d
		}, args: buildArgs(""), wantCode: 2, wantStderr: "mcpb: linux: no such file\n"},
		{name: "build of a bundle that fails its check", prepare: func(t *testing.T) *disk {
			d := goodDisk(t)
			d.files["f-linux"] = []byte("x")
			return d
		}, args: buildArgs(""), wantCode: 1, wantStderr: "mcpb: build: server/rich-presence-linux: not a Linux executable"},
		{name: "build that cannot write", prepare: func(t *testing.T) *disk {
			d := goodDisk(t)
			d.failWrite = true
			return d
		}, args: buildArgs(""), wantCode: 2, wantStderr: "mcpb: read-only\n"},
		{name: "check with a bad flag", prepare: good, args: []string{"check", "-nope"}, wantCode: 2, wantStderr: "flag provided but not defined: -nope"},
		{name: "check without a bundle", prepare: good, args: []string{"check", "-version", "1.0.0"}, wantCode: 2, wantStderr: "mcpb: check needs -version and one bundle\n"},
		{name: "check without a version", prepare: good, args: []string{"check", "x.mcpb"}, wantCode: 2, wantStderr: "mcpb: check needs -version and one bundle\n"},
		{name: "check of a missing file", prepare: good, args: []string{"check", "-version", "1.0.0", "x.mcpb"}, wantCode: 2, wantStderr: "mcpb: no such file\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := &disk{files: map[string][]byte{}}
			if tt.prepare != nil {
				d = tt.prepare(t)
			}
			code, stdout, stderr := runWith(d, tt.args...)
			if code != tt.wantCode {
				t.Errorf("code = %d, want %d (stderr %q)", code, tt.wantCode, stderr)
			}
			if !strings.Contains(stdout, tt.wantStdout) {
				t.Errorf("stdout = %q, want it to contain %q", stdout, tt.wantStdout)
			}
			if !strings.Contains(stderr, tt.wantStderr) {
				t.Errorf("stderr = %q, want it to contain %q", stderr, tt.wantStderr)
			}
		})
	}
}

func TestBuildThenCheck(t *testing.T) {
	d := goodDisk(t)
	if code, _, stderr := runWith(d, buildArgs("")...); code != 0 {
		t.Fatalf("build: %d %s", code, stderr)
	}
	code, stdout, stderr := runWith(d, "check", "-version", testVersion, "out.mcpb")
	if code != 0 || stdout != "out.mcpb: ok\n" || stderr != "" {
		t.Fatalf("check: %d %q %q", code, stdout, stderr)
	}
	code, _, stderr = runWith(d, "check", "-version", "9.9.9", "out.mcpb")
	if code != 1 || !strings.Contains(stderr, "mcpb: out.mcpb: version is \"1.2.3\", want \"9.9.9\"") {
		t.Fatalf("check of another version: %d %q", code, stderr)
	}
}

func TestWriteFile(t *testing.T) {
	name := t.TempDir() + "/out.mcpb"
	if err := writeFile(name, []byte("x")); err != nil {
		t.Fatal(err)
	}
	if err := writeFile(t.TempDir(), []byte("x")); err == nil {
		t.Fatal("writing to a directory succeeded")
	}
}
