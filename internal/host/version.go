package host

import (
	"cmp"
	"strings"
)

// newer reports whether binary version a is newer than b.
//
// A version is read as numbers separated by dots, as in 1.4.0, then
// optionally a hyphen and pre-release identifiers separated by dots, as the
// Go toolchain writes for a build that is not a release:
// v0.0.0-20261003120000-0123456789ab. A leading v and anything from a plus
// sign on are ignored. Numbers compare as numbers and a missing one counts
// as zero; a pre-release is older than its release; pre-release identifiers
// compare as in semantic versioning, so two toolchain versions compare by
// their timestamps.
//
// A version that does not read that way, such as (devel), is neither newer
// nor older than anything. The order is strict, so no two versions are each
// newer than the other.
func newer(a, b string) bool {
	x, ok := parseVersion(a)
	if !ok {
		return false
	}
	y, ok := parseVersion(b)
	return ok && x.compare(y) > 0
}

// version is a binary version taken apart. Every number is a string of
// digits and no identifier is empty.
type version struct {
	numbers    []string
	prerelease []string
}

func parseVersion(s string) (version, bool) {
	s = strings.TrimPrefix(s, "v")
	s, _, _ = strings.Cut(s, "+")
	core, pre, hasPre := strings.Cut(s, "-")
	v := version{numbers: strings.Split(core, ".")}
	for _, n := range v.numbers {
		if !isNumber(n) {
			return version{}, false
		}
	}
	if hasPre {
		v.prerelease = strings.Split(pre, ".")
		for _, id := range v.prerelease {
			if id == "" {
				return version{}, false
			}
		}
	}
	return v, true
}

// compare is negative, zero or positive as v is older than, the same as or
// newer than o.
func (v version) compare(o version) int {
	for i := range max(len(v.numbers), len(o.numbers)) {
		if c := compareNumbers(numberAt(v.numbers, i), numberAt(o.numbers, i)); c != 0 {
			return c
		}
	}
	if len(v.prerelease) == 0 || len(o.prerelease) == 0 {
		// A release is newer than its pre-releases.
		return cmp.Compare(len(o.prerelease), len(v.prerelease))
	}
	for i := range min(len(v.prerelease), len(o.prerelease)) {
		if c := compareIdentifiers(v.prerelease[i], o.prerelease[i]); c != 0 {
			return c
		}
	}
	return cmp.Compare(len(v.prerelease), len(o.prerelease))
}

// numberAt is the number at position i, or zero if there is none.
func numberAt(numbers []string, i int) string {
	if i < len(numbers) {
		return numbers[i]
	}
	return "0"
}

// isNumber reports whether s is one or more digits.
func isNumber(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return s != ""
}

// compareNumbers compares two strings of digits as numbers of any size.
func compareNumbers(a, b string) int {
	a, b = strings.TrimLeft(a, "0"), strings.TrimLeft(b, "0")
	return cmp.Or(cmp.Compare(len(a), len(b)), strings.Compare(a, b))
}

// compareIdentifiers compares two pre-release identifiers: numbers as
// numbers, a number before anything else, and the rest as text.
func compareIdentifiers(a, b string) int {
	switch na, nb := isNumber(a), isNumber(b); {
	case na && nb:
		return compareNumbers(a, b)
	case na:
		return -1
	case nb:
		return 1
	}
	return strings.Compare(a, b)
}
