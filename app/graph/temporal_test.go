package graph

import (
	"context"
	"github.com/tushardhara/dream/core"
	"strings"
	"testing"
)

func temporalRecord(t *testing.T, id core.ID, sequence int64, signal string) MemoryEntry {
	t.Helper()
	entry := memoryFixture(id, sequence)
	entry.Event.Meta.Source = "alice"
	entry.Event.Subject = core.Subject{Principal: "alice"}
	entry.Event.Meta.Parents = []core.ID{"source"}
	entry.Content.Kind = EpisodicMemory
	fact := core.TemporalFact{Version: core.TemporalFactVersion, Account: id, Observer: "alice", Person: "alice", With: "bob", Channel: "chat", Source: "source", Kind: "circumstance", Basis: "self_report", Category: "responsibility", Signal: signal, FreshFor: 30}
	var e error
	entry.Content.Text, e = EncodeTemporal(fact)
	if e != nil {
		t.Fatal(e)
	}
	return entry
}
func TestTemporalGraphBindsAttributionAndApprovedMetadata(t *testing.T) {
	entry := temporalRecord(t, "life", 2, "busy")
	f, e := DecodeTemporal(entry.record())
	if e != nil {
		t.Fatal(e)
	}
	for _, change := range []string{"account", "observer", "source", "self_report", "version", "unknown_field"} {
		bad := temporalRecord(t, "life", 2, "busy")
		fact := f
		switch change {
		case "account":
			fact.Account = "other"
		case "observer":
			fact.Observer = "bob"
		case "source":
			bad.Event.Meta.Parents = nil
		case "self_report":
			fact.Person = "bob"
		case "version":
			fact.Version = "future"
		case "unknown_field":
			bad.Content.Text = strings.TrimSuffix(bad.Content.Text, "}") + `,"unknown":true}`
		}
		if change != "unknown_field" {
			bad.Content.Text, e = EncodeTemporal(fact)
			if e != nil {
				continue
			}
		}
		if _, e = DecodeTemporal(bad.record()); e == nil {
			t.Fatal("invalid temporal envelope", change)
		}
	}
	item := SafeContextItem{Reporter: "alice", Source: "life", Observer: "alice", Subject: entry.Event.Subject, Kind: EpisodicMemory, Text: entry.Content.Text, Parents: entry.Event.Meta.Parents, Confidence: .8, OccurredAt: 1, LearnedAt: 1}
	source := SafeContextItem{Reporter: "alice", Source: "source", Observer: "alice", Confidence: .7, OccurredAt: 1, LearnedAt: 1}
	got, e := TemporalEvidenceFromApproved([]SafeContextItem{item, source}, 2)
	if e != nil || len(got) != 1 || got[0].Confidence != .8 || got[0].SourceConfidence != .7 {
		t.Fatal("source uncertainty lost", got, e)
	}
	if _, e = TemporalEvidenceFromApproved([]SafeContextItem{item}, 2); e == nil {
		t.Fatal("missing source metadata")
	}
	source.Observer = "bob"
	if _, e = TemporalEvidenceFromApproved([]SafeContextItem{item, source}, 2); e == nil {
		t.Fatal("foreign source authorized private context")
	}
}
func TestTemporalCorrectionHistoryAndRevocation(t *testing.T) {
	old := temporalRecord(t, "life", 2, "busy")
	next := temporalRecord(t, "corrected", 3, "available")
	next.Supersedes = "life"
	next.Content.Learned[0].At = 20
	journal := &memoryFake{entries: []MemoryEntry{memoryFixture("source", 1), old, next}}
	service := TemporalService{Memory: MemoryService{Journal: journal}}
	q := memoryQuery()
	q.Subject = core.Subject{Principal: "alice"}
	historical, e := service.Query(context.Background(), q)
	if e != nil || len(historical) != 1 || historical[0].Fact.Signal != "busy" {
		t.Fatal("historical view rewritten", historical, e)
	}
	q.KnownAt = 20
	current, e := service.Query(context.Background(), q)
	if e != nil || len(current) != 1 || current[0].Fact.Signal != "available" {
		t.Fatal("correction ignored", current, e)
	}
	journal.entries[2].Revoked = true
	journal.entries[2].Content = nil
	current, e = service.Query(context.Background(), q)
	if e != nil || len(current) != 0 {
		t.Fatal("revoked correction restored old assumption", current, e)
	}
	for _, change := range []string{"person", "channel", "kind"} {
		record := temporalRecord(t, "new", 4, "available")
		record.Supersedes = "life"
		fact, _ := DecodeTemporal(record.record())
		switch change {
		case "person":
			fact.Person = "bob"
			fact.Basis = "hypothesis"
		case "channel":
			fact.Channel = "phone"
		case "kind":
			fact.Kind = "expectation"
			fact.Signal = ""
			fact.Category = ""
			fact.MinGap = 1
			fact.MaxGap = 3
		}
		_, e = service.Put(context.Background(), testScope, "new", 3, record.record(), fact, "test")
		if e == nil || !strings.Contains(e.Error(), "changes identity") {
			t.Fatal("correction changed owned meaning", change, e)
		}
	}
}
