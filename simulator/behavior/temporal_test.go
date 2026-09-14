package behavior

import (
	"fmt"
	"math"
	"reflect"
	"testing"

	"github.com/tushardhara/dream/core"
)

func temporalInput(t *testing.T, at, upper core.LogicalTime, signal string, confirmed core.LogicalTime) TemporalSituation {
	t.Helper()
	domain := domainSituation(t, core.Childcare, "family", Ask)
	addDomainAccount(t, &domain, "care", core.Childcare, "family", .5)
	domain = laterDomainEvent(domain, core.ID(fmt.Sprintf("temporal-event-%d", at)), at)
	domain.Scoped.Situation.Horizon = at + 3
	proof := domain.Scoped.Situation.Observation.Event
	proof.Event = "temporal-source"
	proof.Rights.Resource = proof.Event
	proof.Confidence = 1
	domain.Scoped.Situation.Sources = append(domain.Scoped.Situation.Sources, proof.Event)
	domain.Scoped.Situation.RelationshipEvidence = append(domain.Scoped.Situation.RelationshipEvidence, proof)
	preference := core.TemporalFact{Version: core.TemporalFactVersion, Account: "rhythm", Observer: "a", Person: "a", With: "b", Channel: "chat", Source: "temporal-source", Kind: "expectation", Basis: "self_report", FreshFor: 365, MinGap: 1, MaxGap: upper}
	diary := core.TemporalFact{Version: core.TemporalFactVersion, Account: "contacts", Observer: "a", Person: "a", With: "b", Channel: "chat", Source: "temporal-source", Kind: "contacts", Basis: "observed", Observations: []core.LogicalTime{0, 1, 2, 3}, ObservedThrough: at, CompleteChannel: true}
	facts := []core.TemporalFact{preference, diary}
	if signal != "" {
		facts = append(facts, core.TemporalFact{Version: core.TemporalFactVersion, Account: "life", Observer: "a", Person: "a", With: "b", Channel: "chat", Source: "temporal-source", Kind: "circumstance", Basis: "self_report", Category: "responsibility", Signal: signal, ConfirmedAt: confirmed, FreshFor: 30})
	}
	evidence := []core.TemporalEvidence{}
	reporters := map[core.ID]core.ID{"temporal-source": "a"}
	for _, fact := range facts {
		reporters[fact.Account] = "a"
		meta := domain.Scoped.Situation.Observation.Event
		meta.Event = fact.Account
		meta.Rights.Resource = fact.Account
		meta.Confidence = 1
		domain.Scoped.Situation.Sources = append(domain.Scoped.Situation.Sources, fact.Account)
		domain.Scoped.Situation.RelationshipEvidence = append(domain.Scoped.Situation.RelationshipEvidence, meta)
		evidence = append(evidence, core.TemporalEvidence{Reporter: "a", SourceReporter: "a", SourceOccurredAt: at, SourceLearnedAt: at, Fact: fact, OccurredAt: at, LearnedAt: at, Confidence: 1, SourceConfidence: 1})
	}
	return TemporalSituation{Reporters: reporters, Domain: domain, Focus: core.TemporalFocus{Version: core.TemporalFocusVersion, Observer: "a", Other: "b", Channel: "chat"}, Evidence: evidence}
}
func TestTemporalHumanInputsChangeActualChoicesAndKeepWait(t *testing.T) {
	actor, _ := NewTemporalActor("a", 0)
	for _, name := range []string{"daily_2", "daily_60", "occasional_60", "busy", "changed", "stale", "sparse", "no_contact"} {
		at, upper := core.LogicalTime(63), core.LogicalTime(3)
		signal := ""
		confirmed := core.LogicalTime(60)
		switch name {
		case "daily_2":
			at = 5
		case "occasional_60":
			upper = 90
		case "busy":
			signal = "busy"
		case "changed":
			signal = "routine_changed"
			upper = 90
		case "stale":
			signal = "available"
			confirmed = 1
			upper = 90
		}
		input := temporalInput(t, at, upper, signal, confirmed)
		if name == "sparse" {
			input.Evidence[1].Fact.CompleteChannel = false
		}
		if name == "no_contact" {
			input.Domain.Scoped.Boundaries[1].Decision = core.Ended
		}
		_, decision, e := ChooseTemporalAction(actor, input, at, math.MaxUint64)
		want := name == "daily_60" || name == "changed" || name == "stale"
		if e != nil {
			t.Fatal(name, e)
		}
		actual := decision.Human.Human.Human.Candidates[decision.Human.Human.Human.Selected].Offer.Kind
		if (actual == Ask) != want {
			t.Fatal("temporal input ignored by action consumer", name, decision)
		}
		_, wait, e := ChooseTemporalAction(actor, input, at, 0)
		if e != nil || wait.Human.Human.Human.Candidates[wait.Human.Human.Human.Selected].Offer.Kind != Wait {
			t.Fatal("WAIT unreachable", e)
		}
	}
}
func TestTemporalHumanClarificationBudgetAndCodec(t *testing.T) {
	actor, _ := NewTemporalActor("a", 0)
	for i, at := range []core.LogicalTime{63, 64, 73, 83} {
		in := temporalInput(t, at, 3, "", 0)
		next, d, e := ChooseTemporalAction(actor, in, at, math.MaxUint64)
		if e != nil {
			t.Fatal(e)
		}
		ask := d.Human.Human.Human.Candidates[d.Human.Human.Human.Selected].Offer.Kind == Ask
		if ask != (i == 0 || i == 2) {
			t.Fatal("clarification burden not bounded", i, ask)
		}
		raw, e := next.Encode()
		if e != nil {
			t.Fatal(e)
		}
		restored, e := DecodeTemporalActor(raw)
		if e != nil || !reflect.DeepEqual(next, restored) {
			t.Fatal("temporal codec", e)
		}
		replay, again, e := ChooseTemporalAction(actor, in, at, math.MaxUint64)
		if e != nil || !reflect.DeepEqual(replay, next) || !reflect.DeepEqual(again, d) {
			t.Fatal("temporal mechanical replay", e)
		}
		actor = restored
	}
	if _, e := DecodeTemporalActor([]byte(`{"Version":"future"}`)); e == nil {
		t.Fatal("future codec")
	}
}
func TestTemporalHumanRequiresCurrentRecordAndSourceMetadata(t *testing.T) {
	actor, _ := NewTemporalActor("a", 0)
	for _, change := range []string{"revoked_source", "revoked_record", "forged_confidence", "foreign", "future"} {
		in := temporalInput(t, 63, 3, "", 0)
		switch change {
		case "revoked_source":
			in.Domain.Scoped.Situation.RelationshipEvidence[0].Rights.Revoked = true
		case "revoked_record":
			in.Domain.Scoped.Situation.RelationshipEvidence[2].Rights.Revoked = true
		case "forged_confidence":
			in.Evidence[0].Confidence = .9
		case "foreign":
			in.Evidence[0].Fact.Observer = "b"
		case "future":
			in.Evidence[0].LearnedAt = 64
		}
		if _, _, e := ChooseTemporalAction(actor, in, 63, math.MaxUint64); e == nil {
			t.Fatal("invalid temporal authority consumed", change)
		}
	}
}
