package graph

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/tushardhara/dream/core"
)

const relationPrefix = "relation.v1:"

type RelationshipDimension struct {
	Name       core.ID         `json:"name"`
	Value      float64         `json:"value"`
	Confidence core.Confidence `json:"confidence"`
}
type Membership struct {
	Member core.Subject  `json:"member"`
	Roles  []core.ID     `json:"roles"`
	Valid  core.Interval `json:"valid"`
}
type RelationshipPattern struct {
	Name       core.ID         `json:"name"`
	Confidence core.Confidence `json:"confidence"`
	Evidence   []core.ID       `json:"evidence"`
}

// RelationState v1 is an observer's evidence-bound account of an edge or group,
// never a canonical truth owned by its participants. Commitments and open loops
// reference independently permissioned memory records, not inferred obligations.
type RelationState struct {
	Version     uint32                  `json:"version"`
	ID          core.ID                 `json:"id"`
	Kind        string                  `json:"kind"`
	From        *core.Subject           `json:"from,omitempty"`
	To          *core.Subject           `json:"to,omitempty"`
	Types       []core.ID               `json:"types"`
	Memberships []Membership            `json:"memberships"`
	Dimensions  []RelationshipDimension `json:"dimensions"`
	Commitments []core.ID               `json:"commitments"`
	OpenLoops   []core.ID               `json:"open_loops"`
	Patterns    []RelationshipPattern   `json:"patterns"`
}

func relationIDs(ids []core.ID, max int) bool {
	if len(ids) > max {
		return false
	}
	seen := map[core.ID]bool{}
	for _, id := range ids {
		if id.Validate() != nil || seen[id] {
			return false
		}
		seen[id] = true
	}
	return true
}
func (v RelationState) Validate(observer core.ID) error {
	if v.Version != 1 || v.ID.Validate() != nil || observer.Validate() != nil || !relationIDs(v.Types, 8) || len(v.Types) == 0 {
		return fmt.Errorf("invalid relation version/id/types")
	}
	switch v.Kind {
	case "edge":
		if v.From == nil || v.To == nil || v.From.ValidateFor(observer) != nil || v.To.ValidateFor(observer) != nil || sameSubject(*v.From, *v.To) {
			return fmt.Errorf("invalid relationship endpoints")
		}
	case "group":
		if v.From != nil || v.To != nil {
			return fmt.Errorf("group has edge endpoints")
		}
	default:
		return fmt.Errorf("unknown relation kind")
	}
	if len(v.Memberships) > 24 || len(v.Dimensions) > 8 || len(v.Patterns) > 8 || !relationIDs(v.Commitments, 8) || !relationIDs(v.OpenLoops, 8) {
		return fmt.Errorf("relation collection budget")
	}
	for i, m := range v.Memberships {
		if m.Member.ValidateFor(observer) != nil || m.Valid.Validate() != nil || !relationIDs(m.Roles, 8) {
			return fmt.Errorf("invalid membership")
		}
		if v.Kind == "edge" && !sameSubject(m.Member, *v.From) && !sameSubject(m.Member, *v.To) {
			return fmt.Errorf("edge membership outside endpoints")
		}
		for _, old := range v.Memberships[:i] {
			if sameSubject(old.Member, m.Member) && (old.Valid.End == nil || m.Valid.Start < *old.Valid.End) && (m.Valid.End == nil || old.Valid.Start < *m.Valid.End) {
				return fmt.Errorf("overlapping membership intervals")
			}
		}
	}
	seen := map[core.ID]bool{}
	for _, d := range v.Dimensions {
		if d.Name.Validate() != nil || seen[d.Name] || math.IsNaN(d.Value) || math.IsInf(d.Value, 0) || d.Value < -1 || d.Value > 1 || d.Confidence.Validate() != nil {
			return fmt.Errorf("invalid uncertain dimension")
		}
		seen[d.Name] = true
	}
	seen = map[core.ID]bool{}
	for _, p := range v.Patterns {
		if p.Name.Validate() != nil || seen[p.Name] || p.Confidence.Validate() != nil || len(p.Evidence) == 0 || !relationIDs(p.Evidence, 8) {
			return fmt.Errorf("invalid attributed pattern")
		}
		seen[p.Name] = true
	}
	return nil
}
func relationSubject(v RelationState) core.Subject {
	if v.Kind == "edge" && v.From != nil {
		return *v.From
	}
	return core.Subject{Principal: v.ID}
}
func sortedRelationIDs(v []core.ID) []core.ID {
	out := append([]core.ID{}, v...)
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// EncodeRelation is a typed inner codec inside the unchanged memory.v1 text
// envelope. The 2048-byte text and 4096-byte outer payload budgets still apply.
func EncodeRelation(v RelationState, observer core.ID) (string, error) {
	if err := v.Validate(observer); err != nil {
		return "", err
	}
	raw, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	var owned RelationState
	if err = json.Unmarshal(raw, &owned); err != nil {
		return "", err
	}
	v = owned
	v.Types = sortedRelationIDs(v.Types)
	v.Commitments = sortedRelationIDs(v.Commitments)
	v.OpenLoops = sortedRelationIDs(v.OpenLoops)
	if v.Memberships == nil {
		v.Memberships = []Membership{}
	}
	if v.Dimensions == nil {
		v.Dimensions = []RelationshipDimension{}
	}
	if v.Patterns == nil {
		v.Patterns = []RelationshipPattern{}
	}
	for i := range v.Memberships {
		v.Memberships[i].Roles = sortedRelationIDs(v.Memberships[i].Roles)
	}
	sort.Slice(v.Memberships, func(i, j int) bool {
		a, _ := json.Marshal(v.Memberships[i].Member)
		b, _ := json.Marshal(v.Memberships[j].Member)
		if string(a) != string(b) {
			return string(a) < string(b)
		}
		return v.Memberships[i].Valid.Start < v.Memberships[j].Valid.Start
	})
	for i := range v.Dimensions {
		if v.Dimensions[i].Value == 0 {
			v.Dimensions[i].Value = 0
		}
		if v.Dimensions[i].Confidence == 0 {
			v.Dimensions[i].Confidence = 0
		}
	}
	sort.Slice(v.Dimensions, func(i, j int) bool { return v.Dimensions[i].Name < v.Dimensions[j].Name })
	for i := range v.Patterns {
		v.Patterns[i].Evidence = sortedRelationIDs(v.Patterns[i].Evidence)
		if v.Patterns[i].Confidence == 0 {
			v.Patterns[i].Confidence = 0
		}
	}
	sort.Slice(v.Patterns, func(i, j int) bool { return v.Patterns[i].Name < v.Patterns[j].Name })
	raw, err = json.Marshal(v)
	if err != nil {
		return "", err
	}
	if len(raw)+len(relationPrefix) > 2048 {
		return "", fmt.Errorf("relation encoded budget")
	}
	return relationPrefix + string(raw), nil
}
func DecodeRelation(r MemoryRecord) (RelationState, error) {
	var v RelationState
	if r.Validate() != nil || r.Content.Kind != RelationshipMemory || !strings.HasPrefix(r.Content.Text, relationPrefix) {
		return v, fmt.Errorf("not a relationship record")
	}
	dec := json.NewDecoder(strings.NewReader(strings.TrimPrefix(r.Content.Text, relationPrefix)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&v); err != nil {
		return v, err
	}
	text, err := EncodeRelation(v, r.Event.Meta.Observer)
	if err != nil {
		return v, err
	}
	if text != r.Content.Text {
		return v, fmt.Errorf("noncanonical relation")
	}
	if !sameSubject(relationSubject(v), r.Event.Subject) {
		return v, fmt.Errorf("relation envelope subject mismatch")
	}
	sources := map[core.ID]bool{}
	for _, id := range memorySources(r) {
		sources[id] = true
	}
	references := append(append([]core.ID{}, v.Commitments...), v.OpenLoops...)
	for _, p := range v.Patterns {
		references = append(references, p.Evidence...)
	}
	for _, id := range references {
		if !sources[id] {
			return v, fmt.Errorf("relation signal lacks permissioned provenance")
		}
	}
	return v, nil
}

type RelationService struct{ Memory MemoryService }

func (s RelationService) Put(ctx context.Context, scope MemoryScope, key core.ID, expected int64, r MemoryRecord, v RelationState, purpose core.ID) (AppendResult, error) {
	text, err := EncodeRelation(v, scope.Owner)
	if err != nil {
		return AppendResult{}, err
	}
	r.Content.Kind = RelationshipMemory
	r.Content.Text = text
	if _, err = DecodeRelation(r); err != nil {
		return AppendResult{}, err
	}
	if s.Memory.Journal == nil {
		return AppendResult{}, fmt.Errorf("relation journal required")
	}
	entries, err := s.Memory.Journal.ReadMemory(ctx, scope)
	if err != nil {
		return AppendResult{}, err
	}
	idx, err := RebuildMemory(scope, entries)
	if err != nil {
		return AppendResult{}, err
	}
	// Source kinds are immutable. The journal rechecks existence, revocation,
	// source knowledge and rights atomically when committing the event.
	for _, set := range []struct {
		IDs  []core.ID
		Kind MemoryKind
	}{{v.Commitments, IntentMemory}, {v.OpenLoops, OpenLoopMemory}} {
		for _, id := range set.IDs {
			e, ok := idx.byID[id]
			if !ok || e.Revoked || e.Content == nil || e.Content.Kind != set.Kind {
				return AppendResult{}, fmt.Errorf("invalid relation signal source kind")
			}
		}
	}
	if r.Supersedes != "" {
		e, ok := idx.byID[r.Supersedes]
		if !ok || e.Revoked || e.Content == nil {
			return AppendResult{}, fmt.Errorf("missing prior relation")
		}
		prior, err := DecodeRelation(e.record())
		if err != nil {
			return AppendResult{}, err
		}
		if prior.ID != v.ID || prior.Kind != v.Kind || (v.Kind == "edge" && (!sameSubject(*prior.From, *v.From) || !sameSubject(*prior.To, *v.To))) {
			return AppendResult{}, fmt.Errorf("relation correction changes identity")
		}
	}
	return s.Memory.Put(ctx, scope, key, expected, r, purpose)
}

type RelationProjection struct {
	Observer        core.ID
	Record          MemoryRecord
	State           RelationState
	ActiveMembers   []Membership
	OpenLoopSignals []core.ID
}

func (s RelationService) Query(ctx context.Context, q MemoryQuery, id core.ID) ([]RelationProjection, error) {
	if id.Validate() != nil {
		return nil, fmt.Errorf("invalid relation id")
	}
	if q.Validate() != nil || s.Memory.Journal == nil {
		return nil, fmt.Errorf("invalid relation query")
	}
	entries, err := s.Memory.Journal.ReadMemory(ctx, q.Scope)
	if err != nil {
		return nil, err
	}
	idx, err := RebuildMemory(q.Scope, entries)
	if err != nil {
		return nil, err
	}
	states := map[core.ID]RelationState{}
	for _, e := range entries {
		if e.Revoked || e.Content == nil || e.Content.Kind != RelationshipMemory || !strings.HasPrefix(e.Content.Text, relationPrefix) {
			continue
		}
		v, err := DecodeRelation(e.record())
		if err != nil {
			return nil, err
		}
		states[e.Event.Meta.ID] = v
	}
	// Apply entity/kind selection before the caller's result budget. Unrelated
	// high-salience memories must not crowd every matching edge out of a query.
	picks := idx.selectMatchingMemory(q, func(r MemoryRecord) bool { v, ok := states[r.Event.Meta.ID]; return ok && v.ID == id })
	out := []RelationProjection{}
	for _, pick := range picks {
		r := idx.byID[pick.ID].record()
		v := states[pick.ID]
		members := []Membership{}
		for _, m := range v.Memberships {
			if m.Valid.Start <= q.ValidAt && (m.Valid.End == nil || q.ValidAt < *m.Valid.End) {
				members = append(members, m)
			}
		}
		out = append(out, RelationProjection{Observer: r.Event.Meta.Observer, Record: r, State: v, ActiveMembers: members, OpenLoopSignals: append([]core.ID{}, v.OpenLoops...)})
	}
	return out, nil
}

type DimensionDelta struct {
	Name                              core.ID
	Before, After, Change             float64
	BeforeConfidence, AfterConfidence core.Confidence
}
type RelationDelta struct {
	Observer, Relation, Before, After core.ID
	Dimensions                        []DimensionDelta
	OpenLoops                         []core.ID
}

// CompareRelation is descriptive, not an intervention selector or inference from
// participation to care. Only named dimensions shared by the same observer and
// same relation are subtracted; absent dimensions are unknown, never zero-filled.
func CompareRelation(before, after RelationProjection) (RelationDelta, error) {
	a, b := before.State, after.State
	if before.Observer != after.Observer || a.ID != b.ID || a.Kind != b.Kind || before.Observer != before.Record.Event.Meta.Observer || after.Observer != after.Record.Event.Meta.Observer {
		return RelationDelta{}, fmt.Errorf("cannot compare different perspectives")
	}
	av, err := DecodeRelation(before.Record)
	if err != nil {
		return RelationDelta{}, err
	}
	bv, err := DecodeRelation(after.Record)
	if err != nil {
		return RelationDelta{}, err
	}
	ar, _ := EncodeRelation(a, before.Observer)
	br, _ := EncodeRelation(b, after.Observer)
	ac, _ := EncodeRelation(av, before.Observer)
	bc, _ := EncodeRelation(bv, after.Observer)
	if ar != ac || br != bc {
		return RelationDelta{}, fmt.Errorf("projection changed independently of source")
	}
	out := RelationDelta{Observer: before.Observer, Relation: a.ID, Before: before.Record.Event.Meta.ID, After: after.Record.Event.Meta.ID, Dimensions: []DimensionDelta{}, OpenLoops: append([]core.ID{}, b.OpenLoops...)}
	for _, next := range b.Dimensions {
		for _, old := range a.Dimensions {
			if next.Name == old.Name {
				out.Dimensions = append(out.Dimensions, DimensionDelta{next.Name, old.Value, next.Value, next.Value - old.Value, old.Confidence, next.Confidence})
			}
		}
	}
	return out, nil
}
