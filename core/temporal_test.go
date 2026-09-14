package core

import "testing"

func temporalPreference(owner, other ID, upper LogicalTime) TemporalEvidence {
	return TemporalEvidence{Reporter: owner, SourceReporter: owner, Fact: TemporalFact{Version: TemporalFactVersion, Account: ID(string(owner) + "-preference"), Observer: owner, Person: owner, With: other, Channel: "chat", Source: ID(string(owner) + "-source"), Kind: "expectation", Basis: "self_report", ConfirmedAt: 0, FreshFor: 365, MinGap: 1, MaxGap: upper}, OccurredAt: 0, LearnedAt: 0, Confidence: 1, SourceConfidence: 1}
}
func temporalDiary(owner, other ID, at LogicalTime) TemporalEvidence {
	return TemporalEvidence{Reporter: owner, SourceReporter: owner, Fact: TemporalFact{Version: TemporalFactVersion, Account: ID(string(owner) + "-diary"), Observer: owner, Person: owner, With: other, Channel: "chat", Source: ID(string(owner) + "-source"), Kind: "contacts", Basis: "observed", Observations: []LogicalTime{0, 1, 2, 3}, ObservedThrough: at, CompleteChannel: true}, SourceOccurredAt: at, SourceLearnedAt: at, OccurredAt: at, LearnedAt: at, Confidence: 1, SourceConfidence: 1}
}
func temporalLife(owner, other ID, signal string, confirmed, fresh LogicalTime) TemporalEvidence {
	return TemporalEvidence{Reporter: owner, SourceReporter: owner, Fact: TemporalFact{Version: TemporalFactVersion, Account: ID(string(owner) + "-life"), Observer: owner, Person: owner, With: other, Channel: "chat", Source: ID(string(owner) + "-source"), Kind: "circumstance", Basis: "self_report", Category: "responsibility", Signal: signal, ConfirmedAt: confirmed, FreshFor: fresh}, SourceOccurredAt: confirmed, SourceLearnedAt: confirmed, OccurredAt: confirmed, LearnedAt: confirmed, Confidence: 1, SourceConfidence: 1}
}
func TestContactGapsUseObserverExpectationsAndChannelEvidence(t *testing.T) {
	f := TemporalFocus{TemporalFocusVersion, "alice", "bob", "chat"}
	for _, gap := range []LogicalTime{2, 60} {
		for _, upper := range []LogicalTime{3, 90} {
			at := 3 + gap
			r := []TemporalEvidence{temporalPreference("alice", "bob", upper), temporalDiary("alice", "bob", at), temporalPreference("bob", "alice", 1)}
			out, e := InterpretTemporal(f, r, at)
			if e != nil || !out.GapKnown || out.ObservedGap != gap || out.Rhythm.MaxGap != upper || out.Recommendation == "clarify" != (gap > upper) {
				t.Fatal("elapsed time or other observer replaced own expectation", out, e)
			}
		}
	}
	for _, kind := range []string{"partial", "unobserved_channel", "sparse", "irregular", "contradiction"} {
		diary := temporalDiary("alice", "bob", 63)
		records := []TemporalEvidence{diary}
		switch kind {
		case "partial":
			records[0].Fact.CompleteChannel = false
			records = append(records, temporalPreference("alice", "bob", 1))
		case "unobserved_channel":
			records[0].Fact.Channel = "phone"
			records = append(records, temporalPreference("alice", "bob", 1))
		case "sparse":
			records[0].Fact.Observations = []LogicalTime{1, 3}
		case "irregular":
			records[0].Fact.Observations = []LogicalTime{0, 1, 20, 21}
		case "contradiction":
			a := temporalPreference("alice", "bob", 1)
			b := a
			b.Fact.Account = "contrary"
			b.Fact.MaxGap = 90
			records = append(records, a, b)
		}
		out, e := InterpretTemporal(f, records, 63)
		if e != nil || out.Status != "unknown" || out.Recommendation != "wait" {
			t.Fatal("missing evidence became rejection/escalation", kind, out, e)
		}
	}
	out, e := InterpretTemporal(f, []TemporalEvidence{temporalDiary("alice", "bob", 5)}, 5)
	if e != nil || out.Rhythm.Basis != "estimated" || out.Rhythm.Samples != 4 || out.Rhythm.Confidence >= 1 || out.Status != "routine" {
		t.Fatal("bounded estimator positive control", out, e)
	}
}
func TestLifeChangesAndStalenessDifferFromHistoricalValidity(t *testing.T) {
	focus := TemporalFocus{TemporalFocusVersion, "alice", "bob", "chat"}
	for _, kind := range []string{"busy", "changed", "stale", "stale_preference", "hypothesis", "future", "other_observer", "expired", "updated_preference"} {
		at := LogicalTime(63)
		records := []TemporalEvidence{temporalPreference("alice", "bob", 90), temporalDiary("alice", "bob", at)}
		life := temporalLife("alice", "bob", "busy", 60, 30)
		switch kind {
		case "changed":
			life.Fact.Signal = "routine_changed"
			life.Fact.Category = "transition"
		case "stale":
			life.Fact.ConfirmedAt = 1
			life.OccurredAt = 1
			life.LearnedAt = 1
			life.SourceOccurredAt = 1
			life.SourceLearnedAt = 1
		case "stale_preference":
			records[0].Fact.FreshFor = 30
			life.Fact.Signal = "available"
		case "hypothesis":
			life.Fact.Basis = "hypothesis"
			life.Fact.Person = "bob"
		case "future":
			life.LearnedAt = 64
		case "other_observer":
			life = temporalLife("bob", "alice", "busy", 60, 30)
		case "updated_preference":
			life.Fact.Signal = "routine_changed"
			records[0].Fact.ConfirmedAt = 61
			records[0].OccurredAt = 61
			records[0].LearnedAt = 61
			records[0].SourceOccurredAt = 61
			records[0].SourceLearnedAt = 61
		case "expired":
			end := LogicalTime(62)
			life.Valid.End = &end
		}
		records = append(records, life)
		out, e := InterpretTemporal(focus, records, at)
		if kind == "future" {
			if e == nil {
				t.Fatal("future knowledge consumed")
			}
			continue
		}
		if e != nil {
			t.Fatal(kind, e)
		}
		want := "routine"
		switch kind {
		case "busy":
			want = "busy"
		case "changed":
			want = "changed"
		case "stale", "stale_preference":
			want = "stale"
		}
		if out.Status != want || out.Recommendation == "clarify" != (want == "stale" || want == "changed") {
			t.Fatal("life applicability ignored or hypothesized", kind, out)
		}
		if kind == "stale" && records[2].Valid.End != nil {
			t.Fatal("staleness deleted historical fact")
		}
	}
}
func TestTemporalBoundsAttributionAndVersion(t *testing.T) {
	good := temporalPreference("alice", "bob", 3)
	for _, kind := range []string{"version", "self_report_owner", "self_source", "unbounded_gap", "missing_freshness", "future_confirmation", "NaN_confidence"} {
		bad := good
		switch kind {
		case "version":
			bad.Fact.Version = "future"
		case "self_report_owner":
			bad.Fact.Person = "bob"
		case "self_source":
			bad.Fact.Source = bad.Fact.Account
		case "unbounded_gap":
			bad.Fact.MaxGap = 3651
		case "missing_freshness":
			bad.Fact.FreshFor = 0
		case "future_confirmation":
			bad.Fact.ConfirmedAt = 2
		case "NaN_confidence":
			bad.SourceConfidence = Confidence(2)
		}
		if bad.Validate(1) == nil {
			t.Fatal("invalid temporal record", kind)
		}
	}
	focus := TemporalFocus{TemporalFocusVersion, "alice", "bob", "chat"}
	if _, e := InterpretTemporal(focus, []TemporalEvidence{good, good}, 1); e == nil {
		t.Fatal("duplicate account")
	}
}
