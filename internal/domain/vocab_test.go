package domain_test

import (
	"testing"

	"github.com/Zafnok/claude-rich-presence/internal/domain"
)

func TestSurfaceValid(t *testing.T) {
	cases := map[domain.Surface]bool{
		domain.SurfaceCode:    true,
		domain.SurfaceDesktop: true,
		"":                    false,
		"web":                 false,
	}
	for s, want := range cases {
		if got := s.Valid(); got != want {
			t.Errorf("Surface(%q).Valid() = %v, want %v", s, got, want)
		}
	}
}

func TestStatusValid(t *testing.T) {
	for _, s := range domain.Statuses() {
		if !s.Valid() {
			t.Errorf("Status(%q).Valid() = false, want true", s)
		}
	}
	if got := len(domain.Statuses()); got != 4 {
		t.Errorf("len(Statuses()) = %d, want 4", got)
	}
	for _, s := range []domain.Status{"", "thinking"} {
		if s.Valid() {
			t.Errorf("Status(%q).Valid() = true, want false", s)
		}
	}
}

func TestToolKindValid(t *testing.T) {
	for _, k := range domain.ToolKinds() {
		if !k.Valid() {
			t.Errorf("ToolKind(%q).Valid() = false, want true", k)
		}
	}
	if got := len(domain.ToolKinds()); got != 8 {
		t.Errorf("len(ToolKinds()) = %d, want 8", got)
	}
	for _, k := range []domain.ToolKind{domain.ToolNone, "Bash"} {
		if k.Valid() {
			t.Errorf("ToolKind(%q).Valid() = true, want false", k)
		}
	}
}

func TestPrivacyValid(t *testing.T) {
	cases := map[domain.Privacy]bool{
		domain.PrivacyMinimal:  true,
		domain.PrivacyStandard: true,
		domain.PrivacyFull:     true,
		// A hidden session is never published, so off is not a level one has.
		domain.PrivacyOff: false,
		"":                false,
		"everything":      false,
	}
	for p, want := range cases {
		if got := p.Valid(); got != want {
			t.Errorf("Privacy(%q).Valid() = %v, want %v", p, got, want)
		}
		if got, want := p.Settable(), want || p == domain.PrivacyOff; got != want {
			t.Errorf("Privacy(%q).Settable() = %v, want %v", p, got, want)
		}
	}
}

func TestKindValid(t *testing.T) {
	seen := map[domain.Kind]bool{}
	for _, k := range domain.Kinds() {
		if !k.Valid() {
			t.Errorf("Kind(%q).Valid() = false, want true", k)
		}
		seen[k] = true
	}
	if len(seen) != 14 {
		t.Errorf("Kinds() has %d distinct kinds, want 14", len(seen))
	}
	for _, k := range []domain.Kind{"", "session_paused"} {
		if k.Valid() {
			t.Errorf("Kind(%q).Valid() = true, want false", k)
		}
	}
}
