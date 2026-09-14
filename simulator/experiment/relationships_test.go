package experiment

import (
	"context"
	"reflect"
	"testing"

	"github.com/tushardhara/dream/simulator/behavior"
)

func TestRelationshipProbeUsesControlledInputsAndSeeds(t *testing.T) {
	r := RelationshipProbeRequest{RelationshipExperimentVersion, RelationshipFixtureHash(), []uint64{11, 23, 37, 53, 71, 97, 131, 173}}
	p, e := GenerateRelationshipProbe(context.Background(), r)
	if e != nil {
		t.Fatal(e)
	}
	q, e := GenerateRelationshipProbe(context.Background(), r)
	if e != nil || !reflect.DeepEqual(p, q) {
		t.Fatal("same frozen probe did not reproduce", e)
	}
	if len(p.Trials) != 16*8 {
		t.Fatal("incomplete controlled experiment")
	}
	selected := map[behavior.Kind]bool{}
	for _, tr := range p.Trials {
		if tr.Case == "spouse" {
			selected[tr.Decision.Candidates[tr.Decision.Selected].Offer.Kind] = true
		}
	}
	if len(selected) < 2 {
		t.Fatal("different seeds changed hashes but not selected actions")
	}
	for _, change := range []string{"omitted", "seed", "source", "draw", "time"} {
		t.Run(change, func(t *testing.T) {
			bad, _ := GenerateRelationshipProbe(context.Background(), r)
			switch change {
			case "omitted":
				bad.Trials = bad.Trials[:len(bad.Trials)-1]
			case "seed":
				bad.Trials[0].Seed++
			case "source":
				bad.Trials[0].InputHash = bad.Trials[1].InputHash
			case "draw":
				bad.Trials[0].Decision.Draw++
			case "time":
				bad.Trials[0].Decision.At++
			}
			if bad.Validate(r) == nil {
				t.Fatal("unbound controlled evidence accepted")
			}
		})
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, e = GenerateRelationshipProbe(ctx, r); e == nil {
		t.Fatal("cancelled generation ran")
	}
}
