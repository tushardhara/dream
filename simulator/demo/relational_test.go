package demo

import (
	"encoding/json"
	"fmt"
	"github.com/tushardhara/dream/core"
	"github.com/tushardhara/dream/simulator/behavior"
	"github.com/tushardhara/dream/simulator/drives"
	rt "github.com/tushardhara/dream/simulator/runtime"
	"reflect"
	"slices"
	"testing"
)

func TestFivePersonDirectionalTopology(t *testing.T) {
	sc, e := RelationalScenario(5, 1, 11)
	if e != nil {
		t.Fatal(e)
	}
	want := map[[2]int]core.ID{{0, 1}: "spouse", {0, 2}: "sibling", {0, 3}: "friend", {0, 4}: "friend", {1, 3}: "acquaintance", {1, 2}: "sibling_in_law"}
	count := 0
	for i, a := range sc.Actors {
		v, e := sc.View(a.ID)
		if e != nil {
			t.Fatal(e)
		}
		if len(v.Contexts) != len(a.Relationships) {
			t.Fatal("unattributed edge")
		}
		for _, r := range v.Contexts {
			count++
			if r.Observer != a.ID || r.Validate(a.ID, r.Other) != nil {
				t.Fatal("foreign perspective")
			}
			found := false
			for pair, kind := range want {
				if (pair[0] == i && human(pair[1]) == r.Other) || (pair[1] == i && human(pair[0]) == r.Other) {
					for _, typ := range r.Types {
						found = found || typ == kind
					}
				}
			}
			if !found {
				t.Fatal("unexpected knowledge", i, r.Other)
			}
			if i == 1 && r.Other == human(2) && !reflect.DeepEqual(r.Types, []core.ID{"acquaintance", "sibling_in_law"}) {
				t.Fatal("missing declared family assumption")
			}
		}
	}
	if count != 12 {
		t.Fatal("directional topology", count)
	}
	if reflect.DeepEqual(sc.Actors[0].Contexts[0].Measures, sc.Actors[1].Contexts[0].Measures) {
		t.Fatal("contradictory perspectives collapsed")
	}
	v, _ := sc.View(human(1))
	for _, r := range v.Contexts {
		if r.Other == human(4) {
			t.Fatal("W knows B")
		}
	}
	v.Contexts[0].Measures[0].Value = -1
	if sc.Actors[1].Contexts[0].Measures[0].Value == -1 {
		t.Fatal("view aliases source")
	}
}
func runRelational(t *testing.T, people, months int, seed uint64) rt.State {
	t.Helper()
	sc, e := RelationalScenario(people, months, seed)
	if e != nil {
		t.Fatal(e)
	}
	g, e := sc.Genesis(rt.Capabilities())
	if e != nil {
		t.Fatal(e)
	}
	s, e := rt.New(g, rt.Budgets{Steps: 100, Events: 100, Horizon: sc.World.Horizon})
	if e != nil {
		t.Fatal(e)
	}
	s, _, _, e = rt.Apply(s, rt.Command{Kind: "resume"}, Handler{})
	if e != nil {
		t.Fatal(e)
	}
	for range 2 * months {
		before, _ := s.Hash()
		next, tr, _, e := rt.Apply(s, rt.Command{Kind: "step"}, Handler{})
		if e != nil {
			t.Fatal(e)
		}
		if len(next.Data) > 256 || len(tr.Draws) != people {
			t.Fatal("bounded checkpoint/draws", len(next.Data), len(tr.Draws))
		}
		raw, _ := json.Marshal(s)
		var recovered rt.State
		if json.Unmarshal(raw, &recovered) != nil {
			t.Fatal("decode")
		}
		again, _, _, e := rt.Apply(recovered, rt.Command{Kind: "step"}, Handler{})
		a, _ := next.Hash()
		b, _ := again.Hash()
		if e != nil || a != b {
			t.Fatal("recovery mismatch", e)
		}
		after, _ := s.Hash()
		if before != after {
			t.Fatal("mutated state")
		}
		s = next
	}
	return s
}
func TestRelationalFiveAnd24CommonEngineRecovery(t *testing.T) {
	for _, n := range []int{5, 24} {
		t.Run(string(rune('A'+n)), func(t *testing.T) {
			state := runRelational(t, n, 2, 11)
			w, e := Projection(state)
			if e != nil {
				t.Fatal(e)
			}
			if w.Version != RelationalVersion || len(w.ActionActors) != n || len(w.ActionDecisions) != 4*n || len(w.Actors) != 0 || len(w.Decisions) != 0 {
				t.Fatal("consumer bypassed common engine")
			}
			for _, a := range w.ActionActors {
				if a.Validate() != nil || len(a.Drives.Applied) != 4 || len(a.Drives.Variables) != drives.Count {
					t.Fatal("drive engine not executed")
				}
			}
			for _, d := range w.ActionDecisions {
				if d.Policy != behavior.ActionPolicy || d.Validate() != nil || d.Stages[11] != "done" {
					t.Fatal("action lifecycle not executed")
				}
			}
			var checkpoint Checkpoint
			_ = json.Unmarshal([]byte(state.Data), &checkpoint)
			checkpoint.WorldHash = "0000000000000000000000000000000000000000000000000000000000000000"
			tampered, _ := json.Marshal(checkpoint)
			state.Data = string(tampered)
			if _, e = Projection(state); e == nil {
				t.Fatal("changed world hash accepted")
			}
			state.Data = string([]byte(state.Data)[:len(state.Data)-1])
			if _, e = Projection(state); e == nil {
				t.Fatal("tampered checkpoint")
			}
		})
	}
}
func TestRelationalDemoRolesGroupsPrivacyAndSeeds(t *testing.T) {
	sc, e := RelationalScenario(24, 2, 11)
	if e != nil {
		t.Fatal(e)
	}
	a, e := reconstruct(sc, 4, "", "", false, nil)
	if e != nil {
		t.Fatal(e)
	}
	// Same seed/genesis exactly reproduces; different seed changes choices.
	same, e := reconstruct(sc, 4, "", "", false, nil)
	if e != nil {
		t.Fatal(e)
	}
	x, _ := a.Hash()
	y, _ := same.Hash()
	if x != y {
		t.Fatal("non deterministic")
	}
	sc.World.Seed = 23
	b, e := reconstruct(sc, 4, "", "", false, nil)
	if e != nil {
		t.Fatal(e)
	}
	z, _ := b.Hash()
	if x == z {
		t.Fatal("seed ignored")
	}
	sc.World.Seed = 11
	sc.Research.Labels[0].Text = "PRIVATE_GROUND_TRUTH_CHANGED"
	b, e = reconstruct(sc, 4, "", "", false, nil)
	if e != nil {
		t.Fatal(e)
	}
	z, _ = b.Hash()
	if x != z {
		t.Fatal("research leaked")
	}
	// Each relationship class changes actual candidates when its supplied reports
	// are ablated. No label counts substitute for this common-engine assertion.
	for _, role := range []core.ID{"spouse", "sibling", "friend"} {
		raw, _ := json.Marshal(sc)
		var changed = sc
		_ = json.Unmarshal(raw, &changed)
		for i := range changed.Actors {
			for j := range changed.Actors[i].Contexts {
				r := &changed.Actors[i].Contexts[j]
				if r.Types[0] == role {
					r.Measures = nil
					r.Details = nil
				}
			}
		}
		b, e = reconstruct(changed, 4, "", "", false, nil)
		if e != nil {
			t.Fatal(e)
		}
		if reflect.DeepEqual(a.ActionDecisions, b.ActionDecisions) {
			t.Fatal("role reports not exercised", role)
		}
	}
	// Changing an observed venue changes members' choices, not an outsider's.
	one, e := reconstruct(sc, 1, "", "", false, nil)
	if e != nil {
		t.Fatal(e)
	}
	var p Period
	_ = json.Unmarshal([]byte(sc.Future[0].Text), &p)
	p.Theme = "compound"
	raw, _ := json.Marshal(p)
	sc.Future[0].Text = string(raw)
	two, e := reconstruct(sc, 1, "", "", false, nil)
	if e != nil {
		t.Fatal(e)
	}
	if reflect.DeepEqual(one.ActionActors[0], two.ActionActors[0]) {
		t.Fatal("group input ignored")
	}
	if !reflect.DeepEqual(one.ActionActors[23], two.ActionActors[23]) {
		t.Fatal("unseen group leaked")
	}
	sc.Actors[0].Facts[0].Grants = sc.Actors[0].Facts[0].Grants[:1]
	if _, e = reconstruct(sc, 1, "", "", false, nil); e == nil {
		t.Fatal("read broadened to derive")
	}
}

func TestRelationalScenarioRequiresVersionedCapability(t *testing.T) {
	sc, e := RelationalScenario(5, 1, 11)
	if e != nil {
		t.Fatal(e)
	}
	old := rt.Capabilities()
	caps := []core.ID{}
	for _, c := range old.Capabilities {
		if c != "relationships.v2" {
			caps = append(caps, c)
		}
	}
	old.Capabilities = caps
	if _, e = sc.Genesis(old); e == nil {
		t.Fatal("old engine silently ignored new relationship state")
	}
	if _, e = sc.Genesis(rt.Capabilities()); e != nil {
		t.Fatal(e)
	}
}

func TestRelationalRepliesAreCausalAndYearBounded(t *testing.T) {
	sc, e := RelationalScenario(24, 12, 11)
	if e != nil {
		t.Fatal(e)
	}
	early, e := reconstruct(sc, 2, "", "", false, nil)
	if e != nil {
		t.Fatal(e)
	}
	for _, o := range early.ActionOutcomes {
		if o.Outcome.Status == core.Observed {
			t.Fatal("same-period message learned as a reply")
		}
	}
	w, e := reconstruct(sc, 24, "", "", false, nil)
	if e != nil {
		t.Fatal(e)
	}
	if len(w.ActionActors) != 24 || len(w.ActionDecisions) != 576 || len(w.ActionOutcomes) != 576 {
		t.Fatal("year history truncated")
	}
	observed := 0
	for index, o := range w.ActionOutcomes {
		if o.Validate() != nil {
			t.Fatal("invalid outcome")
		}
		if o.Outcome.Status == core.Observed {
			observed++
			period := index / 24
			if period+2 >= len(sc.Future) || o.Outcome.LearnedAt < sc.Future[period+2].At {
				t.Fatal("response learned before sender could observe and reply")
			}
		}
	}
	if observed == 0 {
		t.Fatal("actual reply learning never executed")
	}
	for _, a := range w.ActionActors {
		if a.Validate() != nil || len(a.Drives.Applied) != 24 {
			t.Fatal("receipt or memory truncation")
		}
	}
	if w.Resources["shared-time"] >= 12 || w.Resources["shared-time"] < 0 {
		t.Fatal("help failed actual finite resource accounting")
	}
	t.Logf("24 actors, 576 decisions, %d observed replies, shared-time remaining=%d", observed, w.Resources["shared-time"])
}

// The 24-person relational demo's shape is stated as fact in six documents
// (README, backend-demo, requirements, ADR 0016, ADR 0019): 52 undirected edges
// emitted in both directions as 104 directional reports. Nothing pinned it —
// scenario_test.go only asserts edges >= 30 — so a change to the friend offset
// or the sibling modulus would silently leave every one of those documents
// wrong (#79). The per-type counts are asserted separately so a regression
// names which family broke rather than only that the total moved.
//
// Kinds are counted from Relationship.Kind, not RelationshipContext.Types[0]:
// Scenario canonicalisation sorts every slice by its JSON bytes, so a context
// carrying more than one type (the five-person sibling_in_law edge, which also
// carries acquaintance) does not keep its declared kind first. Each context is
// still cross-checked to contain its relationship's declared kind.
func TestTwentyFourPersonTopologyHasFiftyTwoEdgesAnd104DirectionalReports(t *testing.T) {
	for _, c := range []struct {
		people, edges, directional int
		perType                    map[core.ID]int
		wantEdges                  map[[2]int]core.ID
	}{
		{24, 52, 104, map[core.ID]int{"spouse": 12, "sibling": 16, "friend": 24}, documentedTwentyFourEdges()},
		// Positive control on the same helper: if the counting below were wrong,
		// this five-person fixture would not land on its own documented shape.
		{5, 6, 12, map[core.ID]int{"spouse": 1, "sibling": 1, "friend": 2, "acquaintance": 1, "sibling_in_law": 1},
			map[[2]int]core.ID{{0, 1}: "spouse", {0, 2}: "sibling", {0, 3}: "friend", {0, 4}: "friend", {1, 3}: "acquaintance", {1, 2}: "sibling_in_law"}},
	} {
		t.Run(fmt.Sprintf("people=%d", c.people), func(t *testing.T) {
			sc, e := RelationalScenario(c.people, 1, 11)
			if e != nil {
				t.Fatal(e)
			}
			index := map[core.ID]int{}
			for i, a := range sc.Actors {
				index[a.ID] = i
			}
			undirected := map[[2]int]core.ID{}
			directional := 0
			for _, a := range sc.Actors {
				v, e := sc.View(a.ID)
				if e != nil {
					t.Fatal(e)
				}
				if len(v.Contexts) != len(a.Relationships) {
					t.Fatal("unattributed edge for", a.ID)
				}
				kinds := map[core.ID]core.ID{}
				for _, rel := range a.Relationships {
					kinds[rel.Other] = rel.Kind
					from, ok := index[a.ID]
					to, ok2 := index[rel.Other]
					if !ok || !ok2 {
						t.Fatal("relationship names a person outside the roster", a.ID, rel.Other)
					}
					pair := [2]int{min(from, to), max(from, to)}
					if seen, dup := undirected[pair]; dup && seen != rel.Kind {
						t.Fatal("same pair carries two kinds", seen, rel.Kind)
					}
					undirected[pair] = rel.Kind
				}
				for _, r := range v.Contexts {
					directional++
					if r.Observer != a.ID || r.Validate(a.ID, r.Other) != nil {
						t.Fatal("context is not observer-owned", a.ID, r.Other)
					}
					if !slices.Contains(r.Types, kinds[r.Other]) {
						t.Fatal("context dropped its declared kind", kinds[r.Other], r.Types)
					}
				}
			}
			perType := map[core.ID]int{}
			for _, kind := range undirected {
				perType[kind]++
			}
			if len(undirected) != c.edges {
				t.Fatal("distinct undirected edges: want", c.edges, "got", len(undirected))
			}
			if directional != c.directional {
				t.Fatal("directional reports: want", c.directional, "got", directional)
			}
			if !reflect.DeepEqual(perType, c.perType) {
				t.Fatal("per-type edge counts: want", c.perType, "got", perType)
			}
			// Counts alone do not pin the topology: changing the friend offset from
			// 7 to any other value coprime with 24 keeps 52 edges and 12/16/24 per
			// type, so the documents would still be wrong and every count above
			// would still pass. Pin the adjacency too.
			if !reflect.DeepEqual(undirected, c.wantEdges) {
				for pair, kind := range c.wantEdges {
					if got, ok := undirected[pair]; !ok {
						t.Error("missing documented edge", pair, kind)
					} else if got != kind {
						t.Error("edge", pair, "want kind", kind, "got", got)
					}
				}
				for pair, kind := range undirected {
					if _, ok := c.wantEdges[pair]; !ok {
						t.Error("undocumented edge", pair, kind)
					}
				}
				t.Fatal("relational topology no longer matches the documented fixture")
			}
		})
	}
}

// documentedTwentyFourEdges restates the topology the six documents describe:
// spouses pair off, siblings join i to i+2 while i%6 < 4, and every person has a
// friend seven places along. It is deliberately written from the documentation
// rather than read from the scenario, so that changing the generator without
// changing the documents fails here.
func documentedTwentyFourEdges() map[[2]int]core.ID {
	const people = 24
	out := map[[2]int]core.ID{}
	add := func(a, b int, kind core.ID) { out[[2]int{min(a, b), max(a, b)}] = kind }
	for i := 0; i < people; i += 2 {
		add(i, i+1, "spouse")
	}
	for i := range people {
		if i%6 < 4 && i+2 < people {
			add(i, i+2, "sibling")
		}
	}
	for i := range people {
		add(i, (i+7)%people, "friend")
	}
	return out
}
