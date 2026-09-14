package behavior

import (
	"math"
	"reflect"
	"testing"
	"time"

	"github.com/tushardhara/dream/core"
)

func scopedPreferences(target core.ID) []core.Boundary {
	records := []core.Boundary{}
	for _, owner := range []core.ID{"a", target} {
		other := core.ID("a")
		if owner == "a" {
			other = target
		}
		id := core.ID(string(owner) + "-" + string(other))
		records = append(records, core.Boundary{Version: core.BoundaryVersion, Meta: core.Metadata{ID: id, Observer: owner, Source: owner, Sensitivity: core.Restricted, Supporting: []core.ID{"synthetic"}, Confidence: 1, Valid: core.Interval{Start: 0}, RecordedAt: time.Unix(1, 0).UTC(), Rights: core.Rights{Resource: id}}, Principal: owner, With: other, Topic: "money", Class: core.Discussion, Decision: core.Willing, Basis: "self_report", OccurredAt: 0, LearnedAt: 0})
	}
	return records
}
func scopedInput(t *testing.T, target core.ID, kind Kind) ScopedSituation {
	t.Helper()
	_, s := actionFixture(t)
	def, _ := Definition(kind)
	o := offerFixture(def)
	if o.Recipient != "" {
		o.Recipient = target
	}
	s.Offers = []ActionOffer{o}
	return ScopedSituation{Scope: core.InteractionScope{Version: core.InteractionScopeVersion, Initiator: "a", Target: target, Topic: "money", Class: core.Discussion}, Situation: s, Boundaries: scopedPreferences(target)}
}
func TestScopedLeavePreservesUnrelatedConversationAndReplay(t *testing.T) {
	a, e := NewScopedActor("a", 0)
	if e != nil {
		t.Fatal(e)
	}
	in := scopedInput(t, "b", Leave)
	left, d, e := ChooseScopedAction(a, in, 1, math.MaxUint64)
	if e != nil || d.Human.Candidates[d.Human.Selected].Offer.Kind != Leave || left.Human.Contact != "engaged" || len(left.Contacts) != 1 || left.Contacts[0].With != "b" || left.Contacts[0].State != "left" {
		t.Fatal("leave not scoped", left, d, e)
	}
	encoded, e := left.Encode()
	if e != nil {
		t.Fatal(e)
	}
	restored, e := DecodeScopedActor(encoded)
	if e != nil {
		t.Fatal(e)
	}
	sibling := scopedInput(t, "c", Say)
	sibling.Situation.Observation.Event.Event = "sibling"
	sibling.Situation.Observation.Event.Rights.Resource = "sibling"
	sibling.Situation.Sources = append(sibling.Situation.Sources, "sibling")
	next, d, e := ChooseScopedAction(restored, sibling, 4, math.MaxUint64)
	if e != nil || d.Human.Candidates[d.Human.Selected].Offer.Kind != Say {
		t.Fatal("unrelated safe conversation disabled", d, e)
	}
	replay, again, e := ChooseScopedAction(left, sibling, 4, math.MaxUint64)
	if e != nil || !reflect.DeepEqual(next, replay) || !reflect.DeepEqual(d, again) {
		t.Fatal("scoped replay changed", e)
	}
	original := scopedInput(t, "b", Say)
	original.Situation.Observation.Event = sibling.Situation.Observation.Event
	original.Situation.Sources = append(original.Situation.Sources, "sibling")
	_, d, e = ChooseScopedAction(left, original, 4, math.MaxUint64)
	if e != nil || len(d.Human.Candidates) != 1 {
		t.Fatal("left relationship silently reopened", d, e)
	}
}
func TestScopedNoContactBeatsReconnectUtilityAndClassRelabelling(t *testing.T) {
	a, _ := NewScopedActor("a", 0)
	a.Contacts = []ScopedContact{{With: "b", Topic: core.AllTopics, State: "left", At: 0, Evidence: "ending"}}
	in := scopedInput(t, "b", Reconnect)
	_, positive, e := ChooseScopedAction(a, in, 1, math.MaxUint64)
	if e != nil || positive.Human.Candidates[positive.Human.Selected].Offer.Kind != Reconnect {
		t.Fatal("missing beneficial reconnect control", e)
	}
	refused := in.Boundaries[1]
	refused.Meta.ID = "no-contact"
	refused.Meta.Rights.Resource = refused.Meta.ID
	refused.Decision = core.Ended
	refused.Class = core.AllInteractions
	refused.Topic = core.AllTopics
	in.Boundaries = append(in.Boundaries, refused)
	next, d, e := ChooseScopedAction(a, in, 1, math.MaxUint64)
	if e != nil || len(d.Human.Candidates) != 1 || next.Contacts[0].State != "left" {
		t.Fatal("utility overrode no-contact", d, e)
	}
	a, _ = NewScopedActor("a", 0)
	for _, kind := range []Kind{SeekThirdPartySupport, Coordinate, Ask} {
		in = scopedInput(t, "b", kind)
		_, d, e = ChooseScopedAction(a, in, 1, math.MaxUint64)
		if e != nil || len(d.Human.Candidates) != 1 {
			t.Fatal("action class relabelled", kind, d, e)
		}
	}
}
func TestScopedMissingBoundaryWaitAndUnilateralDistance(t *testing.T) {
	for _, kind := range []Kind{Say, Leave, Withdraw} {
		a, _ := NewScopedActor("a", 0)
		in := scopedInput(t, "b", kind)
		in.Boundaries = nil
		_, d, e := ChooseScopedAction(a, in, 1, math.MaxUint64)
		if e != nil {
			t.Fatal(e)
		}
		selected := d.Human.Candidates[d.Human.Selected].Offer
		if kind == Say && selected.Kind != Wait || kind != Say && (selected.Kind != kind || selected.Recipient != "") {
			t.Fatal("willingness or unilateral restraint", kind, d)
		}
	}
	a, _ := NewScopedActor("a", 0)
	a.Human.Contact = "left"
	if _, e := a.Encode(); e == nil {
		t.Fatal("ambiguous legacy contact silently migrated")
	}
	if _, e := DecodeScopedActor([]byte(`{"Version":"future"}`)); e == nil {
		t.Fatal("unknown version")
	}
}

func TestScopedWithdrawalRequiresExplicitReconnect(t *testing.T) {
	a, _ := NewScopedActor("a", 0)
	in := scopedInput(t, "b", Withdraw)
	paused, d, e := ChooseScopedAction(a, in, 1, math.MaxUint64)
	if e != nil || d.Human.Candidates[d.Human.Selected].Offer.Kind != Withdraw || paused.Contacts[0].State != "withdrawn" {
		t.Fatal("withdraw positive control", d, e)
	}
	for _, step := range []struct {
		target core.ID
		kind   Kind
		want   Kind
	}{{"b", Say, Wait}, {"c", Say, Say}, {"b", Reconnect, Reconnect}} {
		in = scopedInput(t, step.target, step.kind)
		in.Situation.Observation.Event.Event = "after-withdraw"
		in.Situation.Observation.Event.Rights.Resource = "after-withdraw"
		in.Situation.Sources = append(in.Situation.Sources, "after-withdraw")
		_, d, e := ChooseScopedAction(paused, in, 4, math.MaxUint64)
		if e != nil || d.Human.Candidates[d.Human.Selected].Offer.Kind != step.want {
			t.Fatal("withdrawal bypass or unrelated/reconnect control", step, d, e)
		}
	}
}

func TestScopedReconnectRestoresOnlyMatchingTopicContact(t *testing.T) {
	a, _ := NewScopedActor("a", 0)
	a.Contacts = []ScopedContact{{With: "b", Topic: "money", State: "left", At: 0, Evidence: "money-ending"}, {With: "b", Topic: "family", State: "withdrawn", At: 0, Evidence: "family-pause"}}
	in := scopedInput(t, "b", Reconnect)
	next, d, e := ChooseScopedAction(a, in, 1, math.MaxUint64)
	if e != nil || d.Human.Candidates[d.Human.Selected].Offer.Kind != Reconnect || scopedContact(next, in.Scope) != "engaged" {
		t.Fatal("explicit topic reconnection ineffective", d, e)
	}
	family := in.Scope
	family.Topic = "family"
	if scopedContact(next, family) != "withdrawn" {
		t.Fatal("topic reconnection erased another topic pause")
	}
}
