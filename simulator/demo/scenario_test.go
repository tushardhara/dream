package demo

import (
	"encoding/json"
	"testing"
)

func TestDemoScenarioScaleAndBoundedHorizon(t *testing.T) {
	for _, people := range []int{2, 4, 24} {
		sc, e := Scenario(people, 12, 11)
		if e != nil {
			t.Fatal(e)
		}
		if len(sc.Public.Humans) != people || sc.World.Horizon != 12*Month || len(sc.Future) != 24 || sc.Future[23].At != sc.World.Horizon-1 {
			t.Fatal("invalid simulated horizon")
		}
		if people == 24 {
			edges := 0
			memberships := map[string]int{}
			for _, a := range sc.Actors {
				edges += len(a.Relationships)
			}
			for _, g := range sc.Public.Groups {
				for _, id := range g.Members {
					memberships[string(id)]++
				}
			}
			if len(sc.Public.Groups) != 8 || edges < 30 {
				t.Fatal("scale missing")
			}
			for _, n := range memberships {
				if n < 2 {
					t.Fatal("groups do not overlap")
				}
			}
		}
		known := map[string]bool{}
		for _, event := range sc.Future {
			var p Period
			if json.Unmarshal([]byte(event.Text), &p) != nil {
				t.Fatal("invalid period")
			}
			known[p.Theme] = true
		}
		for _, theme := range []string{"routine", "joy", "compound", "scarcity", "family", "work"} {
			if !known[theme] {
				t.Fatal("missing period", theme)
			}
		}
	}
	for _, args := range [][2]int{{1, 12}, {25, 12}, {24, 0}, {24, 13}} {
		if _, e := Scenario(args[0], args[1], 11); e == nil {
			t.Fatal("unbounded demo accepted")
		}
	}
}
