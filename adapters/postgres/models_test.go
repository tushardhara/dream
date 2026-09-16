package postgres

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/tushardhara/dream/adapters/model"
	"github.com/tushardhara/dream/app/graph"
	"github.com/tushardhara/dream/app/hws"
	"github.com/tushardhara/dream/core"
	"github.com/tushardhara/dream/migrations"
	rt "github.com/tushardhara/dream/simulator/runtime"
)

type modelFunc func(context.Context, hws.ProviderInput) (hws.ProviderResponse, error)

func (f modelFunc) Generate(c context.Context, i hws.ProviderInput) (hws.ProviderResponse, error) {
	return f(c, i)
}

type afterModelSave struct {
	hws.ModelStore
	failed bool
}

func (s *afterModelSave) FinishModel(c context.Context, a hws.ModelAttempt, r *hws.ModelArtifact, status hws.ProviderStatus) error {
	if e := s.ModelStore.FinishModel(c, a, r, status); e != nil {
		return e
	}
	if !s.failed {
		s.failed = true
		return errors.New("simulated process lost result after durable save")
	}
	return nil
}
func modelRoute() hws.ModelRoute {
	return hws.ModelRoute{Versions: hws.ModelVersions{Schema: "model.v1", Capability: "cognition.v1", Model: "configured-fake", Prompt: "cognition.v1", Policy: "policy.v1"}, Capabilities: []hws.ModelCapability{hws.ModelAppraisal, hws.ModelInterpretation, hws.ModelReconciliation, hws.ModelCandidates}, InputMicrosPerToken: 1, OutputMicrosPerToken: 1, MaxOutputTokens: 256, Limits: hws.ModelLimits{Version: 1, Tokens: 6 * 60000, SpendMicros: 6 * 60000, MaxInFlight: 1, MaxAttempts: 3, AttemptTokens: 60000, AttemptSpendMicros: 60000, Timeout: time.Second}}
}
func TestModelIntegration(t *testing.T) {
	dsn := os.Getenv("DREAM_TEST_DSN")
	if dsn == "" {
		t.Skip("mandatory migration-check supplies fresh cluster")
	}
	ctx := context.Background()
	admin, e := pgxpool.New(ctx, dsn)
	if e != nil {
		t.Fatal(e)
	}
	defer admin.Close()
	if e = Migrate(ctx, admin); e != nil {
		t.Fatal(e)
	}

	t.Run("V3UpgradePreservesExistingRun", func(t *testing.T) {
		exec(t, admin, `CREATE DATABASE dream_model_v3`)
		upgradeConfig, e := pgxpool.ParseConfig(dsn)
		if e != nil {
			t.Fatal(e)
		}
		upgradeConfig.ConnConfig.Database = "dream_model_v3"
		old, e := pgxpool.NewWithConfig(ctx, upgradeConfig)
		if e != nil {
			t.Fatal(e)
		}
		defer old.Close()
		exec(t, old, migrations.Initial)
		exec(t, old, migrations.Runtime)
		exec(t, old, migrations.ReaderScopes)
		prior := New(old)
		manifest := runtimeManifest(t, "model-upgrade")
		snapshot, e := prior.CreateRun(ctx, manifest)
		if e != nil {
			t.Fatal(e)
		}
		if e = Migrate(ctx, old); e != nil {
			t.Fatal(e)
		}
		if e = Migrate(ctx, old); e != nil {
			t.Fatal(e)
		}
		restored, e := prior.LoadRun(ctx, manifest.Scope)
		if e != nil {
			t.Fatal(e)
		}
		before, _ := snapshot.State.Hash()
		after, _ := restored.State.Hash()
		if before != after || snapshot.Revision != restored.Revision {
			t.Fatal("v3 upgrade changed canonical state")
		}
		if e = prior.ConfigureModels(ctx, manifest.Scope, modelRoute().Limits); e != nil {
			t.Fatal("new model budget rejected existing run", e)
		}
		if n := count(t, old, `SELECT count(*) FROM dream.schema_versions WHERE version IN(1,2,3,4,5)`); n != 5 {
			t.Fatal("forward ledger", n)
		}
	})

	exec(t, admin, `CREATE ROLE dream_model_test LOGIN PASSWORD 'disposable_model' IN ROLE dream_writer`)
	cfg, e := pgxpool.ParseConfig(dsn)
	if e != nil {
		t.Fatal(e)
	}
	cfg.ConnConfig.User = "dream_model_test"
	cfg.ConnConfig.Password = "disposable_model"
	db, e := pgxpool.NewWithConfig(ctx, cfg)
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	store := New(db)
	fixture := func(t *testing.T, name string) (hws.ModelRequest, *hws.ViewService, graph.MemoryRecord) {
		t.Helper()
		m := runtimeManifest(t, "model-"+name)
		if _, e := store.CreateRun(ctx, m); e != nil {
			t.Fatal(e)
		}
		realm := hws.ViewRealm{Scope: m.Scope, Principal: "a"}
		scope, _ := realm.MemoryScope()
		record := graph.MemoryRecord{Event: core.Event{Version: 1, Type: graph.MemoryEventType, Stream: "cognition", Subject: core.Subject{Principal: "a"}, Meta: core.Metadata{ID: "own", Observer: "a", Source: "a", Sensitivity: core.Restricted, Confidence: .7, Valid: core.Interval{}, RecordedAt: time.Unix(1, 0), Rights: core.Rights{Resource: "own", Grants: []core.Grant{{Actor: "a", Recipient: "a", Purpose: "simulation", Operation: core.Read}, {Actor: "a", Recipient: "a", Purpose: "simulation", Operation: core.Derive}}}}}, Content: graph.MemoryContent{Version: 1, Kind: graph.EpisodicMemory, Text: "APPROVED_FICTIONAL_MEMORY", Salience: .5, HalfLife: 100, Learned: []graph.Learned{{Actor: "a", At: 0}}}}
		if _, e := (graph.MemoryService{Journal: store}).Put(ctx, scope, "own", 0, record, "simulation"); e != nil {
			t.Fatal(e)
		}
		views, e := hws.NewViewService(store, store, futureClock{}, []hws.ViewGrant{{Caller: "a", Realm: realm, Kind: hws.ActorViewKind, Purpose: "simulation", Operations: []core.Operation{core.Read, core.Derive}}}, store)
		if e != nil {
			t.Fatal(e)
		}
		permit, e := views.Permit("a", realm, hws.ActorViewKind, "simulation")
		if e != nil {
			t.Fatal(e)
		}
		approved, d, e := views.Propose(ctx, permit, []core.ID{"own"}, "a", core.Derive, graph.InternalContext)
		if e != nil || !d.Allowed {
			t.Fatal(d, e)
		}
		return hws.ModelRequest{Scope: m.Scope, Principal: "a", Key: "decision", Capability: hws.ModelAppraisal, Permit: permit, Approved: approved}, views, record
	}

	t.Run("UsageLedgerRetainsReservationsAcrossRestartAndFailure", func(t *testing.T) {
		request, views, _ := fixture(t, "usage-ledger")
		route := modelRoute()
		gateway, _ := hws.NewModelGateway(store, views, model.Fake{}, route)
		if _, e := gateway.Execute(ctx, request); e != nil {
			t.Fatal(e)
		}
		usage, e := New(db).ReadModelUsage(ctx, request.Scope)
		if e != nil || usage.ReservedTokens != 60000 || usage.KnownAttempts != 1 || usage.KnownInputTokens != 1 || usage.KnownOutputTokens != 1 {
			t.Fatal(usage, e)
		}
		if _, e = gateway.Execute(ctx, request); e != nil {
			t.Fatal(e)
		}
		again, e := New(db).ReadModelUsage(ctx, request.Scope)
		if e != nil || again != usage {
			t.Fatal("retry changed ledger", again, e)
		}
		for _, sql := range []string{"UPDATE dream.model_usage SET input_tokens=0", "DELETE FROM dream.model_usage"} {
			if _, e := db.Exec(ctx, sql); e == nil {
				t.Fatal("mutable usage audit")
			}
		}
		request.Key = "outage"
		gateway, _ = hws.NewModelGateway(New(db), views, modelFunc(func(context.Context, hws.ProviderInput) (hws.ProviderResponse, error) {
			return hws.ProviderResponse{}, errors.New("synthetic outage")
		}), route)
		if _, e = gateway.Execute(ctx, request); !errors.Is(e, hws.ErrModelUncertain) {
			t.Fatal(e)
		}
		usage, e = New(db).ReadModelUsage(ctx, request.Scope)
		if e != nil || usage.ReservedTokens != 240000 || usage.ReservedSpendMicros != 240000 || usage.KnownAttempts != 1 || usage.SettledUnknownAttempts != 3 {
			t.Fatal("outage refunded or hidden", usage, e)
		}
		request.Key = "audit-unavailable"
		exec(t, admin, `CREATE FUNCTION dream.reject_usage() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'synthetic audit outage'; END $$; CREATE TRIGGER reject_usage BEFORE INSERT ON dream.model_usage FOR EACH ROW EXECUTE FUNCTION dream.reject_usage()`)
		gateway, _ = hws.NewModelGateway(New(db), views, model.Fake{}, route)
		_, e = gateway.Execute(ctx, request)
		exec(t, admin, `DROP TRIGGER reject_usage ON dream.model_usage; DROP FUNCTION dream.reject_usage()`)
		if e == nil {
			t.Fatal("unaudited provider result accepted")
		}
		usage, e = New(db).ReadModelUsage(ctx, request.Scope)
		if e != nil || usage.KnownAttempts != 1 || usage.RunningAttempts != 1 || usage.ReservedTokens != 300000 {
			t.Fatal("failed settlement was not atomic", usage, e)
		}
		exec(t, admin, `UPDATE dream.model_requests SET expires=clock_timestamp()-interval '1 second' WHERE key='audit-unavailable'`)
		exec(t, admin, `CREATE FUNCTION dream.reject_recovery() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF convert_from(NEW.envelope,'UTF8') LIKE '%model.recovery.v1%' THEN RAISE EXCEPTION 'synthetic audit outage'; END IF; RETURN NEW; END $$; CREATE TRIGGER reject_recovery BEFORE INSERT ON dream.events FOR EACH ROW EXECUTE FUNCTION dream.reject_recovery()`)
		if n, e := store.ReconcileModels(ctx, request.Scope); e == nil || n != 0 {
			t.Fatal("unaudited recovery", n, e)
		}
		exec(t, admin, `DROP TRIGGER reject_recovery ON dream.events; DROP FUNCTION dream.reject_recovery()`)
		var recovered atomic.Int64
		var recoveryWG sync.WaitGroup
		for range 8 {
			recoveryWG.Go(func() {
				n, e := New(db).ReconcileModels(ctx, request.Scope)
				if e != nil {
					t.Error(e)
				}
				recovered.Add(int64(n))
			})
		}
		recoveryWG.Wait()
		if recovered.Load() != 1 {
			t.Fatal("recovery duplicate or missing", recovered.Load())
		}
		usage, e = New(db).ReadModelUsage(ctx, request.Scope)
		if e != nil || usage.UncertainRequests != 1 || usage.ReservedTokens != 300000 || usage.KnownAttempts != 1 {
			t.Fatal("recovery refunded uncertainty", usage, e)
		}
		if _, _, e = store.FailedModel(ctx, request.Scope, request.Key); e == nil {
			t.Fatal("uncertain call became behavioral WAIT")
		}
		if _, e = gateway.Execute(ctx, request); e != nil {
			t.Fatal("explicit retry after recovery", e)
		}
		usage, e = New(db).ReadModelUsage(ctx, request.Scope)
		if e != nil || usage.ReservedTokens != 360000 || usage.KnownAttempts != 2 || usage.UncertainRequests != 0 {
			t.Fatal("retry ledger", usage, e)
		}
		request.Key = "over-total-after-restart"
		gateway, _ = hws.NewModelGateway(New(db), views, model.Fake{}, route)
		if _, e = gateway.Execute(ctx, request); !errors.Is(e, hws.ErrModelBudget) {
			t.Fatal("restart reset budget", e)
		}

	})

	t.Run("ProcessProviderCeilingAcrossRunScopesAndCancellation", func(t *testing.T) {
		requests := make([]hws.ModelRequest, 9)
		gateways := make([]*hws.ModelGateway, 9)
		entered := make(chan struct{}, 9)
		release := make(chan struct{})
		var once sync.Once
		unblock := func() { once.Do(func() { close(release) }) }
		defer unblock()
		var calls atomic.Int64
		provider := modelFunc(func(c context.Context, i hws.ProviderInput) (hws.ProviderResponse, error) {
			if calls.Add(1) > 8 {
				return (model.Fake{}).Generate(c, i)
			}
			entered <- struct{}{}
			<-release
			return (model.Fake{}).Generate(c, i)
		})
		for index := range requests {
			r, v, _ := fixture(t, fmt.Sprintf("host-cap-%d", index))
			requests[index] = r
			gateways[index], _ = hws.NewModelGateway(store, v, provider, modelRoute())
		}
		bounded, cancel := context.WithCancel(ctx)
		defer cancel()
		var wg sync.WaitGroup
		// Retain each goroutine's error. When admission does not happen these are
		// the only evidence of why, and discarding them left the failure below
		// unable to say anything beyond the fact that it failed (#73). Each
		// goroutine writes its own element, and the slice is read only after
		// wg.Wait(), so there is no aliasing and no race.
		failures := make([]error, 8)
		for i := range 8 {
			wg.Go(func() { _, failures[i] = gateways[i].Execute(bounded, requests[i]) })
		}
		admitted := 0
		for range 8 {
			select {
			case <-entered:
				admitted++
			case <-time.After(5 * time.Second):
				unblock()
				wg.Wait()
				t.Fatal("provider admission missing: admitted", admitted, "of 8; per-gateway Execute errors:", failures)
			}
		}
		if _, e := gateways[8].Execute(ctx, requests[8]); !errors.Is(e, hws.ErrModelBusy) {
			t.Fatal("cross-run provider cap absent", e)
		}
		cancel()
		if _, e := gateways[8].Execute(ctx, requests[8]); !errors.Is(e, hws.ErrModelBusy) {
			t.Fatal("cancel freed physically active slots", e)
		}
		if calls.Load() != 8 {
			t.Fatal("too many physical provider calls", calls.Load())
		}
		unblock()
		wg.Wait()
		if _, e := gateways[8].Execute(ctx, requests[8]); e != nil {
			t.Fatal("provider slots did not recover", e)
		}
	})
	t.Run("DurableResponseCrashAndExactlyOnceTransition", func(t *testing.T) {
		request, views, record := fixture(t, "crash")
		var calls int
		provider := modelFunc(func(ctx context.Context, i hws.ProviderInput) (hws.ProviderResponse, error) {
			calls++
			return (model.Fake{}).Generate(ctx, i)
		})
		gateway, e := hws.NewModelGateway(&afterModelSave{ModelStore: store}, views, provider, modelRoute())
		if e != nil {
			t.Fatal(e)
		}
		if _, e = gateway.Execute(ctx, request); e == nil {
			t.Fatal("lost-response fault did not fire")
		}
		gateway, e = hws.NewModelGateway(New(db), views, provider, modelRoute())
		if e != nil {
			t.Fatal(e)
		}
		artifact, e := gateway.Execute(ctx, request)
		if e != nil || calls != 1 {
			t.Fatal("restart called provider", calls, e)
		}
		again, e := gateway.Execute(ctx, request)
		if e != nil || again.Hash != artifact.Hash || calls != 1 {
			t.Fatal("replay changed", e)
		}
		lease, e := store.Acquire(ctx, request.Scope, "worker", time.Minute)
		if e != nil {
			t.Fatal(e)
		}
		if _, e = (hws.Runtime{Store: store}).Execute(ctx, request.Scope, lease, "resume", rt.Command{Kind: "resume"}); e != nil {
			t.Fatal(e)
		}
		runtime := hws.Runtime{Store: store, Handler: modelHandler(func(_ rt.State, _ rt.Input, _ rt.Clock, _ *rt.Random) (rt.Output, error) {
			return rt.Output{Data: artifact.Hash}, nil
		}), Model: &hws.ModelUse{Key: request.Key, Hash: artifact.Hash}}
		request.Approved, _, e = views.Propose(ctx, request.Permit, []core.ID{"own"}, "a", core.Derive, graph.InternalContext)
		if e != nil {
			t.Fatal(e)
		}
		tampered := artifact
		tampered.Output.Observer = "b"
		if _, e = gateway.Apply(ctx, request, tampered, runtime, lease, "tampered"); e == nil {
			t.Fatal("modified model output accepted with old hash")
		}
		receipt, e := gateway.Apply(ctx, request, artifact, runtime, lease, "apply-model")
		if e != nil {
			t.Fatal(e)
		}
		retry, e := runtime.Execute(ctx, request.Scope, lease, "apply-model", rt.Command{Kind: "step"})
		if e != nil || retry != receipt {
			t.Fatal("duplicate commit", retry, e)
		}
		if _, e = runtime.Execute(ctx, request.Scope, lease, "apply-again", rt.Command{Kind: "step"}); e == nil {
			t.Fatal("same artifact applied twice")
		}
		scope, _ := (hws.ViewRealm{Scope: request.Scope, Principal: "a"}).MemoryScope()
		revoke := record.Event
		revoke.Type = "revoke"
		revoke.Meta.ID = "revoke"
		revoke.Meta.Rights = core.Rights{Resource: "revoke"}
		if _, e = graph.RevokeMemory(ctx, store, scope, "revoke", 1, revoke, "own"); e != nil {
			t.Fatal(e)
		}
		if n := count(t, admin, `SELECT count(*) FROM dream.model_payloads p JOIN dream.tombstones t USING(actor,namespace,event_id)`); n != 0 {
			t.Fatal("model artifacts resurrectable", n)
		}
		if _, e = store.LoadRun(ctx, request.Scope); e == nil {
			t.Fatal("derived checkpoint survived source revocation")
		}
		if _, e = gateway.Execute(ctx, request); e == nil {
			t.Fatal("cached response bypassed revocation")
		}
	})
	t.Run("RetriesReservedBudgetAndUnknownUsage", func(t *testing.T) {
		request, views, _ := fixture(t, "retry")
		route := modelRoute()
		calls := 0
		provider := modelFunc(func(ctx context.Context, i hws.ProviderInput) (hws.ProviderResponse, error) {
			calls++
			if calls < 3 {
				return hws.ProviderResponse{Status: hws.ProviderRateLimited}, nil
			}
			return (model.Fake{}).Generate(ctx, i)
		})
		gateway, _ := hws.NewModelGateway(store, views, provider, route)
		if _, e := gateway.Execute(ctx, request); e != nil || calls != 3 {
			t.Fatal(calls, e)
		}
		if n := count(t, admin, `SELECT tokens FROM dream.model_budgets WHERE run='model-retry'`); n != 180000 {
			t.Fatal("retries refunded reservation", n)
		}
		// Budget cannot be enlarged by a restart or a new caller configuration.
		enlarged := route.Limits
		enlarged.Tokens++
		if e := store.ConfigureModels(ctx, request.Scope, enlarged); e == nil {
			t.Fatal("restart reset limits")
		}
		bad := route
		bad.InputMicrosPerToken = 0
		if _, e := hws.NewModelGateway(store, views, provider, bad); e == nil {
			t.Fatal("unknown price accepted")
		}
		request2, views2, _ := fixture(t, "usage")
		unknown := modelFunc(func(ctx context.Context, i hws.ProviderInput) (hws.ProviderResponse, error) {
			r, e := (model.Fake{}).Generate(ctx, i)
			r.UsageKnown = false
			return r, e
		})
		gateway, _ = hws.NewModelGateway(store, views2, unknown, route)
		if _, e := gateway.Execute(ctx, request2); e == nil {
			t.Fatal("unknown usage accepted")
		}
		if n := count(t, admin, `SELECT tokens FROM dream.model_budgets WHERE run='model-usage'`); n != 60000 {
			t.Fatal("unknown usage refunded")
		}
	})

	t.Run("TokenAndSpendExhaustionBeforeProvider", func(t *testing.T) {
		for _, dimension := range []string{"tokens", "spend"} {
			request, views, _ := fixture(t, "exhaust-"+dimension)
			route := modelRoute()
			if dimension == "tokens" {
				route.Limits.Tokens = route.Limits.AttemptTokens
			} else {
				route.Limits.SpendMicros = route.Limits.AttemptSpendMicros
			}
			calls := 0
			provider := modelFunc(func(c context.Context, i hws.ProviderInput) (hws.ProviderResponse, error) {
				calls++
				return (model.Fake{}).Generate(c, i)
			})
			gateway, _ := hws.NewModelGateway(store, views, provider, route)
			if _, e := gateway.Execute(ctx, request); e != nil {
				t.Fatal("first authorized request denied", e)
			}
			request.Key = "second"
			if _, e := gateway.Execute(ctx, request); !errors.Is(e, hws.ErrModelBudget) || calls != 1 {
				t.Fatal("exhausted budget reached provider", dimension, calls, e)
			}
		}
	})
	t.Run("PolicyBeforeOutboundContext", func(t *testing.T) {
		request, views, _ := fixture(t, "outbound")
		calls := 0
		provider := modelFunc(func(c context.Context, i hws.ProviderInput) (hws.ProviderResponse, error) {
			calls++
			if i.Actor != "a" || len(i.Context) != 1 || i.Context[0].Source != "own" || i.Context[0].Text != "APPROVED_FICTIONAL_MEMORY" {
				t.Error("outbound request contains unapproved context")
			}
			return (model.Fake{}).Generate(c, i)
		})
		gateway, _ := hws.NewModelGateway(store, views, provider, modelRoute())
		bad := request
		bad.Approved = graph.ApprovedContext{}
		if _, e := gateway.Execute(ctx, bad); e == nil || calls != 0 {
			t.Fatal("unapproved request reached provider")
		}
		if _, e := gateway.Execute(ctx, request); e != nil || calls != 1 {
			t.Fatal("approved context omitted", e)
		}
	})

	t.Run("ConcurrentReservationAndCancellation", func(t *testing.T) {
		request, views, _ := fixture(t, "concurrent")
		entered := make(chan struct{})
		release := make(chan struct{})
		var calls atomic.Int32
		provider := modelFunc(func(c context.Context, i hws.ProviderInput) (hws.ProviderResponse, error) {
			calls.Add(1)
			close(entered)
			select {
			case <-release:
				return (model.Fake{}).Generate(c, i)
			case <-c.Done():
				return hws.ProviderResponse{}, c.Err()
			}
		})
		gateway, _ := hws.NewModelGateway(store, views, provider, modelRoute())
		var wg sync.WaitGroup
		wg.Add(1)
		var firstErr error
		go func() { defer wg.Done(); _, firstErr = gateway.Execute(ctx, request) }()
		<-entered
		if _, e := gateway.Execute(ctx, request); !errors.Is(e, hws.ErrModelBusy) {
			t.Fatal("duplicate writer", e)
		}
		other := request
		other.Key = "other"
		if _, e := gateway.Execute(ctx, other); !errors.Is(e, hws.ErrModelBusy) {
			t.Fatal("in-flight cap", e)
		}
		close(release)
		wg.Wait()
		if firstErr != nil || calls.Load() != 1 {
			t.Fatal(firstErr, calls.Load())
		}
		cancelled, cancel := context.WithCancel(ctx)
		cancel()
		if _, e := gateway.Execute(cancelled, other); e == nil {
			t.Fatal("cancelled work launched")
		}
	})

	t.Run("PolicyRevokedDuringTransitionDeniesCommit", func(t *testing.T) {
		request, views, _ := fixture(t, "policy-commit")
		lease, e := store.Acquire(ctx, request.Scope, "worker", time.Minute)
		if e != nil {
			t.Fatal(e)
		}
		if _, e = (hws.Runtime{Store: store}).Execute(ctx, request.Scope, lease, "resume", rt.Command{Kind: "resume"}); e != nil {
			t.Fatal(e)
		}
		request.Approved, _, e = views.Propose(ctx, request.Permit, []core.ID{"own"}, "a", core.Derive, graph.InternalContext)
		if e != nil {
			t.Fatal(e)
		}
		gateway, _ := hws.NewModelGateway(store, views, model.Fake{}, modelRoute())
		artifact, e := gateway.Execute(ctx, request)
		if e != nil {
			t.Fatal(e)
		}
		runtime := hws.Runtime{Store: store, Handler: modelHandler(func(_ rt.State, _ rt.Input, _ rt.Clock, _ *rt.Random) (rt.Output, error) {
			views.Revoke(request.Permit)
			return rt.Output{Data: artifact.Hash}, nil
		})}
		if _, e = gateway.Apply(ctx, request, artifact, runtime, lease, "apply"); !errors.Is(e, hws.ErrViewDenied) {
			t.Fatal("revoked policy applied state", e)
		}
		if n := count(t, admin, `SELECT count(*) FROM dream.model_applications WHERE run='model-policy-commit'`); n != 0 {
			t.Fatal("revoked policy consumed artifact")
		}
	})

	t.Run("ExpiredAttemptFencingAndScopeIsolation", func(t *testing.T) {
		request, views, _ := fixture(t, "fencing")
		safe, e := views.ModelContext(ctx, request.Permit, request.Approved, request.Scope, request.Principal)
		if e != nil {
			t.Fatal(e)
		}
		route := modelRoute()
		scope, _ := (hws.ViewRealm{Scope: request.Scope, Principal: "a"}).MemoryScope()
		input := hws.ProviderInput{Version: 1, Key: request.Key, Capability: request.Capability, Versions: route.Versions, Actor: "a", Context: safe.Items(), MaxOutputTokens: route.MaxOutputTokens}
		intent := hws.ModelIntent{Version: 1, MemoryRevision: safe.Revision(), At: safe.KnownAt(), Scope: request.Scope, Principal: "a", MemoryScope: scope, Key: request.Key, Input: input}
		if e = store.ConfigureModels(ctx, request.Scope, route.Limits); e != nil {
			t.Fatal(e)
		}
		first, e := store.BeginModel(ctx, intent, route.Limits)
		if e != nil {
			t.Fatal(e)
		}
		exec(t, admin, `UPDATE dream.model_requests SET expires=clock_timestamp()-interval '1 second' WHERE run='model-fencing'`)
		second, e := store.BeginModel(ctx, intent, route.Limits)
		if e != nil || second.Fence <= first.Fence {
			t.Fatal(second, e)
		}
		if e = store.FinishModel(ctx, first, nil, hws.ProviderUnavailable); !errors.Is(e, hws.ErrConflict) {
			t.Fatal("old attempt overwrote new", e)
		}
		if e = store.FinishModel(ctx, second, nil, hws.ProviderUnavailable); e != nil {
			t.Fatal(e)
		}
		third, e := store.BeginModel(ctx, intent, route.Limits)
		if e != nil {
			t.Fatal(e)
		}
		if e = store.FinishModel(ctx, third, nil, hws.ProviderUnavailable); e != nil {
			t.Fatal(e)
		}
		if _, e = store.BeginModel(ctx, intent, route.Limits); !errors.Is(e, hws.ErrModelUncertain) {
			t.Fatal("restart reset attempts", e)
		}
		if n := count(t, admin, `SELECT tokens FROM dream.model_budgets WHERE run='model-fencing'`); n != 180000 {
			t.Fatal("crash reservation refunded", n)
		}
		wrong := request
		wrong.Scope.Branch = "other"
		if _, e = views.ModelContext(ctx, wrong.Permit, wrong.Approved, wrong.Scope, wrong.Principal); e == nil {
			t.Fatal("cross-branch model authority")
		}
	})
	t.Run("ContextChangeBeforeCanonicalCommitDenies", func(t *testing.T) {
		request, views, record := fixture(t, "stale")
		gateway, _ := hws.NewModelGateway(store, views, model.Fake{}, modelRoute())
		artifact, e := gateway.Execute(ctx, request)
		if e != nil {
			t.Fatal(e)
		}
		scope, _ := (hws.ViewRealm{Scope: request.Scope, Principal: "a"}).MemoryScope()
		corrected := record
		corrected.Event.Meta.ID = "correction"
		corrected.Event.Meta.Rights.Resource = "correction"
		corrected.Supersedes = "own"
		if _, e = (graph.MemoryService{Journal: store}).Put(ctx, scope, "correction", 1, corrected, "simulation"); e != nil {
			t.Fatal(e)
		}
		lease, e := store.Acquire(ctx, request.Scope, "worker", time.Minute)
		if e != nil {
			t.Fatal(e)
		}
		runtime := hws.Runtime{Store: store, Handler: runtimeFake{}}
		if _, e = runtime.Execute(ctx, request.Scope, lease, "resume", rt.Command{Kind: "resume"}); e != nil {
			t.Fatal(e)
		}
		runtime.Model = &hws.ModelUse{Key: request.Key, Hash: artifact.Hash}
		if _, e = runtime.Execute(ctx, request.Scope, lease, "apply-stale", rt.Command{Kind: "step"}); e == nil {
			t.Fatal("same-text correction did not invalidate artifact")
		}
		if n := count(t, admin, `SELECT count(*) FROM dream.model_applications WHERE run='model-stale'`); n != 0 {
			t.Fatal("stale result applied")
		}
	})

	t.Run("RunAndAttemptDeadlinesDenyLateSuccess", func(t *testing.T) {
		request, views, _ := fixture(t, "deadline")
		calls := 0
		provider := modelFunc(func(c context.Context, i hws.ProviderInput) (hws.ProviderResponse, error) {
			calls++
			<-c.Done()
			return (model.Fake{}).Generate(context.Background(), i)
		})
		route := modelRoute()
		route.Limits.Timeout = 100 * time.Millisecond
		gateway, _ := hws.NewModelGateway(store, views, provider, route)
		if _, e := gateway.Execute(ctx, request); e == nil || calls != 3 {
			t.Fatal("late result or retry budget", calls, e)
		}
		if n := count(t, admin, `SELECT count(*) FROM dream.model_requests WHERE run='model-deadline' AND status='complete'`); n != 0 {
			t.Fatal("late success saved")
		}
		request, views, _ = fixture(t, "expired-run")
		calls = 0
		exec(t, admin, `UPDATE dream.runtime_heads SET deadline=clock_timestamp()-interval '1 second' WHERE run='model-expired-run'`)
		gateway, _ = hws.NewModelGateway(store, views, provider, modelRoute())
		if _, e := gateway.Execute(ctx, request); !errors.Is(e, hws.ErrDeadline) || calls != 0 {
			t.Fatal("provider launched after run deadline", calls, e)
		}
	})

	t.Run("RevocationDuringProvider", func(t *testing.T) {
		request, views, record := fixture(t, "revoke")
		provider := modelFunc(func(ctx context.Context, i hws.ProviderInput) (hws.ProviderResponse, error) {
			scope, _ := (hws.ViewRealm{Scope: request.Scope, Principal: "a"}).MemoryScope()
			revoke := record.Event
			revoke.Type = "revoke"
			revoke.Meta.ID = "revoke"
			revoke.Meta.Rights = core.Rights{Resource: "revoke"}
			if _, e := graph.RevokeMemory(ctx, store, scope, "revoke", 1, revoke, "own"); e != nil {
				return hws.ProviderResponse{}, e
			}
			return (model.Fake{}).Generate(ctx, i)
		})
		gateway, _ := hws.NewModelGateway(store, views, provider, modelRoute())
		if _, e := gateway.Execute(ctx, request); e == nil {
			t.Fatal("output after revoke")
		}
		if n := count(t, admin, `SELECT count(*) FROM dream.model_payloads p JOIN dream.tombstones t USING(actor,namespace,event_id)`); n != 0 {
			t.Fatal("pending intent not purged")
		}
	})
}

type modelHandler func(rt.State, rt.Input, rt.Clock, *rt.Random) (rt.Output, error)

func (f modelHandler) Transition(s rt.State, i rt.Input, c rt.Clock, r *rt.Random) (rt.Output, error) {
	return f(s, i, c, r)
}
