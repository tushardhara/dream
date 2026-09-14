package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/tushardhara/dream/adapters/model"
	"os"
	osexec "os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/tushardhara/dream/app/graph"
	"github.com/tushardhara/dream/app/hws"
	"github.com/tushardhara/dream/core"
	"github.com/tushardhara/dream/migrations"
	"github.com/tushardhara/dream/simulator"
	"github.com/tushardhara/dream/simulator/behavior"
	rt "github.com/tushardhara/dream/simulator/runtime"
)

func snapshotMemory(id core.ID, learned core.LogicalTime) graph.MemoryRecord {
	return graph.MemoryRecord{Event: core.Event{Version: 1, Type: graph.MemoryEventType, Stream: id, Subject: core.Subject{Principal: "a"}, Meta: core.Metadata{ID: id, Observer: "a", Source: "a", Sensitivity: core.Restricted, Confidence: .7, Valid: core.Interval{}, RecordedAt: time.Unix(1, 0), Rights: core.Rights{Resource: id, Grants: []core.Grant{{Actor: "a", Recipient: "a", Purpose: "simulation", Operation: core.Read}, {Actor: "a", Recipient: "a", Purpose: "simulation", Operation: core.Derive}}}}}, Content: graph.MemoryContent{Version: 1, Kind: graph.EpisodicMemory, Text: "SYNTHETIC_" + string(id), Salience: .5, HalfLife: 100, Learned: []graph.Learned{{Actor: "a", At: learned}}}}
}
func forkSpec(key hws.SnapshotKey, name string) hws.ForkSpec {
	child := key.Scope
	child.Branch = simulator.BranchID(name)
	child.Run = simulator.RunID(name)
	return hws.ForkSpec{Version: 1, Source: key, Child: child, Mode: hws.FreshSimulation, PairedExogenous: true, Policy: behavior.Policy, MaxDuration: time.Minute}
}
func TestSnapshotIntegration(t *testing.T) {
	dsn := os.Getenv("DREAM_TEST_DSN")
	if dsn == "" {
		t.Skip("mandatory migration-check supplies fresh cluster")
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
	fixture := func(t *testing.T, name string) (hws.SnapshotKey, hws.Lease, graph.MemoryScope) {
		t.Helper()
		m := runtimeManifest(t, "snapshot-"+name)
		if _, e := store.CreateRun(ctx, m); e != nil {
			t.Fatal(e)
		}
		lease, e := store.Acquire(ctx, m.Scope, "snapshot-worker", time.Minute)
		if e != nil {
			t.Fatal(e)
		}
		runtime := hws.Runtime{Store: store, Handler: runtimeFake{}}
		if _, e = runtime.Execute(ctx, m.Scope, lease, "resume", rt.Command{Kind: "resume"}); e != nil {
			t.Fatal(e)
		}
		scope, _ := (hws.ViewRealm{Scope: m.Scope, Principal: "a"}).MemoryScope()
		for _, record := range []graph.MemoryRecord{snapshotMemory("known", 0), snapshotMemory("future", 30)} {
			if _, e = (graph.MemoryService{Journal: store}).Put(ctx, scope, record.Event.Meta.ID, 0, record, "simulation"); e != nil {
				t.Fatal(e)
			}
		}
		key, e := store.CaptureSnapshot(ctx, m.Scope, "cutoff", 2)
		if e != nil {
			t.Fatal(e)
		}
		return key, lease, scope
	}
	t.Run("ExactReplayScopeAndCorruption", func(t *testing.T) {
		key, lease, _ := fixture(t, "exact")
		runtime := hws.Runtime{Store: store, Handler: runtimeFake{}}
		if _, e = runtime.Execute(ctx, key.Scope, lease, "step", rt.Command{Kind: "step"}); e != nil {
			t.Fatal(e)
		}
		bundle, e := store.ReadReplay(ctx, key, 3)
		if e != nil {
			t.Fatal(e)
		}
		for _, mode := range []hws.ReplayMode{hws.EventReplay, hws.RecordedReplay} {
			result, e := hws.Replay(bundle, mode)
			if e != nil || result.Hashes[1] != bundle.Frames[0].After {
				t.Fatal("replay", e)
			}
		}
		rawBundle, _ := json.Marshal(bundle)
		path := filepath.Join(t.TempDir(), "recorded.json")
		if e = os.WriteFile(path, rawBundle, 0600); e != nil {
			t.Fatal(e)
		}
		binary, e := os.Executable()
		if e != nil {
			t.Fatal(e)
		}
		childCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		child := osexec.CommandContext(childCtx, binary, "-test.run=^TestReplayProcessHelper$")
		child.Env = []string{"DREAM_REPLAY_FIXTURE=" + path}
		output, e := child.CombinedOutput()
		if e != nil || !strings.Contains(string(output), "REPLAY_HASH="+bundle.Frames[0].After) {
			t.Fatal("separate process replay failed", e, string(output))
		}
		altered := key
		altered.Scope.Branch = "foreign"
		if _, e = store.ReadSnapshot(ctx, altered); e == nil {
			t.Fatal("cross-branch snapshot read")
		}
		altered = key
		altered.Hash = "0000000000000000000000000000000000000000000000000000000000000000"
		if _, e = store.ReadSnapshot(ctx, altered); e == nil {
			t.Fatal("snapshot hash ignored")
		}
		altered = key
		altered.Hash = ""
		if _, e = store.ReadReplay(ctx, altered, 3); e == nil {
			t.Fatal("empty replay handle hash accepted")
		}
		f, e := store.ReadSnapshot(ctx, key)
		if e != nil {
			t.Fatal(e)
		}
		f.State.Queue[0].Text = "tampered-local"
		again, e := store.ReadSnapshot(ctx, key)
		if e != nil || again.State.Queue[0].Text == "tampered-local" {
			t.Fatal("snapshot alias", e)
		}
		var event core.ID
		var original []byte
		if e = db.QueryRow(ctx, `SELECT h.event_id,p.payload FROM dream.snapshot_heads h JOIN dream.snapshot_payloads p USING(actor,namespace,event_id) WHERE h.actor=$1 AND h.namespace=$2 AND h.run=$3 AND h.id=$4`, key.Scope.Actor, key.Scope.Namespace, key.Scope.Run, key.ID).Scan(&event, &original); e != nil {
			t.Fatal(e)
		}
		alteredPayload := again
		alteredPayload.State.Data = "corruption"
		raw, _ := json.Marshal(alteredPayload)
		exec(t, db, `UPDATE dream.snapshot_payloads SET payload=$4 WHERE actor=$1 AND namespace=$2 AND event_id=$3`, key.Scope.Actor, key.Scope.Namespace, event, raw)
		if _, e = store.ReadSnapshot(ctx, key); e == nil {
			t.Fatal("corrupt snapshot accepted")
		}
		exec(t, db, `UPDATE dream.snapshot_payloads SET payload=$4 WHERE actor=$1 AND namespace=$2 AND event_id=$3`, key.Scope.Actor, key.Scope.Namespace, event, original)
		if _, e = store.CaptureSnapshot(ctx, key.Scope, key.ID, 3); e == nil {
			t.Fatal("snapshot key reused for new boundary")
		}
	})
	t.Run("ConcurrentForksKnowledgeCutoffAndWrites", func(t *testing.T) {
		key, _, parentMemory := fixture(t, "isolation")
		before, e := store.LoadRun(ctx, key.Scope)
		if e != nil {
			t.Fatal(e)
		}
		beforeHash, _ := before.State.Hash()
		parentLater := snapshotMemory("after-cutoff", 0)
		if _, e = (graph.MemoryService{Journal: store}).Put(ctx, parentMemory, "after-cutoff", 0, parentLater, "simulation"); e != nil {
			t.Fatal(e)
		}
		const n = 4
		var wg sync.WaitGroup
		errs := make(chan error, n)
		specs := make([]hws.ForkSpec, n)
		for i := range specs {
			specs[i] = forkSpec(key, fmt.Sprintf("snapshot-child-%d", i))
			wg.Add(1)
			go func(spec hws.ForkSpec) { defer wg.Done(); _, err := New(db).ForkSnapshot(ctx, spec); errs <- err }(specs[i])
		}
		wg.Wait()
		close(errs)
		for err := range errs {
			if err != nil {
				t.Fatal(err)
			}
		}
		for i, spec := range specs {
			childMemory, _ := (hws.ViewRealm{Scope: spec.Child, Principal: "a"}).MemoryScope()
			entries, e := store.ReadMemory(ctx, childMemory)
			if e != nil || len(entries) != 1 || entries[0].Event.Meta.ID != "known" {
				t.Fatal("fork knowledge cutoff", i, len(entries), e)
			}
			record := snapshotMemory(core.ID(fmt.Sprintf("child-only-%d", i)), 0)
			if _, e = (graph.MemoryService{Journal: store}).Put(ctx, childMemory, record.Event.Meta.ID, 0, record, "simulation"); e != nil {
				t.Fatal(e)
			}
			lease, e := store.Acquire(ctx, spec.Child, core.ID(fmt.Sprintf("worker-%d", i)), time.Minute)
			if e != nil {
				t.Fatal(e)
			}
			runtime := hws.Runtime{Store: store, Handler: runtimeFake{}}
			if _, e = runtime.Execute(ctx, spec.Child, lease, "resume", rt.Command{Kind: "resume"}); e != nil {
				t.Fatal(e)
			}
			if _, e = runtime.Execute(ctx, spec.Child, lease, "step", rt.Command{Kind: "step"}); e != nil {
				t.Fatal(e)
			}
			wrong := spec.Child
			wrong.Run = key.Scope.Run
			if _, e = store.LoadRun(ctx, wrong); e == nil {
				t.Fatal("cross-run branch alias")
			}
		}
		after, e := store.LoadRun(ctx, key.Scope)
		if e != nil {
			t.Fatal(e)
		}
		afterHash, _ := after.State.Hash()
		if beforeHash != afterHash || before.Revision != after.Revision {
			t.Fatal("child changed parent")
		}
		for i, spec := range specs {
			scope, _ := (hws.ViewRealm{Scope: spec.Child, Principal: "a"}).MemoryScope()
			entries, e := store.ReadMemory(ctx, scope)
			if e != nil || len(entries) != 2 {
				t.Fatal("sibling memory leaked", i, e)
			}
		}
	})
	t.Run("RevocationAcrossSnapshotChildAndSibling", func(t *testing.T) {
		key, _, parentMemory := fixture(t, "revoke")
		left := forkSpec(key, "revoke-left")
		right := forkSpec(key, "revoke-right")
		if _, e = store.ForkSnapshot(ctx, left); e != nil {
			t.Fatal(e)
		}
		if _, e = store.ForkSnapshot(ctx, right); e != nil {
			t.Fatal(e)
		}
		childMemory, _ := (hws.ViewRealm{Scope: left.Child, Principal: "a"}).MemoryScope()
		record := snapshotMemory("known", 0)
		revoke := record.Event
		revoke.Type = "revoke"
		revoke.Meta.ID = "revoke-child"
		revoke.Meta.Rights = core.Rights{Resource: "revoke-child"}
		if _, e = graph.RevokeMemory(ctx, store, childMemory, "revoke-child", 1, revoke, "known"); e != nil {
			t.Fatal(e)
		}
		if _, e = store.LoadRun(ctx, left.Child); e == nil {
			t.Fatal("child revocation did not invalidate copied canonical state")
		}
		if _, e = store.LoadRun(ctx, right.Child); e != nil {
			t.Fatal("sibling damaged", e)
		}
		if _, e = store.ReadSnapshot(ctx, key); e != nil {
			t.Fatal("parent snapshot damaged", e)
		}
		revoke.Meta.ID = "revoke-parent"
		revoke.Meta.Rights = core.Rights{Resource: "revoke-parent"}
		if _, e = graph.RevokeMemory(ctx, store, parentMemory, "revoke-parent", 1, revoke, "known"); e != nil {
			t.Fatal(e)
		}
		if _, e = store.ReadSnapshot(ctx, key); e == nil {
			t.Fatal("revoked snapshot readable")
		}
		if _, e = store.ForkSnapshot(ctx, forkSpec(key, "resurrection")); e == nil {
			t.Fatal("purged snapshot resurrected")
		}
		if _, e = store.LoadRun(ctx, right.Child); e == nil {
			t.Fatal("revocation missed sibling descendant")
		}
		if _, e = store.CaptureSnapshot(ctx, key.Scope, key.ID, 2); e == nil {
			t.Fatal("purged snapshot recreated under old key")
		}
		if count(t, db, `SELECT count(*) FROM dream.snapshot_payloads p JOIN dream.tombstones t USING(actor,namespace,event_id)`) != 0 {
			t.Fatal("revoked snapshot bytes retained")
		}
	})
	t.Run("SnapshotAfterPurgeCannotDropTombstones", func(t *testing.T) {
		key, _, scope := fixture(t, "already-purged")
		revoke := snapshotMemory("known", 0).Event
		revoke.Type = "revoke"
		revoke.Meta.ID = "purge-before-capture"
		revoke.Meta.Rights = core.Rights{Resource: revoke.Meta.ID}
		if _, err := graph.RevokeMemory(ctx, store, scope, revoke.Meta.ID, 1, revoke, "known"); err != nil {
			t.Fatal(err)
		}
		// The run never consumed this record, so a new redacted snapshot is valid.
		redacted, err := store.CaptureSnapshot(ctx, key.Scope, "after-purge", 2)
		if err != nil {
			t.Fatal(err)
		}
		frozen, err := store.ReadSnapshot(ctx, redacted)
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, memory := range frozen.Memories {
			for _, entry := range memory.Entries {
				if entry.Event.Meta.ID == "known" {
					found = entry.Revoked && entry.Content == nil
				}
			}
		}
		if !found {
			t.Fatal("snapshot lost purged metadata")
		}
		spec := forkSpec(redacted, "purged-child")
		if _, err = store.ForkSnapshot(ctx, spec); err == nil {
			t.Fatal("fork discarded irreversible tombstone")
		}
		if _, err = store.LoadRun(ctx, spec.Child); err == nil {
			t.Fatal("failed fork left child state")
		}
	})
	t.Run("ConcurrentRevocationCannotResurrectFork", func(t *testing.T) {
		key, _, scope := fixture(t, "revoke-race")
		spec := forkSpec(key, "revocation-race-child")
		record := snapshotMemory("known", 0)
		revoke := record.Event
		revoke.Type = "revoke"
		revoke.Meta.ID = "race-revoke"
		revoke.Meta.Rights = core.Rights{Resource: "race-revoke"}
		forkDone := make(chan error, 1)
		revokeDone := make(chan error, 1)
		start := make(chan struct{})
		go func() { <-start; _, err := New(db).ForkSnapshot(ctx, spec); forkDone <- err }()
		go func() {
			<-start
			_, err := graph.RevokeMemory(ctx, New(db), scope, "race-revoke", 1, revoke, "known")
			revokeDone <- err
		}()
		close(start)
		<-forkDone
		if e := <-revokeDone; e != nil {
			t.Fatal(e)
		}
		if _, e = store.ReadSnapshot(ctx, key); e == nil {
			t.Fatal("racing snapshot survived revocation")
		}
		if _, e = store.LoadRun(ctx, spec.Child); e == nil {
			t.Fatal("racing fork resurrected source")
		}
		childScope, _ := (hws.ViewRealm{Scope: spec.Child, Principal: "a"}).MemoryScope()
		entries, e := store.ReadMemory(ctx, childScope)
		if e != nil {
			t.Fatal(e)
		}
		for _, entry := range entries {
			if entry.Event.Meta.ID == "known" && entry.Content != nil {
				t.Fatal("racing child retained revoked bytes")
			}
		}
	})
	t.Run("BranchBudgetAndDeadlineDoNotReset", func(t *testing.T) {
		key, _, _ := fixture(t, "budget")
		realm := hws.ViewRealm{Scope: key.Scope, Principal: "a"}
		views, e := hws.NewViewService(store, store, futureClock{}, []hws.ViewGrant{{Caller: "a", Realm: realm, Kind: hws.ActorViewKind, Purpose: "simulation", Operations: []core.Operation{core.Read, core.Derive}}}, store)
		if e != nil {
			t.Fatal(e)
		}
		permit, e := views.Permit("a", realm, hws.ActorViewKind, "simulation")
		if e != nil {
			t.Fatal(e)
		}
		cap, decision, e := views.Propose(ctx, permit, []core.ID{"known"}, "a", core.Derive, graph.InternalContext)
		if e != nil || !decision.Allowed {
			t.Fatal(decision, e)
		}
		gateway, e := hws.NewModelGateway(store, views, model.Fake{}, modelRoute())
		if e != nil {
			t.Fatal(e)
		}
		if _, e = gateway.Execute(ctx, hws.ModelRequest{Scope: key.Scope, Principal: "a", Key: "budgeted", Capability: hws.ModelInterpretation, Permit: permit, Approved: cap}); e != nil {
			t.Fatal(e)
		}
		key, e = store.CaptureSnapshot(ctx, key.Scope, "with-budget", 2)
		if e != nil {
			t.Fatal(e)
		}
		frozen, e := store.ReadSnapshot(ctx, key)
		if e != nil || frozen.ModelBudget == nil || frozen.ModelBudget.Tokens != 60000 {
			t.Fatal("reserved budget absent", e)
		}
		spec := forkSpec(key, "budget-child")
		spec.MaxDuration = 24 * time.Hour
		parent, e := store.LoadRun(ctx, key.Scope)
		if e != nil {
			t.Fatal(e)
		}
		child, e := store.ForkSnapshot(ctx, spec)
		if e != nil || child.Experiment == nil || child.Experiment.Mode != hws.FreshSimulation || child.Experiment.Exact || child.Deadline.After(parent.Deadline) || child.State.Budget != parent.State.Budget || child.State.Step != parent.State.Step {
			t.Fatal("fork reset duration/step budget", e)
		}
		var tokens, spend int64
		if e = db.QueryRow(ctx, `SELECT tokens,spend FROM dream.model_budgets WHERE actor=$1 AND namespace=$2 AND run=$3`, spec.Child.Actor, spec.Child.Namespace, spec.Child.Run).Scan(&tokens, &spend); e != nil || tokens != 60000 || spend != 60000 {
			t.Fatal("fork reset reserved model budget", e)
		}
		descendant, e := store.CaptureSnapshot(ctx, spec.Child, "child-snapshot", 2)
		if e != nil {
			t.Fatal(e)
		}
		childFrozen, e := store.ReadSnapshot(ctx, descendant)
		if e != nil || childFrozen.Ancestor == nil || *childFrozen.Ancestor != key {
			t.Fatal("immutable model/history ancestor lost", e)
		}
	})
	t.Run("V4UpgradeAndRestrictedSnapshotTable", func(t *testing.T) {
		exec(t, db, `CREATE DATABASE dream_snapshot_v4`)
		cfg, e := pgxpool.ParseConfig(dsn)
		if e != nil {
			t.Fatal(e)
		}
		cfg.ConnConfig.Database = "dream_snapshot_v4"
		old, e := pgxpool.NewWithConfig(ctx, cfg)
		if e != nil {
			t.Fatal(e)
		}
		defer old.Close()
		exec(t, old, migrations.Initial)
		exec(t, old, migrations.Runtime)
		exec(t, old, migrations.ReaderScopes)
		exec(t, old, migrations.Models)
		prior := New(old)
		m := runtimeManifest(t, "snapshot-upgrade")
		original, e := prior.CreateRun(ctx, m)
		if e != nil {
			t.Fatal(e)
		}
		if e = Migrate(ctx, old); e != nil {
			t.Fatal(e)
		}
		if e = Migrate(ctx, old); e != nil {
			t.Fatal(e)
		}
		after, e := prior.LoadRun(ctx, m.Scope)
		if e != nil {
			t.Fatal(e)
		}
		a, _ := original.State.Hash()
		b, _ := after.State.Hash()
		if a != b || count(t, old, `SELECT count(*) FROM dream.schema_versions`) != 6 {
			t.Fatal("upgrade changed existing state")
		}
		if _, e = prior.CaptureSnapshot(ctx, m.Scope, "upgraded", 1); e != nil {
			t.Fatal(e)
		}
		for _, role := range []string{"dream_actor", "dream_private_reader", "dream_research_reader"} {
			var allowed bool
			if e = old.QueryRow(ctx, `SELECT has_table_privilege($1,'dream.snapshot_payloads','SELECT')`, role).Scan(&allowed); e != nil || allowed {
				t.Fatal("snapshot reader grants", e)
			}
		}
		if count(t, old, `SELECT count(*) FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname='dream' AND c.relname='snapshot_payloads' AND c.relrowsecurity AND c.relforcerowsecurity`) != 1 {
			t.Fatal("snapshot RLS not forced")
		}
	})
}

func TestReplayProcessHelper(t *testing.T) {
	path := os.Getenv("DREAM_REPLAY_FIXTURE")
	if path == "" {
		t.Skip("subprocess fixture only")
	}
	raw, e := os.ReadFile(path)
	if e != nil {
		t.Fatal(e)
	}
	var bundle hws.ReplayBundle
	if json.Unmarshal(raw, &bundle) != nil {
		t.Fatal("decode")
	}
	result, e := hws.Replay(bundle, hws.RecordedReplay)
	if e != nil {
		t.Fatal(e)
	}
	fmt.Println("REPLAY_HASH=" + result.Hashes[len(result.Hashes)-1])
}
