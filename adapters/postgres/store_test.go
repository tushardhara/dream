package postgres

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/tushardhara/dream/app/graph"
	"github.com/tushardhara/dream/core"
)

func command(namespace, id core.ID, expected int64) graph.AppendCommand {
	return graph.AppendCommand{Actor: "alice", Namespace: namespace, Operation: "append", Key: id, ExpectedVersion: expected, Class: graph.PrivatePayload, Payload: graph.Payload{Version: 1, Text: "synthetic private observation"}, Event: core.Event{Version: 1, Stream: "journal", Type: "observation", Subject: core.Subject{Principal: "alice"}, OccurredAt: 5, Meta: core.Metadata{ID: id, Observer: "alice", Source: "alice", Sensitivity: core.Restricted, Confidence: .5, Valid: core.Interval{Start: 10}, RecordedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), Rights: core.Rights{Resource: id}}}}
}
func exec(t *testing.T, db *pgxpool.Pool, sql string, args ...any) {
	t.Helper()
	if _, err := db.Exec(context.Background(), sql, args...); err != nil {
		t.Fatal(err)
	}
}
func count(t *testing.T, db *pgxpool.Pool, sql string, args ...any) int {
	t.Helper()
	var n int
	if err := db.QueryRow(context.Background(), sql, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}
func TestIntegration(t *testing.T) {
	dsn := os.Getenv("DREAM_TEST_DSN")
	if dsn == "" {
		t.Skip("real Postgres NOT RUN here; mandatory make migration-check provisions its own disposable instance")
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
	if err = Migrate(ctx, admin); err != nil {
		t.Fatalf("migration rerun: %v", err)
	}
	exec(t, admin, `CREATE ROLE dream_test_writer LOGIN PASSWORD 'disposable_writer' IN ROLE dream_writer; CREATE ROLE alice LOGIN PASSWORD 'disposable_actor' IN ROLE dream_actor;`)
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.User = "dream_test_writer"
	cfg.ConnConfig.Password = "disposable_writer"
	cfg.MaxConns = 8
	db, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := New(db)
	t.Run("AtomicIdempotencyAndConcurrency", func(t *testing.T) {
		c := command("duplicates", "event", 0)
		var wg sync.WaitGroup
		results := make(chan graph.AppendResult, 8)
		errs := make(chan error, 8)
		for i := 0; i < 8; i++ {
			wg.Add(1)
			go func() { defer wg.Done(); r, e := store.Append(ctx, c); results <- r; errs <- e }()
		}
		wg.Wait()
		close(results)
		close(errs)
		for e := range errs {
			if e != nil {
				t.Fatal(e)
			}
		}
		var first graph.AppendResult
		for r := range results {
			if first.Sequence == 0 {
				first = r
			}
			if r != first {
				t.Fatal("duplicate changed result")
			}
		}
		if n := count(t, admin, `SELECT count(*) FROM dream.events WHERE namespace='duplicates'`); n != 1 {
			t.Fatal(n)
		}
		c.Payload.Text = "conflict"
		if _, err := store.Append(ctx, c); !errors.Is(err, ErrIdempotency) {
			t.Fatal(err)
		}
		c = command("versions", "a", 0)
		d := command("versions", "b", 0)
		errs = make(chan error, 2)
		for _, v := range []graph.AppendCommand{c, d} {
			wg.Add(1)
			go func(v graph.AppendCommand) { defer wg.Done(); _, e := store.Append(ctx, v); errs <- e }(v)
		}
		wg.Wait()
		close(errs)
		success, conflict := 0, 0
		for e := range errs {
			if e == nil {
				success++
			} else if errors.Is(e, ErrVersion) {
				conflict++
			} else {
				t.Fatal(e)
			}
		}
		if success != 1 || conflict != 1 {
			t.Fatalf("success=%d conflict=%d", success, conflict)
		}
		if count(t, admin, `SELECT count(*) FROM dream.command_results WHERE namespace='versions'`) != 1 || count(t, admin, `SELECT count(*) FROM dream.outbox WHERE namespace='versions'`) != 1 {
			t.Fatal("non-atomic append")
		}
	})
	t.Run("InterruptedAppendRollsBack", func(t *testing.T) {
		exec(t, admin, `CREATE FUNCTION dream.interrupt_append() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.namespace='interrupt' THEN PERFORM pg_sleep(2); END IF; RETURN NEW; END $$; CREATE TRIGGER interrupt_append BEFORE INSERT ON dream.outbox FOR EACH ROW EXECUTE FUNCTION dream.interrupt_append();`)
		short, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
		defer cancel()
		if _, err := store.Append(short, command("interrupt", "event", 0)); err == nil {
			t.Fatal("interrupted append succeeded")
		}
		// Acquire the journal lock on a fresh connection, ensuring rollback completed.
		tx, err := store.begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		tx.Rollback(ctx)
		for _, table := range []string{"events", "streams", "private_payloads", "command_results", "outbox"} {
			if count(t, admin, `SELECT count(*) FROM dream.`+table+` WHERE namespace='interrupt'`) != 0 {
				t.Fatal("partial append in", table)
			}
		}
		exec(t, admin, `DROP TRIGGER interrupt_append ON dream.outbox; DROP FUNCTION dream.interrupt_append()`)
		if _, err := store.Append(ctx, command("interrupt", "event", 0)); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("TemporalCorrectionProjectionRecovery", func(t *testing.T) {
		old := command("temporal", "old", 0)
		if _, err := store.Append(ctx, old); err != nil {
			t.Fatal(err)
		}
		var before time.Time
		if err := admin.QueryRow(ctx, "SELECT clock_timestamp()").Scan(&before); err != nil {
			t.Fatal(err)
		}
		correction := command("temporal", "correction", 1)
		correction.Supersedes = "old"
		correction.Event.OccurredAt = 2
		correction.Payload.Text = "late correction"
		if _, err := store.Append(ctx, correction); err != nil {
			t.Fatal(err)
		}
		// Force failure after rows have been written but before the checkpoint commit.
		exec(t, admin, `CREATE FUNCTION dream.fail_checkpoint() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'injected checkpoint interruption'; END $$; CREATE TRIGGER fail_checkpoint BEFORE UPDATE ON dream.projection_checkpoints FOR EACH ROW EXECUTE FUNCTION dream.fail_checkpoint()`)
		if _, err := store.Project(ctx, "default", 1000); err == nil {
			t.Fatal("injected failure ignored")
		}
		if count(t, admin, "SELECT count(*) FROM dream.projections") != 0 {
			t.Fatal("projection state escaped rollback")
		}
		if count(t, admin, "SELECT count(*) FROM dream.projection_checkpoints") != 0 {
			t.Fatal("checkpoint escaped rollback")
		}
		exec(t, admin, `DROP TRIGGER fail_checkpoint ON dream.projection_checkpoints; DROP FUNCTION dream.fail_checkpoint()`)
		if _, err := store.Project(ctx, "default", 1000); err != nil {
			t.Fatal(err)
		}
		if n, err := store.Project(ctx, "default", 1000); err != nil || n != 0 {
			t.Fatalf("duplicate delivery: %d %v", n, err)
		}
		id, err := store.Query(ctx, "alice", "temporal", old.Event.Subject, 10, before)
		if err != nil || id != "old" {
			t.Fatalf("historical read: %s %v", id, err)
		}
		id, err = store.Query(ctx, "alice", "temporal", old.Event.Subject, 10, time.Now().Add(time.Second))
		if err != nil || id != "correction" {
			t.Fatalf("correction: %s %v", id, err)
		}
		if _, err = store.Query(ctx, "alice", "temporal", old.Event.Subject, 9, time.Now().Add(time.Second)); !errors.Is(err, pgx.ErrNoRows) {
			t.Fatal("invalid valid-time visibility", err)
		}
		if err = store.ResetProjections(ctx); err != nil {
			t.Fatal(err)
		}
		if _, err = store.Project(ctx, "default", 1000); err != nil {
			t.Fatal(err)
		}
		id, err = store.Query(ctx, "alice", "temporal", old.Event.Subject, 10, before)
		if err != nil || id != "old" {
			t.Fatal("rebuild lost history", err)
		}
	})
	t.Run("RevocationPurgeAndArtifacts", func(t *testing.T) {
		root := command("revoke", "root", 0)
		root.Class = graph.ObservablePayload
		root.Event.Meta.Sensitivity = core.Public
		if _, err := store.Append(ctx, root); err != nil {
			t.Fatal(err)
		}
		child := command("revoke", "child", 1)
		child.Class = graph.PrivatePayload
		child.Event.Meta.Parents = []core.ID{"root"}
		if _, err := store.Append(ctx, child); err != nil {
			t.Fatal(err)
		}
		for _, kind := range []string{"snapshot", "export"} {
			if err := store.RegisterArtifact(ctx, "alice", "revoke", core.ID(kind), kind, []core.ID{"child"}); err != nil {
				t.Fatal(err)
			}
		}
		research := command("revoke", "research-child", 2)
		research.Class = graph.ResearchPayload
		research.Event.Meta.Parents = []core.ID{"root"}
		if _, err := store.Append(ctx, research); err != nil {
			t.Fatal(err)
		}
		revoke := command("revoke", "tombstone", 3)
		revoke.Event.Type = "revoke"
		revoke.Operation = "revoke"
		result, err := store.Revoke(ctx, revoke, "root")
		if err != nil {
			t.Fatal(err)
		}
		again, err := store.Revoke(ctx, revoke, "root")
		if err != nil || result != again {
			t.Fatal("revoke retry", err)
		}
		for _, id := range []core.ID{"root", "child", "research-child", "tombstone"} {
			if _, err := store.ReadPayload(ctx, "alice", "revoke", id); !errors.Is(err, pgx.ErrNoRows) {
				t.Fatal("revoked payload readable", id, err)
			}
		}
		for _, kind := range []string{"snapshot", "export"} {
			valid, err := store.ArtifactValid(ctx, "alice", "revoke", core.ID(kind))
			if err != nil || valid {
				t.Fatal("artifact not invalidated", err)
			}
		}
		if err = store.RegisterArtifact(ctx, "alice", "revoke", "restore", "snapshot", []core.ID{"child"}); !errors.Is(err, ErrRevoked) {
			t.Fatal("snapshot resurrected source", err)
		}
		if err = store.ResetProjections(ctx); err != nil {
			t.Fatal(err)
		}
		if _, err = store.Project(ctx, "default", 1000); err != nil {
			t.Fatal(err)
		}
		if count(t, admin, "SELECT count(*) FROM dream.projections WHERE namespace='revoke'") != 0 {
			t.Fatal("projection resurrected revoked source")
		}
		next := command("revoke", "resurrect", 4)
		next.Event.Meta.Parents = []core.ID{"child"}
		if _, err = store.Append(ctx, next); !errors.Is(err, ErrRevoked) {
			t.Fatal("derived source resurrected", err)
		}
		if count(t, admin, "SELECT count(*) FROM dream.events WHERE namespace='revoke'") != 4 {
			t.Fatal("audit envelopes lost")
		}
	})
	t.Run("PayloadClassCannotBroaden", func(t *testing.T) {
		source := command("class-boundary", "source", 0)
		source.Class = graph.ResearchPayload
		source.Event.Meta.Sensitivity = core.Public
		if _, err := store.Append(ctx, source); err != nil {
			t.Fatal(err)
		}
		child := command("class-boundary", "leak", 1)
		child.Class = graph.ObservablePayload
		child.Event.Meta.Sensitivity = core.Public
		child.Event.Meta.Parents = []core.ID{"source"}
		if _, err := store.Append(ctx, child); err == nil {
			t.Fatal("research payload derivation became actor-observable")
		}
	})
	t.Run("DatabaseGrantsAndActorIsolation", func(t *testing.T) {
		cfg, err := pgxpool.ParseConfig(dsn)
		if err != nil {
			t.Fatal(err)
		}
		cfg.ConnConfig.User = "alice"
		cfg.ConnConfig.Password = "disposable_actor"
		actor, err := pgxpool.NewWithConfig(ctx, cfg)
		if err != nil {
			t.Fatal(err)
		}
		defer actor.Close()
		for _, table := range []string{"private_payloads", "research_payloads", "events", "command_results", "artifact_refs"} {
			_, err := actor.Exec(ctx, "SELECT * FROM dream."+table)
			if err == nil || !strings.Contains(err.Error(), "permission denied") {
				t.Fatal("actor can read", table, err)
			}
		}
		if _, err := actor.Exec(ctx, "SET ROLE dream_writer"); err == nil {
			t.Fatal("actor escalated writer role")
		}
		for _, owner := range []core.ID{"alice", "bob"} {
			c := command("observable", owner, 0)
			c.Actor = owner
			c.Event.Meta.Observer = owner
			c.Event.Meta.Source = owner
			c.Event.Subject.Principal = owner
			c.Class = graph.ObservablePayload
			c.Event.Meta.Sensitivity = core.Public
			if _, err := store.Append(ctx, c); err != nil {
				t.Fatal(err)
			}
		}
		if n := count(t, actor, "SELECT count(*) FROM dream.observable_payloads WHERE namespace='observable'"); n != 1 {
			t.Fatal("actor row isolation failed", n)
		}
		if _, err := db.Exec(ctx, "UPDATE dream.events SET occurred_at=0"); err == nil {
			t.Fatal("writer can rewrite immutable events")
		}
	})
	t.Run("ScopedKeysAndWallTimeIndependentDigest", func(t *testing.T) {
		a := command("hash", "a", 0)
		d1, _ := a.Digest()
		a.Event.Meta.RecordedAt = a.Event.Meta.RecordedAt.Add(time.Hour)
		d2, _ := a.Digest()
		if d1 != d2 {
			t.Fatal("wall time changed logical digest")
		}
		a.Event.OccurredAt++
		d3, _ := a.Digest()
		if d1 == d3 {
			t.Fatal("logical time excluded")
		}
		for i := 0; i < 3; i++ {
			c := command(core.ID(fmt.Sprintf("scope%d", i)), "same-key", 0)
			if _, err := store.Append(ctx, c); err != nil {
				t.Fatal(err)
			}
		}
	})
}
func TestPayloadVersionDispatch(t *testing.T) {
	for _, raw := range []string{`{"version":2,"text":"future"}`, `{"version":1,"text":"ok","extra":1}`, `{"version":1,"text":"ok","text":"ok"}`} {
		if _, err := DecodePayload([]byte(raw)); err == nil {
			t.Fatal("unknown/noncanonical payload accepted")
		}
	}
	if _, err := DecodePayload([]byte(`{"version":1,"text":"ok"}`)); err != nil {
		t.Fatal(err)
	}
}
