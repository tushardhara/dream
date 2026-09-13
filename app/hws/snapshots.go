package hws

import (
	"context"
	"fmt"
	"time"

	"github.com/tushardhara/dream/core"
	"github.com/tushardhara/dream/simulator/behavior"
	rt "github.com/tushardhara/dream/simulator/runtime"
)

type ExperimentLabel struct {
	Mode     ReplayMode  `json:"mode"`
	Exact    bool        `json:"exact"`
	Coupling string      `json:"coupling"`
	Source   SnapshotKey `json:"source"`
}
type ForkSpec struct {
	Version         int           `json:"version"`
	Source          SnapshotKey   `json:"source"`
	Child           Scope         `json:"child"`
	Mode            ReplayMode    `json:"mode"`
	PairedExogenous bool          `json:"paired_exogenous"`
	Policy          string        `json:"policy"`
	Alternative     *rt.Input     `json:"alternative,omitempty"`
	MaxDuration     time.Duration `json:"max_duration_ns"`
}

func (s ForkSpec) Validate() error {
	p := s.Source.Scope
	if s.Version != 1 || p.Validate() != nil || s.Source.ID.Validate() != nil || len(s.Source.Hash) != 64 || s.Child.Validate() != nil || s.Child.Actor != p.Actor || s.Child.Namespace != p.Namespace || s.Child.World != p.World || s.Child.Branch == p.Branch || s.Child.Run == p.Run || s.Mode != FreshSimulation || s.Policy != behavior.Policy || s.MaxDuration <= 0 || s.MaxDuration > 24*time.Hour {
		return fmt.Errorf("invalid fork scope/mode/policy")
	}
	return nil
}
func ForkState(f FrozenState, s ForkSpec) (rt.State, error) {
	if f.Validate() != nil || s.Validate() != nil || f.Scope != s.Source.Scope || f.Hash != s.Source.Hash {
		return rt.State{}, fmt.Errorf("untrusted fork snapshot")
	}
	// A purged entry has no learned-time payload left. Until tombstone-only
	// inheritance is supported, making a child would drop its irreversible ID
	// reservation. Keep redacted replay available, but fail closed on this fork.
	for _, memory := range f.Memories {
		for _, entry := range memory.Entries {
			if entry.Revoked {
				return rt.State{}, fmt.Errorf("fork unavailable: snapshot contains purged memory")
			}
		}
	}
	state, e := f.State.Clone()
	if e != nil {
		return rt.State{}, e
	}
	hash, e := ModelDigest(s.Child)
	if e != nil {
		return rt.State{}, e
	}
	common := state.RandomDomain
	if state.Coupled {
		common = state.ExogenousDomain
	}
	state.Engine = rt.BranchEngineVersion
	state.RandomDomain = core.ID("fork:" + hash)
	state.Coupled = s.PairedExogenous
	state.ExogenousDomain = ""
	if state.Coupled {
		state.ExogenousDomain = common
	}
	state.Status = "paused"
	if s.Alternative != nil {
		state, _, _, e = rt.Apply(state, rt.Command{Kind: "inject", Input: s.Alternative}, nil)
		if e != nil {
			return rt.State{}, e
		}
	}
	return state, state.Validate()
}

type SnapshotStore interface {
	CaptureSnapshot(context.Context, Scope, core.ID, int64) (SnapshotKey, error)
	ReadSnapshot(context.Context, SnapshotKey) (FrozenState, error)
	ReadReplay(context.Context, SnapshotKey, int64) (ReplayBundle, error)
	ForkSnapshot(context.Context, ForkSpec) (Snapshot, error)
}

// SnapshotService returns handles or researcher-authorized replay results. A
// caller-provided snapshot body never authorizes restoring or creating a branch.
type SnapshotService struct {
	Store SnapshotStore
	Views *ViewService
}

func (s SnapshotService) authorize(ctx context.Context, p ViewPermit, scope Scope, operation core.Operation) error {
	if s.Store == nil || s.Views == nil {
		return ErrViewDenied
	}
	_, g, e := s.Views.snapshot(ctx, p)
	if e != nil {
		return e
	}
	if g.Kind != ResearchViewKind || g.Realm.Scope != scope || g.Purpose != "research" {
		return ErrViewDenied
	}
	allowed := false
	for _, op := range g.Operations {
		allowed = allowed || op == operation
	}
	if !allowed {
		return ErrViewDenied
	}
	_, e = s.Views.Research(ctx, p)
	return e
}
func (s SnapshotService) Capture(ctx context.Context, p ViewPermit, scope Scope, key core.ID, revision int64) (SnapshotKey, error) {
	if e := s.authorize(ctx, p, scope, core.Retain); e != nil {
		return SnapshotKey{}, e
	}
	return s.Store.CaptureSnapshot(ctx, scope, key, revision)
}
func (s SnapshotService) Replay(ctx context.Context, p ViewPermit, key SnapshotKey, through int64, mode ReplayMode) (ReplayResult, error) {
	if e := s.authorize(ctx, p, key.Scope, core.Read); e != nil {
		return ReplayResult{}, e
	}
	bundle, e := s.Store.ReadReplay(ctx, key, through)
	if e != nil {
		return ReplayResult{}, e
	}
	result, e := Replay(bundle, mode)
	if e != nil {
		return ReplayResult{}, e
	}
	// No cache of a previously authorized snapshot grants later access.
	if e = s.authorize(ctx, p, key.Scope, core.Read); e != nil {
		return ReplayResult{}, e
	}
	if _, e = s.Store.ReadReplay(ctx, key, through); e != nil {
		return ReplayResult{}, e
	}
	return result, nil
}
func (s SnapshotService) Fork(ctx context.Context, p ViewPermit, spec ForkSpec) (Snapshot, error) {
	if e := s.authorize(ctx, p, spec.Source.Scope, core.Derive); e != nil {
		return Snapshot{}, e
	}
	return s.Store.ForkSnapshot(ctx, spec)
}

type TrajectoryKey struct {
	Snapshot SnapshotKey
	Through  int64
}

func (s SnapshotService) Diff(ctx context.Context, leftPermit, rightPermit ViewPermit, left, right TrajectoryKey) (TrajectoryDifference, error) {
	if e := s.authorize(ctx, leftPermit, left.Snapshot.Scope, core.Read); e != nil {
		return TrajectoryDifference{}, e
	}
	if e := s.authorize(ctx, rightPermit, right.Snapshot.Scope, core.Read); e != nil {
		return TrajectoryDifference{}, e
	}
	a, e := s.Store.ReadReplay(ctx, left.Snapshot, left.Through)
	if e != nil {
		return TrajectoryDifference{}, e
	}
	b, e := s.Store.ReadReplay(ctx, right.Snapshot, right.Through)
	if e != nil {
		return TrajectoryDifference{}, e
	}
	difference, e := DiffTrajectories(a, b)
	if e != nil {
		return TrajectoryDifference{}, e
	}
	if e = s.authorize(ctx, leftPermit, left.Snapshot.Scope, core.Read); e != nil {
		return TrajectoryDifference{}, e
	}
	if e = s.authorize(ctx, rightPermit, right.Snapshot.Scope, core.Read); e != nil {
		return TrajectoryDifference{}, e
	}
	if _, e = s.Store.ReadReplay(ctx, left.Snapshot, left.Through); e != nil {
		return TrajectoryDifference{}, e
	}
	if _, e = s.Store.ReadReplay(ctx, right.Snapshot, right.Through); e != nil {
		return TrajectoryDifference{}, e
	}
	return difference, nil
}
