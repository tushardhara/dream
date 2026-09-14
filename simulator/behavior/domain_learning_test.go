package behavior

import (
	"bytes"
	"encoding/json"
	"math"
	"testing"

	"github.com/tushardhara/dream/core"
	"github.com/tushardhara/dream/simulator/dynamics"
)

func TestObservedLearningWritesOnlyItsDomainAndFrame(t *testing.T) {
	actor, err := NewDomainActor("a", 0)
	if err != nil {
		t.Fatal(err)
	}
	responses := []dynamics.Perceived{}
	steps := []struct {
		domain   core.RelationshipDomain
		frame    core.ID
		response string
		want     float64
	}{
		{core.Childcare, "family", "supportive", .1},
		{core.Finances, "family", "dismissive", -.1},
		{core.Childcare, "business", "supportive", .1},
		{core.Finances, "family", "dismissive", -.2},
	}
	for i, step := range steps {
		at := core.LogicalTime(1 + 4*i)
		in := domainSituation(t, step.domain, step.frame, Help)
		addDomainAccount(t, &in, "care-family", core.Childcare, "family", .3)
		addDomainAccount(t, &in, "finance-family", core.Finances, "family", .3)
		addDomainAccount(t, &in, "care-business", core.Childcare, "business", .3)
		in = laterDomainEvent(in, core.ID("decision-"+string(rune('a'+i))), at)
		for _, response := range responses {
			in.Scoped.Situation.Sources = append(in.Scoped.Situation.Sources, response.Event)
			in.Scoped.Situation.RelationshipEvidence = append(in.Scoped.Situation.RelationshipEvidence, response)
		}
		chosen, decision, err := ChooseDomainAction(actor, in, at, math.MaxUint64)
		if err != nil || decision.Human.Human.Candidates[decision.Human.Human.Selected].Offer.Kind != Help {
			t.Fatalf("step %d actual Help control: %v", i, err)
		}
		response := in.Scoped.Situation.Observation.Event
		response.Event = core.ID("response-" + string(rune('a'+i)))
		response.Rights.Resource = response.Event
		response.OccurredAt = at + 2
		response.LearnedAt = at + 3
		response.Confidence = 1
		learned, err := ResolveDomainAction(chosen, decision.Human.Human.ID, response, in.Scoped.Situation.RelationshipEvidence, "b", step.response)
		if err != nil {
			t.Fatalf("step %d observed learning: %v", i, err)
		}
		wantBuckets := i + 1
		if wantBuckets > 3 {
			wantBuckets = 3
		}
		if len(learned.Memories) != wantBuckets {
			t.Fatalf("step %d erased/duplicated an independently learned context: %+v", i, learned.Memories)
		}
		// Every previously learned other domain/frame must survive byte-identical.
		// This checks writes, independently of the already-tested domainMemory reads.
		for _, before := range actor.Memories {
			if before.Domain == step.domain && before.RoleContext == step.frame {
				continue
			}
			original, _ := json.Marshal(before)
			found := false
			for _, after := range learned.Memories {
				if after.Domain == before.Domain && after.RoleContext == before.RoleContext {
					actual, _ := json.Marshal(after)
					if !bytes.Equal(original, actual) {
						t.Fatalf("step %d learning overwrote unrelated %s/%s", i, before.Domain, before.RoleContext)
					}
					found = true
				}
			}
			if !found {
				t.Fatal("previously learned context disappeared")
			}
		}
		found := false
		for _, memory := range learned.Memories {
			if memory.Domain == step.domain && memory.RoleContext == step.frame {
				if len(memory.Memory) != 1 || memory.Memory[0].Other != "b" || math.Abs(memory.Memory[0].Trust-step.want) > 1e-12 {
					t.Fatalf("step %d response did not update its own context: %+v", i, memory)
				}
				found = true
			}
		}
		if !found {
			t.Fatal("observed outcome did not create its context")
		}
		encoded, err := learned.Encode()
		if err != nil {
			t.Fatal(err)
		}
		actor, err = DecodeDomainActor(encoded)
		if err != nil {
			t.Fatal(err)
		}
		responses = append(responses, response)
	}
}
