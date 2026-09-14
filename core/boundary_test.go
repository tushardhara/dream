package core

import (
	"testing"
	"time"
)

func boundaryFixture(id, owner, with ID, decision Willingness) Boundary {
	return Boundary{Version: BoundaryVersion, Meta: Metadata{ID: id, Observer: owner, Source: owner, Sensitivity: Restricted, Supporting: []ID{"source"}, Confidence: 1, Valid: Interval{Start: 0}, RecordedAt: time.Unix(1, 0).UTC(), Rights: Rights{Resource: id, Grants: []Grant{{Actor: owner, Recipient: owner, Purpose: "boundary", Operation: Read}}}}, Principal: owner, With: with, Topic: "money", Class: Discussion, Decision: decision, Basis: "self_report", OccurredAt: 1, LearnedAt: 1}
}
func boundaryScope() InteractionScope {
	return InteractionScope{Version: InteractionScopeVersion, Initiator: "alice", Target: "bob", Topic: "money", Class: Discussion}
}
func TestBoundaryConsentIsDirectionalScopedAndNotInferred(t *testing.T) {
	scope := boundaryScope()
	a := boundaryFixture("a", "alice", "bob", Willing)
	b := boundaryFixture("b", "bob", "alice", Willing)
	for name, records := range map[string][]Boundary{"missing": {a}, "both": {a, b}} {
		d, e := EvaluateBoundaries(records, scope, 2)
		if e != nil || d.Allowed != (name == "both") {
			t.Fatal(name, d, e)
		}
	}
	other := b
	other.Meta.Observer = "alice"
	other.Meta.Source = "alice"
	other.Basis = "hypothesis"
	if d, e := EvaluateBoundaries([]Boundary{a, other}, scope, 2); e != nil || d.Allowed {
		t.Fatal("another person's hypothesis created consent", d, e)
	}
	for _, change := range []string{"topic", "class", "target"} {
		q := scope
		switch change {
		case "topic":
			q.Topic = "family"
		case "class":
			q.Class = SummarySharing
		case "target":
			q.Target = "charlie"
		}
		if d, e := EvaluateBoundaries([]Boundary{a, b}, q, 2); e != nil || d.Allowed {
			t.Fatal("scope broadened", change, d, e)
		}
	}
	decline := boundaryFixture("decline", "bob", "alice", Declined)
	if d, e := EvaluateBoundaries([]Boundary{a, b, decline}, scope, 2); e != nil || d.Allowed {
		t.Fatal("affirmative score overwrote refusal", d, e)
	}
}
func TestBoundaryBreakExpiryCorrectionsRevocationAndRelay(t *testing.T) {
	scope := boundaryScope()
	a := boundaryFixture("a", "alice", "bob", Willing)
	b := boundaryFixture("b", "bob", "alice", Willing)
	pause := boundaryFixture("pause", "bob", "alice", TakingBreak)
	end := LogicalTime(5)
	pause.Meta.Valid.End = &end
	pause.OccurredAt = 2
	pause.LearnedAt = 2
	records := []Boundary{a, b, pause}
	for _, at := range []LogicalTime{3, 6} {
		if d, e := EvaluateBoundaries(records, scope, at); e != nil || d.Allowed {
			t.Fatal("pause or expiry manufactured consent", d, e)
		}
	}
	fresh := boundaryFixture("fresh", "bob", "alice", Willing)
	fresh.OccurredAt = 7
	fresh.LearnedAt = 7
	records = append(records, fresh)
	if d, e := EvaluateBoundaries(records, scope, 7); e != nil || !d.Allowed {
		t.Fatal("fresh explicit consent rejected", d, e)
	}
	records[3].Revoked = true
	records[3].RevokedAt = 8
	records[3].RevokedBy = "bob"
	if d, e := EvaluateBoundaries(records, scope, 7); e != nil || d.Allowed {
		t.Fatal("backdated replay revived revoked willingness", d, e)
	}
	corrected := boundaryFixture("corrected", "bob", "alice", Willing)
	corrected.LearnedAt = 3
	corrected.OccurredAt = 3
	corrected.Supersedes = []ID{"pause"}
	if d, e := EvaluateBoundaries([]Boundary{a, b, pause, corrected}, scope, 3); e != nil || !d.Allowed {
		t.Fatal("explicit correction ignored", d, e)
	}
	forged := corrected
	forged.Principal = "alice"
	forged.With = "bob"
	forged.Meta.Observer = "alice"
	forged.Meta.Source = "alice"
	if ValidateBoundaryLog([]Boundary{a, b, pause, forged}) == nil {
		t.Fatal("other participant superseded refusal")
	}
	scope.Via = "charlie"
	relay := boundaryFixture("relay", "charlie", "alice", Willing)
	if d, e := EvaluateBoundaries(append([]Boundary{a, b, pause}, relay), scope, 3); e != nil || d.Allowed {
		t.Fatal("relay bypassed original pair pause", d, e)
	}
}
func TestPressureDiffersFromDisagreementAndEndingIsNotBeneficialConsent(t *testing.T) {
	scope := boundaryScope()
	a := boundaryFixture("a", "alice", "bob", Willing)
	b := boundaryFixture("b", "bob", "alice", Willing)
	for _, signal := range []string{"disagreement", "uncertain_pressure", "credible_pressure", "credible_threat"} {
		risk := boundaryFixture("risk", "alice", "bob", PressureSignal)
		risk.Basis = "observed_signal"
		risk.Signal = signal
		d, e := EvaluateBoundaries([]Boundary{a, b, risk}, scope, 2)
		if e != nil || d.Allowed != (signal == "disagreement") {
			t.Fatal(signal, d, e)
		}
	}
	ending := boundaryFixture("ending", "bob", "alice", Ended)
	ending.Topic = AllTopics
	ending.Class = AllInteractions
	if d, e := EvaluateBoundaries([]Boundary{a, b, ending}, scope, 100); e != nil || d.Allowed {
		t.Fatal("ending expired or utility bypass", d, e)
	}
	d1 := boundaryFixture("d1", "bob", "alice", Dismissed)
	d2 := boundaryFixture("d2", "bob", "alice", Dismissed)
	if d, e := EvaluateBoundaries([]Boundary{a, b, d1, d2}, scope, 2); e != nil || d.Allowed || d.Code != "repeated_dismissal" {
		t.Fatal("repeated dismissal ignored", d, e)
	}
	raw, e := EncodeBoundaryLog([]Boundary{a, b, ending})
	if e != nil {
		t.Fatal(e)
	}
	decoded, e := DecodeBoundaryLog(raw)
	if e != nil || len(decoded) != 3 {
		t.Fatal("boundary replay", e)
	}
	if _, e := DecodeBoundaryLog([]byte(`{"Version":"future","Records":[]}`)); e == nil {
		t.Fatal("unknown boundary version")
	}
}

func TestRelayRequiresUnderlyingAndThirdPartyConsent(t *testing.T) {
	s := boundaryScope()
	s.Via = "charlie"
	s.Class = ThirdPartyInvolvement
	records := []Boundary{}
	for _, class := range []InteractionClass{Discussion, ThirdPartyInvolvement} {
		for _, who := range []ID{"alice", "bob", "charlie"} {
			with := ID("alice")
			if who == "alice" {
				with = "bob"
			}
			b := boundaryFixture(ID(string(who)+string(class)), who, with, Willing)
			b.Class = class
			records = append(records, b)
		}
	}
	if d, e := EvaluateBoundaries(records, s, 3); e != nil || !d.Allowed {
		t.Fatal("relay positive", d, e)
	}
	pause := boundaryFixture("pause", "bob", "alice", TakingBreak)
	end := LogicalTime(5)
	pause.Meta.Valid.End = &end
	if d, e := EvaluateBoundaries(append(records, pause), s, 3); e != nil || d.Allowed {
		t.Fatal("third-party action bypassed discussion pause", d, e)
	}
	if d, e := EvaluateBoundaries(records[3:], s, 3); e != nil || d.Allowed {
		t.Fatal("third-party-only consent broadened", d, e)
	}
}
func TestPressureRestrictsSharingAcrossClassesButNotPrivatePreparation(t *testing.T) {
	risk := boundaryFixture("risk", "alice", "bob", PressureSignal)
	risk.Basis = "observed_signal"
	risk.Signal = "credible_pressure"
	for _, class := range []InteractionClass{SummarySharing, PrivatePreparation} {
		s := boundaryScope()
		s.Class = class
		a := boundaryFixture("a", "alice", "bob", Willing)
		a.Class = class
		b := boundaryFixture("b", "bob", "alice", Willing)
		b.Class = class
		d, e := EvaluateBoundaries([]Boundary{a, b, risk}, s, 2)
		if e != nil || d.Allowed != (class == PrivatePreparation) {
			t.Fatal("cross-class pressure handling", class, d, e)
		}
	}
}
func TestRevokedMetadataAndHypothesisCorrectionCannotReviveConsent(t *testing.T) {
	a := boundaryFixture("a", "alice", "bob", Willing)
	b := boundaryFixture("b", "bob", "alice", Willing)
	fresh := boundaryFixture("fresh", "bob", "alice", Willing)
	fresh.LearnedAt = 2
	fresh.Meta.Rights.Revoked = true
	if d, e := EvaluateBoundaries([]Boundary{a, b, fresh}, boundaryScope(), 3); e != nil || d.Allowed {
		t.Fatal("metadata revocation revived old consent", d, e)
	}
	refusal := boundaryFixture("refusal", "bob", "alice", Declined)
	guess := boundaryFixture("guess", "bob", "alice", Willing)
	guess.Basis = "hypothesis"
	guess.Supersedes = []ID{"refusal"}
	if ValidateBoundaryLog([]Boundary{refusal, guess}) == nil {
		t.Fatal("hypothesis superseded self-report refusal")
	}
}

func TestBoundaryProvenanceBounds(t *testing.T) {
	b := boundaryFixture("bounded", "alice", "bob", Willing)
	for i := 0; i < 9; i++ {
		b.Meta.Parents = append(b.Meta.Parents, ID(string(rune('a'+i))))
	}
	if b.Validate() == nil {
		t.Fatal("unbounded policy provenance")
	}
}

func TestBoundarySelfReportOwnershipCannotBeForged(t *testing.T) {
	a := boundaryFixture("a", "alice", "bob", Willing)
	b := boundaryFixture("b", "bob", "alice", Willing)
	if d, e := EvaluateBoundaries([]Boundary{a, b}, boundaryScope(), 2); e != nil || !d.Allowed {
		t.Fatal("valid self-report control", d, e)
	}
	for _, fields := range []string{"observer", "source", "both"} {
		forged := a
		if fields == "observer" || fields == "both" {
			forged.Meta.Observer = "bob"
		}
		if fields == "source" || fields == "both" {
			forged.Meta.Source = "bob"
		}
		if forged.Meta.Validate() != nil {
			t.Fatal("probe metadata must otherwise be valid")
		}
		if forged.Validate() == nil {
			t.Fatal("forged self-report validated", fields)
		}
		if d, e := EvaluateBoundaries([]Boundary{forged, b}, boundaryScope(), 2); e == nil || d.Allowed {
			t.Fatal("third party manufactured willingness", fields, d, e)
		}
	}
}
func TestBoundaryBlanketWillingnessCannotBroadenScope(t *testing.T) {
	a := boundaryFixture("a", "alice", "bob", Willing)
	b := boundaryFixture("b", "bob", "alice", Willing)
	if d, e := EvaluateBoundaries([]Boundary{a, b}, boundaryScope(), 2); e != nil || !d.Allowed {
		t.Fatal("exact-scope positive control", d, e)
	}
	for _, fields := range []string{"topic", "class", "both"} {
		records := []Boundary{a, b}
		for i := range records {
			if fields == "topic" || fields == "both" {
				records[i].Topic = AllTopics
			}
			if fields == "class" || fields == "both" {
				records[i].Class = AllInteractions
			}
			if records[i].Meta.Validate() != nil {
				t.Fatal("probe metadata must otherwise be valid")
			}
			if records[i].Validate() == nil {
				t.Fatal("blanket willingness validated", fields)
			}
		}
		if d, e := EvaluateBoundaries(records, boundaryScope(), 2); e == nil || d.Allowed {
			t.Fatal("blanket grant created consent", fields, d, e)
		}
	}
	b.Topic = AllTopics
	b.Class = AllInteractions
	b.Decision = Declined
	if b.Validate() != nil {
		t.Fatal("blanket refusal must remain representable")
	}
	if d, e := EvaluateBoundaries([]Boundary{a, b}, boundaryScope(), 2); e != nil || d.Allowed {
		t.Fatal("blanket refusal was ignored", d, e)
	}
}
