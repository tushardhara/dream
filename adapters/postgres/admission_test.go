package postgres

import (
	"context"
	"errors"
	"os"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/tushardhara/dream/app/hws"
	"github.com/tushardhara/dream/core"
)

func TestTransportRuntimeRoleAndDurableAdmission(t *testing.T) {
	dsn := os.Getenv("DREAM_TEST_DSN")
	if dsn == "" {
		t.Skip("mandatory migration-check supplies fresh cluster")
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
	if err = New(admin).RuntimeReady(ctx); err == nil {
		t.Fatal("superuser runtime accepted")
	}
	exec(t, admin, `CREATE ROLE dream_api_fixture LOGIN PASSWORD 'disposable_api' IN ROLE dream_writer; CREATE ROLE dream_api_owner_fixture NOLOGIN; GRANT USAGE ON SCHEMA dream TO dream_api_fixture`)
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.User = "dream_api_fixture"
	cfg.ConnConfig.Password = "disposable_api"
	runtime, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	store := New(runtime)
	if err = store.RuntimeReady(ctx); err != nil {
		t.Fatal("non-owner runtime", err)
	}
	// An indirect owner membership must fail even before SET ROLE is used.
	exec(t, admin, `CREATE TABLE dream.api_role_fixture(id integer); ALTER TABLE dream.api_role_fixture OWNER TO dream_api_owner_fixture; GRANT dream_api_owner_fixture TO dream_api_fixture`)
	if err = store.RuntimeReady(ctx); err == nil {
		t.Fatal("reachable table owner accepted")
	}
	exec(t, admin, `REVOKE dream_api_owner_fixture FROM dream_api_fixture; DROP TABLE dream.api_role_fixture`)
	b := hws.RequestBudget{Credential: "quota-fixture", Scope: hws.Scope{Actor: "operator", Namespace: "ns", World: "w", Branch: "b", Run: "r"}, PerMinute: 5, Total: 7}
	run := func(want int64) {
		t.Helper()
		var accepted atomic.Int64
		var wg sync.WaitGroup
		for range 12 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				err := New(runtime).AdmitRequest(ctx, b)
				if err == nil {
					accepted.Add(1)
				} else if !errors.Is(err, hws.ErrAdmission) {
					t.Errorf("admission: %v", err)
				}
			}()
		}
		wg.Wait()
		if accepted.Load() != want {
			t.Fatalf("accepted %d want %d", accepted.Load(), want)
		}
	}
	binding, _ := hws.ModelDigest(struct {
		Credential core.ID
		Scope      hws.Scope
	}{b.Credential, b.Scope})
	namespace := "admission:" + binding
	// Pin the fixture's admission clock using the existing durable rollback
	// protection. Otherwise a concurrent burst straddling a calendar-minute
	// boundary legitimately admits more than five and makes this test flaky.
	// Only the disposable admin can alter these timestamps; production time,
	// the five-request limit and the cumulative budget remain unchanged.
	if err = store.AdmitRequest(ctx, b); err != nil {
		t.Fatal(err)
	}
	exec(t, admin, `UPDATE dream.events SET recorded_at=clock_timestamp()+interval '1 day' WHERE actor=$1 AND namespace=$2`, b.Scope.Actor, namespace)
	run(4) // one recorded admission plus four concurrent admissions fills five.
	run(0)
	// Simulate a prior window without a wall-clock sleep. Admin-only mutation of
	// this fixture's timestamps cannot be done by the runtime writer.
	exec(t, admin, `UPDATE dream.events SET recorded_at=clock_timestamp()-interval '2 minutes' WHERE actor=$1 AND namespace=$2`, b.Scope.Actor, namespace)
	run(2)
	exec(t, admin, `UPDATE dream.events SET recorded_at=clock_timestamp()-interval '2 minutes' WHERE actor=$1 AND namespace=$2`, b.Scope.Actor, namespace)
	run(0) // cumulative total survives every new Store and window.
	b.Credential = "rollback-fixture"
	b.Total = 20
	if err = store.AdmitRequest(ctx, b); err != nil {
		t.Fatal(err)
	}
	binding, _ = hws.ModelDigest(struct {
		Credential core.ID
		Scope      hws.Scope
	}{b.Credential, b.Scope})
	namespace = "admission:" + binding
	exec(t, admin, `UPDATE dream.events SET recorded_at=clock_timestamp()+interval '2 minutes' WHERE actor=$1 AND namespace=$2`, b.Scope.Actor, namespace)
	run(4)
	run(0) // recorded wall-clock rollback cannot refund the same window.
}
