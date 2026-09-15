package demo

import (
	"encoding/json"
	"fmt"

	"github.com/tushardhara/dream/core"
	"github.com/tushardhara/dream/simulator/scenario"
)

const ResponsiveVersion = "backend-demo.v3"

// ResponsePreference is an explicit fictional participation setting. It is
// independent of relationship labels, demographics, data rights and appraisal.
// Permission to discuss an invitation does not mean the invitation is welcome.
type ResponsePreference struct {
	Version                  string
	Discussion, Coordination bool
	Observation              string
	Followup                 bool
}

func ResponsiveScenario(people, months int, seed uint64) (scenario.Scenario, error) {
	sc, e := RelationalScenario(people, months, seed)
	if e != nil {
		return sc, e
	}
	sc.World.ID = "responsive:" + sc.World.ID
	for i := range sc.Actors {
		a := &sc.Actors[i]
		pref := ResponsePreference{Version: "demo-response-preference.v1", Discussion: true, Coordination: true, Observation: "observed", Followup: true}
		raw, _ := json.Marshal(pref)
		id := core.ID(string(a.ID) + ":response-preference")
		a.Facts = append(a.Facts, scenario.Fact{ID: id, Observer: a.ID, Subject: a.ID, Text: string(raw), Confidence: 1, Grants: []core.Grant{{Actor: a.ID, Recipient: a.ID, Purpose: "simulation", Operation: core.Read}, {Actor: a.ID, Recipient: a.ID, Purpose: "simulation", Operation: core.Derive}}})
		a.Knowledge = append(a.Knowledge, scenario.Knowledge{Record: id})
		// Author new coordination/everyday accounts, rather than treating the legacy
		// unscoped vector as universal. These toy values belong to this version only.
		values := []float64{.8, -.7, .2, -.1, .6, -.4}
		for j := range a.Contexts {
			old := a.Contexts[j]
			source := core.ID(string(a.ID) + ":relationship:" + string(old.Other))
			account := responseFocus(a.ID, old.Other).Account
			measures := []core.RelationshipMeasure{}
			for k, kind := range []core.ID{"trust", "expectation", "expected_reaction", "prior_outcome", "stress"} {
				value := values[(i+j+k)%len(values)]
				if kind == "stress" {
					value = .2
				}
				measures = append(measures, core.RelationshipMeasure{Kind: kind, Value: value, Confidence: .8, Source: source})
			}
			a.Contexts[j] = core.RelationshipContext{Version: 2, Account: account, Observer: a.ID, Other: old.Other, Domain: core.PracticalCoordination, RoleContext: "everyday", ContextSource: id, Types: old.Types, Measures: measures}
			a.Facts = append(a.Facts, scenario.Fact{ID: account, Observer: a.ID, Subject: old.Other, Text: "Fictional own coordination/everyday account", Confidence: 1, Grants: []core.Grant{{Actor: a.ID, Recipient: a.ID, Purpose: "simulation", Operation: core.Read}, {Actor: a.ID, Recipient: a.ID, Purpose: "simulation", Operation: core.Derive}}})
			a.Knowledge = append(a.Knowledge, scenario.Knowledge{Record: account})
		}

	}
	for i := range sc.Future {
		var p Period
		_ = json.Unmarshal([]byte(sc.Future[i].Text), &p)
		p.Version = ResponsiveVersion
		raw, _ := json.Marshal(p)
		sc.Future[i].Text = string(raw)
	}
	sc.Requires = append(sc.Requires, "relationships.v3")
	return sc, sc.Validate()
}
func responsePreference(v scenario.ActorView) (ResponsePreference, core.ID, error) {
	id := core.ID(string(v.Actor) + ":response-preference")
	for _, f := range v.Facts {
		if f.ID != id {
			continue
		}
		var p ResponsePreference
		if f.Observer != v.Actor || f.Subject != v.Actor || !permittedFacts(v)[id] || json.Unmarshal([]byte(f.Text), &p) != nil || p.Version != "demo-response-preference.v1" {
			break
		}
		switch p.Observation {
		case "observed", "silence", "lost_observation", "declined_participation":
			return p, id, nil
		}
	}
	return ResponsePreference{}, "", fmt.Errorf("missing permitted response preference")
}
