package host_test

import (
	"fmt"
	"math/rand/v2"
	"os"
	"slices"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/Zafnok/claude-rich-presence/internal/domain"
	"github.com/Zafnok/claude-rich-presence/internal/host"
)

// seedEnv names an environment variable that replaces the seed of the
// randomised test, to explore or to repeat a failure.
const seedEnv = "RICH_PRESENCE_TEST_SEED"

// fixedSeed is what the randomised test runs with unless told otherwise, so
// that a run in CI is the same run every time.
const fixedSeed = 20261004

// dice is a random source that several goroutines may use.
type dice struct {
	mu  sync.Mutex
	rng *rand.Rand
}

func newDice(seed uint64) *dice {
	return &dice{rng: rand.New(rand.NewPCG(seed, seed))}
}

func (d *dice) float() float64 {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.rng.Float64()
}

// holder is the process that holds the lock, or nil.
func (w *world) lockHolder() *proc {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.holder == nil {
		return nil
	}
	return w.holder.p
}

// soleHost returns the one node among live that is host, or nil if none is.
// It fails the test if a node is host without holding the lock, which is
// what two hosts at once would look like: the lock has one holder.
func (w *world) soleHost(live []*proc) *proc {
	w.t.Helper()
	var found *proc
	for _, p := range live {
		if !p.isHost() {
			continue
		}
		// A node gives up the role before it gives up the lock, so a host
		// that is still host after the lock was read was its holder then.
		holder := w.lockHolder()
		if p.isHost() && holder != p {
			w.t.Fatalf("%s is host without holding the lock\n%s", p.name, w.dump())
		}
		if p.isHost() {
			// A host seen earlier in this look may have stood down since.
			if found != nil && found.isHost() {
				w.t.Fatalf("%s and %s are both host\n%s", found.name, p.name, w.dump())
			}
			found = p
		}
	}
	return found
}

// quiet reports whether the nodes have settled: one of them is host, the
// rest follow it, and its registry holds exactly the sessions of them all.
// With no node there is nothing to settle.
func (w *world) quiet(live []*proc) bool {
	w.t.Helper()
	h := w.soleHost(live)
	if len(live) == 0 {
		return true
	}
	if h == nil {
		return false
	}
	for _, p := range live {
		if p != h && !p.isFollower() {
			return false
		}
	}
	return h.holds(live...)
}

func TestABurstOfNodesElectsOneHost(t *testing.T) {
	// A plugin installed while thirteen sessions were open started thirteen
	// adapters within the same second (CRP-001).
	const n = 13
	w := newWorld(t)
	jitter := newDice(7)
	w.jitter = jitter.float

	procs := make([]*proc, n)
	for i := range procs {
		procs[i] = w.spawn(fmt.Sprintf("n%02d", i)).open()
	}
	var ready sync.WaitGroup
	begin := make(chan struct{})
	for _, p := range procs {
		ready.Add(1)
		go func() {
			defer ready.Done()
			<-begin
			p.run()
		}()
	}
	close(begin)
	ready.Wait()

	w.settle("one host that holds all thirteen sessions", func() bool { return w.quiet(procs) })
	h := w.soleHost(procs)
	if !h.shows("13 sessions") {
		t.Errorf("the activity does not count thirteen sessions")
	}
	// The rest keep following: nothing changes when time passes.
	for range 50 {
		w.clock.Advance(host.RetryBase)
		if w.soleHost(procs) != h {
			t.Fatal("the host changed with nothing happening")
		}
	}
	if !w.quiet(procs) {
		t.Errorf("the nodes did not stay settled\n%s", w.dump())
	}
}

func TestRandomStartsPublishesAndStops(t *testing.T) {
	seed := uint64(fixedSeed)
	if text := os.Getenv(seedEnv); text != "" {
		parsed, err := strconv.ParseUint(text, 10, 64)
		if err != nil {
			t.Fatalf("%s is not a seed: %v", seedEnv, err)
		}
		seed = parsed
	}
	// Registered before the world, so that it runs after the world's own
	// checks and reports their failures too.
	t.Cleanup(func() {
		if t.Failed() {
			t.Logf("failed with seed %d; repeat it with %s=%d", seed, seedEnv, seed)
		}
	})
	steps := 400
	if testing.Short() {
		steps = 100
	}

	w := newWorld(t)
	rng := rand.New(rand.NewPCG(seed, 1))
	jitter := newDice(seed)
	w.jitter = jitter.float
	kinds := domain.Kinds()
	// Mostly one version, as in life, and now and then a newer one, whose
	// node makes an older host stand down.
	versions := []string{"1.0.0", "1.0.0", "1.0.0", "1.1.0"}

	var live []*proc
	started := 0
	pick := func() (int, *proc) {
		i := rng.IntN(len(live))
		return i, live[i]
	}
	for step := range steps {
		what := ""
		switch roll := rng.IntN(20); {
		case len(live) == 0, roll < 3 && len(live) < 9:
			started++
			p := w.spawn(fmt.Sprintf("n%03d", started), version(versions[rng.IntN(len(versions))]))
			if rng.IntN(4) > 0 {
				p.open()
			}
			live = append(live, p.run())
			what = "start " + p.name
		case roll < 5:
			i, p := pick()
			p.stop()
			live = slices.Delete(live, i, i+1)
			what = "stop " + p.name
		case roll < 7:
			i, p := pick()
			p.kill()
			live = slices.Delete(live, i, i+1)
			what = "kill " + p.name
		case roll < 9:
			d := time.Duration(1+rng.IntN(3000)) * time.Millisecond
			w.clock.Advance(d)
			what = "advance " + d.String()
		default:
			_, p := pick()
			kind := kinds[rng.IntN(len(kinds))]
			p.publish(kind)
			what = "publish " + string(kind) + " on " + p.name
		}
		w.note(fmt.Sprintf("step %d: %s", step, what))

		// After each step there are never two hosts. Most steps do not wait
		// for the one before to play out, so that starts, stops and kills
		// land on nodes that are in the middle of changing role.
		w.soleHost(live)
		if rng.IntN(3) > 0 {
			continue
		}
		// Once the nodes have settled, the host's registry is the union of
		// the live nodes' sessions.
		w.settle(fmt.Sprintf("the nodes to settle after step %d (%s)", step, what), func() bool { return w.quiet(live) })
	}
	w.settle("the nodes to settle at the end", func() bool { return w.quiet(live) })
}
