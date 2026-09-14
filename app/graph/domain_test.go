package graph

import (
	"context"
	"github.com/tushardhara/dream/core"
	"strings"
	"testing"
)

func domainRelationRecord(t *testing.T, id core.ID, seq int64, domain core.RelationshipDomain, frame core.ID, value float64) MemoryEntry {
	t.Helper()
	entry := memoryFixture(id, seq)
	entry.Event.Subject = core.Subject{Principal: "alice"}
	entry.Event.Meta.Parents = []core.ID{"source", "frame-source"}
	profile := core.RelationshipContext{Version: 2, Account: id, Observer: "alice", Other: "bob", Domain: domain, RoleContext: frame, ContextSource: "frame-source", Types: []core.ID{"sibling", "business_partner"}, Valid: core.Interval{}, Measures: []core.RelationshipMeasure{{Kind: "trust", Value: value, Confidence: .8, Source: "source"}}}
	from, to := core.Subject{Principal: "alice"}, core.Subject{Principal: "bob"}
	v := RelationState{Version: 3, ID: "edge", Kind: "edge", From: &from, To: &to, Types: profile.Types, Context: &profile}
	entry.Content.Kind = RelationshipMemory
	var e error
	entry.Content.Text, e = EncodeRelation(v, "alice")
	if e != nil {
		t.Fatal(e)
	}
	return entry
}
func TestDomainRelationCodecBindsAccountFrameAndVersion(t *testing.T) {
	entry := domainRelationRecord(t, "care", 3, core.Childcare, "family", .8)
	if !strings.HasPrefix(entry.Content.Text, "relation.v3:") {
		t.Fatal("wrong wire version")
	}
	if _, e := DecodeRelation(entry.record()); e != nil {
		t.Fatal(e)
	}
	for _, name := range []string{"account", "frame-lineage", "version", "context-version"} {
		bad := domainRelationRecord(t, "care", 3, core.Childcare, "family", .8)
		v, _ := DecodeRelation(bad.record())
		switch name {
		case "account":
			v.Context.Account = "another-record"
		case "frame-lineage":
			bad.Event.Meta.Parents = []core.ID{"source"}
		case "version":
			v.Version = 2
		case "context-version":
			v.Context.Version = 1
		}
		text, e := EncodeRelation(v, "alice")
		if e != nil {
			if name != "version" && name != "context-version" {
				t.Fatal(e)
			}
			continue
		}
		bad.Content.Text = text
		if _, e := DecodeRelation(bad.record()); e == nil {
			t.Fatal("invalid domain envelope", name)
		}
	}
	item := SafeContextItem{Source: entry.Event.Meta.ID, Observer: "alice", Subject: entry.Event.Subject, Kind: RelationshipMemory, Parents: entry.Event.Meta.Parents, Text: entry.Content.Text, Valid: entry.Event.Meta.Valid}
	safe := SafeContext{items: []SafeContextItem{item}}
	if profiles, e := safe.Relations(); e != nil || len(profiles) != 1 || profiles[0].State.Context.Domain != core.Childcare {
		t.Fatal("safe domain context lost", e)
	}
	safe.items[0].Source = "forged"
	if _, e := safe.Relations(); e == nil {
		t.Fatal("safe account not bound to source")
	}
}
func TestDomainRetrievalCorrectionsAndRevocationPreserveOtherAccounts(t *testing.T) {
	entries := []MemoryEntry{memoryFixture("source", 1), memoryFixture("frame-source", 2), domainRelationRecord(t, "care", 3, core.Childcare, "family", .8), domainRelationRecord(t, "finance", 4, core.Finances, "family", -.7), domainRelationRecord(t, "business", 5, core.Childcare, "business", -.5)}
	journal := &memoryFake{entries: entries}
	svc := RelationService{Memory: MemoryService{journal}}
	q := memoryQuery()
	q.Subject = core.Subject{Principal: "alice"}
	out, e := svc.Query(context.Background(), q, "edge")
	if e != nil || len(out) != 3 {
		t.Fatal("overlapping domain accounts lost", len(out), e)
	}
	corrected := domainRelationRecord(t, "care-corrected", 6, core.Childcare, "family", -.9)
	corrected.Supersedes = "care"
	corrected.Content.Learned[0].At = 20
	journal.entries = append(journal.entries, corrected)
	out, e = svc.Query(context.Background(), q, "edge")
	if e != nil || len(out) != 3 {
		t.Fatal("future correction changed view", e)
	}
	q.KnownAt = 20
	out, e = svc.Query(context.Background(), q, "edge")
	if e != nil || len(out) != 3 {
		t.Fatal("correction erased other domains", len(out), e)
	}
	found := map[core.ID]bool{}
	for _, v := range out {
		found[v.State.Context.Account] = true
	}
	if !found["care-corrected"] || !found["finance"] || !found["business"] || found["care"] {
		t.Fatal("correction scope", found)
	}
	journal.entries[5].Revoked = true
	journal.entries[5].Content = nil
	out, e = svc.Query(context.Background(), q, "edge")
	if e != nil {
		t.Fatal(e)
	}
	for _, v := range out {
		if v.State.Context.Account == "care-corrected" {
			t.Fatal("revoked domain account retrieved")
		}
	}
	// A correction cannot move an account's meaning to another domain/frame.
	for _, change := range []string{"domain", "frame"} {
		fresh := domainRelationRecord(t, "new-correction", 7, core.Childcare, "family", .5)
		fresh.Supersedes = "care"
		v, _ := DecodeRelation(fresh.record())
		if change == "domain" {
			v.Context.Domain = core.Finances
		} else {
			v.Context.RoleContext = "business"
		}
		if _, e := svc.Put(context.Background(), testScope, "correction", 6, fresh.record(), v, "test"); e == nil || !strings.Contains(e.Error(), "domain identity") {
			t.Fatal("correction escaped identity gate", change, e)
		}
	}
}

func TestDomainComparisonCannotSubtractDifferentFrames(t *testing.T) {
	before := domainRelationRecord(t, "before", 1, core.Childcare, "family", .3)
	a, e := DecodeRelation(before.record())
	if e != nil {
		t.Fatal(e)
	}
	for _, kind := range []string{"same", "domain", "frame"} {
		domain, frame := core.Childcare, core.ID("family")
		if kind == "domain" {
			domain = core.Finances
		}
		if kind == "frame" {
			frame = "business"
		}
		after := domainRelationRecord(t, "after", 2, domain, frame, .8)
		b, e := DecodeRelation(after.record())
		if e != nil {
			t.Fatal(e)
		}
		_, e = CompareRelation(RelationProjection{Observer: "alice", Record: before.record(), State: a}, RelationProjection{Observer: "alice", Record: after.record(), State: b})
		if (e == nil) != (kind == "same") {
			t.Fatal("cross-context subtraction", kind, e)
		}
	}
}
