package domain_test

import (
	"errors"
	"fmt"
	"math/rand/v2"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Zafnok/claude-rich-presence/internal/domain"
)

func TestNewRegistryIsEmpty(t *testing.T) {
	if snap := domain.NewRegistry().Snapshot(); len(snap) != 0 {
		t.Errorf("Snapshot() = %v, want no sessions", snap)
	}
}

func TestApplyReturnsTheNewSnapshot(t *testing.T) {
	r := domain.NewRegistry()
	e := event(domain.KindTurnStarted, t0)
	e.SessionID = "b"
	if _, err := r.Apply(e); err != nil {
		t.Fatal(err)
	}
	e.SessionID = "a"
	got, err := r.Apply(e)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(got, r.Snapshot()) {
		t.Errorf("Apply returned %v, Snapshot() is %v", got, r.Snapshot())
	}
	if len(got) != 2 || got[0].ID != "a" || got[1].ID != "b" {
		t.Errorf("snapshot = %v, want sessions a then b", got)
	}
	if got[0].Status != domain.StatusWorking || got[1].Status != domain.StatusWorking {
		t.Errorf("snapshot = %v, want both working", got)
	}
}

func TestInvalidEventLeavesRegistryUnchanged(t *testing.T) {
	r := registryWith(t, session(working))
	before := r.Snapshot()

	bad := []domain.Event{
		{SessionID: "s1", Surface: domain.SurfaceCode, At: t0, Kind: "nonsense"},
		{SessionID: "s1", Surface: domain.SurfaceCode, At: t0, Kind: domain.KindToolStarted},
		{SessionID: "s1", Surface: domain.SurfaceCode, Kind: domain.KindSessionEnded},
		{SessionID: "new", Surface: "web", At: t0, Kind: domain.KindSessionOpened},
	}
	for _, e := range bad {
		snap, err := r.Apply(e)
		if !errors.Is(err, domain.ErrInvalidEvent) {
			t.Errorf("Apply(%+v) = %v, want an error wrapping ErrInvalidEvent", e, err)
		}
		if snap != nil {
			t.Errorf("Apply(%+v) returned a snapshot with its error", e)
		}
		if after := r.Snapshot(); !slices.Equal(after, before) {
			t.Errorf("after Apply(%+v) the registry is %v, want %v", e, after, before)
		}
	}
}

func TestSyncReplacesWholesale(t *testing.T) {
	r := domain.NewRegistry()
	e := event(domain.KindToolStarted, t0)
	if _, err := r.Apply(e); err != nil {
		t.Fatal(err)
	}
	e = event(domain.KindSubagentStarted, t0)
	if _, err := r.Apply(e); err != nil {
		t.Fatal(err)
	}

	want := domain.Session{
		ID:           "s1",
		Surface:      domain.SurfaceDesktop,
		Status:       domain.StatusIdle,
		Privacy:      domain.PrivacyFull,
		Start:        t0.Add(time.Hour),
		LastActivity: t0.Add(2 * time.Hour),
	}
	if err := r.Sync(want); err != nil {
		t.Fatal(err)
	}
	if got := r.Snapshot(); len(got) != 1 || got[0] != want {
		t.Errorf("Snapshot() = %+v, want only %+v", got, want)
	}
}

func TestSyncTwiceIsSyncOnce(t *testing.T) {
	other := session(idle)
	other.ID = "s2"

	once := registryWith(t, other, session(working))
	twice := registryWith(t, other, session(working), session(working))
	if !slices.Equal(once.Snapshot(), twice.Snapshot()) {
		t.Errorf("once = %+v, twice = %+v", once.Snapshot(), twice.Snapshot())
	}
}

func TestInvalidSyncLeavesRegistryUnchanged(t *testing.T) {
	r := registryWith(t, session(working))
	before := r.Snapshot()

	bad := session(idle)
	bad.Subagents = -1
	if err := r.Sync(bad); !errors.Is(err, domain.ErrInvalidSession) {
		t.Errorf("Sync() = %v, want an error wrapping ErrInvalidSession", err)
	}
	if after := r.Snapshot(); !slices.Equal(after, before) {
		t.Errorf("registry is %v, want %v", after, before)
	}
}

func TestRemove(t *testing.T) {
	other := session(idle)
	other.ID = "s2"
	r := registryWith(t, session(working), other)

	r.Remove("s1")
	if got := r.Snapshot(); len(got) != 1 || got[0].ID != "s2" {
		t.Errorf("Snapshot() = %v, want only s2", got)
	}
	r.Remove("unknown")
	if got := r.Snapshot(); len(got) != 1 {
		t.Errorf("removing an unknown id changed the registry: %v", got)
	}
}

func TestSnapshotCannotMutateRegistry(t *testing.T) {
	r := registryWith(t, session(working))
	want := r.Snapshot()

	fromApply, err := r.Apply(event(domain.KindToolFinished, t0))
	if err != nil {
		t.Fatal(err)
	}
	for _, snap := range [][]domain.Session{fromApply, r.Snapshot()} {
		snap[0].Status = domain.StatusIdle
		snap[0].ID = "changed"
		snap[0].Subagents = 99
		_ = append(snap[:0], domain.Session{ID: "injected"})
	}

	if got := r.Snapshot(); !slices.Equal(got, want) {
		t.Errorf("Snapshot() = %+v, want %+v", got, want)
	}
}

// TestRandomSequences drives registries with random operations, valid and
// not, and checks after each one that every session is well formed and that
// a rejected operation changed nothing.
func TestRandomSequences(t *testing.T) {
	ids := []string{"a", "b", "c", "", strings.Repeat("x", domain.MaxIDLen+1)}
	surfaces := []domain.Surface{domain.SurfaceCode, domain.SurfaceDesktop, "web"}
	kinds := append(domain.Kinds(), "nonsense")
	tools := append(domain.ToolKinds(), domain.ToolNone, "Bash")
	models := []string{"", "Opus", strings.Repeat("m", domain.MaxModelLen+1)}
	projects := []string{"", "demo", strings.Repeat("p", domain.MaxProjectLen+1)}
	privacies := []domain.Privacy{"", domain.PrivacyMinimal, domain.PrivacyStandard, domain.PrivacyFull, "all"}
	statuses := append(domain.Statuses(), "thinking")

	for seed := uint64(1); seed <= 50; seed++ {
		t.Run(fmt.Sprint("seed ", seed), func(t *testing.T) {
			rng := rand.New(rand.NewPCG(seed, seed))
			at := func() time.Time {
				if rng.IntN(20) == 0 {
					return time.Time{}
				}
				return t0.Add(time.Duration(rng.IntN(7200)-3600) * time.Second)
			}
			r := domain.NewRegistry()

			for step := 0; step < 400; step++ {
				before := r.Snapshot()
				var err error
				switch rng.IntN(10) {
				case 0:
					r.Remove(ids[rng.IntN(len(ids))])
				case 1:
					err = r.Sync(domain.Session{
						ID:           ids[rng.IntN(len(ids))],
						Surface:      surfaces[rng.IntN(len(surfaces))],
						Status:       statuses[rng.IntN(len(statuses))],
						Tool:         tools[rng.IntN(len(tools))],
						Model:        models[rng.IntN(len(models))],
						Project:      projects[rng.IntN(len(projects))],
						Privacy:      privacies[rng.IntN(len(privacies))],
						Start:        at(),
						LastActivity: at(),
						Subagents:    rng.IntN(5) - 1,
					})
				default:
					_, err = r.Apply(domain.Event{
						SessionID: ids[rng.IntN(3)],
						Surface:   surfaces[rng.IntN(2+rng.IntN(2))],
						At:        at(),
						Kind:      kinds[rng.IntN(len(kinds))],
						Tool:      tools[rng.IntN(len(tools))],
						Model:     models[rng.IntN(len(models))],
						Project:   projects[rng.IntN(2+rng.IntN(2))],
						Privacy:   privacies[rng.IntN(len(privacies))],
					})
				}

				after := r.Snapshot()
				if err != nil && !slices.Equal(after, before) {
					t.Fatalf("step %d: rejected with %v but the registry changed", step, err)
				}
				if !slices.IsSortedFunc(after, func(a, b domain.Session) int { return strings.Compare(a.ID, b.ID) }) {
					t.Fatalf("step %d: snapshot is not ordered by id", step)
				}
				for _, s := range after {
					if err := s.Validate(); err != nil {
						t.Fatalf("step %d: session %+v: %v", step, s, err)
					}
				}
			}
		})
	}
}
