package e2e

import (
	"os"
	"slices"
	"testing"
	"time"

	"github.com/Zafnok/claude-rich-presence/internal/testutil/fakediscord"
)

// The budget for one call of the event tool, from the request being written
// to the response being read, at the 99th percentile (ADR-0008).
//
// On a developer's machine it is ten milliseconds, which is the budget of
// the ticket: the call is a parse, a few lines of memory and a write to a
// pipe, and takes a fraction of that.
//
// In CI the bound is relaxed to a hundred milliseconds. A hosted runner has
// two to four shared cores, and this test runs beside the race-instrumented
// tests of every other package, so a process is sometimes not scheduled for
// tens of milliseconds whatever it is doing. The relaxed bound still fails
// for what the budget exists to catch: a call that waits for Discord or for
// the host, whose timeouts are counted in seconds.
const (
	budgetDeveloper = 10 * time.Millisecond
	budgetCI        = 100 * time.Millisecond
)

// latencyCalls is how many calls a round measures, and latencyRounds how
// many rounds may be tried.
//
// The scenario runs on real time, on a machine that is doing other things:
// compiling, running the other packages' tests, scanning new executables. A
// process that is not scheduled for twenty milliseconds has not been slow.
// So each round also measures a control, the protocol's ping, which travels
// the same pipes and the same server loop and touches no tool, in turn with
// the event tool, so that whatever disturbs one disturbs the other.
//
//   - A round in which the event tool meets the budget settles it: the
//     product can do it, and did.
//   - A round in which the event tool misses the budget and the control
//     meets it counts against the product.
//   - A round in which the control misses the budget too says nothing about
//     the product, and is tried again.
//
// If every round is of the last kind, the machine could not show a no-op
// within the budget, and the budget was not tested. On a developer's machine
// that skips the scenario, with the numbers. In CI it fails: there the bound
// is ten times looser, the scenario must not rot unseen, and a runner that
// cannot answer a ping in a hundred milliseconds is worth knowing about.
const (
	latencyCalls  = 500
	latencyRounds = 5
)

// E10: the event tool answers within the budget, whatever Discord is doing.
//
// The subtests do not run in parallel with each other or with any other
// scenario, so that what is measured is the product and not the suite.
func TestEventLatency(t *testing.T) {
	budget, inCI := budgetDeveloper, os.Getenv("CI") != ""
	if inCI {
		budget = budgetCI
	}
	tests := []struct {
		name    string
		discord func(s *scene)
		// ready is the status that says the scene has settled.
		ready string
	}{
		{"Discord connected", func(s *scene) { s.discord(fakediscord.Behavior{}) }, "Discord: connected"},
		{"Discord not running", func(*scene) {}, "Discord: disconnected"},
		// A Discord that accepts the connection and then says nothing, so
		// that the host is in the middle of a handshake.
		{"Discord not answering", func(s *scene) { s.discord(fakediscord.Behavior{Silent: true}) }, "Discord: connecting"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newScene(t)
			tt.discord(s)
			c := s.start("timed")
			c.awaitStatus("Role: host", tt.ready)

			tools := []string{"Bash", "Read", "Edit", "Grep"}
			met, missed := false, 0
			for round := 1; round <= latencyRounds && !met; round++ {
				events := make([]time.Duration, 0, latencyCalls)
				pings := make([]time.Duration, 0, latencyCalls)
				for i := range latencyCalls {
					began := time.Now()
					c.hook("PreToolUse", "tool_name", tools[i%len(tools)])
					events = append(events, time.Since(began))

					began = time.Now()
					if _, rpcErr := c.Request("ping", nil); rpcErr != nil {
						t.Fatalf("ping failed: %+v", rpcErr)
					}
					pings = append(pings, time.Since(began))
				}
				event, control := percentile99(events), percentile99(pings)
				t.Logf("round %d: 99th percentile of %d calls: the event tool %v, the control %v", round, latencyCalls, event, control)
				switch {
				case event <= budget:
					met = true
				case control <= budget:
					missed++
				}
			}
			c.finish()

			switch {
			case met:
			case missed > 0:
				t.Errorf("the event tool missed the budget of %v in every one of %d rounds, and in %d of them a ping met it", budget, latencyRounds, missed)
			case inCI:
				t.Errorf("the budget of %v could not be tested: in every one of %d rounds a ping missed it too", budget, latencyRounds)
			default:
				t.Skipf("the budget of %v could not be tested: this machine was too busy to answer even a ping within it, in every one of %d rounds", budget, latencyRounds)
			}
		})
	}
}

// percentile99 is the 99th percentile of some durations. It sorts them.
func percentile99(took []time.Duration) time.Duration {
	slices.Sort(took)
	return took[len(took)*99/100]
}
