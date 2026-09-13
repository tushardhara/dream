package postgres

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/tushardhara/dream/app/graph"
	"github.com/tushardhara/dream/app/hws"
	"github.com/tushardhara/dream/core"
)

type policyWriter func(context.Context, graph.SafeContext) (graph.WriterDraft, error)

func (w policyWriter) Write(ctx context.Context, s graph.SafeContext) (graph.WriterDraft, error) {
	return w(ctx, s)
}
func TestInformationPolicyIntegration(t *testing.T) {
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
	exec(t, admin, `CREATE ROLE dream_policy_test LOGIN PASSWORD 'disposable_policy' IN ROLE dream_writer`)
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.User = "dream_policy_test"
	cfg.ConnConfig.Password = "disposable_policy"
	db, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := New(db)
	manifest := runtimeManifest(t, "policy-boundary")
	if _, err = store.CreateRun(ctx, manifest); err != nil {
		t.Fatal(err)
	}
	realm := hws.ViewRealm{Scope: manifest.Scope, Principal: "a"}
	scope, err := realm.MemoryScope()
	if err != nil {
		t.Fatal(err)
	}
	grants := []hws.ViewGrant{{Caller: "a", Realm: realm, Kind: hws.ActorViewKind, Purpose: "simulation", Operations: []core.Operation{core.Read, core.Disclose}}}
	views, err := hws.NewViewService(store, store, futureClock{}, grants, store)
	if err != nil {
		t.Fatal(err)
	}
	permit, err := views.Permit("a", realm, hws.ActorViewKind, "simulation")
	if err != nil {
		t.Fatal(err)
	}
	memory := graph.MemoryService{Journal: store}
	record := graph.MemoryRecord{Event: core.Event{Version: 1, Type: graph.MemoryEventType, Stream: "cognition", Subject: core.Subject{Principal: "a"}, OccurredAt: 0, Meta: core.Metadata{ID: "own", Observer: "a", Source: "a", Sensitivity: core.Restricted, Confidence: .7, Valid: core.Interval{}, RecordedAt: time.Unix(1, 0), Rights: core.Rights{Resource: "own", Grants: []core.Grant{{Actor: "a", Recipient: "a", Purpose: "simulation", Operation: core.Read}, {Actor: "a", Recipient: "a", Purpose: "simulation", Operation: core.Derive}, {Actor: "a", Recipient: "b", Purpose: "simulation", Operation: core.Disclose}}}}}, Content: graph.MemoryContent{Version: 1, Kind: graph.EpisodicMemory, Text: "I felt supported in this fictional interaction", Salience: .5, HalfLife: 100, Learned: []graph.Learned{{Actor: "a", At: 0}}}}
	if _, err = memory.Put(ctx, scope, "own", 0, record, "simulation"); err != nil {
		t.Fatal(err)
	}
	cap, decision, err := views.Propose(ctx, permit, []core.ID{"own"}, "b", core.Disclose, graph.SyntheticSelfDisclosure)
	if err != nil || !decision.Allowed {
		t.Fatal(decision, err)
	}
	writer := policyWriter(func(_ context.Context, c graph.SafeContext) (graph.WriterDraft, error) {
		item := c.Items()[0]
		return graph.WriterDraft{Spans: []graph.WriterSpan{{Source: item.Source, End: len(item.Text)}}}, nil
	})
	out, decision, err := views.Write(ctx, permit, cap, writer)
	if err != nil || !decision.Allowed || out.Text != record.Content.Text {
		t.Fatal(out, decision, err)
	}
	// Confirm that a new future record cannot enter context by asking with a
	// larger client-selected time: Propose always binds to current durable time.
	later := record
	later.Event.Meta.ID = "future"
	later.Event.Meta.Rights.Resource = "future"
	later.Content.Learned = []graph.Learned{{Actor: "a", At: 1}}
	later.Content.Text = "FUTURE_SECRET"
	if _, err = memory.Put(ctx, scope, "future", 1, later, "simulation"); err != nil {
		t.Fatal(err)
	}
	if _, d, err := views.Propose(ctx, permit, []core.ID{"future"}, "b", core.Disclose, graph.SyntheticSelfDisclosure); err != nil || d.Allowed {
		t.Fatal("future model context", d, err)
	}
	cap, decision, err = views.Propose(ctx, permit, []core.ID{"own"}, "b", core.Disclose, graph.SyntheticSelfDisclosure)
	if err != nil || !decision.Allowed {
		t.Fatal(decision, err)
	}
	revokingWriter := policyWriter(func(ctx context.Context, c graph.SafeContext) (graph.WriterDraft, error) {
		revoke := record.Event
		revoke.Type = "revoke"
		revoke.Meta.ID = "revoke"
		revoke.Meta.Rights = core.Rights{Resource: "revoke"}
		if _, err := graph.RevokeMemory(ctx, store, scope, "revoke", 2, revoke, "own"); err != nil {
			return graph.WriterDraft{}, err
		}
		return writer(ctx, c)
	})
	out, decision, err = views.Write(ctx, permit, cap, revokingWriter)
	if err != nil || decision.Allowed || out.Text != "" {
		t.Fatal("revocation during writer returned private output", out, decision, err)
	}
	if count(t, admin, `SELECT count(*) FROM dream.private_payloads WHERE actor='a' AND namespace=$1 AND event_id='own'`, scope.Namespace) != 0 {
		t.Fatal("source not purged")
	}
	if _, d, err := views.Propose(ctx, permit, []core.ID{"own"}, "b", core.Disclose, graph.SyntheticSelfDisclosure); err != nil || d.Allowed {
		t.Fatal("revoked source reauthorized", d, err)
	}
	auditHash := sha256.Sum256([]byte(permit.Binding()))
	auditNamespace := "audit:" + hex.EncodeToString(auditHash[:])
	// CASE guards JSON decoding: SQL may reorder WHERE predicates, and other
	// event types in this shared disposable cluster carry non-JSON text.
	if count(t, admin, `SELECT count(*) FROM dream.events e JOIN dream.private_payloads p ON (e.actor,e.namespace,e.id)=(p.actor,p.namespace,p.event_id) WHERE CASE WHEN e.actor='policy-service' AND e.namespace=$1 AND e.envelope->>'type'='policy.audit.v1' THEN (convert_from(p.payload,'UTF8')::jsonb->>'text')::jsonb->'decision'->>'Action'='WAIT' ELSE false END`, auditNamespace) == 0 {
		t.Fatal("policy denial not durably audited")
	}
	if count(t, admin, `SELECT count(*) FROM dream.events e JOIN dream.private_payloads p ON (e.actor,e.namespace,e.id)=(p.actor,p.namespace,p.event_id) WHERE e.actor='policy-service' AND e.namespace=$1 AND e.envelope->>'type'='policy.audit.v1' AND convert_from(p.payload,'UTF8') LIKE '%felt supported%'`, auditNamespace) != 0 {
		t.Fatal("audit copied private source text")
	}

}
