package postgres

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/tushardhara/dream/app/graph"
	"github.com/tushardhara/dream/core"
	"github.com/tushardhara/dream/migrations"
)

func TestScopedReaderIntegration(t *testing.T) {
	dsn := os.Getenv("DREAM_TEST_DSN")
	if dsn == "" {
		t.Skip("mandatory migration-check provisions a fresh disposable cluster")
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
	exec(t, admin, `CREATE ROLE dream_scoped_reader LOGIN PASSWORD 'disposable_reader' IN ROLE dream_private_reader,dream_research_reader; CREATE ROLE dream_rls_owner LOGIN PASSWORD 'disposable_owner'; GRANT USAGE ON SCHEMA dream TO dream_rls_owner; CREATE ROLE "policy-b" NOLOGIN`)
	connect := func(user, password string) *pgxpool.Pool {
		t.Helper()
		cfg, err := pgxpool.ParseConfig(dsn)
		if err != nil {
			t.Fatal(err)
		}
		cfg.ConnConfig.User = user
		cfg.ConnConfig.Password = password
		db, err := pgxpool.NewWithConfig(ctx, cfg)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(db.Close)
		return db
	}
	reader := connect("dream_scoped_reader", "disposable_reader")
	owner := connect("dream_rls_owner", "disposable_owner")
	store := New(admin)
	for _, actor := range []core.ID{"policy-a", "policy-b"} {
		for _, namespace := range []core.ID{"reader-one", "reader-two"} {
			for i, class := range []graph.PayloadClass{graph.PrivatePayload, graph.ResearchPayload} {
				c := command(namespace, core.ID(class), int64(i))
				c.Actor = actor
				c.Event.Meta.Observer = actor
				c.Event.Meta.Source = actor
				c.Event.Subject.Principal = actor
				c.Class = class
				if _, err = store.Append(ctx, c); err != nil {
					t.Fatal(err)
				}
			}
		}
	}
	t.Run("DefaultDenyAndExactActorNamespaceClass", func(t *testing.T) {
		for _, table := range []string{"private_payloads", "research_payloads"} {
			if n := count(t, reader, `SELECT count(*) FROM dream.`+table); n != 0 {
				t.Fatal("unscoped reader sees rows", table, n)
			}
		}
		exec(t, admin, `INSERT INTO dream.reader_scopes VALUES('dream_scoped_reader','policy-a','reader-one','private')`)
		if n := count(t, reader, `SELECT count(*) FROM dream.private_payloads`); n != 1 {
			t.Fatal("scope did not isolate exact row", n)
		}
		if n := count(t, reader, `SELECT count(*) FROM dream.research_payloads`); n != 0 {
			t.Fatal("private mapping crossed class", n)
		}
		// Changing a custom setting or active role cannot change authenticated login.
		exec(t, reader, `SET dream.actor='policy-b'; SET ROLE dream_private_reader`)
		if n := count(t, reader, `SELECT count(*) FROM dream.private_payloads WHERE actor='policy-b' OR namespace='reader-two'`); n != 0 {
			t.Fatal("identity spoof escaped", n)
		}
		exec(t, reader, `RESET ROLE`)
		if _, err = reader.Exec(ctx, `SET SESSION AUTHORIZATION 'policy-b'`); err == nil || !strings.Contains(err.Error(), "permission denied") {
			t.Fatal("reader changed authenticated identity")
		}
		if _, err = reader.Exec(ctx, `INSERT INTO dream.reader_scopes VALUES('dream_scoped_reader','policy-b','reader-two','research')`); err == nil {
			t.Fatal("reader minted authority")
		}
		exec(t, admin, `DELETE FROM dream.reader_scopes WHERE login='dream_scoped_reader'`)
		if n := count(t, reader, `SELECT count(*) FROM dream.private_payloads`); n != 0 {
			t.Fatal("mapping revocation not immediate", n)
		}
	})
	t.Run("ForcedOwnerIsolationAndRuntimeDenied", func(t *testing.T) {
		for _, table := range []string{"observable_payloads", "private_payloads", "research_payloads", "runtime_payloads", "reader_scopes"} {
			if n := count(t, admin, `SELECT count(*) FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname='dream' AND c.relname=$1 AND c.relrowsecurity AND c.relforcerowsecurity`, table); n != 1 {
				t.Fatal("RLS not enabled and forced", table)
			}
		}
		var original string
		if err = admin.QueryRow(ctx, `SELECT pg_get_userbyid(relowner) FROM pg_class WHERE oid='dream.private_payloads'::regclass`).Scan(&original); err != nil {
			t.Fatal(err)
		}
		exec(t, admin, `ALTER TABLE dream.private_payloads OWNER TO dream_rls_owner`)
		t.Cleanup(func() {
			exec(t, admin, `ALTER TABLE dream.private_payloads OWNER TO `+pgx.Identifier{original}.Sanitize())
		})
		if n := count(t, owner, `SELECT count(*) FROM dream.private_payloads`); n != 0 {
			t.Fatal("table owner bypassed FORCE RLS", n)
		}
		for _, db := range []*pgxpool.Pool{reader, owner} {
			if _, err = db.Exec(ctx, `SELECT * FROM dream.runtime_payloads`); err == nil {
				t.Fatal("unprivileged runtime read")
			}
		}
	})
	t.Run("V2ForwardUpgradePreservesData", func(t *testing.T) {
		exec(t, admin, `CREATE DATABASE dream_reader_upgrade`)
		cfg, err := pgxpool.ParseConfig(dsn)
		if err != nil {
			t.Fatal(err)
		}
		cfg.ConnConfig.Database = "dream_reader_upgrade"
		old, err := pgxpool.NewWithConfig(ctx, cfg)
		if err != nil {
			t.Fatal(err)
		}
		defer old.Close()
		exec(t, old, migrations.Initial)
		exec(t, old, migrations.Runtime)
		if _, err = New(old).Append(ctx, command("reader-upgrade", "legacy", 0)); err != nil {
			t.Fatal(err)
		}
		if err = Migrate(ctx, old); err != nil {
			t.Fatal(err)
		}
		if err = Migrate(ctx, old); err != nil {
			t.Fatal(err)
		}
		if count(t, old, `SELECT count(*) FROM dream.schema_versions WHERE version IN (1,2,3)`) != 3 || count(t, old, `SELECT count(*) FROM dream.private_payloads WHERE namespace='reader-upgrade'`) != 1 {
			t.Fatal("v2 upgrade lost ledger/payload")
		}
	})
}
