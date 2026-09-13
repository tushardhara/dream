package postgres

import (
	"bytes"
	"context"
	"encoding/json"
	"net/url"
	"os"
	osexec "os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/tushardhara/dream/app/graph"
	"github.com/tushardhara/dream/app/hws"
	"github.com/tushardhara/dream/core"
)

func TestOldBackupRestoreRequiresNewRevocations(t *testing.T) {
	dsn := os.Getenv("DREAM_TEST_DSN")
	if dsn == "" {
		t.Skip("mandatory migration-check supplies fresh cluster")
	}
	container := os.Getenv("DREAM_TEST_CONTAINER")
	if !strings.HasPrefix(container, "dream-check-") {
		t.Fatal("restore drill requires scripts/postgres-check.py owned disposable container")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	admin, e := pgxpool.New(ctx, dsn)
	if e != nil {
		t.Fatal(e)
	}
	defer admin.Close()
	exec(t, admin, `CREATE DATABASE dream_old_backup`)
	exec(t, admin, `CREATE DATABASE dream_old_restore`)
	openDB := func(name string) *pgxpool.Pool {
		cfg, e := pgxpool.ParseConfig(dsn)
		if e != nil {
			t.Fatal(e)
		}
		cfg.ConnConfig.Database = name
		db, e := pgxpool.NewWithConfig(ctx, cfg)
		if e != nil {
			t.Fatal(e)
		}
		t.Cleanup(db.Close)
		return db
	}
	sourceDB := openDB("dream_old_backup")
	if e = Migrate(ctx, sourceDB); e != nil {
		t.Fatal(e)
	}
	source := New(sourceDB)
	m := runtimeManifest(t, "old-backup")
	if _, e = source.CreateRun(ctx, m); e != nil {
		t.Fatal(e)
	}
	realm, _ := (hws.ViewRealm{Scope: m.Scope, Principal: "a"}).MemoryScope()
	record := snapshotMemory("known", 0)
	if _, e = (graph.MemoryService{Journal: source}).Put(ctx, realm, "known", 0, record, "simulation"); e != nil {
		t.Fatal(e)
	}
	key, e := source.CaptureSnapshot(ctx, m.Scope, "before-revoke", 1)
	if e != nil {
		t.Fatal(e)
	}
	dump, e := osexec.CommandContext(ctx, "docker", "exec", container, "pg_dump", "-U", "postgres", "-d", "dream_old_backup", "--schema=dream").Output()
	if e != nil {
		t.Fatal("disposable pg_dump failed")
	}
	// The backup predates both this revocation and an event created later.
	later := snapshotMemory("after-backup", 0)
	if _, e = (graph.MemoryService{Journal: source}).Put(ctx, realm, "after-backup", 0, later, "simulation"); e != nil {
		t.Fatal(e)
	}
	for _, id := range []core.ID{"known", "after-backup"} {
		revoke := snapshotMemory(id, 0).Event
		revoke.Type = "revoke"
		revoke.Meta.ID = "revoke-" + id
		revoke.Meta.Rights = core.Rights{Resource: revoke.Meta.ID}
		if _, e = graph.RevokeMemory(ctx, source, realm, revoke.Meta.ID, 1, revoke, id); e != nil {
			t.Fatal(e)
		}
	}
	journal, e := source.ExportRevocations(ctx)
	if e != nil {
		t.Fatal(e)
	}
	hash, e := journal.Digest()
	if e != nil || len(journal.References) < 2 {
		t.Fatal("missing independent revocations", e)
	}
	restore := osexec.CommandContext(ctx, "docker", "exec", "-i", container, "psql", "-X", "-v", "ON_ERROR_STOP=1", "-U", "postgres", "-d", "dream_old_restore")
	restore.Stdin = bytes.NewReader(dump)
	if e = restore.Run(); e != nil {
		t.Fatal("disposable restore failed")
	}
	restoredDB := openDB("dream_old_restore")
	restored := New(restoredDB)
	// Positive control proves this really is an old backup with the old bytes.
	if _, e = restored.ReadSnapshot(ctx, key); e != nil {
		t.Fatal("backup did not contain original snapshot", e)
	}
	if e = restored.QuarantineRestore(ctx, hash); e != nil {
		t.Fatal(e)
	}
	if _, e = restored.ReadSnapshot(ctx, key); e == nil {
		t.Fatal("quarantine allowed snapshot access")
	}
	if _, e = restored.LoadRun(ctx, m.Scope); e == nil {
		t.Fatal("quarantine allowed runtime access")
	}
	if _, e = restored.ReadPayload(ctx, realm.Owner, realm.Namespace, "known"); e == nil {
		t.Fatal("quarantine allowed payload access")
	}
	bad := journal
	bad.References = append([]RevokedReference{}, journal.References[1:]...)
	if e = restored.ApplyRevocationJournal(ctx, bad); e == nil {
		t.Fatal("incomplete revocation journal accepted")
	}
	if _, e = restored.LoadRun(ctx, m.Scope); e == nil {
		t.Fatal("failed restore reopened admission")
	}
	exec(t, restoredDB, `CREATE FUNCTION dream.reject_restore_purge() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'synthetic purge outage'; END $$; CREATE TRIGGER reject_restore_purge BEFORE DELETE ON dream.snapshot_payloads FOR EACH ROW EXECUTE FUNCTION dream.reject_restore_purge()`)
	if e = restored.ApplyRevocationJournal(ctx, journal); e == nil {
		t.Fatal("restore ignored purge failure")
	}
	exec(t, restoredDB, `DROP TRIGGER reject_restore_purge ON dream.snapshot_payloads; DROP FUNCTION dream.reject_restore_purge()`)
	if _, e = restored.LoadRun(ctx, m.Scope); e == nil {
		t.Fatal("partial restore reopened admission")
	}
	if count(t, restoredDB, `SELECT count(*) FROM dream.events WHERE namespace='recovery'`) != 1 {
		t.Fatal("failed restore left committed success audit")
	}
	adminBinary := filepath.Join(t.TempDir(), "hws-admin")
	build := osexec.CommandContext(ctx, "go", "build", "-trimpath", "-o", adminBinary, "./cmd/hws-admin")
	build.Dir = "../.."
	if raw, e := build.CombinedOutput(); e != nil {
		t.Fatalf("admin build: %v %s", e, raw)
	}
	journalFile := filepath.Join(t.TempDir(), "revocations.json")
	rawJournal, _ := json.Marshal(journal)
	if e = os.WriteFile(journalFile, rawJournal, 0600); e != nil {
		t.Fatal(e)
	}
	restoredURL, e := url.Parse(dsn)
	if e != nil {
		t.Fatal("disposable URL invalid")
	}
	restoredURL.Path = "/dream_old_restore"
	apply := osexec.CommandContext(ctx, adminBinary, "--development", "--file", journalFile, "apply-revocations")
	apply.Env = append(os.Environ(), "DREAM_DATABASE_URL="+restoredURL.String())
	if output, e := apply.CombinedOutput(); e != nil || !bytes.Contains(output, []byte("operation complete")) {
		t.Fatal("compiled restore administration failed")
	}

	if _, e = restored.ReadSnapshot(ctx, key); e == nil {
		t.Fatal("old snapshot resurrected purged data")
	}
	if _, e = (graph.MemoryService{Journal: restored}).Put(ctx, realm, "after-backup", 0, later, "simulation"); e == nil {
		t.Fatal("recreated revoked ID absent from backup")
	}
	if count(t, restoredDB, `SELECT count(*) FROM dream.snapshot_payloads p JOIN dream.tombstones t USING(actor,namespace,event_id)`) != 0 {
		t.Fatal("restored snapshot payload survived purge")
	}
	if count(t, restoredDB, `SELECT count(*) FROM dream.events WHERE namespace='recovery'`) != 2 {
		t.Fatal("restore audit incomplete")
	}
	// The canonical genesis is known and not itself derived from the memory; it
	// remains usable while the snapshot containing that memory is invalidated.
	if _, e = restored.LoadRun(ctx, m.Scope); e != nil {
		t.Fatal("unaffected durable event lost", e)
	}
	if _, e = restored.Project(ctx, "restore-drill", 100); e != nil {
		t.Fatal(e)
	}
	if _, e = restored.ReadPayload(ctx, realm.Owner, realm.Namespace, "known"); e == nil {
		t.Fatal("projection recovery resurrected revoked source")
	}
}
