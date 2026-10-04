package host_test

import (
	"testing"

	"github.com/Zafnok/claude-rich-presence/internal/host"
)

func TestNewer(t *testing.T) {
	// Each row is strictly older than the row after it.
	ascending := []string{
		"v0.0.0-20261003120000-0123456789ab",
		"v0.0.0-20261004090000-0123456789ab",
		"0.9",
		"0.9.1",
		"0.10.0",
		"1.0.0-1",
		"1.0.0-2",
		"1.0.0-10",
		"1.0.0-alpha",
		"1.0.0-alpha.1",
		"1.0.0-alpha.beta",
		"1.0.0-beta",
		"1.0.0-rc.1",
		"1.0.0",
		"1.0.1-0.20261003120000-0123456789ab",
		"1.0.1",
		"1.4.0",
		"v2",
		"2.0.0.1",
		"10.0.0",
		"99999999999999999999999999.0.0",
		"100000000000000000000000000.0.0",
	}
	for i, older := range ascending {
		for j, other := range ascending {
			if got, want := host.Newer(other, older), j > i; got != want {
				t.Errorf("newer(%q, %q) is %v, want %v", other, older, got, want)
			}
		}
	}

	// Different spellings of one version: neither is newer.
	for _, same := range [][2]string{
		{"1.4.0", "v1.4.0"},
		{"1.4.0", "1.4"},
		{"1.4.0", "1.4.0.0"},
		{"1.4.0", "1.04.0"},
		{"1.4.0", "1.4.0+dirty"},
		{"1.0.0-rc.1", "1.0.0-rc.01+build.5"},
	} {
		if host.Newer(same[0], same[1]) || host.Newer(same[1], same[0]) {
			t.Errorf("one of %q and %q is newer than the other, want neither", same[0], same[1])
		}
	}

	// What is not a version has no rank: it is neither newer nor older.
	for _, odd := range []string{"", "(devel)", "unknown", "1.x", "1..0", ".1", "1.0.0-", "1.0.0-rc..1", "-1", "v"} {
		if host.Newer(odd, "0.0.1") || host.Newer("999.0.0", odd) || host.Newer(odd, odd) {
			t.Errorf("%q was ranked against a version, want no rank", odd)
		}
	}
}

// FuzzNewer checks that the order is strict for any three strings: nothing
// is newer than itself, no two are each newer than the other, and newer
// carries across. Without that, two processes could each ask the other to
// stand down.
func FuzzNewer(f *testing.F) {
	f.Add("1.4.0", "1.3.2", "1.0.0")
	f.Add("v0.0.0-20261003120000-0123456789ab+dirty", "(devel)", "1.0.0-rc.1")
	f.Add("1.0.0-alpha.1", "1.0.0-alpha", "1.0.0-1")
	f.Add("", "v", "1..0")
	f.Add("1.0", "1.0.0", "1.00.0-0")
	f.Fuzz(func(t *testing.T, a, b, c string) {
		if host.Newer(a, a) {
			t.Errorf("%q is newer than itself", a)
		}
		ab, ba, bc, ac := host.Newer(a, b), host.Newer(b, a), host.Newer(b, c), host.Newer(a, c)
		if ab && ba {
			t.Errorf("%q and %q are each newer than the other", a, b)
		}
		if ab && bc && !ac {
			t.Errorf("%q is newer than %q, which is newer than %q, but %q is not newer than %q", a, b, c, a, c)
		}
	})
}
