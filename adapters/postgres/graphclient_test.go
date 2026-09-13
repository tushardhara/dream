package postgres

import (
	"context"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/tushardhara/dream/app/graph"
	"github.com/tushardhara/dream/app/hws"
	"github.com/tushardhara/dream/core"
	"github.com/tushardhara/dream/examples/graphclient"
	"math"
	"os"
	"reflect"
	"testing"
	"time"
)

func TestGraphClientIntegration(t *testing.T) {
	dsn := os.Getenv("DREAM_TEST_DSN")
	if dsn == "" {
		t.Skip("mandatory make migration-check supplies fresh disposable PostgreSQL")
	}
	ctx := context.Background()
	admin, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	if err = Migrate(ctx, admin); err != nil {
		t.Fatal(err)
	}
	exec(t, admin, `CREATE ROLE dream_graphclient_test LOGIN PASSWORD 'disposable_graphclient' IN ROLE dream_writer`)
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.User = "dream_graphclient_test"
	cfg.ConnConfig.Password = "disposable_graphclient"
	db, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	report, err := graphclient.Run(ctx, New(db))
	if err != nil {
		t.Fatal(err)
	}
	if report.AliceView == report.BobView || !report.Corrected || !report.Revoked || report.Exported != 2 {
		t.Fatal(report)
	}
	if count(t, admin, `SELECT count(*) FROM dream.simulator_associations WHERE namespace='second-host'`) != 0 {
		t.Fatal("second host created simulation state")
	}
	t.Logf("independent perspectives preserved; correction=%v revocation=%v permitted exports=%d; no world/run", report.Corrected, report.Revoked, report.Exported)
}

func TestRelationIntegration(t *testing.T) {
	dsn := os.Getenv("DREAM_TEST_DSN")
	if dsn == "" {
		t.Skip("mandatory make migration-check supplies fresh disposable PostgreSQL")
	}
	ctx := context.Background()
	admin, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	if err = Migrate(ctx, admin); err != nil {
		t.Fatal(err)
	}
	exec(t, admin, `CREATE ROLE dream_relation_test LOGIN PASSWORD 'disposable_relation' IN ROLE dream_writer`)
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.User = "dream_relation_test"
	cfg.ConnConfig.Password = "disposable_relation"
	db, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := New(db)
	memory := graph.MemoryService{Journal: store}
	relations := graph.RelationService{Memory: memory}
	scope := graph.MemoryScope{Owner: "alice", Namespace: "relation-integration"}
	q := graph.MemoryQuery{Scope: scope, Actor: "alice", Purpose: scope.Namespace, Subject: core.Subject{Principal: "alice"}, ValidAt: 10, KnownAt: 10, RecordedAsOf: time.Date(9999, 1, 1, 0, 0, 0, 0, time.UTC), Limit: 10}
	put := func(r graph.MemoryRecord, v int64) {
		t.Helper()
		if _, err := memory.Put(ctx, scope, r.Event.Meta.ID, v, r, scope.Namespace); err != nil {
			t.Fatal(err)
		}
	}
	source := memoryRecord(scope.Namespace, "source")
	put(source, 0)
	loop := memoryRecord(scope.Namespace, "loop")
	loop.Content.Kind = graph.OpenLoopMemory
	expiry := core.LogicalTime(100)
	loop.Content.Expires = &expiry
	put(loop, 1)
	intent := memoryRecord(scope.Namespace, "intent")
	intent.Content.Kind = graph.IntentMemory
	intent.Content.Expires = &expiry
	put(intent, 2)
	a, b := core.Subject{Principal: "alice"}, core.Subject{Principal: "bob"}
	state := graph.RelationState{Version: 1, ID: "edge", Kind: "edge", From: &a, To: &b, Types: []core.ID{"colleague", "friend"}, Dimensions: []graph.RelationshipDimension{{Name: "perceived_support", Value: -.4, Confidence: .3}}, Memberships: []graph.Membership{{Member: a, Roles: []core.ID{"participant"}, Valid: core.Interval{Start: 0}}}, Commitments: []core.ID{"intent"}, OpenLoops: []core.ID{"loop"}, Patterns: []graph.RelationshipPattern{{Name: "repeated_delay", Confidence: .2, Evidence: []core.ID{"source"}}}}
	edge := memoryRecord(scope.Namespace, "edge-before")
	edge.Event.Meta.Parents = []core.ID{"source", "loop", "intent"}
	if _, err = relations.Put(ctx, scope, "edge-before", 3, edge, state, scope.Namespace); err != nil {
		t.Fatal(err)
	}
	before, err := relations.Query(ctx, q, "edge")
	if err != nil || len(before) != 1 || len(before[0].OpenLoopSignals) != 1 || before[0].State.Patterns[0].Confidence != .2 {
		t.Fatal(before, err)
	}
	modifiers, err := (hws.EdgeContextService{Relations: relations}).Query(ctx, q, "edge")
	if err != nil || len(modifiers) != 1 || modifiers[0].Observer != "alice" || modifiers[0].Dimensions[0].Value != -.4 || modifiers[0].Dimensions[0].Confidence != .3 || len(modifiers[0].Roles) != 1 {
		t.Fatal("edge modifiers", modifiers, err)
	}
	foreign := q
	foreign.Actor = "bob"
	if _, err = (hws.EdgeContextService{Relations: relations}).Query(ctx, foreign, "edge"); err == nil {
		t.Fatal("foreign belief used as own context")
	}
	invalid := state
	invalid.ID = "other-edge"
	correction := memoryRecord(scope.Namespace, "edge-after")
	correction.Event.Meta.Parents = edge.Event.Meta.Parents
	correction.Supersedes = "edge-before"
	if _, err = relations.Put(ctx, scope, "edge-after", 4, correction, invalid, scope.Namespace); err == nil {
		t.Fatal("correction changed entity identity")
	}
	invalid = state
	invalid.OpenLoops = []core.ID{"source"}
	if _, err = relations.Put(ctx, scope, "edge-after", 4, correction, invalid, scope.Namespace); err == nil {
		t.Fatal("ordinary source promoted to open loop")
	}
	state.Dimensions = []graph.RelationshipDimension{{Name: "perceived_support", Value: .2, Confidence: .5}}
	correction.Content.Learned[0].At = 30
	if _, err = relations.Put(ctx, scope, "edge-after", 4, correction, state, scope.Namespace); err != nil {
		t.Fatal(err)
	}
	old, err := relations.Query(ctx, q, "edge")
	if err != nil || len(old) != 1 || old[0].Record.Event.Meta.ID != "edge-before" {
		t.Fatal("historical edge", old, err)
	}
	q.KnownAt = 30
	after, err := relations.Query(ctx, q, "edge")
	if err != nil || len(after) != 1 {
		t.Fatal(after, err)
	}
	delta, err := graph.CompareRelation(before[0], after[0])
	if err != nil || len(delta.Dimensions) != 1 || math.Abs(delta.Dimensions[0].Change-.6) > 1e-12 {
		t.Fatal(delta, err)
	}
	entries, err := store.ReadMemory(ctx, scope)
	if err != nil {
		t.Fatal(err)
	}
	incremental, _ := graph.NewMemoryIndex(scope)
	for _, e := range entries {
		if err = incremental.Apply(e); err != nil {
			t.Fatal(err)
		}
	}
	rebuilt, err := graph.RebuildMemory(scope, entries)
	if err != nil {
		t.Fatal(err)
	}
	h1, _ := incremental.LogicalHash()
	h2, _ := rebuilt.LogicalHash()
	if h1 != h2 {
		t.Fatal("relation projection rebuild hash")
	}
	fresh := graph.RelationService{Memory: graph.MemoryService{Journal: New(db)}}
	recovered, err := fresh.Query(ctx, q, "edge")
	if err != nil || !reflect.DeepEqual(recovered, after) {
		t.Fatal("recovered projection differs", err)
	}
	revoke := command(scope.Namespace, "revoke-edge-source", 5)
	revoke.Event.Type = "revoke"
	if _, err = store.Revoke(ctx, revoke, "source"); err != nil {
		t.Fatal(err)
	}
	if out, err := fresh.Query(ctx, q, "edge"); err != nil || len(out) != 0 {
		t.Fatal("revocation did not deny edge", out, err)
	}
	if out, err := (hws.EdgeContextService{Relations: fresh}).Query(ctx, q, "edge"); err != nil || len(out) != 0 {
		t.Fatal("revoked context modifier", out, err)
	}
}
