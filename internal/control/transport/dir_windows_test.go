package transport_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf16"

	"github.com/Zafnok/claude-rich-presence/internal/control/transport"
)

// Windows does not honour Unix permission bits. What keeps other users out
// of the runtime directory is the access control it inherits from the
// user's temporary directory. This checks what can be checked: a directory
// made by Prepare under the real temporary directory grants nothing to the
// broad groups that would include another user.
func TestRuntimeDirectoryUnderTempAdmitsNoOtherUser(t *testing.T) {
	base, err := os.MkdirTemp(os.TempDir(), "rp")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(base) })
	dir := filepath.Join(base, "rich-presence")
	if err := transport.Prepare(dir); err != nil {
		t.Fatalf("Prepare() error = %v", err)
	}

	// icacls /save writes the access control list in SDDL, where the broad
	// groups have fixed two-letter names in every display language.
	saved := filepath.Join(base, "acl.txt")
	cmd := exec.Command("icacls", dir, "/save", saved)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("icacls: %v\n%s", err, out)
	}
	raw, err := os.ReadFile(saved)
	if err != nil {
		t.Fatal(err)
	}
	units := make([]uint16, len(raw)/2)
	for i := range units {
		units[i] = uint16(raw[2*i]) | uint16(raw[2*i+1])<<8
	}
	sddl := string(utf16.Decode(units))
	t.Logf("access control of %s: %s", dir, strings.TrimSpace(sddl))

	if !strings.Contains(sddl, "(A;") {
		t.Fatalf("no access entry found in %q", sddl)
	}
	for name, sid := range map[string]string{
		"Everyone":            "WD",
		"Users":               "BU",
		"Authenticated Users": "AU",
		"Interactive":         "IU",
		"Guests":              "BG",
		"Anonymous":           "AN",
	} {
		if strings.Contains(sddl, ";;;"+sid+")") {
			t.Errorf("the directory grants access to %s: %s", name, sddl)
		}
	}
}
