package hws

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/tushardhara/dream/simulator/scenario"
	"io"
	"sort"
	"time"

	"github.com/tushardhara/dream/app/graph"
	"github.com/tushardhara/dream/core"
	rt "github.com/tushardhara/dream/simulator/runtime"
)

const SnapshotVersion = "snapshot.v1"
const MaxSnapshotBytes = 8 << 20
const MaxReplayFrames = 128

type ReplayMode string

const (
	EventReplay     ReplayMode = "event_replay"
	RecordedReplay  ReplayMode = "recorded_decision_replay"
	FreshSimulation ReplayMode = "fresh_resimulation"
)

type SnapshotKey struct {
	Scope Scope   `json:"scope"`
	ID    core.ID `json:"id"`
	Hash  string  `json:"hash"`
}
type FrozenMemory struct {
	Scope   graph.MemoryScope   `json:"scope"`
	Entries []graph.MemoryEntry `json:"entries"`
}
type ModelBudgetSnapshot struct {
	Limits ModelLimits `json:"limits"`
	Tokens int64       `json:"tokens"`
	Spend  int64       `json:"spend"`
}
type FrozenState struct {
	ModelBudget *ModelBudgetSnapshot `json:"model_budget,omitempty"`
	Ancestor    *SnapshotKey         `json:"ancestor,omitempty"`
	Version     string               `json:"version"`
	Scope       Scope                `json:"scope"`
	Revision    int64                `json:"revision"`
	Event       core.ID              `json:"event"`
	Offset      int64                `json:"offset"`
	Engine      string               `json:"engine"`
	RNG         string               `json:"rng"`
	State       rt.State             `json:"state"`
	Memories    []FrozenMemory       `json:"memories"`
	Models      []ModelUse           `json:"models"`
	Hash        string               `json:"hash"`
}

func (f FrozenState) Validate() error {
	if f.Version != SnapshotVersion || f.Scope.Validate() != nil || f.Revision < 1 || f.Event.Validate() != nil || f.Offset < 1 || !rt.SupportedEngine(f.Engine) || f.RNG != rt.RNGVersion || f.State.Engine != f.Engine || f.State.RNG != f.RNG || f.State.Genesis.World != f.Scope.World || f.State.Validate() != nil || len(f.Memories) > 24 || len(f.Models) > 128 {
		return fmt.Errorf("invalid snapshot manifest")
	}
	if b := f.ModelBudget; b != nil {
		if b.Limits.Validate() != nil || b.Tokens < 0 || b.Spend < 0 || b.Tokens > b.Limits.Tokens || b.Spend > b.Limits.SpendMicros {
			return fmt.Errorf("invalid snapshot model budget")
		}
	}
	var sc scenario.Scenario
	if json.Unmarshal(f.State.Genesis.Payload, &sc) != nil || len(f.Memories) != len(sc.Public.Humans) {
		return fmt.Errorf("incomplete snapshot actor realms")
	}
	actors := map[core.ID]bool{}
	for _, actor := range sc.Public.Humans {
		actors[actor.ID] = true
	}
	if f.Ancestor != nil {
		p := f.Ancestor
		if p.Scope.Validate() != nil || p.Scope.Actor != f.Scope.Actor || p.Scope.Namespace != f.Scope.Namespace || p.Scope.World != f.Scope.World || p.Scope.Run == f.Scope.Run || p.ID.Validate() != nil || len(p.Hash) != 64 {
			return fmt.Errorf("invalid snapshot ancestor reference")
		}
	}
	seen := map[core.ID]bool{}
	for _, memory := range f.Memories {
		expected, e := (ViewRealm{Scope: f.Scope, Principal: memory.Scope.Owner}).MemoryScope()
		if e != nil || expected != memory.Scope || !actors[memory.Scope.Owner] || seen[memory.Scope.Owner] {
			return fmt.Errorf("snapshot knowledge realm mismatch")
		}
		seen[memory.Scope.Owner] = true
		if _, e = graph.RebuildMemory(memory.Scope, memory.Entries); e != nil {
			return e
		}
	}
	seen = map[core.ID]bool{}
	for _, model := range f.Models {
		decoded, err := hex.DecodeString(model.Hash)
		if model.Key.Validate() != nil || err != nil || len(decoded) != 32 || seen[model.Key] {
			return fmt.Errorf("invalid snapshot model references")
		}
		seen[model.Key] = true
	}
	copy := f
	copy.Hash = ""
	hash, e := ModelDigest(copy)
	if e != nil || hash != f.Hash {
		return fmt.Errorf("snapshot hash mismatch")
	}
	raw, e := json.Marshal(f)
	if e != nil || len(raw) > MaxSnapshotBytes {
		return fmt.Errorf("snapshot byte budget")
	}
	return nil
}
func SealSnapshot(f FrozenState) (FrozenState, error) {
	f.Hash = ""
	hash, e := ModelDigest(f)
	if e != nil {
		return FrozenState{}, e
	}
	f.Hash = hash
	return f, f.Validate()
}
func DecodeSnapshot(raw []byte, expected string) (FrozenState, error) {
	var f FrozenState
	if len(raw) > MaxSnapshotBytes {
		return f, fmt.Errorf("snapshot byte budget")
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if e := d.Decode(&f); e != nil {
		return f, e
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		return f, fmt.Errorf("trailing snapshot data")
	}
	if f.Hash != expected {
		return f, fmt.Errorf("untrusted snapshot hash")
	}
	return f, f.Validate()
}

type ReplayFrame struct {
	OperationalStop bool           `json:"operational_stop,omitempty"`
	Revision        int64          `json:"revision"`
	Before          string         `json:"before"`
	After           string         `json:"after"`
	Command         rt.Command     `json:"command"`
	State           rt.State       `json:"state"`
	Transition      *rt.Transition `json:"transition,omitempty"`
	Model           *ModelUse      `json:"model,omitempty"`
}
type ReplayBundle struct {
	Snapshot FrozenState   `json:"snapshot"`
	Frames   []ReplayFrame `json:"frames"`
}
type ReplayResult struct {
	Mode   ReplayMode `json:"mode"`
	Exact  bool       `json:"exact"`
	Hashes []string   `json:"hashes"`
	Final  rt.State   `json:"final"`
}
type recordedTransition struct{ transition *rt.Transition }

func (h recordedTransition) Transition(_ rt.State, i rt.Input, c rt.Clock, r *rt.Random) (rt.Output, error) {
	t := h.transition
	if t == nil || i != t.Input || c.Now() != i.At {
		return rt.Output{}, fmt.Errorf("recorded input mismatch")
	}
	for _, draw := range t.Draws {
		if e := r.ReplayDraw(draw); e != nil {
			return rt.Output{}, e
		}
	}
	// Detach slices; replay never mutates a stored artifact.
	b, e := json.Marshal(t)
	if e != nil {
		return rt.Output{}, e
	}
	var owned rt.Transition
	if json.Unmarshal(b, &owned) != nil {
		return rt.Output{}, fmt.Errorf("invalid recorded output")
	}
	return rt.Output{Data: owned.Output, Events: owned.Generated, Consume: owned.Consumed}, nil
}

// Replay has no provider or infrastructure port. Fresh generation is explicitly
// a different operation and cannot be silently substituted after a replay miss.
func Replay(bundle ReplayBundle, mode ReplayMode) (ReplayResult, error) {
	if bundle.Snapshot.Validate() != nil || len(bundle.Frames) > MaxReplayFrames || (mode != EventReplay && mode != RecordedReplay) {
		return ReplayResult{}, fmt.Errorf("invalid or non-exact replay mode")
	}
	state, e := bundle.Snapshot.State.Clone()
	if e != nil {
		return ReplayResult{}, e
	}
	hash, _ := state.Hash()
	result := ReplayResult{Mode: mode, Exact: true, Hashes: []string{hash}}
	for index, frame := range bundle.Frames {
		after, err := frame.State.Hash()
		if err != nil || frame.State.Validate() != nil || frame.Command.Validate() != nil || frame.Revision != bundle.Snapshot.Revision+int64(index)+1 || frame.Before != hash || frame.After != after {
			return ReplayResult{}, fmt.Errorf("corrupt or discontinuous replay frame")
		}
		if mode == RecordedReplay {
			next, transition, _, err := rt.Apply(state, frame.Command, recordedTransition{frame.Transition})
			if frame.OperationalStop {
				next, err = state.Clone()
				next.Status = "budget"
				transition = nil
			}
			if err != nil {
				return ReplayResult{}, err
			}
			a, _ := json.Marshal(transition)
			b, _ := json.Marshal(frame.Transition)
			computed, _ := next.Hash()
			if computed != after || !bytes.Equal(a, b) {
				return ReplayResult{}, fmt.Errorf("recorded replay divergence")
			}
			state = next
		} else {
			state, err = frame.State.Clone()
			if err != nil {
				return ReplayResult{}, err
			}
		}
		hash = after
		result.Hashes = append(result.Hashes, hash)
	}
	result.Final = state
	return result, nil
}

type TrajectoryDifference struct {
	First           int       `json:"first"`
	ChangedInputs   []core.ID `json:"changed_inputs"`
	ChangedEvidence []core.ID `json:"changed_evidence"`
	FollowOn        int       `json:"follow_on"`
	CausalClaim     bool      `json:"causal_claim"`
}

func logicalMemories(f FrozenState) map[string]string {
	out := map[string]string{}
	for _, memory := range f.Memories {
		for _, entry := range memory.Entries {
			entry.Sequence = 0
			entry.Event.Meta.RecordedAt = time.Unix(0, 0).UTC()
			raw, _ := json.Marshal(entry)
			out[string(memory.Scope.Owner)+"\x00"+string(entry.Event.Meta.ID)] = string(raw)
		}
	}
	return out
}
func DiffTrajectories(a, b ReplayBundle) (TrajectoryDifference, error) {
	ar, e := Replay(a, EventReplay)
	if e != nil {
		return TrajectoryDifference{}, e
	}
	br, e := Replay(b, EventReplay)
	if e != nil {
		return TrajectoryDifference{}, e
	}
	out := TrajectoryDifference{First: -1, ChangedInputs: []core.ID{}, ChangedEvidence: []core.ID{}}
	am, bm := logicalMemories(a.Snapshot), logicalMemories(b.Snapshot)
	evidence := map[core.ID]bool{}
	for _, f := range []FrozenState{a.Snapshot, b.Snapshot} {
		for _, memory := range f.Memories {
			for _, entry := range memory.Entries {
				k := string(memory.Scope.Owner) + "\x00" + string(entry.Event.Meta.ID)
				if am[k] != bm[k] {
					evidence[entry.Event.Meta.ID] = true
					out.First = 0
				}
			}
		}
	}
	for id := range evidence {
		out.ChangedEvidence = append(out.ChangedEvidence, id)
	}
	sort.Slice(out.ChangedEvidence, func(i, j int) bool { return out.ChangedEvidence[i] < out.ChangedEvidence[j] })
	ac, _ := json.Marshal(struct {
		Models []ModelUse
		Budget *ModelBudgetSnapshot
	}{a.Snapshot.Models, a.Snapshot.ModelBudget})
	bc, _ := json.Marshal(struct {
		Models []ModelUse
		Budget *ModelBudgetSnapshot
	}{b.Snapshot.Models, b.Snapshot.ModelBudget})
	if !bytes.Equal(ac, bc) {
		out.First = 0
	}
	changedInputs := map[core.ID]bool{}
	maximum := len(ar.Hashes)
	if len(br.Hashes) > maximum {
		maximum = len(br.Hashes)
	}
	for i := 0; i < maximum; i++ {
		hashDifferent := i >= len(ar.Hashes) || i >= len(br.Hashes) || ar.Hashes[i] != br.Hashes[i]
		traceDifferent := false
		if i > 0 && i <= len(a.Frames) && i <= len(b.Frames) {
			x, _ := json.Marshal(struct {
				Command    rt.Command
				Transition *rt.Transition
				Model      *ModelUse
			}{a.Frames[i-1].Command, a.Frames[i-1].Transition, a.Frames[i-1].Model})
			y, _ := json.Marshal(struct {
				Command    rt.Command
				Transition *rt.Transition
				Model      *ModelUse
			}{b.Frames[i-1].Command, b.Frames[i-1].Transition, b.Frames[i-1].Model})
			traceDifferent = !bytes.Equal(x, y)
		}
		if !hashDifferent && !traceDifferent {
			continue
		}
		if out.First < 0 {
			out.First = i
		} else if hashDifferent && i > out.First {
			out.FollowOn++
		}
		if i == 0 {
			leftQueue, rightQueue := map[core.ID]string{}, map[core.ID]string{}
			for _, input := range a.Snapshot.State.Queue {
				raw, _ := json.Marshal(input)
				leftQueue[input.ID] = string(raw)
			}
			for _, input := range b.Snapshot.State.Queue {
				raw, _ := json.Marshal(input)
				rightQueue[input.ID] = string(raw)
			}
			for _, queue := range []map[core.ID]string{leftQueue, rightQueue} {
				for id := range queue {
					if leftQueue[id] != rightQueue[id] {
						changedInputs[id] = true
					}
				}
			}
		} else {
			for _, frames := range [][]ReplayFrame{a.Frames, b.Frames} {
				if i-1 >= len(frames) {
					continue
				}
				f := frames[i-1]
				if f.Command.Input != nil {
					changedInputs[f.Command.Input.ID] = true
				}
				if f.Transition != nil {
					changedInputs[f.Transition.Input.ID] = true
				}
			}
		}
	}
	for id := range changedInputs {
		out.ChangedInputs = append(out.ChangedInputs, id)
	}
	sort.Slice(out.ChangedInputs, func(i, j int) bool { return out.ChangedInputs[i] < out.ChangedInputs[j] })
	return out, nil
}
