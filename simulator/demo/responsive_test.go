package demo

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/tushardhara/dream/core"
	"github.com/tushardhara/dream/simulator/behavior"
	rt "github.com/tushardhara/dream/simulator/runtime"
	"github.com/tushardhara/dream/simulator/scenario"
)

func TestResponsiveActualConsumers(t *testing.T) {
	for _, people := range []int{5, 24} {
		t.Run(string(rune('A'+people)), func(t *testing.T) {
			sc, e := ResponsiveScenario(people, 12, 11)
			if e != nil {
				t.Fatal(e)
			}
			w, e := reconstruct(sc, 24, "", "", false, nil)
			if e != nil {
				t.Fatal(e)
			}
			counts := map[string]int{}
			phases := map[string]int{}
			replies := 0
			offers := map[behavior.Kind]int{}
			for _, o := range w.OutcomeObservations {
				if o.Validate() != nil {
					t.Fatal("invalid observation", o)
				}
				if o.Position == "recipient" {
					counts[o.Appraisal]++
					phases[o.Phase]++
					if o.Reply != "" {
						replies++
					}
				}
				if o.Meta.Rights.Allows(core.PermissionRequest{Resource: o.Meta.ID, Context: core.Grant{Actor: o.Meta.Observer, Recipient: o.Other, Purpose: "simulation", Operation: core.Read}}) {
					t.Fatal("private appraisal shared")
				}
			}
			for _, d := range w.ActionDecisions {
				for _, c := range d.Candidates {
					offers[c.Offer.Kind]++
				}
			}
			if counts["dismissive"] == 0 || counts["unresolved"]+counts["neutral"] == 0 || phases["later"] == 0 || replies == 0 || offers[behavior.Decline] == 0 || offers[behavior.Apologize] == 0 || offers[behavior.Withdraw] == 0 {
				t.Fatalf("missing actual adverse/null/linked/diverse paths: counts=%v phases=%v replies=%d offers=%v", counts, phases, replies, offers)
			}
			t.Logf("people=%d appraisals=%v phases=%v linked=%d offers=%v", people, counts, phases, replies, offers)
			for _, o := range w.ActionOutcomes {
				if o.Outcome.Status == core.Observed {
					t.Fatal("delivery/reply promoted to observed success")
				}
			}
			for _, a := range w.ActionActors {
				if len(a.Memory) != 0 || len(a.Beliefs) != 0 {
					t.Fatal("unscoped learning retained")
				}
			}
			again, e := reconstruct(sc, 24, "", "", false, nil)
			if e != nil {
				t.Fatal(e)
			}
			a, _ := w.Hash()
			b, _ := again.Hash()
			if a != b {
				t.Fatal("non-deterministic recipient projection")
			}
		})
	}
}
func TestResponsiveRuntimeReplay(t *testing.T) {
	sc, e := ResponsiveScenario(5, 2, 17)
	if e != nil {
		t.Fatal(e)
	}
	g, e := sc.Genesis(rt.Capabilities())
	if e != nil {
		t.Fatal(e)
	}
	state, e := rt.New(g, rt.Budgets{Steps: 100, Events: 100, Horizon: sc.World.Horizon})
	if e != nil {
		t.Fatal(e)
	}
	state, _, _, e = rt.Apply(state, rt.Command{Kind: "resume"}, Handler{})
	if e != nil {
		t.Fatal(e)
	}
	for range sc.Future {
		raw, _ := json.Marshal(state)
		var recovered rt.State
		_ = json.Unmarshal(raw, &recovered)
		next, tr, _, e := rt.Apply(state, rt.Command{Kind: "step"}, Handler{})
		if e != nil {
			t.Fatal(e)
		}
		replay, _, _, e := rt.Apply(recovered, rt.Command{Kind: "step"}, Handler{})
		if e != nil {
			t.Fatal(e)
		}
		if len(tr.Draws) != 5 || len(next.Data) > 256 || !reflect.DeepEqual(next, replay) {
			t.Fatal("recorded draw budget or replay changed")
		}
		state = next
	}
	w, e := Projection(state)
	if e != nil || w.Version != ResponsiveVersion || len(w.RecipientDecisions) == 0 {
		t.Fatal("current response projection", e)
	}
	var cp Checkpoint
	_ = json.Unmarshal([]byte(state.Data), &cp)
	cp.Version = RelationalVersion
	raw, _ := json.Marshal(cp)
	state.Data = string(raw)
	if _, e = Projection(state); e == nil {
		t.Fatal("v3 reinterpreted as legacy")
	}
}
func setResponsePreference(t *testing.T, sc *scenario.Scenario, owner core.ID, preference ResponsePreference) {
	t.Helper()
	for i := range sc.Actors {
		if sc.Actors[i].ID != owner {
			continue
		}
		for j := range sc.Actors[i].Facts {
			f := &sc.Actors[i].Facts[j]
			if f.ID == core.ID(string(owner)+":response-preference") {
				raw, _ := json.Marshal(preference)
				f.Text = string(raw)
				return
			}
		}
	}
	t.Fatal("missing fixture preference")
}
func TestResponsiveMissingAndPrivateContext(t *testing.T) {
	base, e := ResponsiveScenario(5, 4, 11)
	if e != nil {
		t.Fatal(e)
	}
	for _, reason := range []string{"silence", "lost_observation", "declined_participation", "missing_followup"} {
		raw, _ := json.Marshal(base)
		var sc scenario.Scenario
		_ = json.Unmarshal(raw, &sc)
		for _, a := range sc.Actors {
			p := ResponsePreference{Version: "demo-response-preference.v1", Discussion: true, Coordination: true, Observation: reason, Followup: true}
			if reason == "missing_followup" {
				p.Observation = "observed"
				p.Followup = false
			}
			setResponsePreference(t, &sc, a.ID, p)
		}
		w, e := reconstruct(sc, len(sc.Future), "", "", false, nil)
		if e != nil {
			t.Fatal(reason, e)
		}
		found := 0
		for _, o := range w.OutcomeObservations {
			if o.Position != "recipient" || reason == "missing_followup" && o.Phase != "later" {
				continue
			}
			found++
			if o.Missing != reason || o.Appraisal != "" || o.Benefit != nil || o.Burden != nil || o.Status == core.Observed {
				t.Fatal("missing interpreted as welfare", reason, o)
			}
		}
		if found == 0 {
			t.Fatal("no actual missing outcomes", reason)
		}
	}
	// Relabelling the relationship does not choose a different appraisal; changing
	// its attributed expectation does. The native choices may also change, so match
	// the first common actual received action and compare its probability vector.
	original, e := reconstruct(base, 8, "", "", false, nil)
	if e != nil {
		t.Fatal(e)
	}
	for i := range base.Actors {
		for j := range base.Actors[i].Contexts {
			r := &base.Actors[i].Contexts[j]
			for k := range r.Measures {
				if r.Measures[k].Kind == "expectation" {
					r.Measures[k].Value = -r.Measures[k].Value
				}
			}
		}
	}
	changed, e := reconstruct(base, 8, "", "", false, nil)
	if e != nil {
		t.Fatal(e)
	}
	matched, affected := 0, 0
	for _, a := range original.RecipientDecisions {
		for _, b := range changed.RecipientDecisions {
			if a.Observation.Action == b.Observation.Action && a.Observation.Meta.Observer == b.Observation.Meta.Observer && a.Observation.Phase == b.Observation.Phase {
				matched++
				if a.Probabilities != b.Probabilities {
					affected++
				}
			}
		}
	}
	if matched == 0 || affected == 0 {
		t.Fatal("actual consumer ignores own recipient expectation", matched, affected)
	}
}

func TestResponsiveObservedLearningChangesActualChoice(t *testing.T) {
	observed, e := ResponsiveScenario(5, 12, 11)
	if e != nil {
		t.Fatal(e)
	}
	raw, _ := json.Marshal(observed)
	var missing scenario.Scenario
	_ = json.Unmarshal(raw, &missing)
	for _, a := range missing.Actors {
		setResponsePreference(t, &missing, a.ID, ResponsePreference{Version: "demo-response-preference.v1", Discussion: true, Coordination: true, Observation: "silence", Followup: true})
	}
	a, e := reconstruct(observed, 24, "", "", false, nil)
	if e != nil {
		t.Fatal(e)
	}
	b, e := reconstruct(missing, 24, "", "", false, nil)
	if e != nil {
		t.Fatal(e)
	}
	changed := 0
	for i, x := range a.ActionDecisions {
		y := b.ActionDecisions[i]
		if x.Draw != y.Draw || x.ID != y.ID {
			t.Fatal("exogenous native randomness changed")
		}
		if !reflect.DeepEqual(x.Candidates, y.Candidates) {
			changed++
			if x.At <= observed.Future[1].At {
				t.Fatal("response changed choice before learning was available")
			}
			break
		}
	}
	if changed == 0 {
		t.Fatal("actual native choices ignore observed outcome learning")
	}
}

func TestResponsivePreferenceAndKnowledgeFailClosed(t *testing.T) {
	sc, e := ResponsiveScenario(5, 3, 11)
	if e != nil {
		t.Fatal(e)
	}
	for _, a := range sc.Actors {
		setResponsePreference(t, &sc, a.ID, ResponsePreference{Version: "demo-response-preference.v1", Observation: "observed", Followup: true})
	}
	w, e := reconstruct(sc, 6, "", "", false, nil)
	if e != nil {
		t.Fatal(e)
	}
	for _, d := range w.ActionDecisions {
		if d.Candidates[d.Selected].Offer.Delivered() {
			t.Fatal("discussion/coordination refusal bypassed")
		}
	}
	sc, e = ResponsiveScenario(5, 1, 11)
	if e != nil {
		t.Fatal(e)
	}
	for j := range sc.Actors[0].Facts {
		if sc.Actors[0].Facts[j].ID == core.ID(string(sc.Actors[0].ID)+":response-preference") {
			sc.Actors[0].Facts[j].Grants = nil
		}
	}
	if _, e = reconstruct(sc, 2, "", "", false, nil); e == nil {
		t.Fatal("revoked own preference ignored")
	}
}
