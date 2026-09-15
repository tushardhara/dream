package behavior

import (
	"math"
	"reflect"
	"testing"

	"github.com/tushardhara/dream/core"
	"github.com/tushardhara/dream/simulator/dynamics"
)

func recipientFixture(t *testing.T, welcome float64) (ActionActor, RecipientInput) {
	t.Helper()
	a, e := NewActionActor("b", 0)
	if e != nil {
		t.Fatal(e)
	}
	p := func(id core.ID, at core.LogicalTime) dynamics.Perceived {
		return dynamics.Perceived{Event: id, Actor: "b", OccurredAt: at, LearnedAt: at, Confidence: 1, Rights: core.Rights{Resource: id, Grants: []core.Grant{{Actor: "b", Recipient: "b", Purpose: "simulation", Operation: core.Read}, {Actor: "b", Recipient: "b", Purpose: "simulation", Operation: core.Derive}}}}
	}
	return a, RecipientInput{Delivery: ReceivedAction{Interaction: "interaction", Decision: "invitation", Sender: "a", Recipient: "b", Kind: "invite", EffectAt: 1, Source: p("received-invitation", 2)}, Focus: core.RelationshipFocus{Version: core.RelationshipFocusVersion, Domain: core.PracticalCoordination, RoleContext: "everyday"}, Context: DisclosureContext{Observer: "b", Recipient: "a", RoleExpectation: ContextValue{Value: welcome, Confidence: 1, Evidence: "own-context"}, Trust: ContextValue{Value: .2, Confidence: 1, Evidence: "own-context"}}, Current: []dynamics.Perceived{p("own-context", 0)}, Availability: "observed", Phase: "immediate"}
}
func responseProof(o core.OutcomeObservation) dynamics.Perceived {
	return dynamics.Perceived{Event: o.Meta.ID, Actor: o.Meta.Observer, OccurredAt: o.OccurredAt, LearnedAt: o.LearnedAt, Confidence: o.Meta.Confidence, Rights: o.Meta.Rights}
}
func TestRecipientContextAndPrivateStateChangeAppraisalNotActionKind(t *testing.T) {
	a, in := recipientFixture(t, 1)
	good, e := RespondToAction(a, in, 2, math.MaxUint64/3)
	if e != nil || good.Observation.Appraisal != "supportive" {
		t.Fatal("welcomed control", good, e)
	}
	_, unwanted := recipientFixture(t, -1)
	bad, e := RespondToAction(a, unwanted, 2, math.MaxUint64/3)
	if e != nil || bad.Observation.Appraisal != "dismissive" {
		t.Fatal("unwanted invitation labelled success", bad, e)
	}
	for _, kind := range []core.ID{"help", "support", "decline", "say"} {
		changed := unwanted
		changed.Delivery.Kind = kind
		got, e := RespondToAction(a, changed, 2, math.MaxUint64/3)
		if e != nil || got.Observation.Appraisal != bad.Observation.Appraisal || got.Probabilities != bad.Probabilities {
			t.Fatal("kind replaced recipient context", kind, e)
		}
	}
	stressed := a
	stressed.Private.Fear = 1
	stressed.Private.Appraisal[0] = 1
	private, e := RespondToAction(stressed, in, 2, math.MaxUint64/3)
	if e != nil || private.Probabilities == good.Probabilities {
		t.Fatal("native private state ignored", e)
	}
	again, e := RespondToAction(a, in, 2, math.MaxUint64/3)
	if e != nil || !reflect.DeepEqual(again, good) {
		t.Fatal("recipient replay changed", e)
	}
	ownBefore := a
	_, e = RespondToAction(a, in, 2, 0)
	if e != nil || !reflect.DeepEqual(a, ownBefore) {
		t.Fatal("appraisal mutated native history")
	}
	seen := map[string]bool{}
	for i := uint64(0); i < 64; i++ {
		r, e := RespondToAction(a, in, 2, i*(math.MaxUint64/64))
		if e != nil {
			t.Fatal(e)
		}
		seen[r.Observation.Appraisal] = true
	}
	if len(seen) != 5 {
		t.Fatal("response categories unreachable", seen)
	}
}
func TestMissingResponseNeverAssertsBenefitAndUncertaintyIsExplicit(t *testing.T) {
	a, in := recipientFixture(t, 1)
	for _, reason := range []string{"silence", "lost_observation", "declined_participation", "missing_followup"} {
		in.Availability = reason
		r, e := RespondToAction(a, in, 2, 0)
		if e != nil || r.Observation.Status == core.Observed || r.Observation.Appraisal != "" || r.Observation.Benefit != nil || r.Observation.Burden != nil {
			t.Fatal("missing response became benefit", reason, e)
		}
	}
	in.Availability = "observed"
	in.Context = DisclosureContext{Observer: "b", Recipient: "a"}
	r, e := RespondToAction(a, in, 2, 0)
	if e != nil || r.Observation.Appraisal != "unresolved" || r.Observation.Benefit != nil {
		t.Fatal("unknown context invented positive response", r, e)
	}
}
func TestRecipientEvidenceAndObserverBoundaries(t *testing.T) {
	for _, kind := range []string{"foreign_context", "revoked_context", "revoked_delivery", "future", "missing_source"} {
		a, in := recipientFixture(t, 1)
		switch kind {
		case "foreign_context":
			in.Context.Observer = "a"
		case "revoked_context":
			in.Current[0].Rights.Revoked = true
		case "revoked_delivery":
			in.Delivery.Source.Rights.Revoked = true
		case "future":
			in.Delivery.Source.LearnedAt = 3
		case "missing_source":
			in.Current = nil
		}
		if _, e := RespondToAction(a, in, 2, 0); e == nil {
			t.Fatal("unavailable recipient input accepted", kind)
		}
	}
}
func TestOutcomeLearningCorrectsContributionsAndKeepsExpectedBenefitSeparate(t *testing.T) {
	a, in := recipientFixture(t, 1)
	d, e := RespondToAction(a, in, 2, math.MaxUint64/3)
	if e != nil {
		t.Fatal(e)
	}
	first := d.Observation
	log := []core.OutcomeObservation{first}
	proofs := append(in.Current, in.Delivery.Source, responseProof(first))
	memory, e := OutcomeLearning(log, "b", in.Focus, 2, proofs)
	if e != nil || len(memory) != 1 || memory[0].Trust <= 0 {
		t.Fatal("actual recipient evidence did not train", memory, e)
	}
	expected := first
	expected.Meta.ID = "expected"
	expected.Meta.Rights.Resource = "expected"
	expected.Kind = "expected"
	expected.Position = "sender"
	alone, e := OutcomeLearning([]core.OutcomeObservation{expected}, "b", in.Focus, 2, append(proofs, responseProof(expected)))
	if e != nil || len(alone) != 0 {
		t.Fatal("asserted benefit trained state", alone, e)
	}
	_, unwanted := recipientFixture(t, -1)
	unwanted.Current[0].Event = "new-context"
	unwanted.Current[0].Rights.Resource = "new-context"
	unwanted.Current[0].OccurredAt = 5
	unwanted.Current[0].LearnedAt = 5
	unwanted.Context.RoleExpectation.Evidence = "new-context"
	unwanted.Context.Trust.Evidence = "new-context"
	historicalProofs := append([]dynamics.Perceived{}, proofs...)
	proofs = append(proofs, unwanted.Current[0])
	correction, e := RespondToAction(a, unwanted, 5, math.MaxUint64/3)
	if e != nil {
		t.Fatal(e)
	}
	correction.Observation.Supersedes = first.Meta.ID
	log, e = core.AppendOutcome(log, correction.Observation)
	if e != nil {
		t.Fatal(e)
	}
	proofs = append(proofs, responseProof(correction.Observation))
	now, e := OutcomeLearning(log, "b", in.Focus, 5, proofs)
	if e != nil || len(now) != 1 || now[0].Trust >= 0 || len(now[0].Evidence) != 1 {
		t.Fatal("correction double-counted/reused old positive contribution", now, e)
	}
	old, e := OutcomeLearning(log, "b", in.Focus, 2, historicalProofs)
	if e != nil || !reflect.DeepEqual(old, memory) {
		t.Fatal("historical learning rewritten", old, e)
	}
	other := in.Focus
	other.Domain = core.Finances
	different, e := OutcomeLearning(log, "b", other, 5, proofs)
	if e != nil || len(different) != 0 {
		t.Fatal("response learning crossed domains", e)
	}
	log[1].Meta.Rights.Revoked = true
	denied, e := OutcomeLearning(log, "b", in.Focus, 5, proofs)
	if e != nil || len(denied) != 0 {
		t.Fatal("revocation resurrected positive assumption", denied, e)
	}
}

func TestDomainObservationConsumerRebuildsCorrectionWithoutCrossFrameTransfer(t *testing.T) {
	native, in := recipientFixture(t, 1)
	in.Focus.Account = "coordination-account"
	profile := core.RelationshipContext{Version: 2, Account: in.Focus.Account, Observer: "b", Other: "a", Domain: in.Focus.Domain, RoleContext: in.Focus.RoleContext, ContextSource: "own-context", Types: []core.ID{"knows"}, Measures: []core.RelationshipMeasure{{Kind: "expectation", Value: 1, Confidence: 1, Source: "own-context"}}}
	account := in.Current[0]
	account.Event = profile.Account
	account.Rights.Resource = profile.Account
	current := append(append([]dynamics.Perceived{}, in.Current...), account, in.Delivery.Source)
	first, e := RespondToAction(native, in, 2, math.MaxUint64/3)
	if e != nil {
		t.Fatal(e)
	}
	log := []core.OutcomeObservation{first.Observation}
	current = append(current, responseProof(first.Observation))
	actor, _ := NewDomainActor("b", 0)
	actor.Memories = []DomainMemory{{Domain: core.Finances, RoleContext: "business", Memory: []Memory{{Other: "a", Trust: .8, Evidence: []core.ID{"finance-proof"}}}}}
	originalFinance := domainCopy(actor.Memories[0])
	positive, e := LearnDomainObservations(actor, log, profile, 2, current)
	if e != nil {
		t.Fatal(e)
	}
	if got := domainMemory(positive, in.Focus); len(got) != 1 || got[0].Trust <= 0 {
		t.Fatal("domain consumer lost actual recipient learning", got)
	}
	later := domainCopy(in)
	later.Phase = "later"
	later.Delivery.Source.Event = "later-delivery"
	later.Delivery.Source.Rights.Resource = "later-delivery"
	later.Delivery.Source.OccurredAt = 4
	later.Delivery.Source.LearnedAt = 4
	laterResponse, e := RespondToAction(native, later, 4, math.MaxUint64/3)
	if e != nil {
		t.Fatal(e)
	}
	log, e = core.AppendOutcome(log, laterResponse.Observation)
	if e != nil {
		t.Fatal(e)
	}
	current = append(current, later.Delivery.Source, responseProof(laterResponse.Observation))
	revised := domainCopy(later)
	revised.Context.RoleExpectation.Value = -1
	revised.Context.RoleExpectation.Evidence = "reconsidered-context"
	revised.Context.Trust.Evidence = "reconsidered-context"
	source := revised.Current[0]
	source.Event = "reconsidered-context"
	source.Rights.Resource = source.Event
	source.OccurredAt = 5
	source.LearnedAt = 5
	revised.Current = []dynamics.Perceived{source}
	correction, e := RespondToAction(native, revised, 5, math.MaxUint64/3)
	if e != nil {
		t.Fatal(e)
	}
	correction.Observation.Supersedes = laterResponse.Observation.Meta.ID
	log, e = core.AppendOutcome(log, correction.Observation)
	if e != nil {
		t.Fatal(e)
	}
	current = append(current, source, responseProof(correction.Observation))
	negative, e := LearnDomainObservations(positive, log, profile, 5, current)
	if e != nil {
		t.Fatal(e)
	}
	got := domainMemory(negative, in.Focus)
	if len(got) != 1 || got[0].Trust >= 0 || len(got[0].Evidence) != 1 || got[0].Evidence[0] != correction.Observation.Meta.ID {
		t.Fatal("correction double-counted or failed to replace contribution", got)
	}
	if !reflect.DeepEqual(negative.Memories[0], originalFinance) {
		t.Fatal("recipient correction changed unrelated finance/business memory")
	}
	if len(negative.Human.Human.Memory) != 0 {
		t.Fatal("unscoped memory retained")
	}
	for i := range current {
		if current[i].Event == profile.Account {
			current[i].Rights.Revoked = true
		}
	}
	if _, e = LearnDomainObservations(negative, log, profile, 5, current); e == nil {
		t.Fatal("revoked domain account learned")
	}
}
