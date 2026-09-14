// Package demo supplies a bounded synthetic backend demonstration, not a study
// result or a validated model of human behavior.
package demo

import (
	"encoding/json"
	"fmt"

	"github.com/tushardhara/dream/core"
	"github.com/tushardhara/dream/simulator"
	"github.com/tushardhara/dream/simulator/dynamics"
	"github.com/tushardhara/dream/simulator/scenario"
)

const Version = "backend-demo.v1"
const Day = 24 * dynamics.Hour
const Month = 30 * Day // Explicit fixed-duration model month, not a calendar month.
type Period struct {
	Version string  `json:"version"`
	Index   int     `json:"index"`
	Theme   string  `json:"theme"`
	Group   core.ID `json:"group"`
}

var themes = []string{"routine", "joy", "routine", "scarcity", "support", "compound", "routine", "joy", "work", "family", "routine", "repair"}

func human(i int) core.ID { return core.ID(fmt.Sprintf("person:%02d", i+1)) }
func Scenario(people, months int, seed uint64) (scenario.Scenario, error) {
	if (people != 2 && people != 4 && people != 24) || months < 1 || months > 12 {
		return scenario.Scenario{}, fmt.Errorf("demo supports 2/4/24 people and 1..12 simulated months")
	}
	return buildScenario(people, months, seed)
}
func buildScenario(people, months int, seed uint64) (scenario.Scenario, error) {
	if months < 1 || months > 12 {
		return scenario.Scenario{}, fmt.Errorf("demo months bound")
	}
	sc := scenario.Scenario{Version: 1, World: scenario.World{ID: simulator.WorldID(fmt.Sprintf("demo:%d:%d:%d", people, months, seed)), Seed: seed, Horizon: core.LogicalTime(months) * Month}, Requires: []core.ID{}, Public: scenario.Public{Resources: []scenario.Resource{{ID: "shared-time", Capacity: 12, Available: 12}}}, Research: scenario.Research{Labels: []scenario.Label{{ID: "evaluator-only", Text: "DEMO_RESEARCH_LABEL_CANARY"}}}}
	for i := 0; i < people; i++ {
		id := human(i)
		sc.Public.Humans = append(sc.Public.Humans, scenario.Human{ID: id, Name: fmt.Sprintf("Fictional Adult %02d", i+1), Age: 25 + i})
		source := core.ID(string(id) + ":own-note")
		fact := scenario.Fact{ID: source, Observer: id, Subject: id, Text: "OWN_FICTIONAL_NOTE:" + string(id), Confidence: .7, Valid: core.Interval{Start: 0}, Grants: []core.Grant{{Actor: id, Recipient: id, Purpose: "simulation", Operation: core.Read}, {Actor: id, Recipient: id, Purpose: "simulation", Operation: core.Derive}}}
		actor := scenario.Actor{ID: id, Facts: []scenario.Fact{fact}, Knowledge: []scenario.Knowledge{{Record: source, LearnedAt: 0}}, Memories: []scenario.Memory{{ID: core.ID(string(id) + ":memory"), Evidence: source, Text: "own uncertain recollection"}}}
		distances := []int{1}
		if people > 2 {
			distances = append(distances, people/3+1)
		}
		for j, offset := range distances {
			actor.Relationships = append(actor.Relationships, scenario.Relationship{ID: core.ID(fmt.Sprintf("edge:%02d:%d", i, j)), Other: human((i + offset) % people), Kind: []core.ID{"family", "work"}[j], Evidence: source})
		}
		sc.Actors = append(sc.Actors, actor)
	}
	groups := 2
	if people == 24 {
		groups = 8
	}
	for g := 0; g < groups; g++ {
		kind := []string{"family", "work", "friends"}[g%3]
		group := scenario.Group{ID: core.ID(fmt.Sprintf("%s:%d", kind, g))}
		members := people
		if people == 24 {
			members = 6
		}
		for j := 0; j < members; j++ {
			group.Members = append(group.Members, human((g*(people/groups)+j)%people))
		}
		sc.Public.Groups = append(sc.Public.Groups, group)
	}
	for p := 1; p <= 2*months; p++ {
		theme := themes[(p-1)%len(themes)]
		period := Period{Version: Version, Index: p, Theme: theme, Group: sc.Public.Groups[(p-1)%groups].ID}
		raw, _ := json.Marshal(period)
		at := core.LogicalTime((p+1)/2) * Month
		if p == 2*months {
			at--
		}
		if p%2 == 1 {
			at -= Day
		}
		sc.Future = append(sc.Future, scenario.Scheduled{ID: core.ID(fmt.Sprintf("period:%02d", p)), At: at, Kind: "observation", Actor: human(0), Text: string(raw)})
	}
	return sc, sc.Validate()
}
