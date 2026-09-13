package postgres

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/tushardhara/dream/app/hws"
	"github.com/tushardhara/dream/core"
	"github.com/tushardhara/dream/migrations"
	"github.com/tushardhara/dream/simulator"
	"github.com/tushardhara/dream/simulator/dynamics"
	rt "github.com/tushardhara/dream/simulator/runtime"
	"github.com/tushardhara/dream/simulator/scenario"
	"os"
	osexec "os/exec"
	"strings"
	"sync"
	"testing"
	"time"
)

type appraisalPerception struct{}

func (appraisalPerception) Perceive(i rt.Input) (dynamics.Perceived, error) {
	return dynamics.Perceived{Event: i.ID, Actor: i.Actor, OccurredAt: i.At, LearnedAt: i.At, Confidence: .8, Signals: dynamics.Signals{Effort: .5, OtherNeed: .7}, Rights: core.Rights{Resource: i.ID, Grants: []core.Grant{{Actor: i.Actor, Recipient: i.Actor, Purpose: "simulation", Operation: core.Read}, {Actor: i.Actor, Recipient: i.Actor, Purpose: "simulation", Operation: core.Derive}}}}, nil
}

type futureClock struct{}

func (futureClock) Now() time.Time { return time.Date(9999, 1, 1, 0, 0, 0, 0, time.UTC) }

type runtimeFake struct {
	block bool
	fail  bool
}

func (f runtimeFake) Transition(s rt.State, i rt.Input, c rt.Clock, r *rt.Random) (rt.Output, error) {
	n, err := r.Draw("choice.v1")
	if err != nil {
		return rt.Output{}, err
	}
	if f.block {
		fmt.Println("DREAM_CHILD_READY")
		select {}
	}
	if f.fail {
		return rt.Output{}, errors.New("fake failure")
	}
	return rt.Output{Data: fmt.Sprintf("%s/%s/%d/%d", s.Data, i.ID, c.Now(), n)}, nil
}
func runtimeManifest(t testing.TB, run string) hws.Manifest {
	t.Helper()
	s := scenario.Scenario{Version: 1, World: scenario.World{ID: "runtime-world", Seed: 42, Horizon: 100}, Public: scenario.Public{Humans: []scenario.Human{{ID: "a", Name: "Synthetic A", Age: 30}, {ID: "b", Name: "Synthetic B", Age: 40}}}, Actors: []scenario.Actor{{ID: "a"}, {ID: "b"}}, Future: []scenario.Scheduled{{ID: "e2", At: 20, Kind: "observation", Actor: "a", Text: "two"}, {ID: "e1", At: 20, Kind: "observation", Actor: "b", Text: "one"}, {ID: "e3", At: 40, Kind: "observation", Actor: "b", Text: "three"}}}
	g, err := s.Genesis(rt.Capabilities())
	if err != nil {
		t.Fatal(err)
	}
	return hws.Manifest{Version: 1, Scope: hws.Scope{Actor: "operator", Namespace: "runtime-test", World: s.World.ID, Branch: simulator.BranchID(run), Run: simulator.RunID(run)}, Genesis: g, Budget: rt.Budgets{Steps: 100, Events: 100, Horizon: 100}, MaxDuration: time.Hour}
}
func TestRuntimeIntegration(t *testing.T) {
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
	exec(t, admin, `CREATE ROLE dream_runtime_test LOGIN PASSWORD 'disposable_runtime' IN ROLE dream_writer`)
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.User = "dream_runtime_test"
	cfg.ConnConfig.Password = "disposable_runtime"
	db, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := New(db)
	svc := hws.Runtime{Store: store, Handler: runtimeFake{}}
	start := func(name string) (hws.Manifest, hws.Lease) {
		t.Helper()
		m := runtimeManifest(t, name)
		if _, err := store.CreateRun(ctx, m); err != nil {
			t.Fatal(err)
		}
		l, err := store.Acquire(ctx, m.Scope, "worker", time.Minute)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := svc.Execute(ctx, m.Scope, l, "resume", rt.Command{Kind: "resume"}); err != nil {
			t.Fatal(err)
		}
		return m, l
	}

	t.Run("ForwardUpgradePreservesJournal", func(t *testing.T) {
		exec(t, admin, `CREATE DATABASE dream_runtime_upgrade`)
		upgradeCfg, err := pgxpool.ParseConfig(dsn)
		if err != nil {
			t.Fatal(err)
		}
		upgradeCfg.ConnConfig.Database = "dream_runtime_upgrade"
		old, err := pgxpool.NewWithConfig(ctx, upgradeCfg)
		if err != nil {
			t.Fatal(err)
		}
		defer old.Close()
		exec(t, old, migrations.Initial)
		if _, err = New(old).Append(ctx, command("upgrade", "legacy", 0)); err != nil {
			t.Fatal(err)
		}
		if err = Migrate(ctx, old); err != nil {
			t.Fatal(err)
		}
		if err = Migrate(ctx, old); err != nil {
			t.Fatal(err)
		}
		if count(t, old, `SELECT count(*) FROM dream.schema_versions WHERE version IN(1,2)`) != 2 || count(t, old, `SELECT count(*) FROM dream.events WHERE namespace='upgrade' AND id='legacy'`) != 1 {
			t.Fatal("migration lost ledger or event")
		}
	})
	t.Run("RestrictedRuntimeGrants", func(t *testing.T) {
		for _, role := range []string{"dream_actor", "dream_private_reader", "dream_research_reader"} {
			for _, table := range []string{"runtime_heads", "runtime_payloads", "runtime_operations", "runtime_lease_audit"} {
				if count(t, admin, `SELECT has_table_privilege($1,$2,'SELECT')::int`, role, "dream."+table) != 0 {
					t.Fatalf("%s reads %s", role, table)
				}
			}
		}
	})
	t.Run("CreateIdempotencyAndScope", func(t *testing.T) {
		m, l := start("create")
		snap, err := store.CreateRun(ctx, m)
		if err != nil || snap.Revision != 2 {
			t.Fatalf("retry %v %+v", err, snap)
		}
		changed := m
		changed.MaxDuration = time.Minute
		if _, err = store.CreateRun(ctx, changed); !errors.Is(err, hws.ErrCommand) {
			t.Fatal("manifest conflict", err)
		}
		wrong := m.Scope
		wrong.Branch = "different"
		if _, err = store.LoadRun(ctx, wrong); err == nil {
			t.Fatal("scope escape")
		}
		if _, err = svc.Execute(ctx, m.Scope, l, "resume", rt.Command{Kind: "cancel"}); !errors.Is(err, hws.ErrCommand) {
			t.Fatal("key conflict", err)
		}
	})
	t.Run("LeaseReclaimFencingRenewAndOverlap", func(t *testing.T) {
		m, old := start("lease")
		if _, err = store.Acquire(ctx, m.Scope, "other", time.Minute); !errors.Is(err, hws.ErrLease) {
			t.Fatal("overlap", err)
		}
		renewed, err := store.Renew(ctx, m.Scope, old, time.Minute)
		if err != nil || renewed.Fence != old.Fence {
			t.Fatal("renew", err)
		}
		exec(t, admin, `UPDATE dream.runtime_heads SET expires=clock_timestamp()-interval '1 second' WHERE run=$1`, m.Scope.Run)
		next, err := store.Acquire(ctx, m.Scope, "replacement", time.Minute)
		if err != nil || next.Fence <= old.Fence {
			t.Fatal("reclaim", err)
		}
		if _, err = svc.Execute(ctx, m.Scope, old, "stale", rt.Command{Kind: "step"}); !errors.Is(err, hws.ErrLease) {
			t.Fatal("stale writer", err)
		}
		if _, err = store.Renew(ctx, m.Scope, old, time.Minute); !errors.Is(err, hws.ErrLease) {
			t.Fatal("stale renew", err)
		}
		if _, err = svc.Execute(ctx, m.Scope, next, "fresh", rt.Command{Kind: "step"}); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("SameHolderFencingAndImmutableCommit", func(t *testing.T) {
		m, old := start("same-holder")
		exec(t, admin, `UPDATE dream.runtime_heads SET expires=clock_timestamp()-interval '1 second' WHERE run='same-holder'`)
		fresh, err := store.Acquire(ctx, m.Scope, old.Holder, time.Minute)
		if err != nil || fresh.Fence <= old.Fence {
			t.Fatal(err)
		}
		if _, err = svc.Execute(ctx, m.Scope, old, "stale", rt.Command{Kind: "step"}); !errors.Is(err, hws.ErrLease) {
			t.Fatal("same-holder stale fence accepted", err)
		}
		snap, err := store.LoadRun(ctx, m.Scope)
		if err != nil {
			t.Fatal(err)
		}
		next, tr, done, err := rt.Apply(snap.State, rt.Command{Kind: "step"}, runtimeFake{})
		if err != nil {
			t.Fatal(err)
		}
		next.Budget.Steps++
		if _, err = store.CommitRun(ctx, hws.Commit{Scope: m.Scope, Lease: fresh, Key: "tamper", Command: rt.Command{Kind: "step"}, Expected: snap.Revision, State: next, Transition: tr, Done: done}); err == nil {
			t.Fatal("budget mutation accepted")
		}
	})
	t.Run("SimultaneousDuplicateAndVersion", func(t *testing.T) {
		m, l := start("parallel")
		var wg sync.WaitGroup
		for range 8 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_, e := svc.Execute(ctx, m.Scope, l, "one-step", rt.Command{Kind: "step"})
				if e != nil {
					t.Error(e)
				}
			}()
		}
		wg.Wait()
		snap, err := store.LoadRun(ctx, m.Scope)
		if err != nil || snap.State.Step != 1 {
			t.Fatal("double step", err, snap.State.Step)
		}
		if n := count(t, db, `SELECT count(*) FROM dream.runtime_operations WHERE run='parallel' AND key='one-step'`); n != 1 {
			t.Fatal(n)
		}
		next, tr, done, err := rt.Apply(snap.State, rt.Command{Kind: "step"}, runtimeFake{})
		if err != nil {
			t.Fatal(err)
		}
		c := hws.Commit{Scope: m.Scope, Lease: l, Key: "v1", Command: rt.Command{Kind: "step"}, Expected: snap.Revision, State: next, Transition: tr, Done: done}
		if _, err = store.CommitRun(ctx, c); err != nil {
			t.Fatal(err)
		}
		c.Key = "v2"
		if _, err = store.CommitRun(ctx, c); !errors.Is(err, hws.ErrConflict) {
			t.Fatal("version conflict", err)
		}
	})
	t.Run("RunUntilRecoveryAndControlBoundary", func(t *testing.T) {
		m, l := start("until")
		c := rt.Command{Kind: "run-until", Until: 30}
		first, err := svc.Execute(ctx, m.Scope, l, "until", c)
		if err != nil || first.Done {
			t.Fatal("first boundary", err, first)
		}
		other := hws.Runtime{Store: New(db), Handler: runtimeFake{}}
		if _, err = other.Execute(ctx, m.Scope, l, "competing", rt.Command{Kind: "step"}); !errors.Is(err, hws.ErrConflict) {
			t.Fatal("overlapping op", err)
		}
		if _, err = other.Execute(ctx, m.Scope, l, "pause", rt.Command{Kind: "pause"}); err != nil {
			t.Fatal(err)
		}
		receipt, err := svc.Execute(ctx, m.Scope, l, "until", c)
		if err != nil || !receipt.Done || receipt.Status != "interrupted" {
			t.Fatal("interrupted receipt", err, receipt)
		}
		if _, err = svc.Execute(ctx, m.Scope, l, "resume2", rt.Command{Kind: "resume"}); err != nil {
			t.Fatal(err)
		}
		for range 5 {
			r, e := other.Execute(ctx, m.Scope, l, "until2", c)
			if e != nil {
				t.Fatal(e)
			}
			if r.Done {
				break
			}
		}
		snap, err := store.LoadRun(ctx, m.Scope)
		if err != nil || snap.State.At != 30 || snap.State.Step != 2 {
			t.Fatal("run-until checkpoint", err, snap.State.At, snap.State.Step)
		}
		if _, err = svc.Execute(ctx, m.Scope, l, "cancel", rt.Command{Kind: "cancel"}); err != nil {
			t.Fatal(err)
		}
		if _, err = svc.Execute(ctx, m.Scope, l, "after", rt.Command{Kind: "resume"}); err == nil {
			t.Fatal("cancel resurrected")
		}
	})
	t.Run("RollbackOutputsRNGAndOperation", func(t *testing.T) {
		m, l := start("rollback")
		before, _ := store.LoadRun(ctx, m.Scope)
		hash, _ := before.State.Hash()
		exec(t, admin, `CREATE FUNCTION dream.fail_runtime_payload() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF EXISTS(SELECT 1 FROM dream.runtime_heads WHERE run='rollback' AND revision=2) THEN RAISE EXCEPTION 'disposable commit fault'; END IF; RETURN NEW; END $$; CREATE TRIGGER runtime_fault BEFORE INSERT ON dream.runtime_payloads FOR EACH ROW EXECUTE FUNCTION dream.fail_runtime_payload()`)
		_, err = svc.Execute(ctx, m.Scope, l, "step", rt.Command{Kind: "step"})
		exec(t, admin, `DROP TRIGGER runtime_fault ON dream.runtime_payloads; DROP FUNCTION dream.fail_runtime_payload()`)
		if err == nil {
			t.Fatal("fault ignored")
		}
		after, _ := store.LoadRun(ctx, m.Scope)
		got, _ := after.State.Hash()
		if got != hash || after.Revision != before.Revision {
			t.Fatal("partial advance")
		}
		op, e := store.Operation(ctx, m.Scope, "step")
		if e != nil || op != nil {
			t.Fatal("partial operation", e)
		}
		if _, err = svc.Execute(ctx, m.Scope, l, "step", rt.Command{Kind: "step"}); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("LeaseExpiryDuringCommitRollsBack", func(t *testing.T) {
		m, l := start("expire-commit")
		exec(t, admin, `UPDATE dream.runtime_heads SET expires=clock_timestamp()+interval '1 second' WHERE run='expire-commit'`)
		exec(t, admin, `CREATE FUNCTION dream.delay_runtime() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN PERFORM pg_sleep(1.2); RETURN NEW; END $$; CREATE TRIGGER runtime_delay BEFORE INSERT ON dream.runtime_payloads FOR EACH ROW EXECUTE FUNCTION dream.delay_runtime()`)
		_, err := svc.Execute(ctx, m.Scope, l, "step", rt.Command{Kind: "step"})
		exec(t, admin, `DROP TRIGGER runtime_delay ON dream.runtime_payloads; DROP FUNCTION dream.delay_runtime()`)
		if !errors.Is(err, hws.ErrLease) {
			t.Fatal("expired commit", err)
		}
		snap, e := store.LoadRun(ctx, m.Scope)
		if e != nil || snap.State.Step != 0 {
			t.Fatal("expired commit advanced", e)
		}
	})

	t.Run("DatabaseClockOverridesHostSkew", func(t *testing.T) {
		m, l := start("host-skew")
		skewed := svc
		skewed.Clock = futureClock{}
		if _, err := skewed.Execute(ctx, m.Scope, l, "step", rt.Command{Kind: "step"}); !errors.Is(err, hws.ErrConflict) {
			t.Fatal("host clock overrode database deadline", err)
		}
		snap, err := store.LoadRun(ctx, m.Scope)
		if err != nil || snap.State.Status != "running" || snap.State.Step != 0 {
			t.Fatal("skew changed state", err)
		}
	})
	t.Run("OperationalBudgetDiscardsComputedTransition", func(t *testing.T) {
		m, l := start("duration")
		exec(t, admin, `UPDATE dream.runtime_heads SET deadline=clock_timestamp()-interval '1 second' WHERE run=$1`, m.Scope.Run)
		r, err := svc.Execute(ctx, m.Scope, l, "step", rt.Command{Kind: "step"})
		if err != nil || r.Status != "budget" || !r.Done {
			t.Fatal(r, err)
		}
		snap, _ := store.LoadRun(ctx, m.Scope)
		if snap.State.Step != 0 || len(snap.State.Positions) != 0 {
			t.Fatal("deadline advanced logical state")
		}
	})

	t.Run("DeadlineCrossingDuringTransaction", func(t *testing.T) {
		m, l := start("deadline-commit")
		exec(t, admin, `UPDATE dream.runtime_heads SET deadline=clock_timestamp()+interval '1 second' WHERE run='deadline-commit'`)
		exec(t, admin, `CREATE FUNCTION dream.delay_deadline() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN PERFORM pg_sleep(1.2); RETURN NEW; END $$; CREATE TRIGGER deadline_delay BEFORE INSERT ON dream.runtime_payloads FOR EACH ROW EXECUTE FUNCTION dream.delay_deadline()`)
		r, err := svc.Execute(ctx, m.Scope, l, "step", rt.Command{Kind: "step"})
		exec(t, admin, `DROP TRIGGER deadline_delay ON dream.runtime_payloads; DROP FUNCTION dream.delay_deadline()`)
		if err != nil || r.Status != "budget" {
			t.Fatal("deadline crossing", r, err)
		}
		snap, e := store.LoadRun(ctx, m.Scope)
		if e != nil || snap.State.Step != 0 || len(snap.State.Positions) != 0 {
			t.Fatal("deadline advanced", e)
		}
	})

	t.Run("AppraisalPersistenceAndRetry", func(t *testing.T) {
		m, l := start("appraisal")
		app := hws.Runtime{Store: store, Handler: hws.AppraisalHandler{Source: appraisalPerception{}}}
		first, err := app.Execute(ctx, m.Scope, l, "appraise-first", rt.Command{Kind: "step"})
		if err != nil {
			t.Fatal(err)
		}
		snap, err := store.LoadRun(ctx, m.Scope)
		if err != nil {
			t.Fatal(err)
		}
		checkpoint, err := hws.DecodeAppraisalCheckpoint(snap.State.Data)
		if err != nil {
			t.Fatal(err)
		}
		if checkpoint.Last == nil || checkpoint.Last.Cause != "e1" {
			t.Fatal("appraisal not persisted")
		}
		resumed := hws.Runtime{Store: New(db), Handler: hws.AppraisalHandler{Source: appraisalPerception{}}}
		repeated, err := resumed.Execute(ctx, m.Scope, l, "appraise-first", rt.Command{Kind: "step"})
		if err != nil || first != repeated {
			t.Fatal("appraisal retry changed receipt", err)
		}
		if _, err = resumed.Execute(ctx, m.Scope, l, "appraise-second", rt.Command{Kind: "step"}); err != nil {
			t.Fatal(err)
		}
		after, err := store.LoadRun(ctx, m.Scope)
		if err != nil {
			t.Fatal(err)
		}
		c, err := hws.DecodeAppraisalCheckpoint(after.State.Data)
		if err != nil {
			t.Fatal(err)
		}
		applied := 0
		for _, s := range c.Actors {
			applied += len(s.Applied)
		}
		if applied != 2 {
			t.Fatal("double appraisal after restart", applied)
		}
	})
	t.Run("RevocationPurgesAndPreventsResume", func(t *testing.T) {
		m, l := start("revoke-runtime")
		if _, err = svc.Execute(ctx, m.Scope, l, "inject", rt.Command{Kind: "inject", Input: &rt.Input{ID: "private-injection", At: 5, Kind: "observation", Actor: "a", Text: "PRIVATE_RUNTIME_CANARY", Priority: 1}}); err != nil {
			t.Fatal(err)
		}
		snap, _ := store.LoadRun(ctx, m.Scope)
		c := runtimeEvent(m.Scope, snap.Revision+1, snap.State.At, "", "revoke runtime")
		c.Event.Type = "revoke"
		if _, err = store.Revoke(ctx, c, runtimeID(string(m.Scope.Run), 1)); err != nil {
			t.Fatal(err)
		}
		if _, err = store.LoadRun(ctx, m.Scope); err == nil {
			t.Fatal("revoked state readable")
		}
		if _, err = svc.Execute(ctx, m.Scope, l, "after", rt.Command{Kind: "step"}); err == nil {
			t.Fatal("revoked run advanced")
		}
		if n := count(t, db, `SELECT count(*) FROM dream.runtime_operations WHERE run='revoke-runtime'`); n != 0 {
			t.Fatal("private request retained")
		}
		if n := count(t, db, `SELECT count(*) FROM dream.runtime_payloads p JOIN dream.tombstones t USING(actor,namespace,event_id)`); n != 0 {
			t.Fatal("checkpoint retained")
		}
	})
	t.Run("KillRestartMatchesCleanTrajectory", func(t *testing.T) {
		m := runtimeManifest(t, "killed")
		if _, err = store.CreateRun(ctx, m); err != nil {
			t.Fatal(err)
		}
		child := osexec.Command(os.Args[0], "-test.run=^TestRuntimeCrashHelper$")
		child.Env = append(os.Environ(), "DREAM_RUNTIME_CHILD=1")
		out, err := child.StdoutPipe()
		if err != nil {
			t.Fatal(err)
		}
		if err = child.Start(); err != nil {
			t.Fatal(err)
		}
		defer child.Process.Kill()
		ready := make(chan bool, 1)
		go func() {
			scan := bufio.NewScanner(out)
			for scan.Scan() {
				if strings.Contains(scan.Text(), "DREAM_CHILD_READY") {
					ready <- true
					return
				}
			}
			ready <- false
		}()
		select {
		case ok := <-ready:
			if !ok {
				t.Fatal("child exited before checkpoint")
			}
		case <-time.After(15 * time.Second):
			t.Fatal("child startup timeout")
		}
		if err = child.Process.Kill(); err != nil {
			t.Fatal(err)
		}
		_ = child.Wait()
		snap, err := store.LoadRun(ctx, m.Scope)
		if err != nil || snap.State.Step != 0 || len(snap.State.Positions) != 0 {
			t.Fatal("killed callback advanced state", err)
		}
		exec(t, admin, `UPDATE dream.runtime_heads SET expires=clock_timestamp()-interval '1 second' WHERE run='killed'`)
		l, err := store.Acquire(ctx, m.Scope, "replacement", time.Minute)
		if err != nil {
			t.Fatal(err)
		}
		a, err := svc.Execute(ctx, m.Scope, l, "step", rt.Command{Kind: "step"})
		if err != nil {
			t.Fatal(err)
		}
		clean, lc := start("clean")
		b, err := svc.Execute(ctx, clean.Scope, lc, "step", rt.Command{Kind: "step"})
		if err != nil || a.Hash != b.Hash {
			t.Fatal("trajectory changed by PID/run/lease/restart", err, a.Hash, b.Hash)
		}
	})
}
func TestRuntimeCrashHelper(t *testing.T) {
	if os.Getenv("DREAM_RUNTIME_CHILD") != "1" {
		t.Skip("child helper only")
	}
	ctx := context.Background()
	cfg, err := pgxpool.ParseConfig(os.Getenv("DREAM_TEST_DSN"))
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.User = "dream_runtime_test"
	cfg.ConnConfig.Password = "disposable_runtime"
	db, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := New(db)
	m := runtimeManifest(t, "killed")
	l, err := store.Acquire(ctx, m.Scope, "child", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	svc := hws.Runtime{Store: store, Handler: runtimeFake{block: true}}
	if _, err = svc.Execute(ctx, m.Scope, l, "resume", rt.Command{Kind: "resume"}); err != nil {
		t.Fatal(err)
	}
	_, err = svc.Execute(ctx, m.Scope, l, "step", rt.Command{Kind: "step"})
	t.Fatal("child unexpectedly returned", err)
}
