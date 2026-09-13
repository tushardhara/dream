package graph

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/tushardhara/dream/core"
)

var testScope = MemoryScope{"alice", "memory-test"}

func memoryFixture(id core.ID, seq int64) MemoryEntry {
	return MemoryEntry{Sequence: seq, Event: core.Event{Version: 1, Type: MemoryEventType, Stream: "memories", Subject: core.Subject{Principal: "bob"}, OccurredAt: 1, Meta: core.Metadata{ID: id, Observer: "alice", Source: "alice", Sensitivity: core.Restricted, Confidence: .6, Valid: core.Interval{Start: 0}, RecordedAt: time.Unix(seq, 0).UTC(), Rights: core.Rights{Resource: id, Grants: []core.Grant{{Actor: "alice", Recipient: "alice", Purpose: "test", Operation: core.Read}, {Actor: "alice", Recipient: "alice", Purpose: "test", Operation: core.Derive}, {Actor: "bob", Recipient: "bob", Purpose: "test", Operation: core.Read}}}}}, Content: &MemoryContent{Version: 1, Kind: EpisodicMemory, Text: "synthetic " + string(id), Salience: .8, HalfLife: 10, Learned: []Learned{{"alice", 2}, {"bob", 20}}}}
}
func memoryQuery() MemoryQuery {
	return MemoryQuery{Scope: testScope, Actor: "alice", Purpose: "test", Subject: core.Subject{Principal: "bob"}, ValidAt: 5, KnownAt: 10, RecordedAsOf: time.Unix(10000, 0), Limit: 50}
}

type memoryFake struct {
	entries []MemoryEntry
	err     error
}

func (f *memoryFake) ReadMemory(_ context.Context, s MemoryScope) ([]MemoryEntry, error) {
	if s != testScope {
		return []MemoryEntry{}, nil
	}
	return f.entries, f.err
}
func (f *memoryFake) AppendMemory(context.Context, AppendCommand) (AppendResult, error) {
	return AppendResult{}, fmt.Errorf("test read port only")
}
func selected(t testing.TB, f *memoryFake, q MemoryQuery) []core.ID {
	t.Helper()
	out, err := (MemoryService{f}).Retrieve(context.Background(), q)
	if err != nil {
		t.Fatal(err)
	}
	ids := []core.ID{}
	for _, s := range out {
		ids = append(ids, s.Record.Event.Meta.ID)
	}
	return ids
}
func has(ids []core.ID, id core.ID) bool {
	for _, s := range ids {
		if s == id {
			return true
		}
	}
	return false
}

func TestTemporalContradictionDisclosureAndCorrection(t *testing.T) {
	a := memoryFixture("a", 1)
	b := memoryFixture("b", 2)
	b.Content.Kind = ClaimMemory
	b.Event.Meta.Contradicting = []core.ID{"a"}
	f := &memoryFake{entries: []MemoryEntry{a, b}}
	q := memoryQuery()
	if got := selected(t, f, q); !reflect.DeepEqual(got, []core.ID{"a", "b"}) {
		t.Fatal("contradictions collapsed", got)
	}
	q.Actor = "bob"
	q.KnownAt = 19
	if got := selected(t, f, q); len(got) != 0 {
		t.Fatal("future disclosure", got)
	}
	q.KnownAt = 20
	if len(selected(t, f, q)) != 2 {
		t.Fatal("late disclosure missing")
	}
	q.RecordedAsOf = time.Unix(1, 0)
	if got := selected(t, f, q); !reflect.DeepEqual(got, []core.ID{"a"}) {
		t.Fatal("recorded-as-of", got)
	}
	c := memoryFixture("c", 3)
	c.Content.Learned = []Learned{{"alice", 30}, {"bob", 40}}
	c.Supersedes = "a"
	c.Event.Meta.Valid.Start = 4
	f.entries = append(f.entries, c)
	q = memoryQuery()
	q.KnownAt = 29
	if !has(selected(t, f, q), "a") {
		t.Fatal("correction leaked before learned-at")
	}
	q.KnownAt = 30
	got := selected(t, f, q)
	if has(got, "a") || has(got, "b") || !has(got, "c") {
		t.Fatal("obsolete evidence retained", got)
	}
	q.ValidAt = 3
	if !has(selected(t, f, q), "a") || has(selected(t, f, q), "c") {
		t.Fatal("valid interval correction leaked")
	}
	q.ValidAt = 5
	q.RecordedAsOf = time.Unix(2, 0)
	if !has(selected(t, f, q), "a") {
		t.Fatal("historical correction not preserved")
	}
}

func TestEvidencePermissionsCacheAndImmediateRevocation(t *testing.T) {
	a := memoryFixture("source", 1)
	b := memoryFixture("summary", 2)
	b.Content.Kind = NarrativeMemory
	b.Event.Meta.Parents = []core.ID{"source"}
	f := &memoryFake{entries: []MemoryEntry{a, b}}
	svc := MemoryService{f}
	q := memoryQuery()
	out, cache, hit, err := svc.RetrieveCached(context.Background(), q, MemoryCache{})
	if err != nil || hit || len(out) != 2 {
		t.Fatal(out, hit, err)
	}
	out[0].Record.Content.Text = "caller mutation"
	out[0].Rationale.Codes[0] = "tampered"
	out, _, hit, err = svc.RetrieveCached(context.Background(), q, cache)
	if err != nil || !hit || out[0].Record.Content.Text == "caller mutation" || out[0].Rationale.Codes[0] != "scope" {
		t.Fatal("cache alias", out, err)
	}
	raw, _ := json.Marshal(cache)
	if strings.Contains(string(raw), "synthetic") {
		t.Fatal("cache retained text")
	}
	// With the source denied but summary still granted, recursive evidence must
	// deny the summary as well. No async projector/cache flush is invoked.
	f.entries[0].Event.Meta.Rights.Grants = nil
	out, _, hit, err = svc.RetrieveCached(context.Background(), q, cache)
	if err != nil || hit || len(out) != 0 {
		t.Fatal("source permissions escaped", out, err)
	}
	f.entries[0] = memoryFixture("source", 1)
	f.entries[0].Revoked = true
	f.entries[0].Content = nil
	out, _, hit, err = svc.RetrieveCached(context.Background(), q, cache)
	if err != nil || hit || len(out) != 0 {
		t.Fatal("revocation escaped", out, err)
	}
	f.entries[0] = memoryFixture("source", 1)
	f.entries[0].Content.Learned[0].At = 11
	if len(selected(t, f, q)) != 0 {
		t.Fatal("future source escaped into derivative")
	}
	f.entries[0] = memoryFixture("source", 1)
	q.Purpose = "another"
	if len(selected(t, f, q)) != 0 {
		t.Fatal("purpose escaped")
	}
	q = memoryQuery()
	q.Actor = "mallory"
	if len(selected(t, f, q)) != 0 {
		t.Fatal("audience escaped")
	}
	q = memoryQuery()
	q.Scope.Namespace = "other"
	if len(selected(t, f, q)) != 0 {
		t.Fatal("namespace escaped")
	}
}

func TestMaterialInvalidationAndExpiry(t *testing.T) {
	a := memoryFixture("source", 1)
	b := memoryFixture("summary", 2)
	b.Content.Kind = NarrativeMemory
	b.Event.Meta.Parents = []core.ID{"source"}
	c := memoryFixture("correction", 3)
	c.Supersedes = "source"
	c.Content.Learned[0].At = 100
	f := &memoryFake{entries: []MemoryEntry{a, b, c}}
	q := memoryQuery()
	q.RecordedAsOf = time.Unix(2, 0)
	got := selected(t, f, q)
	if !has(got, "source") || has(got, "summary") {
		t.Fatal("material summary not invalidated", got)
	}
	for _, kind := range []MemoryKind{OpenLoopMemory, IntentMemory} {
		e := memoryFixture("loop", 1)
		e.Content.Kind = kind
		expires := core.LogicalTime(10)
		e.Content.Expires = &expires
		f.entries = []MemoryEntry{e}
		q = memoryQuery()
		q.KnownAt = 9
		if len(selected(t, f, q)) != 1 {
			t.Fatal("premature expiry")
		}
		q.KnownAt = 10
		if len(selected(t, f, q)) != 0 {
			t.Fatal("expiry boundary")
		}
	}
}

func TestMemoryReplayNeverResurrectsAndLogicalHash(t *testing.T) {
	a := memoryFixture("a", 1)
	b := memoryFixture("b", 2)
	b.Event.Meta.Supporting = []core.ID{"a"}
	idx, _ := NewMemoryIndex(testScope)
	for _, e := range []MemoryEntry{a, b, a, b} {
		if err := idx.Apply(e); err != nil {
			t.Fatal(err)
		}
	}
	revoked := a
	revoked.Revoked = true
	revoked.Content = nil
	if err := idx.Apply(revoked); err != nil {
		t.Fatal(err)
	}
	if err := idx.Apply(a); err != nil {
		t.Fatal(err)
	}
	rebuilt, err := RebuildMemory(testScope, []MemoryEntry{revoked, b})
	if err != nil {
		t.Fatal(err)
	}
	hash, _ := idx.LogicalHash()
	other, _ := rebuilt.LogicalHash()
	if hash != other {
		t.Fatal("incremental and rebuild disagree")
	}
	if len(idx.selectMemory(memoryQuery())) != 0 {
		t.Fatal("replay resurrected source/derivative")
	}
	// Distinct operational record times and sequence numbers must not change the
	// logical content hash, including metadata with nanosecond logical precision.
	b.Event.Meta.RecordedAt = time.Unix(1000, 0)
	b.Sequence = 20
	revoked.Event.Meta.RecordedAt = time.Unix(999, 0)
	revoked.Sequence = 10
	rebuilt, err = RebuildMemory(testScope, []MemoryEntry{revoked, b})
	if err != nil {
		t.Fatal(err)
	}
	other, _ = rebuilt.LogicalHash()
	if hash != other {
		t.Fatal("wall metadata entered logical hash")
	}
	conflict := b
	conflict.Event.Meta.Source = "changed"
	if err = idx.Apply(conflict); err == nil {
		t.Fatal("conflicting duplicate accepted")
	}
	for _, entries := range [][]MemoryEntry{{b, a}, {func() MemoryEntry { e := memoryFixture("a", 1); e.Event.Meta.Parents = []core.ID{"b"}; return e }(), b}} {
		if _, err = RebuildMemory(testScope, entries); err == nil {
			t.Fatal("forward/cyclic evidence accepted")
		}
	}
}

func TestMemoryValidationBudgetsAndCanonical(t *testing.T) {
	cases := map[string]func(*MemoryRecord){"version": func(r *MemoryRecord) { r.Content.Version = 2 }, "kind": func(r *MemoryRecord) { r.Content.Kind = "truth" }, "nan": func(r *MemoryRecord) { r.Content.Salience = math.NaN() }, "infinity": func(r *MemoryRecord) { r.Content.Salience = math.Inf(1) }, "decay": func(r *MemoryRecord) { r.Content.HalfLife = 0 }, "future": func(r *MemoryRecord) { r.Content.Learned[0].At = 0 }, "duplicate": func(r *MemoryRecord) { r.Content.Learned = append(r.Content.Learned, r.Content.Learned[0]) }, "claim": func(r *MemoryRecord) { r.Content.Kind = ClaimMemory }, "summary": func(r *MemoryRecord) { r.Content.Kind = NarrativeMemory }, "expiry": func(r *MemoryRecord) { r.Content.Kind = IntentMemory }, "text": func(r *MemoryRecord) { r.Content.Text = strings.Repeat("a", 2049) }, "foreign_reference": func(r *MemoryRecord) {
		r.Event.Subject = core.Subject{Reference: &core.NonparticipantReference{Observer: "bob", LocalID: "same"}}
	}}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			r := memoryFixture("a", 1).record()
			mutate(&r)
			if _, err := EncodeMemoryPayload(r); err == nil {
				t.Fatal("invalid memory accepted")
			}
		})
	}
	r := memoryFixture("a", 1).record()
	p, err := EncodeMemoryPayload(r)
	if err != nil {
		t.Fatal(err)
	}
	reordered := r
	reordered.Content.Learned = []Learned{{"bob", 20}, {"alice", 2}}
	p2, _ := EncodeMemoryPayload(reordered)
	if p != p2 {
		t.Fatal("audience order changed encoding")
	}
	for _, text := range []string{strings.Replace(p.Text, `"version":1`, `"version":1,"version":1`, 1), strings.Replace(p.Text, `"version":1`, `"version":2`, 1), strings.Replace(p.Text, `"version":1`, `"extra":1,"version":1`, 1), p.Text + " {}"} {
		bad := p
		bad.Text = text
		if _, err = DecodeMemoryPayload(r.Event, "", bad); err == nil {
			t.Fatal("noncanonical content accepted")
		}
	}
	idx, _ := NewMemoryIndex(testScope)
	for i := 1; i <= MaxMemoryRecords; i++ {
		if err = idx.Apply(memoryFixture(core.ID(fmt.Sprintf("id-%04d", i)), int64(i))); err != nil {
			t.Fatal(err)
		}
	}
	if err = idx.Apply(memoryFixture("overflow", 513)); err == nil {
		t.Fatal("scope budget ignored")
	}
	q := memoryQuery()
	q.Limit = 3
	if len(idx.selectMemory(q)) != 3 {
		t.Fatal("result budget ignored")
	}
	q.Limit = 51
	if q.Validate() == nil {
		t.Fatal("unbounded query")
	}
	// A reference local ID does not identify a person across observer scopes.
	q = memoryQuery()
	q.Subject = core.Subject{Reference: &core.NonparticipantReference{Observer: "bob", LocalID: "same"}}
	if q.Validate() == nil {
		t.Fatal("foreign placeholder query")
	}
}

func TestMemoryDecayAndStableRanking(t *testing.T) {
	a := memoryFixture("z", 1)
	b := memoryFixture("a", 2)
	f := &memoryFake{entries: []MemoryEntry{a, b}}
	q := memoryQuery()
	q.KnownAt = 12
	out, err := (MemoryService{f}).Retrieve(context.Background(), q)
	if err != nil || len(out) != 2 {
		t.Fatal(out, err)
	}
	if out[0].Record.Event.Meta.ID != "a" || math.Abs(out[0].Rationale.DecayedSalience-.4) > 1e-12 || out[0].Rationale.Recency != .5 || out[0].Rationale.Score != .425 {
		t.Fatal("analytic ranking golden", out)
	}
}

func TestMemoryAppendKnowledgeAndSourceBoundary(t *testing.T) {
	a := memoryFixture("source", 1)
	r := memoryFixture("derived", 2).record()
	r.Event.Meta.Parents = []core.ID{"source"}
	p, _ := EncodeMemoryPayload(r)
	c := AppendCommand{Actor: "alice", Namespace: testScope.Namespace, Operation: "memory.put", Class: PrivatePayload, Event: r.Event, Payload: p}
	if err := ValidateMemoryAppend(testScope, []MemoryEntry{a}, c); err != nil {
		t.Fatal(err)
	}
	r.Content.Learned[1].At = 19
	c.Payload, _ = EncodeMemoryPayload(r)
	if err := ValidateMemoryAppend(testScope, []MemoryEntry{a}, c); err == nil {
		t.Fatal("derivative disclosed before source knowledge")
	}
	a.Revoked = true
	a.Content = nil
	if err := ValidateMemoryAppend(testScope, []MemoryEntry{a}, c); err == nil {
		t.Fatal("revoked source used")
	}
}

func FuzzMemoryTemporalBoundary(f *testing.F) {
	f.Add(uint16(20), uint16(10), uint16(15), true)
	f.Add(uint16(2), uint16(30), uint16(1), false)
	f.Fuzz(func(t *testing.T, known, learned, valid uint16, allowed bool) {
		a := memoryFixture("source", 1)
		a.Content.Learned[0].At = core.LogicalTime(1 + learned%1000)
		b := memoryFixture("derived", 2)
		b.Event.Meta.Parents = []core.ID{"source"}
		b.Content.Learned[0].At = a.Content.Learned[0].At + 2
		end := core.LogicalTime(500)
		a.Event.Meta.Valid.End = &end
		if !allowed {
			a.Event.Meta.Rights.Grants = nil
		}
		fake := &memoryFake{entries: []MemoryEntry{a, b}}
		q := memoryQuery()
		q.KnownAt = core.LogicalTime(known % 1000)
		q.ValidAt = core.LogicalTime(valid % 1000)
		svc := MemoryService{fake}
		out, cache, _, err := svc.RetrieveCached(context.Background(), q, MemoryCache{})
		if err != nil {
			t.Fatal(err)
		}
		for _, s := range out {
			if !allowed || q.ValidAt >= end || q.KnownAt < a.Content.Learned[0].At {
				t.Fatal("unauthorized/future/invalid evidence returned")
			}
			if s.Record.Event.Meta.ID == "derived" && q.KnownAt < b.Content.Learned[0].At {
				t.Fatal("future derivative returned")
			}
		}
		if allowed && q.ValidAt < end && q.KnownAt >= b.Content.Learned[0].At && len(out) != 2 {
			t.Fatal("eligible records disappeared")
		}
		fake.entries[0].Revoked = true
		fake.entries[0].Content = nil
		out, _, _, err = svc.RetrieveCached(context.Background(), q, cache)
		if err != nil || len(out) != 0 {
			t.Fatal("revocation bypass", out, err)
		}
	})
}
