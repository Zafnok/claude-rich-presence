package presence

import (
	"cmp"
	"slices"

	"github.com/Zafnok/claude-rich-presence/internal/domain"
)

// statusRank and surfaceRank order sessions for focus: higher wins. A value
// that is not listed ranks lowest.
var statusRank = map[domain.Status]int{
	domain.StatusWorking:    3,
	domain.StatusWaiting:    2,
	domain.StatusCompacting: 1,
	domain.StatusIdle:       0,
}

var surfaceRank = map[domain.Surface]int{
	domain.SurfaceCode:    1,
	domain.SurfaceDesktop: 0,
}

// focus picks the session the activity describes: working over waiting over
// compacting over idle, then code over desktop, then the most recent
// activity, then the lowest session id, so that the same sessions in any
// order give the same answer. It must not be called with no sessions.
func focus(sessions []domain.Session) domain.Session {
	return slices.MinFunc(sessions, func(a, b domain.Session) int {
		return cmp.Or(
			cmp.Compare(statusRank[b.Status], statusRank[a.Status]),
			cmp.Compare(surfaceRank[b.Surface], surfaceRank[a.Surface]),
			b.LastActivity.Compare(a.LastActivity),
			cmp.Compare(a.ID, b.ID),
		)
	})
}
