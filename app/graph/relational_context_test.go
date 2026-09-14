package graph

import (
	"github.com/tushardhara/dream/core"
	"strings"
	"testing"
)

func relationalRecord(t *testing.T) (MemoryRecord, RelationState) {
	t.Helper()
	entry := memoryFixture("relational", 2)
	entry.Event.Subject = core.Subject{Principal: "alice"}
	entry.Event.Meta.Parents = []core.ID{"source"}
	from, to := core.Subject{Principal: "alice"}, core.Subject{Principal: "bob"}
	profile := core.RelationshipContext{Version: 1, Observer: "alice", Other: "bob", Types: []core.ID{"spouse"}, Valid: core.Interval{}, Details: []core.RelationshipDetail{{Kind: "view_of_other", Sources: []core.ID{"source"}}}, Measures: []core.RelationshipMeasure{{Kind: "trust", Value: -.6, Confidence: .4, Source: "source"}}}
	state := RelationState{Version: 2, ID: "edge", Kind: "edge", From: &from, To: &to, Types: []core.ID{"spouse"}, Context: &profile}
	entry.Content.Kind = RelationshipMemory
	var e error
	entry.Content.Text, e = EncodeRelation(state, "alice")
	if e != nil {
		t.Fatal(e)
	}
	return entry.record(), state
}
func TestRelationshipV2CodecPermissionEnvelope(t *testing.T) {
	r, v := relationalRecord(t)
	if !strings.HasPrefix(r.Content.Text, "relation.v2:") || len(r.Content.Text) > 2048 {
		t.Fatal("version/bound")
	}
	if _, e := DecodeRelation(r); e != nil {
		t.Fatal(e)
	}
	// Each negative changes one envelope or source authority constraint.
	for _, name := range []string{"lineage", "observer", "subject", "time", "version", "unknown_field"} {
		t.Run(name, func(t *testing.T) {
			r, _ := relationalRecord(t)
			switch name {
			case "lineage":
				r.Event.Meta.Parents = nil
			case "observer":
				r.Event.Meta.Observer = "bob"
			case "subject":
				r.Event.Subject = core.Subject{Principal: "bob"}
			case "time":
				r.Event.Meta.Valid.Start = 1
			case "version":
				r.Content.Text = strings.Replace(r.Content.Text, "relation.v2:", "relation.v1:", 1)
			case "unknown_field":
				r.Content.Text = strings.Replace(r.Content.Text, "\"version\":2", "\"unknown\":true,\"version\":2", 1)
			}
			if _, e := DecodeRelation(r); e == nil {
				t.Fatal("forged context accepted")
			}
		})
	}
	v.Version = 1
	if _, e := EncodeRelation(v, "alice"); e == nil {
		t.Fatal("new context silently entered v1")
	}
	item := SafeContextItem{Source: r.Event.Meta.ID, Observer: r.Event.Meta.Observer, Subject: r.Event.Subject, Kind: RelationshipMemory, Parents: r.Event.Meta.Parents, Text: r.Content.Text, Valid: r.Event.Meta.Valid}
	safe := SafeContext{items: []SafeContextItem{item}}
	if got, e := safe.Relations(); e != nil || len(got) != 1 || got[0].State.Context.Measures[0].Value != -.6 {
		t.Fatal("safe typed context lost", got, e)
	}
	safe.items[0].Parents = nil
	if _, e := safe.Relations(); e == nil {
		t.Fatal("safe context broadened provenance")
	}
}
