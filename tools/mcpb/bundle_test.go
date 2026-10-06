package main

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"errors"
	"io/fs"
	"os"
	"strings"
	"testing"
)

const testVersion = "1.2.3"

// peFile is the smallest file debug/pe accepts, for the given machine type.
func peFile(machine uint16) []byte {
	b := make([]byte, 128)
	copy(b, "MZ")
	binary.LittleEndian.PutUint32(b[0x3c:], 128)
	b = append(b, "PE\x00\x00"...)
	h := make([]byte, 20)
	binary.LittleEndian.PutUint16(h, machine)
	return append(b, h...)
}

// elfFile is the smallest 64-bit ELF file debug/elf accepts, for the given
// machine type.
func elfFile(machine uint16) []byte {
	b := make([]byte, 64)
	copy(b, "\x7fELF")
	b[4], b[5], b[6] = 2, 1, 1
	binary.LittleEndian.PutUint16(b[16:], 2)
	binary.LittleEndian.PutUint16(b[18:], machine)
	binary.LittleEndian.PutUint32(b[20:], 1)
	binary.LittleEndian.PutUint16(b[52:], 64)
	return b
}

// machoSlice is the smallest 64-bit Mach-O file debug/macho accepts.
func machoSlice(cpu uint32) []byte {
	b := make([]byte, 32)
	binary.LittleEndian.PutUint32(b, 0xfeedfacf)
	binary.LittleEndian.PutUint32(b[4:], cpu)
	binary.LittleEndian.PutUint32(b[12:], 2)
	return b
}

// fatFile joins slices into a universal binary.
func fatFile(cpus ...uint32) []byte {
	be := binary.BigEndian
	head := make([]byte, 8+20*len(cpus))
	be.PutUint32(head, 0xcafebabe)
	be.PutUint32(head[4:], uint32(len(cpus)))
	var body []byte
	for i, cpu := range cpus {
		slice := machoSlice(cpu)
		offset := uint32(len(head) + len(body))
		entry := head[8+20*i:]
		be.PutUint32(entry, cpu)
		be.PutUint32(entry[8:], offset)
		be.PutUint32(entry[12:], uint32(len(slice)))
		body = append(body, slice...)
	}
	return append(head, body...)
}

const (
	cpuAmd64 = 0x1000007
	cpuArm64 = 0x100000c
)

// goodFiles are inputs for assemble that pass. The manifest and the notices
// are the repository's own, so the checks run on the real manifest.
func goodFiles(t *testing.T) map[string][]byte {
	t.Helper()
	read := func(name string) []byte {
		data, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		return data
	}
	return map[string][]byte{
		"manifest": read("../../extension/manifest.json"),
		"icon":     read("../../extension/icon.png"),
		"license":  read("../../LICENSE.md"),
		"notices":  read("../../extension/THIRD-PARTY-NOTICES.md"),
		"windows":  peFile(0x8664),
		"darwin":   fatFile(cpuArm64, cpuAmd64),
		"linux":    elfFile(62),
	}
}

// zipOf writes entries without compression, so that a test can corrupt one.
func zipOf(t *testing.T, entries []entry) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, e := range entries {
		h := &zip.FileHeader{Name: e.name, Method: zip.Store}
		h.SetMode(e.mode)
		w, err := zw.CreateHeader(h)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write(e.data); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// entriesOf lists a good bundle's entries, with the manifest passed through
// edit and the binaries' modes passed through mode.
func entriesOf(t *testing.T, edit func(string) string, mode func(string, fs.FileMode) fs.FileMode) []entry {
	t.Helper()
	files := goodFiles(t)
	manifest, _ := substituteVersion(files["manifest"], testVersion)
	list := []entry{
		{manifestPath, []byte(edit(string(manifest))), 0o644},
		{iconPath, files["icon"], 0o644},
		{licensePath, files["license"], 0o644},
		{noticesPath, files["notices"], 0o644},
	}
	for _, b := range binaries {
		list = append(list, entry{b.path, files[b.source], mode(b.path, 0o755)})
	}
	return list
}

func keep(s string) string                         { return s }
func sameMode(_ string, m fs.FileMode) fs.FileMode { return m }
func replacing(old, new string) func(string) string {
	return func(s string) string { return strings.Replace(s, old, new, 1) }
}

func TestAssembleProducesABundleThatChecks(t *testing.T) {
	archive, problems := assemble(testVersion, goodFiles(t))
	if len(problems) > 0 {
		t.Fatalf("problems: %v", problems)
	}
	if problems := check(archive, testVersion); len(problems) > 0 {
		t.Fatalf("check: %v", problems)
	}
	// The archive keeps the modes, which a zip archive does not do unless
	// they are set when it is written.
	zr, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range zr.File {
		wantExec := strings.HasPrefix(f.Name, "server/")
		if gotExec := f.Mode()&0o111 == 0o111; gotExec != wantExec {
			t.Errorf("%s: mode %v", f.Name, f.Mode())
		}
		if !f.Modified.Equal(fixedTime) {
			t.Errorf("%s: modified %v", f.Name, f.Modified)
		}
	}
}

func TestAssembleIsDeterministic(t *testing.T) {
	a, _ := assemble(testVersion, goodFiles(t))
	b, _ := assemble(testVersion, goodFiles(t))
	if len(a) == 0 || !bytes.Equal(a, b) {
		t.Fatal("two builds from the same inputs differ")
	}
}

func TestAssembleCarriesTheVersion(t *testing.T) {
	archive, _ := assemble("4.5.6-rc.1", goodFiles(t))
	if problems := check(archive, "4.5.6-rc.1"); len(problems) > 0 {
		t.Fatal(problems)
	}
	if problems := check(archive, testVersion); len(problems) == 0 {
		t.Fatal("a bundle of another version passed")
	}
}

func TestAssembleRefusesWhatDoesNotCheck(t *testing.T) {
	files := goodFiles(t)
	files["linux"] = []byte("not a binary")
	archive, problems := assemble(testVersion, files)
	if archive != nil || len(problems) != 1 || !strings.Contains(problems[0], "server/rich-presence-linux") {
		t.Fatalf("got %v, %v", archive != nil, problems)
	}
}

func TestAssembleNeedsTheVersionToken(t *testing.T) {
	for _, manifest := range []string{"{}", versionToken + versionToken} {
		files := goodFiles(t)
		files["manifest"] = []byte(manifest)
		archive, problems := assemble(testVersion, files)
		if archive != nil || len(problems) != 1 || !strings.Contains(problems[0], "exactly once") {
			t.Errorf("%q: got %v, %v", manifest, archive != nil, problems)
		}
	}
}

// failingWriter fails every write.
type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("disk full") }

func TestWriteZipReportsAFailedWrite(t *testing.T) {
	err := writeZip(failingWriter{}, []entry{{"a", bytes.Repeat([]byte("x"), 1<<16), 0o644}})
	if err == nil || !strings.Contains(err.Error(), "disk full") {
		t.Fatalf("err = %v", err)
	}
}

// failAfter fails every write once it has taken limit bytes.
type failAfter struct{ limit int }

func (f *failAfter) Write(p []byte) (int, error) {
	if f.limit < len(p) {
		return 0, errors.New("disk full")
	}
	f.limit -= len(p)
	return len(p), nil
}

func TestWriteZipReportsAFailureWhileWritingAnEntry(t *testing.T) {
	// A large incompressible-enough entry forces writes through to the
	// destination before the archive is closed.
	data := make([]byte, 1<<20)
	for i := range data {
		data[i] = byte(i*7 + i>>8)
	}
	err := writeZip(&failAfter{limit: 100}, []entry{{"a", data, 0o644}, {"b", data, 0o644}})
	if err == nil || !strings.Contains(err.Error(), "disk full") {
		t.Fatalf("err = %v", err)
	}
}

func TestCheckRejects(t *testing.T) {
	tests := []struct {
		name string
		edit func(string) string
		mode func(string, fs.FileMode) fs.FileMode
		drop string
		add  *entry
		want string
	}{
		{name: "a good bundle passes"},
		{name: "wrong manifest version", edit: replacing(`"manifest_version": "0.3"`, `"manifest_version": "0.2"`), want: "manifest_version must be 0.3"},
		{name: "wrong server name", edit: replacing(`"name": "presence"`, `"name": "other"`), want: "last segment of the hooks"},
		{name: "brand in the display name", edit: replacing(`"Rich Presence"`, `"Claude Presence"`), want: "brand names"},
		{name: "no display name", edit: replacing(`"display_name": "Rich Presence"`, `"display_name": ""`), want: "display_name, author.name"},
		{name: "wrong license", edit: replacing(`"license": "MIT"`, `"license": "GPL-3.0"`), want: "license must be MIT"},
		{name: "version is not semantic", edit: replacing(testVersion, "one"), want: "not a semantic version"},
		{name: "version differs from the source", edit: replacing(testVersion, "9.9.9"), want: `version is "9.9.9"`},
		{name: "no unaffiliated notice", edit: replacing("not affiliated", "nothing"), want: "unaffiliated notice"},
		{name: "manifest is not JSON", edit: func(string) string { return "{" }, want: "manifest.json is not valid"},
		{name: "unknown manifest key", edit: replacing(`"license"`, `"licence"`), want: "manifest.json is not valid"},
		{name: "wrong server type", edit: replacing(`"type": "binary"`, `"type": "node"`), want: "server.type must be binary"},
		{name: "wrong arguments", edit: replacing(`"args": ["mcp"],`, `"args": ["status"],`), want: "mcp_config.args"},
		{name: "entry point differs", edit: replacing(`"entry_point": "server/rich-presence-linux"`, `"entry_point": "server/rich-presence.exe"`), want: "entry_point must be"},
		{name: "unsupported platform", edit: replacing(`"win32": {`, `"plan9": {`), want: "not a supported platform"},
		{name: "missing platform", edit: replacing(`"win32": {`, `"plan9": {`), want: "no entry for win32"},
		{name: "wrong command", edit: replacing(`"${__dirname}/server/rich-presence.exe"`, `"${__dirname}/server/other.exe"`), want: "win32.command must be"},
		{name: "override without environment", edit: replacing(`"darwin": {
          "command": "${__dirname}/server/rich-presence-darwin",
          "args": ["mcp"],
          "env": {`, `"darwin": {
          "command": "${__dirname}/server/rich-presence-darwin",
          "args": ["mcp"],
          "env": {
            "EXTRA": "x",`), want: "darwin must repeat"},
		{name: "environment refers to an undefined setting", edit: replacing(`${user_config.privacy}`, `${user_config.nothing}`), want: "user_config.nothing, which is not defined"},
		{name: "setting not passed", edit: replacing(`"discord_application_id": {`, `"unused": {`), want: "user_config.unused is not passed"},
		{name: "unknown setting type", edit: replacing(`"type": "string",
      "title": "Privacy level"`, `"type": "text",
      "title": "Privacy level"`), want: `has type "text"`},
		{name: "setting without a description", edit: replacing(`"title": "Privacy level",`, `"title": "",`), want: "needs a title"},
		{name: "required setting", edit: replacing(`"required": false`, `"required": true`), want: "must not be required"},
		{name: "setting with a default", edit: replacing(`"title": "Privacy level",`, `"title": "Privacy level", "default": "standard",`), want: "user_config.privacy must have no default"},
		{name: "tools not generated", edit: replacing(`"tools_generated": true`, `"tools_generated": false`), want: "tools_generated"},
		{name: "tools listed", edit: replacing(`"tools_generated": true`, `"tools_generated": true, "tools": []`), want: "must not list tools"},
		{name: "wrong platforms", edit: replacing(`["darwin", "linux", "win32"]`, `["darwin", "linux"]`), want: "compatibility.platforms"},
		{name: "icon missing from the bundle", edit: replacing(`"icon.png"`, `"other.png"`), want: "names other.png"},
		{name: "mac binary not executable", mode: func(p string, m fs.FileMode) fs.FileMode {
			if p == "server/rich-presence-darwin" {
				return 0o644
			}
			return m
		}, want: "server/rich-presence-darwin is not executable"},
		{name: "linux binary not executable", mode: func(p string, m fs.FileMode) fs.FileMode {
			if p == "server/rich-presence-linux" {
				return 0o644
			}
			return m
		}, want: "server/rich-presence-linux is not executable"},
		{name: "windows binary needs no mode", mode: func(p string, m fs.FileMode) fs.FileMode {
			if p == "server/rich-presence.exe" {
				return 0o644
			}
			return m
		}},
		{name: "missing license", drop: licensePath, want: "LICENSE.md is missing"},
		{name: "missing notices", drop: noticesPath, want: "THIRD-PARTY-NOTICES.md is missing"},
		{name: "missing binary", drop: "server/rich-presence-linux", want: "server/rich-presence-linux is missing"},
		{name: "missing manifest", drop: manifestPath, want: "manifest.json is missing"},
		{name: "extra file", add: &entry{"server/extra", []byte("x"), 0o644}, want: "server/extra is not part of the bundle layout"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			edit, mode := tt.edit, tt.mode
			if edit == nil {
				edit = keep
			}
			if mode == nil {
				mode = sameMode
			}
			var list []entry
			for _, e := range entriesOf(t, edit, mode) {
				if e.name != tt.drop {
					list = append(list, e)
				}
			}
			if tt.add != nil {
				list = append(list, *tt.add)
			}
			problems := strings.Join(check(zipOf(t, list), testVersion), "\n")
			if tt.want == "" {
				if problems != "" {
					t.Fatalf("problems: %s", problems)
				}
				return
			}
			if !strings.Contains(problems, tt.want) {
				t.Fatalf("problems %q do not contain %q", problems, tt.want)
			}
		})
	}
}

func TestCheckRejectsWrongArchitectures(t *testing.T) {
	tests := []struct {
		name   string
		source string
		data   []byte
		want   string
	}{
		{"windows on arm64", "windows", peFile(0xaa64), "Windows machine type 0xaa64"},
		{"windows is not an executable", "windows", []byte("MZ"), "not a Windows executable"},
		{"linux on arm64", "linux", elfFile(183), "Linux machine"},
		{"linux is not an executable", "linux", []byte("MZ"), "not a Linux executable"},
		{"mac binary is thin", "darwin", machoSlice(cpuArm64), "not a universal Mac binary"},
		{"mac binary lacks arm64", "darwin", fatFile(cpuAmd64), "Mac architectures"},
		{"mac binary lacks amd64", "darwin", fatFile(cpuArm64), "Mac architectures"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			files := goodFiles(t)
			files[tt.source] = tt.data
			archive, problems := assemble(testVersion, files)
			if archive != nil || !strings.Contains(strings.Join(problems, "\n"), tt.want) {
				t.Fatalf("problems %q do not contain %q", problems, tt.want)
			}
		})
	}
}

func TestCheckRejectsAnArchiveThatIsNotAZip(t *testing.T) {
	problems := check([]byte("plain text"), testVersion)
	if len(problems) != 1 || !strings.Contains(problems[0], "not a zip archive") {
		t.Fatalf("problems = %v", problems)
	}
}

func TestCheckReportsAnEntryThatCannotBeRead(t *testing.T) {
	// Entries are stored, so changing a byte of one breaks the checksum of
	// that entry alone.
	for _, marker := range []string{"manifest_version", string(elfFile(62)[:4])} {
		archive := zipOf(t, entriesOf(t, keep, sameMode))
		i := bytes.Index(archive, []byte(marker))
		if i < 0 {
			t.Fatalf("%q not found in the archive", marker)
		}
		archive[i+3] ^= 0xff
		problems := strings.Join(check(archive, testVersion), "; ")
		if !strings.Contains(problems, "cannot be read") {
			t.Errorf("%q: problems = %q", marker, problems)
		}
	}
}

func TestCheckReportsAnEntryWithAnUnknownCompression(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, e := range entriesOf(t, keep, sameMode) {
		h := &zip.FileHeader{Name: e.name, Method: zip.Store}
		h.SetMode(e.mode)
		if e.name == licensePath {
			h.Method = 99
		}
		w, err := zw.CreateRaw(&zip.FileHeader{Name: h.Name, Method: h.Method, CRC32: 0, CompressedSize64: uint64(len(e.data)), UncompressedSize64: uint64(len(e.data)), ExternalAttrs: h.ExternalAttrs, CreatorVersion: h.CreatorVersion})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write(e.data); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	// The license is not read by check, so ask for it directly.
	zr, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatal(err)
	}
	c := checker{files: map[string]*zip.File{}}
	for _, f := range zr.File {
		c.files[f.Name] = f
	}
	if _, ok := c.read(licensePath); ok || len(c.problems) != 1 {
		t.Fatalf("ok=%v problems=%v", ok, c.problems)
	}
}

func TestMapsEqual(t *testing.T) {
	a := map[string]string{"k": "v"}
	tests := []struct {
		name string
		b    map[string]string
		want bool
	}{
		{"same", map[string]string{"k": "v"}, true},
		{"different length", map[string]string{}, false},
		{"different value", map[string]string{"k": "w"}, false},
		{"different key", map[string]string{"j": "v"}, false},
	}
	for _, tt := range tests {
		if got := mapsEqual(a, tt.b); got != tt.want {
			t.Errorf("%s: got %v", tt.name, got)
		}
	}
}
