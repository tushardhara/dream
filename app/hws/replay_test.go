package hws

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/tushardhara/dream/app/graph"
	"github.com/tushardhara/dream/core"
	"github.com/tushardhara/dream/simulator"
	"github.com/tushardhara/dream/simulator/behavior"
	rt "github.com/tushardhara/dream/simulator/runtime"
	"github.com/tushardhara/dream/simulator/scenario"
)

func frozenFixture(t testing.TB, state rt.State) FrozenState {
	t.Helper()
	f := FrozenState{Version: SnapshotVersion, Scope: Scope{Actor: "operator", Namespace: "replay-test", World: state.Genesis.World, Branch: "parent", Run: "parent"}, Revision: 1, Event: "boundary", Offset: 1, Engine: state.Engine, RNG: state.RNG, State: state, Memories: []FrozenMemory{}, Models: []ModelUse{}}
	var sc scenario.Scenario
	_ = json.Unmarshal(state.Genesis.Payload, &sc)
	for _, human := range sc.Public.Humans {
		scope, _ := (ViewRealm{Scope: f.Scope, Principal: human.ID}).MemoryScope()
		f.Memories = append(f.Memories, FrozenMemory{Scope: scope, Entries: []graph.MemoryEntry{}})
	}
	f, e := SealSnapshot(f)
	if e != nil {
		t.Fatal(e)
	}
	return f
}
func appendReplay(t testing.TB, b ReplayBundle, command rt.Command, h rt.Handler) ReplayBundle {
	t.Helper()
	state := b.Snapshot.State
	if len(b.Frames) > 0 {
		state = b.Frames[len(b.Frames)-1].State
	}
	before, _ := state.Hash()
	next, tr, _, e := rt.Apply(state, command, h)
	if e != nil {
		t.Fatal(e)
	}
	after, _ := next.Hash()
	b.Frames = append(b.Frames, ReplayFrame{Revision: b.Snapshot.Revision + int64(len(b.Frames)) + 1, Before: before, After: after, Command: command, State: next, Transition: tr})
	return b
}
func TestRecordedAndEventReplayExactAndTampering(t *testing.T) {
	state := cognitiveWorld(t)
	f := frozenFixture(t, state)
	h := CognitiveHandler{Source: cognitiveFunc(func(i rt.Input) (CognitiveFrame, error) { return cognitiveFrame(i), nil })}
	bundle := ReplayBundle{Snapshot: f, Frames: []ReplayFrame{}}
	bundle = appendReplay(t, bundle, rt.Command{Kind: "step"}, h)
	bundle = appendReplay(t, bundle, rt.Command{Kind: "step"}, h)
	for _, mode := range []ReplayMode{EventReplay, RecordedReplay} {
		result, e := Replay(bundle, mode)
		if e != nil || !result.Exact || result.Hashes[2] != bundle.Frames[1].After {
			t.Fatal("non-exact replay", mode, e)
		}
	}
	raw, _ := json.Marshal(bundle)
	var restart ReplayBundle
	if json.Unmarshal(raw, &restart) != nil {
		t.Fatal("decode")
	}
	result, e := Replay(restart, RecordedReplay)
	if e != nil || result.Hashes[2] != bundle.Frames[1].After {
		t.Fatal("recorded restart changed", e)
	}
	if _, e = Replay(bundle, FreshSimulation); e == nil {
		t.Fatal("fresh generation disguised as replay")
	}
	if _, e = DiffTrajectories(bundle, restart); e != nil {
		t.Fatal(e)
	}
	for _, which := range []string{"state", "draw", "command", "before", "revision", "snapshot", "unknown_engine"} {
		t.Run(which, func(t *testing.T) {
			var copy ReplayBundle
			_ = json.Unmarshal(raw, &copy)
			switch which {
			case "state":
				copy.Frames[0].State.Data = "tampered"
			case "draw":
				copy.Frames[0].Transition.Draws[0].Value++
			case "command":
				copy.Frames[0].Command.Kind = "pause"
			case "before":
				copy.Frames[0].Before = strings.Repeat("0", 64)
			case "revision":
				copy.Frames[0].Revision++
			case "snapshot":
				copy.Snapshot.State.Genesis.ScenarioHash = "bad"
			case "unknown_engine":
				copy.Snapshot.Engine = "future"
			}
			if _, e := Replay(copy, RecordedReplay); e == nil {
				t.Fatal("corruption accepted")
			}
		})
	}
	snapRaw, _ := json.Marshal(f)
	if _, e = DecodeSnapshot(snapRaw, strings.Repeat("0", 64)); e == nil {
		t.Fatal("caller hash bypass")
	}
	if _, e = DecodeSnapshot(append(snapRaw, []byte("{}")...), f.Hash); e == nil {
		t.Fatal("trailing snapshot")
	}
}
func TestForkRandomnessAlternativesAndParentImmutability(t *testing.T) {
	frozen := frozenFixture(t, cognitiveWorld(t))
	before, _ := json.Marshal(frozen)
	spec := ForkSpec{Version: 1, Source: SnapshotKey{Scope: frozen.Scope, ID: "snap", Hash: frozen.Hash}, Child: frozen.Scope, Mode: FreshSimulation, PairedExogenous: true, Policy: behavior.Policy, MaxDuration: time.Minute}
	spec.Child.Branch = "left"
	spec.Child.Run = "left"
	left, e := ForkState(frozen, spec)
	if e != nil {
		t.Fatal(e)
	}
	spec.Child.Branch = "right"
	spec.Child.Run = "right"
	right, e := ForkState(frozen, spec)
	if e != nil {
		t.Fatal(e)
	}
	l := rt.NewScopedRandom(42, left.Positions, left.RandomDomain, left.ExogenousDomain, left.Coupled)
	r := rt.NewScopedRandom(42, right.Positions, right.RandomDomain, right.ExogenousDomain, right.Coupled)
	le, _ := l.Draw("exogenous:weather")
	re, _ := r.Draw("exogenous:weather")
	if le != re {
		t.Fatal("paired exogenous coupling lost")
	}
	lc, _ := l.Draw("human-choice:a")
	rc, _ := r.Draw("human-choice:a")
	if lc == rc {
		t.Fatal("post-fork endogenous RNG not isolated")
	}
	after, _ := json.Marshal(frozen)
	if string(before) != string(after) {
		t.Fatal("parent mutated")
	}
	spec.Alternative = &rt.Input{ID: "alternative", At: 5, Kind: "observation", Actor: "a", Text: "permitted synthetic alternative", Priority: 1}
	changed, e := ForkState(frozen, spec)
	if e != nil || len(changed.Queue) != len(right.Queue)+1 {
		t.Fatal("alternative missing", e)
	}
	spec.Alternative.At = 0
	if _, e = ForkState(frozen, spec); e == nil {
		t.Fatal("retroactive fork input")
	}
	spec.Alternative = nil
	spec.Policy = "unimplemented-policy"
	if _, e = ForkState(frozen, spec); e == nil {
		t.Fatal("unknown policy silently adopted")
	}
	spec.Policy = behavior.Policy
	spec.Child.World = simulator.WorldID("foreign")
	if _, e = ForkState(frozen, spec); e == nil {
		t.Fatal("cross-world fork")
	}
}
func TestReservedCapacityAffordanceContinuesAndReplays(t *testing.T) {
	state := cognitiveWorld(t)
	var sc scenario.Scenario
	_ = json.Unmarshal(state.Genesis.Payload, &sc)
	sc.Future = append(sc.Future, scenario.Scheduled{ID: "future-reservation", At: 50, Until: 60, Kind: "reservation", Actor: "b", Text: "PRIVATE_FUTURE_CANARY", Resource: "hours", Units: 2})
	g, e := sc.Genesis(rt.Capabilities())
	if e != nil {
		t.Fatal(e)
	}
	state, e = rt.New(g, state.Budget)
	if e != nil {
		t.Fatal(e)
	}
	state.Status = "running"
	h := CognitiveHandler{Source: cognitiveFunc(func(i rt.Input) (CognitiveFrame, error) {
		f := cognitiveFrame(i)
		f.Situation.Offers = []behavior.Offer{{Kind: behavior.Help, Recipient: "b", Resource: "hours", Units: 1, Duration: 1}}
		return f, nil
	})}
	bundle := appendReplay(t, ReplayBundle{Snapshot: frozenFixture(t, state), Frames: []ReplayFrame{}}, rt.Command{Kind: "step"}, h)
	checkpoint, e := DecodeCognitiveCheckpoint(bundle.Frames[0].State.Data)
	if e != nil || checkpoint.Last.Candidates[checkpoint.Last.Selected].Offer.Kind != behavior.Wait || bundle.Frames[0].State.Available["hours"] != 2 {
		t.Fatal("affordance filter did not choose affordable WAIT", e)
	}
	if _, e = Replay(bundle, RecordedReplay); e != nil {
		t.Fatal("affordable choice did not replay", e)
	}
}

func TestTrajectoryDiffNamesAlternativeAndFollowOn(t *testing.T) {
	base := ReplayBundle{Snapshot: frozenFixture(t, cognitiveWorld(t)), Frames: []ReplayFrame{}}
	input := rt.Input{ID: "alternative", At: 5, Kind: "observation", Actor: "a", Text: "left", Priority: 1}
	left := appendReplay(t, base, rt.Command{Kind: "inject", Input: &input}, nil)
	// Commands are immutable records; do not mutate the left command's pointer.
	other := input
	other.Text = "right"
	right := appendReplay(t, base, rt.Command{Kind: "inject", Input: &other}, nil)
	difference, e := DiffTrajectories(left, right)
	if e != nil || difference.First != 1 || difference.CausalClaim || len(difference.ChangedInputs) != 1 || difference.ChangedInputs[0] != core.ID("alternative") {
		t.Fatal("missing precise trajectory difference", difference, e)
	}
	identical, e := DiffTrajectories(left, left)
	if e != nil || identical.First != -1 || len(identical.ChangedInputs) != 0 || identical.FollowOn != 0 {
		t.Fatal("identical trajectory differs", e)
	}
}
