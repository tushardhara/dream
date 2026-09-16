package core

import (
	"encoding/json"
	"testing"
	"time"
)

func outcomeObservation(id, observer, other ID, at LogicalTime, appraisal string) OutcomeObservation {
	benefit, burden := .5, 0.0
	if appraisal == "dismissive" {
		benefit = -.5
		burden = .7
	}
	return OutcomeObservation{Version: OutcomeObservationVersion, Meta: Metadata{ID: id, Observer: observer, Source: observer, Sensitivity: Restricted, Confidence: .8, Supporting: []ID{"own-observation"}, RecordedAt: time.Unix(int64(at)+1, 0).UTC(), Rights: Rights{Resource: id, Grants: []Grant{{Actor: observer, Recipient: observer, Purpose: "simulation", Operation: Derive}}}}, Interaction: "interaction", Action: "action", Reply: "reply", Participant: observer, Other: other, Focus: RelationshipFocus{Version: RelationshipFocusVersion, Domain: PracticalCoordination, RoleContext: "everyday"}, Kind: "observed", Position: "recipient", Phase: "immediate", Basis: "self_report", OccurredAt: at, LearnedAt: at, Status: Observed, Appraisal: appraisal, Benefit: &benefit, Burden: &burden}
}

func TestUnresolvedOutcomeCannotAssertBenefitOrBurden(t *testing.T) {
	benefit, burden, zero := .4, .1, 0.0
	for _, tc := range []struct {
		name            string
		benefit, burden *float64
		valid           bool
	}{
		{name: "no welfare assertion", valid: true},
		{name: "benefit only", benefit: &benefit},
		{name: "burden only", burden: &burden},
		{name: "both dimensions", benefit: &benefit, burden: &burden},
		{name: "zero benefit is still an assertion", benefit: &zero},
		{name: "zero burden is still an assertion", burden: &zero},
	} {
		t.Run(tc.name, func(t *testing.T) {
			o := outcomeObservation("unresolved", "alice", "bob", 2, "unresolved")
			o.Benefit, o.Burden = tc.benefit, tc.burden
			if err := o.Validate(); (err == nil) != tc.valid {
				t.Fatalf("observed unresolved outcome: Validate() = %v, want valid=%t", err, tc.valid)
			}
		})
	}
}

func TestOutcomeAccountsCorrectionAndHistoricalProjection(t *testing.T) {
	sender := outcomeObservation("sender", "alice", "bob", 2, "supportive")
	sender.Kind = "expected"
	sender.Position = "sender"
	recipient := outcomeObservation("recipient", "bob", "alice", 3, "dismissive")
	log := []OutcomeObservation{sender, recipient}
	before, _ := json.Marshal(log)
	corrected := outcomeObservation("corrected", "bob", "alice", 5, "neutral")
	corrected.Supersedes = recipient.Meta.ID
	next, e := AppendOutcome(log, corrected)
	if e != nil {
		t.Fatal(e)
	}
	after, _ := json.Marshal(log)
	if string(before) != string(after) {
		t.Fatal("append rewrote history")
	}
	grant := Grant{Actor: "bob", Recipient: "bob", Purpose: "simulation", Operation: Derive}
	old, e := CurrentOutcomes(next, "bob", recipient.Focus, 4, grant)
	if e != nil || len(old) != 1 || old[0].Appraisal != "dismissive" {
		t.Fatal("historical outcome changed", old, e)
	}
	current, e := CurrentOutcomes(next, "bob", recipient.Focus, 5, grant)
	if e != nil || len(current) != 1 || current[0].Appraisal != "neutral" {
		t.Fatal("correction not applied", current, e)
	}
	alice, e := CurrentOutcomes(next, "alice", sender.Focus, 5, Grant{Actor: "alice", Recipient: "alice", Purpose: "simulation", Operation: Derive})
	if e != nil || len(alice) != 1 || alice[0].Kind != "expected" || alice[0].Appraisal != "supportive" {
		t.Fatal("perspectives collapsed", alice, e)
	}
	next[2].Meta.Rights.Revoked = true
	current, e = CurrentOutcomes(next, "bob", recipient.Focus, 5, grant)
	if e != nil || len(current) != 0 {
		t.Fatal("revoked correction resurrected earlier claim", current, e)
	}
	// Returned pointers and metadata are detached from the immutable log.
	*old[0].Benefit = 1
	if *next[1].Benefit == 1 {
		t.Fatal("projection aliases history")
	}
}
func TestOutcomeMissingAndCorrectionIdentity(t *testing.T) {
	original := outcomeObservation("own", "alice", "bob", 2, "supportive")
	for _, missing := range []string{"silence", "lost_observation", "declined_participation", "missing_followup"} {
		o := original
		o.Status = Censored
		o.Basis = "unobserved"
		o.Missing = missing
		o.Appraisal = ""
		o.Benefit = nil
		o.Burden = nil
		if o.Validate() != nil {
			t.Fatal(missing)
		}
		o.Appraisal = "supportive"
		if o.Validate() == nil {
			t.Fatal("missing became success")
		}
	}
	for _, change := range []string{"observer", "participant", "phase", "action", "kind", "domain", "frame", "future_parent"} {
		o := outcomeObservation("new", "alice", "bob", 3, "dismissive")
		o.Supersedes = original.Meta.ID
		switch change {
		case "observer":
			o.Meta.Observer = "bob"
		case "participant":
			o.Participant = "carol"
		case "phase":
			o.Phase = "later"
		case "action":
			o.Action = "different"
		case "kind":
			o.Kind = "expected"
		case "domain":
			o.Focus.Domain = Finances
		case "frame":
			o.Focus.RoleContext = "business"
		case "future_parent":
			o.Supersedes = "absent"
		}
		if _, e := AppendOutcome([]OutcomeObservation{original}, o); e == nil {
			t.Fatal("correction changed identity", change)
		}
	}
	missing := original
	missing.Status = Unknown
	missing.Basis = "unobserved"
	missing.Missing = "lost_observation"
	missing.Appraisal = ""
	missing.Benefit = nil
	missing.Burden = nil
	resolved := outcomeObservation("resolved", "alice", "bob", 4, "mixed")
	resolved.Supersedes = missing.Meta.ID
	if _, e := AppendOutcome([]OutcomeObservation{missing}, resolved); e != nil {
		t.Fatal("later explicit report cannot resolve unknown", e)
	}
	foreign := original
	foreign.Meta.Source = "bob"
	if foreign.Validate() == nil {
		t.Fatal("another person invented self-report")
	}
	future := original
	future.Version = "future"
	if future.Validate() == nil {
		t.Fatal("future version")
	}
}

// #81, part 3: core/outcome_observation.go carries 24 guard messages and none is
// quoted by any test. These decide whether a later account is treated as
// somebody's own report, somebody else's report about them, or an absence — the
// distinction the whole delayed-outcome evaluation rests on, since #58's
// comparisons are only as honest as the refusal to invent an observation.
//
// The existing tests here assert Validate() != nil, so a mutation tripping a
// neighbouring guard passes them unchanged. The positive control runs first.
func TestOutcomeObservationGuardReachability(t *testing.T) {
	base := outcomeObservation("o1", "alice", "bob", 5, "supportive")
	if e := base.Validate(); e != nil {
		t.Fatal("positive control rejected; every case below would be unattributable", e)
	}
	for _, c := range []struct {
		name, want string
		change     func(*OutcomeObservation)
	}{
		{"learned_before_it_happened", "invalid outcome observation", func(o *OutcomeObservation) { o.LearnedAt = 1 }},
		{"participant_is_also_the_other", "invalid outcome observation", func(o *OutcomeObservation) { o.Other = o.Participant }},
		{"reply_is_the_action_it_replies_to", "invalid outcome links", func(o *OutcomeObservation) { o.Reply = o.Action }},
		{"supersedes_itself", "invalid outcome links", func(o *OutcomeObservation) { o.Supersedes = o.Meta.ID }},
		{"unknown_position", "invalid outcome position", func(o *OutcomeObservation) { o.Position = "bystander" }},
		{"unknown_phase", "invalid outcome kind/phase", func(o *OutcomeObservation) { o.Phase = "eventually" }},
		{"unknown_kind", "invalid outcome kind/phase", func(o *OutcomeObservation) { o.Kind = "predicted" }},
		// The product-thesis guards: one person's account cannot be authored by
		// another, and a report about someone must name who it came from.
		{"self_report_authored_by_another", "foreign self-report", func(o *OutcomeObservation) { o.Participant = "carol" }},
		{"reported_without_a_reply_to_attribute_it", "unattributed reported outcome", func(o *OutcomeObservation) {
			o.Basis = "reported"
			o.Meta.Observer = o.Other
			o.Meta.Source = o.Participant
			o.Reply = ""
		}},
		{"unknown_basis", "invalid outcome basis", func(o *OutcomeObservation) { o.Basis = "inferred" }},
		{"unobserved_claiming_observation", "missing report asserts observation", func(o *OutcomeObservation) { o.Basis = "unobserved" }},
		// An absence must stay an absence.
		{"observed_without_evidence", "observed outcome needs evidence", func(o *OutcomeObservation) { o.Meta.Supporting = nil }},
		{"observed_without_dimensions", "observed dimensions missing", func(o *OutcomeObservation) { o.Benefit = nil; o.Burden = nil }},
		{"unresolved_still_asserting_benefit", "unresolved outcome asserts benefit", func(o *OutcomeObservation) { o.Appraisal = "unresolved" }},
		{"unknown_appraisal", "invalid appraisal", func(o *OutcomeObservation) { o.Appraisal = "lovely" }},
	} {
		t.Run(c.name, func(t *testing.T) {
			o := outcomeObservation("o1", "alice", "bob", 5, "supportive")
			c.change(&o)
			e := o.Validate()
			if e == nil {
				t.Fatal("mutated outcome accepted")
			}
			if e.Error() != c.want {
				t.Fatal("wrong guard reached: want "+c.want+", got", e)
			}
		})
	}
}
