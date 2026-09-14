package temporalexperiment

import (
	"context"
	"github.com/tushardhara/dream/app/assistance"
	"github.com/tushardhara/dream/simulator/behavior"
	"testing"
)

func askProbability(d behavior.ActionDecision) float64 {
	for _, c := range d.Candidates {
		if c.Offer.Kind == behavior.Ask {
			return c.Probability
		}
	}
	return 0
}
func TestActualTemporalCompositionScenesAndReplay(t *testing.T) {
	for _, s := range Scenes() {
		t.Run(s.Name, func(t *testing.T) {
			out, e := Run(context.Background(), s, "temporal", 35, 7, "unseen", nil)
			if e != nil {
				t.Fatal(e)
			}
			if out.PrivacyViolations != 0 || out.BoundaryViolations != 0 || len(out.Humans) != 2 {
				t.Fatal(out)
			}
			want := s.Name == "daily_60" || s.Name == "work_transition" || s.Name == "stale_experience"
			if out.Helper.Delivered != want {
				t.Fatal("actual helper ignored scene", out.Helper)
			}
			for i, h := range out.Humans {
				humanWant := want || s.Name == "observer_disagreement" && i == 0
				if (askProbability(h.Decision.Human.Human.Human) > 0) != humanWant {
					t.Fatal("actual human ignored temporal scene", i, h.Decision)
				}
			}
			replay, e := Run(context.Background(), s, "temporal", 35, 7, "unseen", &out)
			if e != nil || assistance.Digest(replay) != assistance.Digest(out) {
				t.Fatal("replay", e)
			}
		})
	}
}
func TestAgeAndHiddenChannelAreNotConsumerCoefficients(t *testing.T) {
	s := Scenes()[1]
	reference, e := Run(context.Background(), s, "temporal", 35, 17, "one", nil)
	if e != nil {
		t.Fatal(e)
	}
	for _, age := range []int{0, 18, 75, 120} {
		got, e := Run(context.Background(), s, "temporal", age, 17, "changed-secret", nil)
		if e != nil || assistance.Digest(got) != assistance.Digest(reference) {
			t.Fatal("age/hidden activity changed consumers", age, e)
		}
	}
}
func TestMatchedArmsKeepHumanInputsFixed(t *testing.T) {
	for _, s := range Scenes() {
		base, e := Run(context.Background(), s, "temporal", 35, 9, "unseen", nil)
		if e != nil {
			t.Fatal(e)
		}
		for _, arm := range []string{"context_off", "simple"} {
			got, e := Run(context.Background(), s, arm, 35, 9, "unseen", nil)
			if e != nil {
				t.Fatal(s.Name, arm, e)
			}
			if assistance.Digest(got.Humans) != assistance.Digest(base.Humans) {
				t.Fatal("helper arm changed human model", s.Name, arm)
			}
		}
	}
}

func TestEvaluationRecordsAdverseAndNullOutcomes(t *testing.T) {
	report, e := Evaluate(context.Background(), []uint64{1, 2, 3, 4})
	if e != nil {
		t.Fatal(e)
	}
	if len(report.Rows) != 60 {
		t.Fatal("incomplete comparison")
	}
	unnecessary, missed, matchedBaseline := 0, 0, 0
	for i, row := range report.Rows {
		m := row.Metrics
		if m.Trials != m.Appropriate+m.Unnecessary+m.Missed {
			t.Fatal("invalid denominator", row)
		}
		if row.Consumer == "human" && row.Policy == "temporal" {
			missed += m.Missed
			if m.StatusAgreement == nil {
				t.Fatal("uncertainty not measured")
			}
		}
		if row.Consumer == "helper" && row.Policy == "context_off" {
			unnecessary += m.Unnecessary
			if m == report.Rows[i+1].Metrics {
				matchedBaseline++
			}
		}
		if row.Policy == "context_off" && m.StatusAgreement != nil {
			t.Fatal("invented baseline uncertainty")
		}
	}
	if unnecessary == 0 || missed == 0 || matchedBaseline != len(Scenes()) {
		t.Fatal("adverse/null outcomes concealed", unnecessary, missed, matchedBaseline)
	}
}
