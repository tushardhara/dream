package evals

import (
	"fmt"
	"reflect"
	"testing"
)

func TestCompleteImmutableSourceRegistry(t *testing.T) {
	names := []string{"persistence", "plasticity", "relational_specificity", "hidden_state", "partial_observability", "memory", "slow_dynamics", "emergence", "stochasticity", "reproducibility", "counterfactual_branching", "ground_truth_isolation", "calibration_readiness", "prompt_dominance", "hidden_state_no_value", "no_relational_specificity", "personality_stereotype_collapse", "unnatural_agreement", "unrealistic_memory_determinism", "no_alternative_futures", "human_calibration_failure", "incompatible_model_laws", "no_human_prediction_transfer"}
	r := AlignmentRegistry()
	if len(ExitCriteria()) != 13 || len(PrimaryFalsifiers()) != 10 || len(r) != 23 {
		t.Fatal("missing source entry")
	}
	seen := map[string]bool{}
	for i, e := range r {
		section, ordinal := "hws19", i+1
		if i >= 13 {
			section, ordinal = "hws20", i-12
		}
		if e.ID != fmt.Sprintf("%s.%02d", section, ordinal) {
			t.Fatal("stable source ID changed", e.ID)
		}
		if e.Name != names[i] || seen[e.ID] || e.ID == "" || len(e.EvidenceRequirements) == 0 || e.SourceWording == "" || e.SourceSection == "" {
			t.Fatal("source registry renamed, duplicated or incomplete", i, e)
		}
		seen[e.ID] = true
	}
	if AlignmentRegistryHash() != "c506317e495330499bf8c841178293cd81ee31a9b77ad1e37f5272b074bf4417" {
		t.Fatal("complete source wording/requirements registry changed", AlignmentRegistryHash())
	}
	if r[12].SourceWording != "real human data can be imported and compared." {
		t.Fatal("future-path limitation substituted for calibration source")
	}
	if r[2].Name == r[2].SourceWording {
		t.Fatal("normalized name replaced source wording")
	}
	before := AlignmentRegistry()
	r[0].Name = "changed"
	r[0].EvidenceRequirements[0] = "none"
	if !reflect.DeepEqual(before, AlignmentRegistry()) {
		t.Fatal("caller mutated registry")
	}
	for _, bad := range [][]SourceCriterion{before[:22], append(before, before[0]), append([]SourceCriterion{before[1]}, before[1:]...)} {
		if sameAlignmentRegistry(bad) {
			t.Fatal("omitted/duplicate registry accepted")
		}
	}
}
