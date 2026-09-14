package behavior

import (
	"encoding/json"
	"github.com/tushardhara/dream/core"
	"math"
	"reflect"
	"testing"
)

func relationAccount(role core.ID, trust, expectation, history float64) core.RelationshipContext {
	return core.RelationshipContext{Version: 1, Observer: "a", Other: "b", Types: []core.ID{role}, Valid: core.Interval{}, Details: []core.RelationshipDetail{{Kind: "history", Sources: []core.ID{"e"}}}, Measures: []core.RelationshipMeasure{{Kind: "trust", Value: trust, Confidence: .8, Source: "e"}, {Kind: "expectation", Value: expectation, Confidence: .7, Source: "e"}, {Kind: "prior_outcome", Value: history, Confidence: .9, Source: "e"}, {Kind: "expected_reaction", Value: .5, Confidence: .6, Source: "e"}}}
}
func relationalChoice(t *testing.T, r *core.RelationshipContext, focus bool) (ActionActor, ActionDecision) {
	t.Helper()
	a, s := actionFixture(t)
	s.Contexts = nil
	s.Offers = []ActionOffer{{Kind: Ask, Recipient: "b", Evidence: []core.ID{"e"}, Duration: 1}, {Kind: Decline, Recipient: "b", Evidence: []core.ID{"e"}, Duration: 1}, {Kind: Reveal, Recipient: "b", Evidence: []core.ID{"e"}, Duration: 1, Mode: Full}}
	s.Disclosure = &DisclosureGrant{Recipient: "b", Mode: Full, Sources: []core.ID{"e"}}
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
	profiles := []core.RelationshipContext{relationAccount("spouse", .8, .7, .6), relationAccount("sibling", .1, -.3, -.5), relationAccount("friend", .5, .2, .1), relationAccount("acquaintance", -.2, -.5, -.2)}
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
	for _, name := range []string{"foreign", "revoked", "future", "expired", "duplicate", "nan"} {
		t.Run(name, func(t *testing.T) {
			_, s := actionFixture(t)
			s.Contexts = nil
			r := relationAccount("friend", .5, .2, .1)
			switch name {
			case "foreign":
				r.Observer = "c"
			case "revoked":
				s.Sources = nil
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
