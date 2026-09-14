package behavior

import (
	"encoding/json"
	"github.com/tushardhara/dream/core"
	"github.com/tushardhara/dream/simulator/drives"
	"github.com/tushardhara/dream/simulator/dynamics"
	"github.com/tushardhara/dream/simulator/internal/reference"
	"math"
	"reflect"
	"testing"
)

// The generator and the original #42 assertions consume exactly the same
// versioned input fixture. Keeping this test in behavior avoids an experiment
// import cycle; it reads data, never evaluator labels or expected outcomes.
type relationshipReferenceFixture struct {
	Version   string           `json:"version"`
	At        core.LogicalTime `json:"at"`
	Actor     ActionActor      `json:"actor"`
	Situation ActionSituation  `json:"situation"`
	Profiles  []struct {
		Name    string                   `json:"name"`
		Context core.RelationshipContext `json:"context"`
	} `json:"profiles"`
}

func relationshipReference(t testing.TB) relationshipReferenceFixture {
	t.Helper()
	raw := reference.Relationships()
	var f relationshipReferenceFixture
	if e := json.Unmarshal(raw, &f); e != nil || f.Version != "relationship-comparisons.v1" || f.At != 5 || len(f.Profiles) != 7 {
		t.Fatal("controlled input fixture", e)
	}
	return f
}

func relationAccount(role core.ID, trust, expectation, history float64) core.RelationshipContext {
	return core.RelationshipContext{Version: 1, Observer: "a", Other: "b", Types: []core.ID{role}, Valid: core.Interval{}, Details: []core.RelationshipDetail{{Kind: "history", Sources: []core.ID{"e"}}}, Measures: []core.RelationshipMeasure{{Kind: "trust", Value: trust, Confidence: .8, Source: "e"}, {Kind: "expectation", Value: expectation, Confidence: .7, Source: "e"}, {Kind: "prior_outcome", Value: history, Confidence: .9, Source: "e"}, {Kind: "expected_reaction", Value: .5, Confidence: .6, Source: "e"}}}
}
func relationalChoice(t *testing.T, r *core.RelationshipContext, focus bool) (ActionActor, ActionDecision) {
	t.Helper()
	f := relationshipReference(t)
	a, s := f.Actor, f.Situation
	if r != nil {
		var e error
		s, e = ApplyRelationship(s, *r, "a", 5, focus)
		if e != nil {
			t.Fatal(e)
		}
	}
	next, d, e := ChooseAction(a, s, 5, 123456789)
	if e != nil {
		t.Fatal(e)
	}
	return next, d
}
func probability(d ActionDecision, kind Kind) float64 {
	for _, c := range d.Candidates {
		if c.Offer.Kind == kind {
			return c.Probability
		}
	}
	return 0
}
func TestRelationalControlledSpecificityAndAblations(t *testing.T) {
	f := relationshipReference(t)
	profiles := []core.RelationshipContext{}
	for _, p := range f.Profiles[:4] {
		profiles = append(profiles, p.Context)
	}
	decisions := []ActionDecision{}
	for _, r := range profiles {
		_, d := relationalChoice(t, &r, true)
		decisions = append(decisions, d)
		t.Logf("%s P(ask)=%.9f P(decline)=%.9f P(reveal)=%.9f", r.Types[0], probability(d, Ask), probability(d, Decline), probability(d, Reveal))
	}
	for i, a := range decisions {
		for _, b := range decisions[:i] {
			if math.Abs(probability(a, Ask)-probability(b, Ask)) < 1e-6 {
				t.Fatal("relationship context failed same-event distribution sensitivity")
			}
		}
	}
	// Labels alone cannot override the exact same person's attributed history.
	original := profiles[0]
	_, baseline := relationalChoice(t, &original, true)
	for _, role := range []core.ID{"sibling", "friend", "acquaintance", "manager"} {
		changed := original
		changed.Types = []core.ID{role}
		_, d := relationalChoice(t, &changed, true)
		if !reflect.DeepEqual(baseline, d) {
			t.Fatal("role label shortcut", role)
		}
	}
	// Use a nonsaturated baseline; vary only expectation, history, or stress.
	original = relationAccount("spouse", .1, .2, .1)
	_, baseline = relationalChoice(t, &original, true)
	for _, kind := range []core.ID{"expectation", "prior_outcome", "stress"} {
		raw, _ := json.Marshal(original)
		var changed core.RelationshipContext
		_ = json.Unmarshal(raw, &changed)
		found := false
		for i := range changed.Measures {
			if changed.Measures[i].Kind == kind {
				changed.Measures[i].Value = -.8
				found = true
			}
		}
		if !found {
			changed.Measures = append(changed.Measures, core.RelationshipMeasure{Kind: kind, Value: .9, Confidence: 1, Source: "e"})
		}
		_, d := relationalChoice(t, &changed, true)
		if reflect.DeepEqual(baseline.Candidates, d.Candidates) {
			t.Fatal("context ablation ignored", kind)
		}
	}
	// Counterexample: hostile spouse history vs supportive acquaintance, same actor.
	hostile := relationAccount("spouse", -.9, .8, -.9)
	supportive := relationAccount("acquaintance", .9, -.2, .9)
	_, h := relationalChoice(t, &hostile, true)
	_, s := relationalChoice(t, &supportive, true)
	if probability(h, Reveal) != 0 || probability(s, Reveal) <= 0 {
		t.Fatal("individual history did not reverse role expectation")
	}
	// Removing relationship knowledge leaves permitted questions and WAIT available.
	_, unknown := relationalChoice(t, nil, true)
	if probability(unknown, Ask) <= 0 || probability(unknown, Wait) <= 0 || probability(unknown, Reveal) != 0 {
		t.Fatal("unknown relationship invented consent or blocked unrelated action")
	}
	// Relationship appraisal is live, independently of consequence weighting.
	a, _ := relationalChoice(t, &original, true)
	b, _ := relationalChoice(t, &original, false)
	if reflect.DeepEqual(a.Drives.Variables, b.Drives.Variables) {
		t.Fatal("relationship never reached drive appraisal")
	}
}
func TestRelationshipAccessAndTimeFailClosed(t *testing.T) {
	for _, name := range []string{"foreign", "revoked", "unretrieved_detail", "future", "expired", "duplicate", "nan"} {
		t.Run(name, func(t *testing.T) {
			_, s := actionFixture(t)
			s.Contexts = nil
			s.RelationshipEvidence = []dynamics.Perceived{s.Observation.Event}
			r := relationAccount("friend", .5, .2, .1)
			switch name {
			case "foreign":
				r.Observer = "c"
			case "revoked":
				s.Sources = nil
			case "unretrieved_detail":
				r.Details[0].Sources = []core.ID{"unpermitted-private-report"}
			case "future":
				r.Valid.Start = 6
			case "expired":
				end := core.LogicalTime(5)
				r.Valid.End = &end
			case "duplicate":
				s.Contexts = []DisclosureContext{{Observer: "a", Recipient: "b"}}
			case "nan":
				r.Measures[0].Value = math.NaN()
			}
			if _, e := ApplyRelationship(s, r, "a", 5, true); e == nil {
				t.Fatal("invalid relationship accepted")
			}
		})
	}
}

func TestRelationshipExtremeValidMeasures(t *testing.T) {
	for _, sign := range []float64{1, -1} {
		a, s := actionFixture(t)
		s.Contexts = nil
		s.RelationshipEvidence = []dynamics.Perceived{s.Observation.Event}
		p := s.Observation.Event
		p.Confidence = 1
		s.RelationshipEvidence = []dynamics.Perceived{p}
		r := relationAccount("friend", 0, 0, 0)
		r.Measures = nil
		for _, kind := range []core.ID{"trust", "expectation", "expected_reaction", "protective_intent", "social_norm", "prior_outcome", "sensitivity", "fear", "pride", "shame", "stress"} {
			v := sign
			switch kind {
			case "sensitivity", "fear", "pride", "shame", "stress":
				v = -sign
			}
			r.Measures = append(r.Measures, core.RelationshipMeasure{Kind: kind, Value: v, Confidence: 1, Source: "e"})
		}
		next, e := ApplyRelationship(s, r, "a", 5, true)
		if e != nil {
			t.Fatal(e)
		}
		if _, _, e = ChooseAction(a, next, 5, 0); e != nil {
			t.Fatal("valid extreme report became invalid memory", e)
		}
		if next.Relationships[0].Disclosure != sign {
			t.Fatal("bounded projection", next.Relationships[0].Disclosure)
		}
	}
}
func retainedRelationshipFixture(t *testing.T) (ActionSituation, core.RelationshipContext) {
	t.Helper()
	_, s := actionFixture(t)
	s.Contexts = nil
	s.RelationshipEvidence = []dynamics.Perceived{s.Observation.Event}
	s.Observation.Event.OccurredAt = 4
	s.Observation.Event.LearnedAt = 5
	r := relationAccount("friend", .8, .2, .5)
	for i := range r.Measures {
		r.Measures[i].Source = "history-report"
	}
	r.Details[0].Sources = []core.ID{"history-report"}
	s.Sources = append(s.Sources, "history-report")
	p := s.Observation.Event
	p.Event = "history-report"
	p.Rights.Resource = p.Event
	p.OccurredAt = 1
	p.LearnedAt = 2
	p.Confidence = .4
	s.RelationshipEvidence = []dynamics.Perceived{p}
	return s, r
}
func TestRelationshipRetainsRetrievedTimeAndUncertainty(t *testing.T) {
	s, r := retainedRelationshipFixture(t)
	out, e := ApplyRelationship(s, r, "a", 5, true)
	if e != nil {
		t.Fatal(e)
	}
	cue := out.Observation.Context[drives.Relationships]
	if cue.Evidence.OccurredAt != 1 || cue.Evidence.LearnedAt != 2 || cue.Evidence.Event != "history-report" {
		t.Fatal("current event replaced retained source times", cue.Evidence)
	}
	if math.Abs(float64(out.Contexts[0].Trust.Confidence)-.8*.4) > 1e-12 {
		t.Fatal("source uncertainty discarded")
	}
	s.RelationshipEvidence[0].Confidence = 0
	out, e = ApplyRelationship(s, r, "a", 5, true)
	if e != nil || out.Contexts[0].Trust.Confidence != 0 {
		t.Fatal("unknown source became certain", e)
	}
}
func TestRelationshipEvidenceMetadataFailsClosed(t *testing.T) {
	for _, name := range []string{"missing", "foreign", "future", "duplicate", "revoked"} {
		t.Run(name, func(t *testing.T) {
			s, r := retainedRelationshipFixture(t)
			switch name {
			case "missing":
				s.RelationshipEvidence = nil
			case "foreign":
				s.RelationshipEvidence[0].Actor = "b"
			case "future":
				s.RelationshipEvidence[0].LearnedAt = 6
			case "duplicate":
				s.RelationshipEvidence = append(s.RelationshipEvidence, s.RelationshipEvidence[0])
			case "revoked":
				s.RelationshipEvidence[0].Rights.Revoked = true
			}
			if _, e := ApplyRelationship(s, r, "a", 5, true); e == nil {
				t.Fatal("invalid retained source accepted")
			}
		})
	}
}
