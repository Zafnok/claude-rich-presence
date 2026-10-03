package domain_test

import (
	"testing"
	"time"

	"github.com/Zafnok/claude-rich-presence/internal/domain"
)

func TestActivityEqual(t *testing.T) {
	base := domain.Activity{
		Details:    "Claude Code",
		State:      "Editing",
		Start:      t0,
		LargeImage: "logo",
		LargeText:  "large",
		SmallImage: "working",
		SmallText:  "small",
		Type:       domain.ActivityPlaying,
	}
	with := func(mutate func(*domain.Activity)) domain.Activity {
		a := base
		mutate(&a)
		return a
	}
	cases := []struct {
		name  string
		other domain.Activity
		want  bool
	}{
		{"identical", base, true},
		{"same instant in another zone", with(func(a *domain.Activity) { a.Start = t0.In(time.FixedZone("x", 3600)) }), true},
		{"details", with(func(a *domain.Activity) { a.Details = "x" }), false},
		{"state", with(func(a *domain.Activity) { a.State = "x" }), false},
		{"start", with(func(a *domain.Activity) { a.Start = t0.Add(time.Second) }), false},
		{"no start", with(func(a *domain.Activity) { a.Start = time.Time{} }), false},
		{"large image", with(func(a *domain.Activity) { a.LargeImage = "x" }), false},
		{"large text", with(func(a *domain.Activity) { a.LargeText = "x" }), false},
		{"small image", with(func(a *domain.Activity) { a.SmallImage = "x" }), false},
		{"small text", with(func(a *domain.Activity) { a.SmallText = "x" }), false},
		{"type", with(func(a *domain.Activity) { a.Type = domain.ActivityWatching }), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := base.Equal(tc.other); got != tc.want {
				t.Errorf("Equal = %v, want %v", got, tc.want)
			}
			if got := tc.other.Equal(base); got != tc.want {
				t.Errorf("Equal reversed = %v, want %v", got, tc.want)
			}
		})
	}
	if !(domain.Activity{}).Equal(domain.Activity{}) {
		t.Error("two empty activities are not equal")
	}
}

func TestActivityTypesAreDistinct(t *testing.T) {
	types := []domain.ActivityType{
		domain.ActivityPlaying, domain.ActivityListening, domain.ActivityWatching, domain.ActivityCompeting,
	}
	seen := map[domain.ActivityType]bool{}
	for _, typ := range types {
		seen[typ] = true
	}
	if len(seen) != len(types) {
		t.Errorf("activity types collide: %v", types)
	}
	if (domain.Activity{}).Type != domain.ActivityPlaying {
		t.Error("the zero activity type is not playing")
	}
}
