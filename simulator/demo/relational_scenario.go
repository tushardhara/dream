package demo

import (
	"encoding/json"
	"fmt"
	"github.com/tushardhara/dream/core"
	"github.com/tushardhara/dream/simulator/scenario"
)

const RelationalVersion = "backend-demo.v2"

// RelationalScenario is the current synthetic consumer of the shared 22-drive,
// 27-action engine. Scenario remains the frozen v1 replay fixture factory.
func RelationalScenario(people, months int, seed uint64) (scenario.Scenario, error) {
	if people != 2 && people != 4 && people != 5 && people != 24 {
		return scenario.Scenario{}, fmt.Errorf("relational demo supports 2/4/5/24 fictional adults")
	}
	sc, e := buildScenario(people, months, seed)
	if e != nil {
		return sc, e
	}
	sc.World.ID = "relational:" + sc.World.ID
	for i := range sc.Actors {
		sc.Actors[i].Relationships = nil
		sc.Actors[i].Contexts = nil
	}
	type edge struct {
		from, to int
		kind     core.ID
	}
	edges := []edge{}
	if people == 5 {
		// Fixture assumption: S is H's sibling; W knows S as sibling-in-law.
		// There is deliberately no W-B knowledge. No gender coefficient exists.
		names := []string{"H", "W", "S", "A", "B"}
		for i := range names {
			sc.Public.Humans[i].Name = "Fictional Adult " + names[i]
		}
		edges = []edge{{0, 1, "spouse"}, {0, 2, "sibling"}, {0, 3, "friend"}, {0, 4, "friend"}, {1, 3, "acquaintance"}, {1, 2, "sibling_in_law"}}
	} else {
		for i := 0; i < people; i += 2 {
			edges = append(edges, edge{i, i + 1, "spouse"})
		}
		if people >= 4 {
			for i := 0; i < people; i++ {
				if i%6 < 4 && i+2 < people {
					edges = append(edges, edge{i, i + 2, "sibling"})
				}
			}
		}
		if people == 24 {
			for i := 0; i < people; i++ {
				edges = append(edges, edge{i, (i + 7) % people, "friend"})
			}
		}
	}
	for ei, edge := range edges {
		for direction, pair := range [][2]int{{edge.from, edge.to}, {edge.to, edge.from}} {
			a := &sc.Actors[pair[0]]
			other := human(pair[1])
			source := core.ID(string(a.ID) + ":relationship:" + string(other))
			a.Facts = append(a.Facts, scenario.Fact{ID: source, Observer: a.ID, Subject: other, Text: "Fictional observer report: my relationship history and expectations about " + string(other), Confidence: .75, Valid: core.Interval{}, Grants: []core.Grant{{Actor: a.ID, Recipient: a.ID, Purpose: "simulation", Operation: core.Read}, {Actor: a.ID, Recipient: a.ID, Purpose: "simulation", Operation: core.Derive}}})
			a.Knowledge = append(a.Knowledge, scenario.Knowledge{Record: source, LearnedAt: 0})
			a.Relationships = append(a.Relationships, scenario.Relationship{ID: core.ID(fmt.Sprintf("rel:%02d:%02d", pair[0], pair[1])), Other: other, Kind: edge.kind, Evidence: source})
			// Explicit fictional reports, assigned by edge/direction, not role or sex.
			// Reverse perspectives intentionally disagree and retain uncertainty.
			values := []float64{.85, -.55, .35, .1, -.2, .65}
			trust := values[(ei+direction*3)%len(values)]
			expectation := values[(ei+1+direction)%len(values)]
			measure := func(kind core.ID, value float64) core.RelationshipMeasure {
				return core.RelationshipMeasure{Kind: kind, Value: value, Confidence: .75, Source: source}
			}
			r := core.RelationshipContext{Version: 1, Observer: a.ID, Other: other, Types: []core.ID{edge.kind}, Valid: core.Interval{}, Details: []core.RelationshipDetail{{Kind: "history", Sources: []core.ID{source}}, {Kind: "view_of_other", Sources: []core.ID{source}}}, Measures: []core.RelationshipMeasure{measure("trust", trust), measure("closeness", values[(ei+2+direction)%len(values)]), measure("expectation", expectation), measure("expected_reaction", trust*.7), measure("prior_outcome", trust*.8), measure("friction", -trust*.5)}}
			if people == 5 && edge.kind == "sibling_in_law" {
				r.Types = append(r.Types, "acquaintance")
			}
			a.Contexts = append(a.Contexts, r)

		}
	}
	for i := range sc.Future {
		var p Period
		_ = json.Unmarshal([]byte(sc.Future[i].Text), &p)
		p.Version = RelationalVersion
		raw, _ := json.Marshal(p)
		sc.Future[i].Text = string(raw)
	}
	return sc, sc.Validate()
}
