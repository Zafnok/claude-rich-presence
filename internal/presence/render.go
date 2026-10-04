package presence

import (
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Zafnok/claude-rich-presence/internal/domain"
)

// Limits on a text line. Discord rejects a line shorter than two characters.
// The upper limit is 128: Discord's old library counts it in bytes and other
// sources in characters, so a line is cut to 128 bytes, which is within the
// limit however it is counted.
const (
	minLineChars = 2
	maxLineBytes = 128
	ellipsis     = "…"
)

// Settings are the display settings the renderer needs.
type Settings struct {
	// IdleClear is how long every session must have been idle before the
	// activity is cleared. Zero or less means it is never cleared.
	IdleClear time.Duration
}

// Render returns the one activity Discord should show for a registry
// snapshot, or false for "show nothing". It is a pure function: the same
// sessions, in any order, with the same time and settings give the same
// result.
//
// The elapsed timer is the focus session's own start time. It jumps when
// focus moves to another session and at no other time.
func Render(sessions []domain.Session, now time.Time, set Settings) (domain.Activity, bool) {
	if len(sessions) == 0 || allIdleSince(sessions, now, set.IdleClear) {
		return domain.Activity{}, false
	}
	s := focus(sessions)
	surface := surfacePhrases[s.Surface]
	a := domain.Activity{
		Details:    line(surface),
		Start:      s.Start,
		LargeImage: assetLogo,
		LargeText:  surface,
		Type:       domain.ActivityPlaying,
	}
	// Anything but standard or full is treated as minimal, where only "in
	// use, and for how long" is shown. The status stays out of the small
	// image as well as the text.
	if s.Privacy != domain.PrivacyStandard && s.Privacy != domain.PrivacyFull {
		return a, true
	}

	status := statusPhrases[phraseKey{s.Surface, s.Status, s.Tool}]
	count := ""
	if len(sessions) > 1 {
		count = strconv.Itoa(len(sessions)) + " sessions"
	}
	a.State = line(status, s.Model, count)
	a.SmallImage = statusImages[s.Status]
	a.SmallText = status
	if s.Privacy == domain.PrivacyFull {
		a.Details = line(surface, s.Project)
	}
	return a, true
}

// allIdleSince reports whether every session is idle and was last active
// more than period ago. A period of zero or less never matches.
func allIdleSince(sessions []domain.Session, now time.Time, period time.Duration) bool {
	if period <= 0 {
		return false
	}
	for _, s := range sessions {
		if s.Status != domain.StatusIdle || now.Sub(s.LastActivity) <= period {
			return false
		}
	}
	return true
}

// ClearsIn returns how long after now Render first shows nothing for these
// sessions, if none of them changes. It reports false when that moment does
// not come, because there are no sessions, one of them is not idle or the
// activity is never cleared, and when it has passed already.
//
// The host arms a timer with it, so that the activity is cleared when the
// idle period ends and not at the next event.
func ClearsIn(sessions []domain.Session, now time.Time, set Settings) (time.Duration, bool) {
	if len(sessions) == 0 || set.IdleClear <= 0 {
		return 0, false
	}
	var last time.Time
	for _, s := range sessions {
		if s.Status != domain.StatusIdle {
			return 0, false
		}
		if s.LastActivity.After(last) {
			last = s.LastActivity
		}
	}
	// Render clears once more than the period has passed, so the moment is
	// the smallest step after it.
	wait := last.Add(set.IdleClear).Sub(now) + 1
	return wait, wait > 0
}

// line joins the parts that are not empty into one text line within
// Discord's limits. A line that is too long is cut between characters and
// ends with an ellipsis; one that is too short is left out.
func line(parts ...string) string {
	var kept []string
	for _, p := range parts {
		if p != "" {
			kept = append(kept, p)
		}
	}
	s := strings.ToValidUTF8(strings.Join(kept, separator), "")
	if len(s) > maxLineBytes {
		cut := maxLineBytes - len(ellipsis)
		for !utf8.RuneStart(s[cut]) {
			cut--
		}
		s = s[:cut] + ellipsis
	}
	if utf8.RuneCountInString(s) < minLineChars {
		return ""
	}
	return s
}
