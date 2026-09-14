package behavior

import (
	"math"
	"reflect"
	"testing"

	"github.com/tushardhara/dream/core"
	"github.com/tushardhara/dream/simulator/dynamics"
)

func domainSituation(t *testing.T, domain core.RelationshipDomain, frame core.ID, kind Kind) DomainSituation {
	t.Helper()
	in := scopedInput(t, "b", kind)
	in.Scope.Topic = core.ID(domain)
	in.Scope.Class = actionScopeClass(in.Situation.Offers[0])
	for i := range in.Boundaries {
		in.Boundaries[i].Topic = in.Scope.Topic
		in.Boundaries[i].Class = in.Scope.Class
	}
	in.Situation.Contexts = nil
	in.Situation.RelationshipEvidence = []dynamics.Perceived{in.Situation.Observation.Event}
	return DomainSituation{Scoped: in, Focus: core.RelationshipFocus{Version: core.RelationshipFocusVersion, Domain: domain, RoleContext: frame}}
}
func addDomainAccount(t *testing.T, in *DomainSituation, account core.ID, domain core.RelationshipDomain, frame core.ID, trust float64) {
	t.Helper()
	legacy := relationAccount("sibling", trust, .2, .1)
	r, e := core.BindRelationshipDomain(legacy, account, domain, frame, "e", legacy.Measures)
	if e != nil {
		t.Fatal(e)
	}
	in.Accounts = append(in.Accounts, r)
	meta := in.Scoped.Situation.Observation.Event
	meta.Event = account
	meta.Rights.Resource = account
	meta.Confidence = 1
	in.Scoped.Situation.Sources = append(in.Scoped.Situation.Sources, account)
	in.Scoped.Situation.RelationshipEvidence = append(in.Scoped.Situation.RelationshipEvidence, meta)
}
func laterDomainEvent(in DomainSituation, id core.ID, at core.LogicalTime) DomainSituation {
	in = domainCopy(in)
	event := &in.Scoped.Situation.Observation.Event
	event.Event = id
	event.Rights.Resource = id
	event.OccurredAt = at
	event.LearnedAt = at
	in.Scoped.Situation.Sources = append(in.Scoped.Situation.Sources, id)
	return in
}
func TestDomainAndRoleContextsDriveActualHumanChoices(t *testing.T) {
	a, _ := NewDomainActor("a", 0)
	in := domainSituation(t, core.Childcare, "family", Help)
	addDomainAccount(t, &in, "family-care", core.Childcare, "family", .8)
	addDomainAccount(t, &in, "business-care", core.Childcare, "business", -.8)
	_, family, e := ChooseDomainAction(a, in, 1, math.MaxUint64)
	if e != nil || family.Selection != "selected" {
		t.Fatal(e)
	}
	business := domainCopy(in)
	business.Focus.RoleContext = "business"
	_, work, e := ChooseDomainAction(a, business, 1, math.MaxUint64)
	if e != nil || reflect.DeepEqual(family.Human.Human.Candidates, work.Human.Human.Candidates) {
		t.Fatal("role-context input ignored", e)
	}
	for _, role := range []core.ID{"spouse", "friend", "sibling", "manager"} {
		changed := domainCopy(in)
		for i := range changed.Accounts {
			changed.Accounts[i].Types = []core.ID{role}
		}
		_, d, e := ChooseDomainAction(a, changed, 1, math.MaxUint64)
		if e != nil || !reflect.DeepEqual(family.Human.Human.Candidates, d.Human.Human.Candidates) {
			t.Fatal("type label assigned numeric trust", role, e)
		}
	}
	in.Focus.RoleContext = ""
	_, d, e := ChooseDomainAction(a, in, 1, math.MaxUint64)
	if e != nil || d.Selection != "ambiguous" || len(d.Human.Human.Candidates) != 1 {
		t.Fatal("ambiguous context facilitated", d, e)
	}
}
func TestLearnedChildcareTrustDoesNotTransferToFinancesOrSecrets(t *testing.T) {
	a, _ := NewDomainActor("a", 0)
	care := domainSituation(t, core.Childcare, "family", Help)
	addDomainAccount(t, &care, "care", core.Childcare, "family", .1)
	addDomainAccount(t, &care, "finance", core.Finances, "family", .1)
	before, d, e := ChooseDomainAction(a, care, 1, math.MaxUint64)
	if e != nil || d.Human.Human.Candidates[d.Human.Human.Selected].Offer.Kind != Help {
		t.Fatal("actual help control", d, e)
	}
	response := care.Scoped.Situation.Observation.Event
	response.Event = "childcare-response"
	response.Rights.Resource = response.Event
	response.OccurredAt = 3
	response.LearnedAt = 4
	response.Confidence = 1
	learned, e := ResolveDomainAction(before, d.Human.Human.ID, response, care.Scoped.Situation.RelationshipEvidence, "b", "supportive")
	if e != nil || len(learned.Memories) != 1 || learned.Memories[0].Domain != core.Childcare || learned.Memories[0].Memory[0].Trust <= 0 {
		t.Fatal("observed childcare learning missing", e)
	}
	next := laterDomainEvent(care, "next", 5)
	next.Scoped.Situation.Sources = append(next.Scoped.Situation.Sources, response.Event)
	next.Scoped.Situation.RelationshipEvidence = append(next.Scoped.Situation.RelationshipEvidence, response)
	_, plain, e := ChooseDomainAction(before, next, 5, math.MaxUint64)
	if e != nil {
		t.Fatal(e)
	}
	_, changed, e := ChooseDomainAction(learned, next, 5, math.MaxUint64)
	if e != nil || reflect.DeepEqual(plain.Human.Human.Candidates, changed.Human.Human.Candidates) {
		t.Fatal("domain learning ignored by consumer", e)
	}
	finance := domainCopy(next)
	finance.Focus.Domain = core.Finances
	finance.Scoped.Scope.Topic = core.ID(core.Finances)
	for i := range finance.Scoped.Boundaries {
		finance.Scoped.Boundaries[i].Topic = finance.Scoped.Scope.Topic
	}
	_, base, e := ChooseDomainAction(before, finance, 5, math.MaxUint64)
	if e != nil || base.Selection != "selected" || len(base.Human.Human.Candidates) < 2 {
		t.Fatal("finance control missing", e)
	}
	_, after, e := ChooseDomainAction(learned, finance, 5, math.MaxUint64)
	if e != nil || !reflect.DeepEqual(base.Human.Human.Candidates, after.Human.Human.Candidates) {
		t.Fatal("childcare learning leaked into financial trust", e)
	}
	// Explicit disclosure data rights and interpersonal willingness do not turn a
	// childcare account into a confidentiality account.
	secret := domainSituation(t, core.Confidentiality, "family", Reveal)
	addDomainAccount(t, &secret, "only-care", core.Childcare, "family", .9)
	secret.Scoped.Situation.Disclosure = &DisclosureGrant{Recipient: "b", Mode: Full, Sources: []core.ID{"e"}}
	_, private, e := ChooseDomainAction(a, secret, 1, math.MaxUint64)
	if e != nil || len(private.Human.Human.Candidates) != 1 {
		t.Fatal("childcare trust granted private disclosure", private, e)
	}
	if _, e := ResolveDomainAction(learned, d.Human.Human.ID, response, care.Scoped.Situation.RelationshipEvidence, "b", "supportive"); e == nil {
		t.Fatal("response learned twice")
	}
	encoded, e := learned.Encode()
	if e != nil {
		t.Fatal(e)
	}
	restored, e := DecodeDomainActor(encoded)
	if e != nil || !reflect.DeepEqual(learned, restored) {
		t.Fatal("domain learned-state codec", e)
	}
	_, replay, e := ChooseDomainAction(restored, next, 5, math.MaxUint64)
	if e != nil || !reflect.DeepEqual(changed, replay) {
		t.Fatal("domain mechanical replay", e)
	}
}
func TestDomainEvidenceRevocationUnknownAndVersionFailClosed(t *testing.T) {
	a, _ := NewDomainActor("a", 0)
	in := domainSituation(t, core.Childcare, "family", Help)
	addDomainAccount(t, &in, "care", core.Childcare, "family", .5)
	for _, change := range []string{"missing_account", "revoked_frame", "future_account", "unscoped_memory", "foreign", "wrong_topic"} {
		t.Run(change, func(t *testing.T) {
			input := domainCopy(in)
			actor := domainCopy(a)
			switch change {
			case "missing_account":
				input.Scoped.Situation.RelationshipEvidence = input.Scoped.Situation.RelationshipEvidence[:1]
			case "revoked_frame":
				input.Scoped.Situation.RelationshipEvidence[0].Rights.Revoked = true
			case "future_account":
				input.Scoped.Situation.RelationshipEvidence[1].LearnedAt = 2
			case "unscoped_memory":
				actor.Human.Human.Memory = []Memory{{Other: "b", Trust: .9, Evidence: []core.ID{"e"}}}
			case "foreign":
				input.Accounts[0].Observer = "b"
				input.Accounts[0].Other = "a"
			case "wrong_topic":
				input.Scoped.Scope.Topic = "finances"
			}
			if _, _, e := ChooseDomainAction(actor, input, 1, math.MaxUint64); e == nil {
				t.Fatal("invalid domain authority consumed", change)
			}
		})
	}
	chosen, d, e := ChooseDomainAction(a, in, 1, math.MaxUint64)
	if e != nil {
		t.Fatal(e)
	}
	response := in.Scoped.Situation.Observation.Event
	response.Event = "response"
	response.Rights.Resource = response.Event
	response.OccurredAt = 3
	response.LearnedAt = 4
	current := domainCopy(in.Scoped.Situation.RelationshipEvidence)
	current[1].Rights.Revoked = true
	if _, e := ResolveDomainAction(chosen, d.Human.Human.ID, response, current, "b", "supportive"); e == nil {
		t.Fatal("revoked account authorized learning")
	}
	if _, e := DecodeDomainActor([]byte(`{"Version":"future"}`)); e == nil {
		t.Fatal("unknown domain actor version")
	}
	if _, e := ApplyRelationship(in.Scoped.Situation, in.Accounts[0], "a", 1, true); e == nil {
		t.Fatal("legacy adapter silently ignored domain")
	}
}

func TestLearnedDomainMemoryRequiresCurrentEvidenceAndSameFrame(t *testing.T) {
	a, _ := NewDomainActor("a", 0)
	in := domainSituation(t, core.Childcare, "family", Help)
	addDomainAccount(t, &in, "family", core.Childcare, "family", .2)
	addDomainAccount(t, &in, "business", core.Childcare, "business", .2)
	before, d, e := ChooseDomainAction(a, in, 1, math.MaxUint64)
	if e != nil {
		t.Fatal(e)
	}
	response := in.Scoped.Situation.Observation.Event
	response.Event = "response"
	response.Rights.Resource = response.Event
	response.OccurredAt = 3
	response.LearnedAt = 4
	learned, e := ResolveDomainAction(before, d.Human.Human.ID, response, in.Scoped.Situation.RelationshipEvidence, "b", "supportive")
	if e != nil {
		t.Fatal(e)
	}
	next := laterDomainEvent(in, "next", 5)
	next.Scoped.Situation.Sources = append(next.Scoped.Situation.Sources, response.Event)
	next.Scoped.Situation.RelationshipEvidence = append(next.Scoped.Situation.RelationshipEvidence, response)
	for _, revoke := range []string{"e", "family", "response"} {
		bad := domainCopy(next)
		for i := range bad.Scoped.Situation.RelationshipEvidence {
			if string(bad.Scoped.Situation.RelationshipEvidence[i].Event) == revoke {
				bad.Scoped.Situation.RelationshipEvidence[i].Rights.Revoked = true
			}
		}
		if _, _, e := ChooseDomainAction(learned, bad, 5, math.MaxUint64); e == nil {
			t.Fatal("retained learning ignored revocation", revoke)
		}
	}
	business := domainCopy(next)
	business.Focus.RoleContext = "business"
	_, baseline, e := ChooseDomainAction(before, business, 5, math.MaxUint64)
	if e != nil {
		t.Fatal(e)
	}
	_, actual, e := ChooseDomainAction(learned, business, 5, math.MaxUint64)
	if e != nil || !reflect.DeepEqual(baseline.Human.Human.Candidates, actual.Human.Human.Candidates) {
		t.Fatal("learning crossed role frame", e)
	}
}
