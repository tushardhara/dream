package postgres

import (
	"context"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/tushardhara/dream/app/graph"
	"github.com/tushardhara/dream/app/hws"
	"github.com/tushardhara/dream/core"
	"os"
	"testing"
	"time"
)

func TestRelationalCorrectionSnapshotReplayRevocation(t *testing.T) {
	dsn := os.Getenv("DREAM_TEST_DSN")
	if dsn == "" {
		t.Skip("mandatory migration-check supplies disposable PostgreSQL")
	}
	ctx := context.Background()
	db, e := pgxpool.New(ctx, dsn)
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	if e = Migrate(ctx, db); e != nil {
		t.Fatal(e)
	}
	store := New(db)
	m := runtimeManifest(t, "relational-context")
	if _, e = store.CreateRun(ctx, m); e != nil {
		t.Fatal(e)
	}
	scope, e := (hws.ViewRealm{Scope: m.Scope, Principal: "a"}).MemoryScope()
	if e != nil {
		t.Fatal(e)
	}
	mem := graph.MemoryService{Journal: store}
	rels := graph.RelationService{Memory: mem}
	source := snapshotMemory("relationship-source", 0)
	if _, e = mem.Put(ctx, scope, "source", 0, source, "simulation"); e != nil {
		t.Fatal(e)
	}
	from, to := core.Subject{Principal: "a"}, core.Subject{Principal: "b"}
	profile := core.RelationshipContext{Version: 1, Observer: "a", Other: "b", Types: []core.ID{"spouse"}, Valid: core.Interval{}, Details: []core.RelationshipDetail{{Kind: "view_of_other", Sources: []core.ID{source.Event.Meta.ID}}}, Measures: []core.RelationshipMeasure{{Kind: "trust", Value: -.7, Confidence: .6, Source: source.Event.Meta.ID}}}
	state := graph.RelationState{Version: 2, ID: "relationship", Kind: "edge", From: &from, To: &to, Types: profile.Types, Context: &profile}
	before := snapshotMemory("relationship-before", 0)
	before.Event.Meta.Parents = []core.ID{source.Event.Meta.ID}
	if _, e = rels.Put(ctx, scope, "before", 0, before, state, "simulation"); e != nil {
		t.Fatal(e)
	}
	key, e := store.CaptureSnapshot(ctx, m.Scope, "relationship-before", 1)
	if e != nil {
		t.Fatal(e)
	}
	q := graph.MemoryQuery{Scope: scope, Actor: "a", Purpose: "simulation", Subject: from, ValidAt: 5, KnownAt: 5, RecordedAsOf: time.Date(9999, 1, 1, 0, 0, 0, 0, time.UTC), Limit: 10}
	old, e := rels.Query(ctx, q, "relationship")
	if e != nil || len(old) != 1 || old[0].State.Context.Measures[0].Value != -.7 {
		t.Fatal("initial perspective", old, e)
	}
	after := snapshotMemory("relationship-after", 10)
	after.Event.Stream = before.Event.Stream
	after.Event.Meta.Parents = before.Event.Meta.Parents
	after.Supersedes = before.Event.Meta.ID
	profile.Measures[0].Value = .4
	if _, e = rels.Put(ctx, scope, "after", 1, after, state, "simulation"); e != nil {
		t.Fatal(e)
	}
	historical, e := rels.Query(ctx, q, "relationship")
	if e != nil || len(historical) != 1 || historical[0].Record.Event.Meta.ID != before.Event.Meta.ID {
		t.Fatal("correction erased as-of history", e)
	}
	q.KnownAt = 10
	current, e := rels.Query(ctx, q, "relationship")
	if e != nil || len(current) != 1 || current[0].State.Context.Measures[0].Value != .4 {
		t.Fatal("correction lost current perspective", e)
	}
	if _, e = store.ReadSnapshot(ctx, key); e != nil {
		t.Fatal("correction purged historical snapshot", e)
	}
	for _, kind := range []string{"snapshot", "export"} {
		if e = store.RegisterArtifact(ctx, scope.Owner, scope.Namespace, core.ID("relation-"+kind), kind, []core.ID{after.Event.Meta.ID}); e != nil {
			t.Fatal(e)
		}
	}
	q.Actor = "b"
	if got, e := rels.Query(ctx, q, "relationship"); e != nil || len(got) != 0 {
		t.Fatal("spouse acquired private state", e)
	}
	q.Actor = "a"
	revoke := source.Event
	revoke.Type = "revoke"
	revoke.Meta.ID = "revoke-relation"
	revoke.Meta.Rights = core.Rights{Resource: revoke.Meta.ID}
	if _, e = graph.RevokeMemory(ctx, store, scope, "revoke", 1, revoke, source.Event.Meta.ID); e != nil {
		t.Fatal(e)
	}
	if got, e := rels.Query(ctx, q, "relationship"); e != nil || len(got) != 0 {
		t.Fatal("revoked projection survived", e)
	}
	if _, e = store.ReadSnapshot(ctx, key); e == nil {
		t.Fatal("revoked snapshot survived")
	}
	if _, e = store.ReadReplay(ctx, key, 1); e == nil {
		t.Fatal("revoked replay survived")
	}
	for _, kind := range []string{"snapshot", "export"} {
		valid, e := store.ArtifactValid(ctx, scope.Owner, scope.Namespace, core.ID("relation-"+kind))
		if e != nil || valid {
			t.Fatal("revoked derivative survived", kind, e)
		}
	}
	// This run never consumed the relationship: its untouched genesis need not
	// be invalidated. The derived snapshot/replay/export above must be denied.
	if run, e := store.LoadRun(ctx, m.Scope); e != nil || run.State.Data != "" {
		t.Fatal("unrelated genesis damaged or relationship resurrected", e)
	}
}
