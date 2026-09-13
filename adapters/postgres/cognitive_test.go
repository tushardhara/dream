package postgres

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/tushardhara/dream/adapters/model"
	"github.com/tushardhara/dream/app/graph"
	"github.com/tushardhara/dream/app/hws"
	"github.com/tushardhara/dream/core"
	"github.com/tushardhara/dream/simulator/behavior"
	rt "github.com/tushardhara/dream/simulator/runtime"
)

type cognitivePlan func(context.Context, rt.Input, graph.SafeContext) (hws.CognitiveFrame, error)

func (p cognitivePlan) Plan(c context.Context, i rt.Input, s graph.SafeContext) (hws.CognitiveFrame, error) {
	return p(c, i, s)
}

type quoteOwn struct{}

func (quoteOwn) Write(_ context.Context, s graph.SafeContext) (graph.WriterDraft, error) {
	i := s.Items()[0]
	return graph.WriterDraft{Spans: []graph.WriterSpan{{Source: i.Source, Start: 0, End: len(i.Text)}}}, nil
}
func TestCognitiveIntegration(t *testing.T) {
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
	for _, name := range []string{"recorded", "disclosure", "revoked_during_plan", "forged_operational", "outage", "refused"} {
		t.Run(name, func(t *testing.T) {
			m := runtimeManifest(t, "cognitive-"+name)
			if _, e = store.CreateRun(ctx, m); e != nil {
				t.Fatal(e)
			}
			lease, e := store.Acquire(ctx, m.Scope, "cognitive-worker", time.Minute)
			if e != nil {
				t.Fatal(e)
			}
			runtime := hws.Runtime{Store: store}
			if _, e = runtime.Execute(ctx, m.Scope, lease, "resume", rt.Command{Kind: "resume"}); e != nil {
				t.Fatal(e)
			}
			realm := hws.ViewRealm{Scope: m.Scope, Principal: "b"}
			scope, _ := realm.MemoryScope()
			record := graph.MemoryRecord{Event: core.Event{Version: 1, Type: graph.MemoryEventType, Stream: "cognitive-input", Subject: core.Subject{Principal: "b"}, Meta: core.Metadata{ID: "e1", Observer: "b", Source: "b", Sensitivity: core.Restricted, Confidence: .7, Valid: core.Interval{}, RecordedAt: time.Unix(1, 0), Rights: core.Rights{Resource: "e1", Grants: []core.Grant{{Actor: "b", Recipient: "b", Purpose: "simulation", Operation: core.Read}, {Actor: "b", Recipient: "b", Purpose: "simulation", Operation: core.Derive}, {Actor: "b", Recipient: "a", Purpose: "simulation", Operation: core.Disclose}}}}}, Content: graph.MemoryContent{Version: 1, Kind: graph.EpisodicMemory, Text: "OWN_SYNTHETIC_EXPERIENCE", Salience: .5, HalfLife: 100, Learned: []graph.Learned{{Actor: "b", At: 0}}}}
			if _, e = (graph.MemoryService{Journal: store}).Put(ctx, scope, "e1", 0, record, "simulation"); e != nil {
				t.Fatal(e)
			}
			views, e := hws.NewViewService(store, store, futureClock{}, []hws.ViewGrant{{Caller: "b", Realm: realm, Kind: hws.ActorViewKind, Purpose: "simulation", Operations: []core.Operation{core.Read, core.Derive, core.Disclose}}}, store)
			if e != nil {
				t.Fatal(e)
			}
			permit, e := views.Permit("b", realm, hws.ActorViewKind, "simulation")
			if e != nil {
				t.Fatal(e)
			}
			cap, decision, e := views.Propose(ctx, permit, []core.ID{"e1"}, "b", core.Derive, graph.InternalContext)
			if e != nil || !decision.Allowed {
				t.Fatal(decision, e)
			}
			request := hws.ModelRequest{Scope: m.Scope, Principal: "b", Key: "interpret", Capability: hws.ModelInterpretation, Permit: permit, Approved: cap}
			calls := 0
			provider := modelFunc(func(c context.Context, i hws.ProviderInput) (hws.ProviderResponse, error) {
				calls++
				if name == "outage" {
					return hws.ProviderResponse{Status: hws.ProviderUnavailable}, nil
				}
				if name == "refused" {
					return hws.ProviderResponse{Status: hws.ProviderRefused}, nil
				}
				return (model.Fake{}).Generate(c, i)
			})
			gateway, e := hws.NewModelGateway(store, views, provider, modelRoute())
			if e != nil {
				t.Fatal(e)
			}
			artifact, e := gateway.Execute(ctx, request)
			if name != "outage" && name != "refused" && e != nil {
				t.Fatal(e)
			}
			if (name == "outage" || name == "refused") && e == nil {
				t.Fatal("failure unexpectedly generated artifact")
			}
			if name == "refused" {
				if _, _, err := store.FailedModel(ctx, request.Scope, request.Key); err == nil {
					t.Fatal("refusal accepted as outage evidence")
				}
			}
			if name == "outage" {
				if _, use, err := store.FailedModel(ctx, request.Scope, request.Key); err != nil || !use.Failed || len(use.Hash) != 64 {
					t.Fatal("missing durable outage evidence", err)
				}
			}
			planner := cognitivePlan(func(_ context.Context, i rt.Input, s graph.SafeContext) (hws.CognitiveFrame, error) {
				if len(s.Items()) != 1 || s.Items()[0].Source != "e1" {
					t.Fatal("unapproved planner input")
				}
				p, _ := (appraisalPerception{}).Perceive(i)
				f := hws.CognitiveFrame{Situation: behavior.Situation{Perceived: p, Horizon: 80, Offers: []behavior.Offer{{Kind: behavior.Ask, Recipient: "a", Duration: 1}}}}
				if name == "disclosure" {
					f.Situation.Offers = []behavior.Offer{{Kind: behavior.SelfDisclose, Recipient: "a", Duration: 1}}
				}
				if name == "forged_operational" {
					f.Situation.Outage = true
				}
				if name == "revoked_during_plan" {
					views.Revoke(permit)
				}
				return f, nil
			})
			service := hws.CognitiveService{Gateway: gateway, Views: views, Runtime: runtime, Planner: planner}
			var disclosure *hws.SelfDisclosure
			if name == "disclosure" {
				disclosure = &hws.SelfDisclosure{Recipient: "a", Sources: []core.ID{"e1"}, Writer: quoteOwn{}}
			}
			var receipt hws.Receipt
			use := hws.ModelUse{Key: request.Key, Hash: artifact.Hash}
			if name == "outage" || name == "refused" {
				receipt, e = service.Step(ctx, request, lease, "cognitive-step", nil)

			} else {
				receipt, e = service.Apply(ctx, request, artifact, lease, "cognitive-step", disclosure)
			}
			if name == "revoked_during_plan" || name == "forged_operational" || name == "refused" {
				if e == nil {
					t.Fatal("unsafe preparation committed")
				}
				snap, e := store.LoadRun(ctx, m.Scope)
				if e != nil || snap.State.Step != 0 {
					t.Fatal("failed preparation changed state", e)
				}
				return
			}
			if e != nil {
				t.Fatal(e)
			}
			snap, e := New(db).LoadRun(ctx, m.Scope)
			if e != nil {
				t.Fatal(e)
			}
			checkpoint, e := hws.DecodeCognitiveCheckpoint(snap.State.Data)
			if e != nil {
				t.Fatal(e)
			}
			if checkpoint.Last.Actor != "b" || (name != "outage" && checkpoint.ModelHash != artifact.Hash) || len(checkpoint.Outcomes) != 1 || checkpoint.Actors[1].State.Applied[0].Event != "e1" {
				t.Fatal("missing persisted cognitive record")
			}
			// Crash/restart uses the durable runtime operation receipt, no generation.
			if name == "outage" {
				use.Hash = checkpoint.ModelHash
				use.Failed = true
				if !checkpoint.Last.Operational || !checkpoint.Outcomes[0].Operational || len(checkpoint.Last.Candidates) != 1 {
					t.Fatal("failed provider became behavior")
				}
			}
			retryRuntime := hws.Runtime{Store: New(db), Model: &use}
			retry, e := retryRuntime.Execute(ctx, request.Scope, lease, "cognitive-step", rt.Command{Kind: "step"})
			if e != nil || retry != receipt || calls != map[bool]int{true: 3, false: 1}[name == "outage"] {
				t.Fatal("restart reapplied or regenerated", e)
			}
			deliveries := 0
			for _, q := range snap.State.Queue {
				if strings.HasPrefix(string(q.ID), "delivery:") {
					deliveries++
					if name == "disclosure" && !strings.Contains(q.Text, "OWN_SYNTHETIC_EXPERIENCE") {
						t.Fatal("own-fiction disclosure denied")
					}
					if name != "disclosure" && strings.Contains(q.Text, "EXPERIENCE") {
						t.Fatal("internal context leaked")
					}
					var content map[string]any
					if json.Unmarshal([]byte(q.Text), &content) != nil || content["recipient"] != "a" || content["sender"] != "b" {
						t.Fatal("incorrect delivery")
					}
				}
			}
			if name == "disclosure" && (deliveries != 1 || checkpoint.Last.Candidates[checkpoint.Last.Selected].Offer.Kind != behavior.SelfDisclose) {
				t.Fatal("positive self-disclosure fixture did not select disclosure")
			}
			if name == "outage" && deliveries != 0 {
				t.Fatal("outage delivered behavior")
			}
			snapshotKey, e := store.CaptureSnapshot(ctx, request.Scope, "after-cognition", receipt.Revision)
			if e != nil {
				t.Fatal(e)
			}
			frozen, e := store.ReadSnapshot(ctx, snapshotKey)
			if e != nil || len(frozen.Models) != 1 || frozen.Models[0] != use {
				t.Fatal("snapshot lost exact model reference", e)
			}
			// The ordinary source revocation mechanism reaches model-derived cognition,
			// including compressed state and queued own-fiction disclosure.
			revoke := record.Event
			revoke.Meta.ID = "revoke-e1"
			revoke.Meta.Rights = core.Rights{Resource: "revoke-e1"}
			revoke.Type = "revoke"
			if _, e = graph.RevokeMemory(ctx, store, scope, "revoke", 1, revoke, "e1"); e != nil {
				t.Fatal(e)
			}
			if _, e = store.ReadSnapshot(ctx, snapshotKey); e == nil {
				t.Fatal("revoked cognition snapshot survived")
			}
			if _, e = store.LoadRun(ctx, m.Scope); e == nil {
				t.Fatal("revoked cognition resurrected")
			}
		})
	}
}
