package evals

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/tushardhara/dream/simulator/behavior"
	"github.com/tushardhara/dream/simulator/experiment"
)

func TestAlignmentMechanicsRejectUnsupportedPass(t *testing.T) {
	dataset, plan := alignmentFixture(t)
	probe, err := experiment.GenerateRelationshipProbe(context.Background(), plan.Probe)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(probe)
	clone := func() *experiment.RelationshipProbe {
		var p experiment.RelationshipProbe
		if err := json.Unmarshal(raw, &p); err != nil {
			t.Fatal(err)
		}
		return &p
	}
	find := func(p *experiment.RelationshipProbe, name string) *experiment.RelationshipTrial {
		for i := range p.Trials {
			if p.Trials[i].Seed == plan.Probe.Seeds[0] && p.Trials[i].Case == name {
				return &p.Trials[i]
			}
		}
		t.Fatal("missing counterevidence fixture", name)
		return nil
	}
	cases := []struct {
		name   string
		want   AlignmentStatus
		change func(*AlignmentEvidence)
	}{
		{"relational_context_sensitivity", AlignmentFail, func(e *AlignmentEvidence) {
			find(e.First, "spouse").Decision = find(e.First, "sibling").Decision
		}},
		{"label_invariance", AlignmentFail, func(e *AlignmentEvidence) {
			find(e.First, "label_sibling").Decision = find(e.First, "friend").Decision
		}},
		{"expectation_history_stress_ablations", AlignmentFail, func(e *AlignmentEvidence) {
			find(e.First, "ablate_expectation").Decision = find(e.First, "ablation_baseline").Decision
		}},
		{"history_reversal", AlignmentFail, func(e *AlignmentEvidence) {
			find(e.First, "hostile_spouse").Decision = find(e.First, "supportive_acquaintance").Decision
		}},
		{"unknown_permission_boundary", AlignmentFail, func(e *AlignmentEvidence) {
			find(e.First, "unknown").Decision = find(e.First, "spouse").Decision
		}},
		{"relationship_appraisal", AlignmentFail, func(e *AlignmentEvidence) {
			find(e.First, "ablation_baseline").AfterDrivesHash = find(e.First, "without_appraisal").AfterDrivesHash
		}},
		{"event_changes_drive_state", AlignmentFail, func(e *AlignmentEvidence) {
			tr := find(e.First, "spouse")
			tr.AfterDrivesHash = tr.BeforeDrivesHash
		}},
		{"selected_action_seed_variation", AlignmentInconclusive, func(e *AlignmentEvidence) {
			// A valid generator can observe just WAIT at every seed. Retain each
			// original draw, with a valid one-candidate distribution, rather than
			// relying on an invalid Selected index or a request-binding failure.
			for i := range e.First.Trials {
				tr := &e.First.Trials[i]
				if tr.Case == "spouse" {
					d := &tr.Decision
					wait := d.Candidates[0]
					if wait.Offer.Kind != behavior.Wait {
						t.Fatal("missing WAIT counterevidence")
					}
					wait.Probability = 1
					d.Candidates = []behavior.ActionCandidate{wait}
					d.Selected, d.Explored = 0, false
				}
			}
		}},
		{"repeated_generation", AlignmentFail, func(e *AlignmentEvidence) {
			e.Repeat.Trials[0].AfterStateHash = strings.Repeat("a", 64)
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			evidence := AlignmentEvidence{First: clone(), Repeat: clone()}
			tc.change(&evidence)
			// Counterevidence is structurally valid, complete and bound to the
			// original inputs. The report must record the failed named mechanic;
			// a codec/size/hash rejection is not credited as a negative verdict.
			for _, p := range []*experiment.RelationshipProbe{evidence.First, evidence.Repeat} {
				if err := p.Validate(plan.Probe); err != nil {
					t.Fatal("invalid counterevidence setup", err)
				}
			}
			r, err := BuildAlignmentReport(plan, evidence)
			if err != nil || r.Verify(plan, dataset, alignmentReceipt(r, fixtureNow)) != nil {
				t.Fatal("honest negative report rejected", err)
			}
			found := false
			for _, m := range r.Mechanics {
				if m.Name == tc.name {
					found = true
					if m.Status != tc.want {
						t.Fatalf("unsupported mechanic PASS: %s got %s want %s", tc.name, m.Status, tc.want)
					}
				}
			}
			if !found {
				t.Fatal("named mechanic omitted", tc.name)
			}
		})
	}
	for _, evidence := range []AlignmentEvidence{{}, {First: clone()}} {
		r, err := BuildAlignmentReport(plan, evidence)
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range r.Mechanics {
			if evidence.First == nil && m.Status != AlignmentNotTested {
				t.Fatal("unrun mechanic overstated", m)
			}
			if evidence.First != nil && m.Name == "repeated_generation" && m.Status != AlignmentInconclusive {
				t.Fatal("missing repeat overstated", m)
			}
		}
	}
}

func TestAlignmentPlanRejectsEveryInvalidField(t *testing.T) {
	_, plan := alignmentFixture(t)
	if err := plan.Validate(); err != nil {
		t.Fatal("valid control rejected", err)
	}
	cases := []struct {
		name   string
		change func(*AlignmentPlan)
	}{
		{"version", func(p *AlignmentPlan) { p.Version = "unrecognized" }},
		{"source_revision", func(p *AlignmentPlan) { p.SourceRevision = "not-a-git-hash" }},
		{"source_tree", func(p *AlignmentPlan) { p.SourceTree = "not-a-git-hash" }},
		{"source_extract", func(p *AlignmentPlan) { p.SourceExtractSHA256 = strings.Repeat("e", 64) }},
		{"evaluator_artifact", func(p *AlignmentPlan) { p.EvaluatorArtifact = "not-a-sha256" }},
		{"generator_artifact", func(p *AlignmentPlan) { p.GeneratorArtifact = "image:latest" }},
		{"model", func(p *AlignmentPlan) { p.Model = "unrecognized" }},
		{"dataset_hash", func(p *AlignmentPlan) { p.DatasetHash = "not-a-sha256" }},
		{"registry_version", func(p *AlignmentPlan) { p.RegistryVersion = "unrecognized" }},
		{"registry_hash", func(p *AlignmentPlan) { p.RegistryHash = strings.Repeat("e", 64) }},
		{"probe_version", func(p *AlignmentPlan) { p.Probe.Version = "unrecognized" }},
		{"probe_fixture", func(p *AlignmentPlan) { p.Probe.FixtureHash = strings.Repeat("e", 64) }},
		{"probe_seeds_missing", func(p *AlignmentPlan) { p.Probe.Seeds = nil }},
		{"probe_seeds_insufficient", func(p *AlignmentPlan) { p.Probe.Seeds = []uint64{11} }},
		{"probe_seeds_duplicate", func(p *AlignmentPlan) { p.Probe.Seeds = []uint64{11, 11} }},
		{"hypothesis", func(p *AlignmentPlan) { p.Hypothesis = "post-hoc hypothesis" }},
		{"decision_rule", func(p *AlignmentPlan) { p.DecisionRule = "any observed relationship counts as PASS" }},
		{"evidence_scope", func(p *AlignmentPlan) { p.EvidenceScope = "whatever the run produced" }},
		{"registration_missing", func(p *AlignmentPlan) { p.RegisteredAt = time.Time{} }},
		{"registration_not_utc", func(p *AlignmentPlan) { p.RegisteredAt = p.RegisteredAt.In(time.FixedZone("offset", 3600)) }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			bad := plan
			tc.change(&bad)
			if bad.Validate() == nil {
				t.Fatal("invalid frozen plan field accepted", tc.name)
			}
		})
	}
}

func TestAlignmentTamperedRetainedPlanCannotScore(t *testing.T) {
	dataset, plan := alignmentFixture(t)
	for _, field := range []string{"decision_rule", "evidence_scope", "both"} {
		t.Run(field, func(t *testing.T) {
			bad := plan
			if field != "evidence_scope" {
				bad.DecisionRule = "any observed relationship counts as PASS"
			}
			if field != "decision_rule" {
				bad.EvidenceScope = "whatever the run produced"
			}
			// Model the on-disk retained plan, not just an altered report whose
			// separately retained expected plan is still pristine.
			raw, _ := json.Marshal(bad)
			var retained AlignmentPlan
			if err := json.Unmarshal(raw, &retained); err != nil {
				t.Fatal(err)
			}
			calls := 0
			g := relationshipGeneratorFunc(func(ctx context.Context, req experiment.RelationshipProbeRequest) (experiment.RelationshipProbe, error) {
				calls++
				return experiment.GenerateRelationshipProbe(ctx, req)
			})
			report, receipt, err := RunAlignment(context.Background(), dataset, retained, g, fixtureNow)
			if err == nil || report.Verify(retained, dataset, receipt) == nil || calls != 0 {
				t.Fatal("tampered retained plan reached scoring/verified report", field, calls, err)
			}
		})
	}
}
