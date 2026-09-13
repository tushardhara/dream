package postgres

import (
	"context"
	"errors"
	"os"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/tushardhara/dream/app/graph"
	"github.com/tushardhara/dream/core"
)

func memoryRecord(ns, id core.ID) graph.MemoryRecord {
	e := command(ns, id, 0).Event
	e.Type = graph.MemoryEventType
	e.Meta.Valid.Start = 0
	e.Meta.Rights.Grants = append(e.Meta.Rights.Grants, core.Grant{Actor: "alice", Recipient: "alice", Purpose: ns, Operation: core.Read}, core.Grant{Actor: "bob", Recipient: "bob", Purpose: ns, Operation: core.Read})
	return graph.MemoryRecord{Event: e, Content: graph.MemoryContent{Version: 1, Kind: graph.EpisodicMemory, Text: "synthetic private memory " + string(id), Salience: .8, HalfLife: 10, Learned: []graph.Learned{{Actor: "alice", At: 5}, {Actor: "bob", At: 20}}}}
}
func TestMemoryIntegration(t *testing.T) {
	dsn := os.Getenv("DREAM_TEST_DSN")
	if dsn == "" {
		t.Skip("mandatory make migration-check supplies disposable Postgres")
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
	exec(t, admin, `CREATE ROLE dream_memory_test LOGIN PASSWORD 'disposable_memory' IN ROLE dream_writer`)
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.User = "dream_memory_test"
	cfg.ConnConfig.Password = "disposable_memory"
	db, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := New(db)
	svc := graph.MemoryService{Journal: store}
	scope := graph.MemoryScope{Owner: "alice", Namespace: "memory-integration"}
	query := graph.MemoryQuery{Scope: scope, Actor: "alice", Purpose: scope.Namespace, Subject: core.Subject{Principal: "alice"}, ValidAt: 10, KnownAt: 10, RecordedAsOf: time.Date(9999, 1, 1, 0, 0, 0, 0, time.UTC), Limit: 50}
	put := func(r graph.MemoryRecord, v int64) graph.AppendResult {
		t.Helper()
		out, err := svc.Put(ctx, scope, r.Event.Meta.ID, v, r, scope.Namespace)
		if err != nil {
			t.Fatal(err)
		}
		return out
	}
	a := memoryRecord(scope.Namespace, "source")
	first := put(a, 0)
	t.Run("ScopedIdempotencyAndTrustedRecordedAt", func(t *testing.T) {
		a.Event.Meta.RecordedAt = time.Unix(9, 0)
		again := put(a, 0)
		if first != again {
			t.Fatal("wall time changed retry")
		}
		var wg sync.WaitGroup
		errs := make(chan error, 8)
		for i := 0; i < 8; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				r, e := svc.Put(ctx, scope, "source", 0, a, scope.Namespace)
				if e == nil && r != first {
					e = errors.New("receipt changed")
				}
				errs <- e
			}()
		}
		wg.Wait()
		close(errs)
		for e := range errs {
			if e != nil {
				t.Fatal(e)
			}
		}
		changed := a
		changed.Content.Text = "conflict"
		if _, err = svc.Put(ctx, scope, "source", 0, changed, scope.Namespace); !errors.Is(err, ErrIdempotency) {
			t.Fatal("idempotency conflict", err)
		}
		entries, e := store.ReadMemory(ctx, scope)
		if e != nil || len(entries) != 1 || entries[0].Event.Meta.RecordedAt.Equal(a.Event.Meta.RecordedAt) {
			t.Fatal("recorded time not database stamped", entries, e)
		}
		other := scope
		other.Namespace = "memory-other"
		if _, e = svc.Put(ctx, other, "source", 0, a, scope.Namespace); e != nil {
			t.Fatal("namespace idempotency collision", e)
		}
	})
	b := memoryRecord(scope.Namespace, "summary")
	b.Content.Kind = graph.NarrativeMemory
	b.Event.Meta.Parents = []core.ID{"source"}
	put(b, 1)
	t.Run("TemporalAudienceAndCachedRead", func(t *testing.T) {
		q := query
		q.Actor = "bob"
		q.KnownAt = 19
		out, e := svc.Retrieve(ctx, q)
		if e != nil || len(out) != 0 {
			t.Fatal("future disclosure", out, e)
		}
		q.KnownAt = 20
		out, e = svc.Retrieve(ctx, q)
		if e != nil || len(out) != 2 {
			t.Fatal("late disclosure missing", out, e)
		}
		entries, e := store.ReadMemory(ctx, scope)
		if e != nil {
			t.Fatal(e)
		}
		q.RecordedAsOf = entries[0].Event.Meta.RecordedAt
		out, e = svc.Retrieve(ctx, q)
		if e != nil || len(out) != 1 || out[0].Record.Event.Meta.ID != "source" {
			t.Fatal("recorded-as-of", out, e)
		}
		q = query
		q.Actor = "mallory"
		out, e = svc.Retrieve(ctx, q)
		if e != nil || len(out) != 0 {
			t.Fatal("restricted audience", out, e)
		}
	})
	t.Run("ObserverOwnedReferenceIsolation", func(t *testing.T) {
		for _, owner := range []core.ID{"alice", "bob"} {
			scope := graph.MemoryScope{Owner: owner, Namespace: "memory-references"}
			r := memoryRecord(scope.Namespace, "same-id")
			r.Event.Meta.Observer = owner
			r.Event.Subject = core.Subject{Reference: &core.NonparticipantReference{Observer: owner, LocalID: "same-local-id"}}
			r.Content.Text = "synthetic perspective of " + string(owner)
			if _, e := svc.Put(ctx, scope, "same-key", 0, r, scope.Namespace); e != nil {
				t.Fatal(e)
			}
			q := query
			q.Scope = scope
			q.Actor = owner
			q.Purpose = scope.Namespace
			q.Subject = r.Event.Subject
			q.KnownAt = 20
			out, e := svc.Retrieve(ctx, q)
			if e != nil || len(out) != 1 || out[0].Record.Content.Text != r.Content.Text {
				t.Fatal("observer references pooled", out, e)
			}
			foreign := "alice"
			if owner == "alice" {
				foreign = "bob"
			}
			q.Subject.Reference = &core.NonparticipantReference{Observer: core.ID(foreign), LocalID: "same-local-id"}
			if _, e = svc.Retrieve(ctx, q); e == nil {
				t.Fatal("foreign reference query accepted")
			}
		}
	})
	t.Run("AppendNegativesAreAtomic", func(t *testing.T) {
		c := memoryRecord(scope.Namespace, "bad")
		c.Event.Meta.Parents = []core.ID{"missing"}
		if _, e := svc.Put(ctx, scope, "bad", 2, c, scope.Namespace); e == nil {
			t.Fatal("unknown evidence accepted")
		}
		c.Event.Meta.Parents = []core.ID{"source"}
		c.Content.Learned[1].At = 19
		if _, e := svc.Put(ctx, scope, "bad", 2, c, scope.Namespace); e == nil {
			t.Fatal("future source knowledge accepted")
		}
		c.Content.Learned[1].At = 20
		c.Event.Meta.Rights.Grants = append(c.Event.Meta.Rights.Grants, core.Grant{Actor: "mallory", Recipient: "mallory", Purpose: scope.Namespace, Operation: core.Read})
		if _, e := svc.Put(ctx, scope, "bad", 2, c, scope.Namespace); e == nil {
			t.Fatal("rights broadened")
		}
		if count(t, admin, `SELECT count(*) FROM dream.events WHERE namespace=$1`, scope.Namespace) != 2 {
			t.Fatal("failed append leaked journal state")
		}
		// Inject failure after payload insert, proving the full transaction rolls back.
		exec(t, admin, `CREATE FUNCTION dream.fail_memory() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.namespace='memory-integration' AND NEW.event_id='rollback' THEN RAISE EXCEPTION 'synthetic rollback'; END IF; RETURN NEW; END $$; CREATE TRIGGER fail_memory BEFORE INSERT ON dream.outbox FOR EACH ROW EXECUTE FUNCTION dream.fail_memory()`)
		c = memoryRecord(scope.Namespace, "rollback")
		if _, e := svc.Put(ctx, scope, "rollback", 2, c, scope.Namespace); e == nil {
			t.Fatal("fault not applied")
		}
		exec(t, admin, `DROP TRIGGER fail_memory ON dream.outbox; DROP FUNCTION dream.fail_memory()`)
		for _, table := range []string{"events", "private_payloads", "command_results", "outbox"} {
			column := "event_id"
			if table == "events" {
				column = "id"
			}
			if count(t, admin, `SELECT count(*) FROM dream.`+table+` WHERE namespace=$1 AND `+column+`='rollback'`, scope.Namespace) != 0 {
				t.Fatal("partial transaction", table)
			}
		}
	})
	t.Run("CorrectionInvalidatesMaterialCache", func(t *testing.T) {
		out, cache, _, e := svc.RetrieveCached(ctx, query, graph.MemoryCache{})
		if e != nil || len(out) != 2 {
			t.Fatal(out, e)
		}
		c := memoryRecord(scope.Namespace, "corrected")
		c.Supersedes = "source"
		c.Content.Learned[0].At = 30
		put(c, 2)
		historical := query
		out, _, hit, e := svc.RetrieveCached(ctx, historical, cache)
		if e != nil || hit || len(out) != 1 || out[0].Record.Event.Meta.ID != "source" {
			t.Fatal("material invalidation", out, hit, e)
		}
		q := query
		q.KnownAt = 30
		out, e = svc.Retrieve(ctx, q)
		if e != nil || len(out) != 1 || out[0].Record.Event.Meta.ID != "corrected" {
			t.Fatal("correction query", out, e)
		}
	})
	t.Run("ImmediatePurgeCacheAndRebuild", func(t *testing.T) {
		query.KnownAt = 30
		out, cache, _, e := svc.RetrieveCached(ctx, query, graph.MemoryCache{})
		if e != nil || len(out) != 1 {
			t.Fatal(out, e)
		}
		before, e := store.ReadMemory(ctx, scope)
		if e != nil {
			t.Fatal(e)
		}
		incremental, e := graph.RebuildMemory(scope, before)
		if e != nil {
			t.Fatal(e)
		}
		revoke := command(scope.Namespace, "revoke-memory", 3)
		revoke.Event.Type = "revoke"
		if _, e = store.Revoke(ctx, revoke, "source"); e != nil {
			t.Fatal(e)
		}
		// No projector or invalidation worker runs between revoke and cached access.
		out, _, hit, e := svc.RetrieveCached(ctx, query, cache)
		if e != nil || hit || len(out) != 0 {
			t.Fatal("purged cache leaked", out, hit, e)
		}
		if count(t, admin, `SELECT count(*) FROM dream.private_payloads WHERE namespace=$1`, scope.Namespace) != 0 {
			t.Fatal("payloads not purged")
		}
		after, e := store.ReadMemory(ctx, scope)
		if e != nil || len(after) != 3 {
			t.Fatal(after, e)
		}
		for _, entry := range after {
			if !entry.Revoked || entry.Content != nil {
				t.Fatal("purge snapshot", entry)
			}
			if e = incremental.Apply(entry); e != nil {
				t.Fatal(e)
			}
		}
		for _, old := range before {
			if e = incremental.Apply(old); e != nil {
				t.Fatal(e)
			}
		}
		rebuilt, e := graph.RebuildMemory(scope, after)
		if e != nil {
			t.Fatal(e)
		}
		h1, _ := incremental.LogicalHash()
		h2, _ := rebuilt.LogicalHash()
		if h1 != h2 {
			t.Fatal("replay resurrected purged data")
		}
		if _, e = svc.Put(ctx, scope, "source", 0, a, scope.Namespace); e == nil {
			t.Fatal("revoked idempotent retry accepted")
		}
		if e = store.ResetProjections(ctx); e != nil {
			t.Fatal(e)
		}
		for {
			n, e := store.Project(ctx, "memory-rebuild", 100)
			if e != nil {
				t.Fatal(e)
			}
			if n == 0 {
				break
			}
		}
		again, e := store.ReadMemory(ctx, scope)
		if e != nil || !reflect.DeepEqual(after, again) {
			t.Fatal("projection rebuild changed canonical memory", e)
		}
	})
}
