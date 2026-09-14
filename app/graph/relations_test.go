package graph

import (
	"context"
	"fmt"
	"math"
	"reflect"
	"sync"
	"testing"

	"github.com/tushardhara/dream/core"
)

func relationFixture(id core.ID, seq int64, value float64) MemoryEntry {
	e := memoryFixture(id, seq)
	from, to := core.Subject{Principal: "bob"}, core.Subject{Principal: "alice"}
	v := RelationState{Version: 1, ID: "edge", Kind: "edge", From: &from, To: &to, Types: []core.ID{"colleague", "friend"}, Dimensions: []RelationshipDimension{{Name: "perceived_support", Value: value, Confidence: .4}}, Memberships: []Membership{{Member: to, Roles: []core.ID{"participant"}, Valid: core.Interval{Start: 0}}}}
	e.Content.Kind = RelationshipMemory
	e.Content.Text, _ = EncodeRelation(v, "alice")
	return e
}
func TestRelationPerspectiveHistoryDeltaAndRebuild(t *testing.T) {
	a := relationFixture("before", 1, -.6)
	b := relationFixture("after", 2, .4)
	b.Supersedes = "before"
	b.Content.Learned[0].At = 20
	f := &memoryFake{entries: []MemoryEntry{a, b}}
	svc := RelationService{Memory: MemoryService{f}}
	q := memoryQuery()
	first, err := svc.Query(context.Background(), q, "edge")
	if err != nil || len(first) != 1 || first[0].State.Dimensions[0].Value != -.6 {
		t.Fatal(first, err)
	}
	q.KnownAt = 20
	second, err := svc.Query(context.Background(), q, "edge")
	if err != nil || len(second) != 1 {
		t.Fatal(second, err)
	}
	delta, err := CompareRelation(first[0], second[0])
	if err != nil || len(delta.Dimensions) != 1 || delta.Dimensions[0].Change != 1 || delta.Dimensions[0].BeforeConfidence != .4 {
		t.Fatal(delta, err)
	}
	foreign := second[0]
	foreign.Observer = "bob"
	if _, err = CompareRelation(first[0], foreign); err == nil {
		t.Fatal("opposing observers averaged")
	}
	idx, _ := NewMemoryIndex(testScope)
	for _, e := range f.entries {
		if err = idx.Apply(e); err != nil {
			t.Fatal(err)
		}
	}
	rebuilt, err := RebuildMemory(testScope, f.entries)
	if err != nil {
		t.Fatal(err)
	}
	h1, _ := idx.LogicalHash()
	h2, _ := rebuilt.LogicalHash()
	if h1 != h2 {
		t.Fatal("relation rebuild mismatch")
	}
	f.entries[0].Revoked = true
	f.entries[0].Content = nil
	f.entries[1].Revoked = true
	f.entries[1].Content = nil
	if got, err := svc.Query(context.Background(), q, "edge"); err != nil || len(got) != 0 {
		t.Fatal("revoked relation visible", got, err)
	}
}
func TestTemporalMembershipDoesNotGrantHistory(t *testing.T) {
	e := memoryFixture("group-state", 1)
	e.Event.Subject = core.Subject{Principal: "group"}
	e.Content.Kind = RelationshipMemory
	start := core.LogicalTime(20)
	v := RelationState{Version: 1, ID: "group", Kind: "group", Types: []core.ID{"project"}, Memberships: []Membership{{Member: core.Subject{Principal: "alice"}, Roles: []core.ID{"organizer"}, Valid: core.Interval{Start: 0, End: &start}}, {Member: core.Subject{Principal: "bob"}, Roles: []core.ID{"participant"}, Valid: core.Interval{Start: 20}}}}
	e.Content.Text, _ = EncodeRelation(v, "alice")
	e.Event.Meta.Rights.Grants = e.Event.Meta.Rights.Grants[:2] // Alice only; Bob's membership creates no grant.
	f := &memoryFake{entries: []MemoryEntry{e}}
	svc := RelationService{Memory: MemoryService{f}}
	q := memoryQuery()
	q.Subject = e.Event.Subject
	q.ValidAt = 19
	q.KnownAt = 30
	before, err := svc.Query(context.Background(), q, "group")
	if err != nil || len(before) != 1 || len(before[0].ActiveMembers) != 1 || before[0].ActiveMembers[0].Member.Principal != "alice" {
		t.Fatal(before, err)
	}
	q.ValidAt = 20
	after, err := svc.Query(context.Background(), q, "group")
	if err != nil || len(after) != 1 || len(after[0].ActiveMembers) != 1 || after[0].ActiveMembers[0].Member.Principal != "bob" {
		t.Fatal(after, err)
	}
	if len(after[0].State.Dimensions) != 0 {
		t.Fatal("participation inferred care")
	}
	q.Actor = "bob"
	if got, err := svc.Query(context.Background(), q, "group"); err != nil || len(got) != 0 {
		t.Fatal("new member inherited private history", got, err)
	}
}
func TestRelationFiltersBeforeLimitAndPreservesLegacyMemory(t *testing.T) {
	f := &memoryFake{}
	for i := 1; i <= 60; i++ {
		e := memoryFixture(core.ID(fmt.Sprintf("memory-%d", i)), int64(i))
		e.Content.Kind = RelationshipMemory
		f.entries = append(f.entries, e)
	}
	relation := relationFixture("edge-state", 61, .2)
	relation.Content.Salience = 0
	f.entries = append(f.entries, relation)
	q := memoryQuery()
	q.Limit = 1
	out, err := (RelationService{Memory: MemoryService{f}}).Query(context.Background(), q, "edge")
	if err != nil || len(out) != 1 || out[0].Record.Event.Meta.ID != "edge-state" {
		t.Fatal("unrelated memories consumed entity result budget", out, err)
	}
}
func TestRelationCodecAndSignals(t *testing.T) {
	r := relationFixture("r", 1, .2).record()
	v, err := DecodeRelation(r)
	if err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*RelationState){"version": func(v *RelationState) { v.Version = 2 }, "nonfinite": func(v *RelationState) { v.Dimensions[0].Value = math.NaN() }, "unknown confidence": func(v *RelationState) { v.Dimensions[0].Confidence = 2 }, "duplicate types": func(v *RelationState) { v.Types = []core.ID{"friend", "friend"} }, "overlap": func(v *RelationState) { v.Memberships = append(v.Memberships, v.Memberships[0]) }, "pattern evidence": func(v *RelationState) { v.Patterns = []RelationshipPattern{{Name: "pattern", Confidence: .2}} }} {
		t.Run(name, func(t *testing.T) {
			v, _ := DecodeRelation(r)
			mutate(&v)
			if _, err := EncodeRelation(v, "alice"); err == nil {
				t.Fatal("invalid relation accepted")
			}
		})
	}
	v.OpenLoops = []core.ID{"missing-loop"}
	r.Content.Text, _ = EncodeRelation(v, "alice")
	if _, err = DecodeRelation(r); err == nil {
		t.Fatal("signal bypassed provenance")
	}
	r.Event.Meta.Parents = []core.ID{"missing-loop"}
	if _, err = DecodeRelation(r); err != nil {
		t.Fatal(err)
	}
	// Encoding and querying shared immutable values must remain safe concurrently.
	original, _ := EncodeRelation(v, "alice")
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 30; j++ {
				encoded, err := EncodeRelation(v, "alice")
				if err != nil || encoded != original {
					t.Error("concurrent codec changed")
				}
			}
		}()
	}
	wg.Wait()
}
func TestExportRequiresEverySourceAndRecipient(t *testing.T) {
	a := memoryFixture("source", 1)
	b := memoryFixture("claim", 2)
	b.Content.Kind = ClaimMemory
	b.Event.Meta.Supporting = []core.ID{"source"}
	grant := core.Grant{Actor: "alice", Recipient: "reader", Purpose: "test", Operation: core.Export}
	b.Event.Meta.Rights.Grants = append(b.Event.Meta.Rights.Grants, grant)
	f := &memoryFake{entries: []MemoryEntry{a, b}}
	svc := MemoryService{f}
	q := memoryQuery()
	out, err := svc.Export(context.Background(), q, "reader")
	if err != nil || len(out.Records) != 0 {
		t.Fatal("source export permission ignored", out, err)
	}
	f.entries[0].Event.Meta.Rights.Grants = append(f.entries[0].Event.Meta.Rights.Grants, grant)
	out, err = svc.Export(context.Background(), q, "reader")
	if err != nil || len(out.Records) != 2 {
		t.Fatal(out, err)
	}
	out.Records[0].Content.Learned[0].Actor = "changed"
	out2, err := svc.Export(context.Background(), q, "reader")
	if err != nil || reflect.DeepEqual(out, out2) {
		t.Fatal("export aliases journal", err)
	}
	denied, err := svc.Export(context.Background(), q, "other")
	if err != nil || len(denied.Records) != 0 {
		t.Fatal("recipient ignored", denied, err)
	}
	f.entries[0].Revoked = true
	f.entries[0].Content = nil
	out, err = svc.Export(context.Background(), q, "reader")
	if err != nil || len(out.Records) != 0 {
		t.Fatal("revoked source exported", out, err)
	}
}

type perspectiveJournal map[MemoryScope][]MemoryEntry

func (p perspectiveJournal) ReadMemory(_ context.Context, s MemoryScope) ([]MemoryEntry, error) {
	return p[s], nil
}
func (p perspectiveJournal) AppendMemory(context.Context, AppendCommand) (AppendResult, error) {
	return AppendResult{}, fmt.Errorf("read-only test journal")
}
func TestOpposingRelationshipViewsStaySeparate(t *testing.T) {
	alice := relationFixture("view", 1, -.8)
	bob := relationFixture("view", 1, .8)
	bob.Event.Meta.Observer = "bob"
	aliceScope, bobScope := testScope, MemoryScope{Owner: "bob", Namespace: testScope.Namespace}
	svc := RelationService{Memory: MemoryService{Journal: perspectiveJournal{aliceScope: {alice}, bobScope: {bob}}}}
	q := memoryQuery()
	q.KnownAt = 30
	a, err := svc.Query(context.Background(), q, "edge")
	if err != nil {
		t.Fatal(err)
	}
	q.Scope = bobScope
	q.Actor = "bob"
	b, err := svc.Query(context.Background(), q, "edge")
	if err != nil {
		t.Fatal(err)
	}
	if len(a) != 1 || len(b) != 1 || a[0].Observer != "alice" || b[0].Observer != "bob" || a[0].State.Dimensions[0].Value != -.8 || b[0].State.Dimensions[0].Value != .8 {
		t.Fatal("opposing relationship views collapsed", a, b)
	}
	if _, err = CompareRelation(a[0], b[0]); err == nil {
		t.Fatal("cross-observer delta fabricated")
	}
}

func TestApprovedRelationProjectionKeepsObserverAndProvenance(t *testing.T) {
	v := RelationState{Version: 1, ID: "edge", Kind: "edge", From: &core.Subject{Principal: "a"}, To: &core.Subject{Principal: "b"}, Types: []core.ID{"friend"}, Dimensions: []RelationshipDimension{{Name: "trust", Value: -.5, Confidence: .6}}}
	text, e := EncodeRelation(v, "a")
	if e != nil {
		t.Fatal(e)
	}
	item := SafeContextItem{Source: "relation-evidence", Observer: "a", Subject: core.Subject{Principal: "a"}, Kind: RelationshipMemory, Text: text}
	context := SafeContext{items: []SafeContextItem{item}}
	relations, e := context.Relations()
	if e != nil || len(relations) != 1 || relations[0].Observer != "a" || relations[0].Source != "relation-evidence" || relations[0].State.Dimensions[0].Value != -.5 {
		t.Fatal("relation attribution lost", e)
	}
	relations[0].State.Dimensions[0].Value = 1
	again, e := context.Relations()
	if e != nil || again[0].State.Dimensions[0].Value != -.5 {
		t.Fatal("shared relation state")
	}
	bad := item
	bad.Subject = core.Subject{Principal: "b"}
	if _, e = (SafeContext{items: []SafeContextItem{bad}}).Relations(); e == nil {
		t.Fatal("envelope mismatch")
	}
	v.OpenLoops = []core.ID{"unknown-source"}
	bad = item
	bad.Text, e = EncodeRelation(v, "a")
	if e != nil {
		t.Fatal(e)
	}
	if _, e = (SafeContext{items: []SafeContextItem{bad}}).Relations(); e == nil {
		t.Fatal("missing relation provenance")
	}
}
