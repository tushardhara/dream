package postgres

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/tushardhara/dream/adapters/model"
	api "github.com/tushardhara/dream/adapters/transport"
	pb "github.com/tushardhara/dream/adapters/transport/gen/dream/v1"
	"github.com/tushardhara/dream/app/graph"
	"github.com/tushardhara/dream/app/hws"
	"github.com/tushardhara/dream/core"
	agentclient "github.com/tushardhara/dream/examples/agentclient"
	"github.com/tushardhara/dream/simulator/behavior"
	rt "github.com/tushardhara/dream/simulator/runtime"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/proto"
)

func TestAuthenticatedServerPostgresOperationsAndExport(t *testing.T) {
	dsn := os.Getenv("DREAM_TEST_DSN")
	if dsn == "" {
		t.Skip("mandatory fresh PostgreSQL cluster")
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
	exec(t, admin, `CREATE ROLE dream_transport_fixture LOGIN PASSWORD 'disposable_transport' IN ROLE dream_writer; GRANT USAGE ON SCHEMA dream TO dream_transport_fixture`)
	cfg, _ := pgxpool.ParseConfig(dsn)
	cfg.ConnConfig.User = "dream_transport_fixture"
	cfg.ConnConfig.Password = "disposable_transport"
	db, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := New(db)
	manifest := runtimeManifest(t, "authenticated-api")
	if _, err = store.CreateRun(ctx, manifest); err != nil {
		t.Fatal(err)
	}
	wire := &pb.Scope{Operator: string(manifest.Scope.Actor), Namespace: string(manifest.Scope.Namespace), World: string(manifest.Scope.World), Branch: string(manifest.Scope.Branch), Run: string(manifest.Scope.Run)}
	secrets := []string{"synthetic-research-token-longer-than-thirty-two", "synthetic-actor-token-longer-than-thirty-two", "synthetic-external-token-longer-than-thirty-two"}
	configs := []api.Credential{}
	for i, role := range []hws.ViewKind{hws.ResearchViewKind, hws.ActorViewKind, hws.ExternalViewKind} {
		sum := sha256.Sum256([]byte(secrets[i]))
		caller := core.ID("researcher")
		purpose := core.ID("research")
		if role == hws.ActorViewKind {
			caller = "a"
			purpose = "simulation"
		}
		if role == hws.ExternalViewKind {
			caller = "external"
			purpose = "simulation"
		}
		configs = append(configs, api.Credential{ID: caller, TokenSHA256: hex.EncodeToString(sum[:]), Caller: caller, Role: role, Scope: manifest.Scope, Principal: "a", Purpose: purpose, Expires: time.Now().Add(time.Hour), RequestsPerMinute: 10000, TotalRequests: 10000})
	}
	childCredential := configs[0]
	childCredential.ID = "child-researcher"
	childCredential.Scope.Branch = "api-child"
	childCredential.Scope.Run = "api-child"
	childSecret := "synthetic-child-token-longer-than-thirty-two"
	childHash := sha256.Sum256([]byte(childSecret))
	childCredential.TokenSHA256 = hex.EncodeToString(childHash[:])
	configs = append(configs, childCredential)
	auth, err := api.NewAuthenticator(configs, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	grants := []hws.ViewGrant{}
	for _, c := range configs {
		grants = append(grants, hws.ViewGrant{Caller: c.Caller, Realm: hws.ViewRealm{Scope: c.Scope, Principal: c.Principal}, Kind: c.Role, Purpose: c.Purpose, Operations: []core.Operation{core.Read, core.Retain, core.Derive, core.Export}})
	}
	views, err := hws.NewViewService(store, store, futureClock{}, grants, store)
	if err != nil {
		t.Fatal(err)
	}
	backend := &api.Backend{Auth: auth, Store: store, Views: views, Handler: modelHandler(func(rt.State, rt.Input, rt.Clock, *rt.Random) (rt.Output, error) { return rt.Output{}, nil })}
	servers, err := api.NewServers(ctx, api.ServerConfig{Auth: auth, Backend: backend, Development: true, RuntimeReady: store.RuntimeReady, Admission: func(ctx context.Context, c api.Credential) error {
		return store.AdmitRequest(ctx, hws.RequestBudget{Credential: c.ID, Scope: c.Scope, PerMinute: c.RequestsPerMinute, Total: c.TotalRequests})
	}})
	if err != nil {
		t.Fatal(err)
	}
	defer servers.Close()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = servers.ServeGRPC(listener) }()
	conn, err := grpc.NewClient(listener.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	client := pb.NewResearchClient(conn)
	research := metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+secrets[0])
	actor := metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+secrets[1])
	if _, err = client.ResearchView(actor, &pb.Query{Scope: wire}); err == nil {
		t.Fatal("actor reads research")
	}
	if _, err = client.ActorView(actor, &pb.Query{Scope: wire}); err != nil {
		t.Fatal("actor own view", err)
	}
	wrong := proto.Clone(wire).(*pb.Scope)
	wrong.Branch = "other"
	if _, err = client.Control(research, &pb.ControlRequest{Scope: wrong, OperationId: "resume", Command: "resume"}); err == nil {
		t.Fatal("cross-branch write")
	}
	first, err := client.Control(research, &pb.ControlRequest{Scope: wire, OperationId: "resume", Command: "resume"})
	if err != nil {
		t.Fatal(err)
	}
	again, err := client.Control(research, &pb.ControlRequest{Scope: wire, OperationId: "resume", Command: "resume"})
	if err != nil || !proto.Equal(first, again) {
		t.Fatal("idempotent command", err)
	}
	if _, err = client.Control(research, &pb.ControlRequest{Scope: wire, OperationId: "resume", Command: "pause"}); err == nil {
		t.Fatal("idempotency payload conflict")
	}
	snapshot, err := client.CaptureSnapshot(research, &pb.SnapshotRequest{Scope: wire, Key: "export-source", Revision: 2})
	if err != nil {
		t.Fatal(err)
	}
	childWire := proto.Clone(wire).(*pb.Scope)
	childWire.Branch = "api-child"
	childWire.Run = "api-child"
	forkRequest := &pb.ForkRequest{Source: snapshot, Child: childWire, Policy: "human-actions.v1", PairedExogenous: true, MaxDurationSeconds: 60}
	if _, err = client.Fork(research, forkRequest); err != nil {
		t.Fatal("authorized fork", err)
	}
	if _, err = client.ResearchView(research, &pb.Query{Scope: childWire}); err == nil {
		t.Fatal("parent token reads child")
	}
	childContext := metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+childSecret)
	if _, err = client.ResearchView(childContext, &pb.Query{Scope: childWire}); err != nil {
		t.Fatal("mapped child token", err)
	}
	if _, err = client.Control(childContext, &pb.ControlRequest{Scope: childWire, OperationId: "child-resume", Command: "resume"}); err != nil {
		t.Fatal(err)
	}
	var untilReceipt hws.Receipt
	for range 4 {
		out, err := client.Control(childContext, &pb.ControlRequest{Scope: childWire, OperationId: "child-until", Command: "run-until", Until: 30})
		if err != nil {
			t.Fatal("bounded run-until", err)
		}
		if json.Unmarshal(out.CanonicalJson, &untilReceipt) != nil {
			t.Fatal("receipt")
		}
		if untilReceipt.Done {
			break
		}
	}
	if !untilReceipt.Done {
		t.Fatal("run-until failed to finish")
	}
	if _, err = client.GetOperation(childContext, &pb.OperationRequest{Scope: childWire, OperationId: "child-until"}); err != nil {
		t.Fatal("durable operation status", err)
	}
	for _, kind := range []string{"pause", "resume", "cancel"} {
		if _, err = client.Control(childContext, &pb.ControlRequest{Scope: childWire, OperationId: "child-" + kind + "-after-until", Command: kind}); err != nil {
			t.Fatal("lifecycle", kind, err)
		}
	}
	forged := proto.Clone(forkRequest).(*pb.ForkRequest)
	forged.Child.Branch = "unmapped"
	forged.Child.Run = "unmapped"
	if _, err = client.Fork(research, forged); err == nil {
		t.Fatal("caller invented fork scope")
	}
	if _, err = client.Control(research, &pb.ControlRequest{Scope: wire, OperationId: "step", Command: "step"}); err != nil {
		t.Fatal(err)
	}
	submitted, err := client.SubmitExport(research, &pb.ExportRequest{Scope: wire, ExportId: "research-export", Kind: "research", Source: snapshot, ThroughRevision: 3, FromTime: 0, ThroughTime: 100})
	if err != nil {
		t.Fatal("submit", err)
	}
	var want struct {
		BodyHash string `json:"body_sha256"`
		Bytes    int    `json:"bytes"`
	}
	if err = json.Unmarshal(submitted.CanonicalJson, &want); err != nil {
		t.Fatal(err)
	}
	body := []byte{}
	cursor := ""
	for range 128 {
		page, err := client.DownloadExport(research, &pb.DownloadRequest{Scope: wire, ExportId: "research-export", PageSize: 256, Cursor: cursor})
		if err != nil {
			t.Fatal("download", err)
		}
		sum := sha256.Sum256(page.Page)
		if hex.EncodeToString(sum[:]) != page.PageSha256 {
			t.Fatal("page checksum")
		}
		body = append(body, page.Page...)
		cursor = page.NextCursor
		if cursor == "" {
			break
		}
	}
	sum := sha256.Sum256(body)
	if cursor != "" || len(body) != want.Bytes || hex.EncodeToString(sum[:]) != want.BodyHash {
		t.Fatal("export roundtrip manifest mismatch")
	}
	if _, err = client.DownloadExport(actor, &pb.DownloadRequest{Scope: wire, ExportId: "research-export", PageSize: 256}); err == nil {
		t.Fatal("cross-principal export IDOR")
	}

	// A private source added after the research snapshot stays outside that
	// immutable snapshot while explicit private exports can include it.
	memoryScope, _ := (hws.ViewRealm{Scope: manifest.Scope, Principal: "a"}).MemoryScope()
	memory := snapshotMemory("api-private", 0)
	memory.Event.Meta.Rights.Grants = append(memory.Event.Meta.Rights.Grants,
		core.Grant{Actor: "a", Recipient: "a", Purpose: "simulation", Operation: core.Export},
		core.Grant{Actor: "external", Recipient: "external", Purpose: "simulation", Operation: core.Read})
	memory.Content.Learned = append(memory.Content.Learned, graph.Learned{Actor: "external", At: 0})
	if _, err = (graph.MemoryService{Journal: store}).Put(ctx, memoryScope, "api-private", 0, memory, "simulation"); err != nil {
		t.Fatal(err)
	}
	privateRequest := &pb.ExportRequest{Scope: wire, ExportId: "private-export", Kind: "private", SourceIds: []string{"api-private"}, FromTime: 0, ThroughTime: 100}
	if _, err = client.SubmitExport(actor, privateRequest); err != nil {
		t.Fatal("private submit", err)
	}
	page, err := client.DownloadExport(actor, &pb.DownloadRequest{Scope: wire, ExportId: "private-export", PageSize: 65536})
	if err != nil || !bytes.Contains(page.Page, []byte("SYNTHETIC_api-private")) {
		t.Fatal("private download", err)
	}
	for _, cursor := range []string{"invalid", base64.RawURLEncoding.EncodeToString([]byte(`{"Manifest":"wrong","Offset":0}`)), base64.RawURLEncoding.EncodeToString([]byte(`{"Manifest":"` + page.ManifestSha256 + `","Offset":-1}`))} {
		if _, err = client.DownloadExport(actor, &pb.DownloadRequest{Scope: wire, ExportId: "private-export", PageSize: 256, Cursor: cursor}); err == nil {
			t.Fatal("invalid cursor")
		}
	}
	// The dummy external port can ingest only currently authorized observations.
	external := metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+secrets[2])
	dummy, err := agentclient.New(conn, wire)
	if err != nil {
		t.Fatal(err)
	}
	observed, err := dummy.Observe(external, []string{"api-private"})
	if err != nil || !bytes.Contains(observed.CanonicalJson, []byte("SYNTHETIC_api-private")) {
		t.Fatal("external observation", err)
	}
	var observedClock struct{ At int64 }
	if json.Unmarshal(observed.CanonicalJson, &observedClock) != nil || observedClock.At != 20 {
		t.Fatal("external current clock unavailable")
	}
	future := snapshotMemory("api-future", 90)
	future.Event.Meta.Rights.Grants = memory.Event.Meta.Rights.Grants
	future.Event.Meta.Rights.Resource = "api-future"
	future.Content.Learned = append(future.Content.Learned, graph.Learned{Actor: "external", At: 90})
	if _, err = (graph.MemoryService{Journal: store}).Put(ctx, memoryScope, "api-future", 0, future, "simulation"); err != nil {
		t.Fatal(err)
	}
	if _, err = dummy.Observe(external, []string{"api-future"}); err == nil {
		t.Fatal("external future knowledge")
	}
	if _, err = client.ResearchView(external, &pb.Query{Scope: wire}); err == nil {
		t.Fatal("external GodState")
	}
	if _, err = dummy.RequestWait(external, "external-wait", "external-intent", "a", observedClock.At); err != nil {
		t.Fatal("external requested intent", err)
	}
	if _, err = dummy.RequestWait(external, "external-other", "external-other", "b", 20); err == nil {
		t.Fatal("external foreign actor write")
	}
	revoke := memory.Event
	revoke.Type = "revoke"
	revoke.Meta.ID = "api-private-revoke"
	revoke.Meta.Rights = core.Rights{Resource: revoke.Meta.ID}
	if _, err = graph.RevokeMemory(ctx, store, memoryScope, revoke.Meta.ID, 1, revoke, "api-private"); err != nil {
		t.Fatal(err)
	}
	if _, err = client.DownloadExport(actor, &pb.DownloadRequest{Scope: wire, ExportId: "private-export", PageSize: 256}); err == nil {
		t.Fatal("private source resurrected on download")
	}
	if _, err = dummy.Observe(external, []string{"api-private"}); err == nil {
		t.Fatal("external purged source")
	}

	// Exercise the real model gateway/cognitive commit boundary through the API.
	// Only the provider is fake; source approval, reservation and model-use lineage
	// are the production implementations, and the callback is outside SQL.
	evidence := snapshotMemory("e2", 20)
	evidence.Event.OccurredAt = 20
	if _, err = (graph.MemoryService{Journal: store}).Put(ctx, memoryScope, "model-evidence", 0, evidence, "simulation"); err != nil {
		t.Fatal(err)
	}
	var modelCalls atomic.Int64
	provider := modelFunc(func(ctx context.Context, in hws.ProviderInput) (hws.ProviderResponse, error) {
		modelCalls.Add(1)
		return (model.Fake{}).Generate(ctx, in)
	})
	gateway, err := hws.NewModelGateway(store, views, provider, modelRoute())
	if err != nil {
		t.Fatal(err)
	}
	backend.StepExecutor = func(ctx context.Context, sc hws.Scope, lease hws.Lease, key core.ID, before func(context.Context) error) (hws.Receipt, error) {
		permit, err := views.Permit("a", hws.ViewRealm{Scope: sc, Principal: "a"}, hws.ActorViewKind, "simulation")
		if err != nil {
			return hws.Receipt{}, err
		}
		approved, decision, err := views.Propose(ctx, permit, []core.ID{"e2"}, "a", core.Derive, graph.InternalContext)
		if err != nil || !decision.Allowed {
			return hws.Receipt{}, hws.ErrViewDenied
		}
		planner := cognitivePlan(func(_ context.Context, i rt.Input, safe graph.SafeContext) (hws.CognitiveFrame, error) {
			p, err := (appraisalPerception{}).Perceive(i)
			if err != nil {
				return hws.CognitiveFrame{}, err
			}
			return hws.CognitiveFrame{Situation: behavior.Situation{Perceived: p, Horizon: 80, Offers: []behavior.Offer{{Kind: behavior.Ask, Recipient: "b", Duration: 1}}}}, nil
		})
		service := hws.CognitiveService{Gateway: gateway, Views: views, Runtime: hws.Runtime{Store: store, BeforeCommit: before}, Planner: planner}
		return service.Step(ctx, hws.ModelRequest{Scope: sc, Principal: "a", Key: key, Capability: hws.ModelInterpretation, Permit: permit, Approved: approved}, lease, key, nil)
	}
	var simultaneous sync.WaitGroup
	replies := make([]*pb.Document, 12)
	failures := make([]error, 12)
	for i := range replies {
		simultaneous.Add(1)
		go func(i int) {
			defer simultaneous.Done()
			replies[i], failures[i] = client.Control(research, &pb.ControlRequest{Scope: wire, OperationId: "api-model-step", Command: "step"})
		}(i)
	}
	simultaneous.Wait()
	for i := range replies {
		if failures[i] != nil || !proto.Equal(replies[0], replies[i]) {
			t.Fatal("concurrent model operation retry", i, failures[i])
		}
	}
	if modelCalls.Load() != 1 {
		t.Fatal("concurrent retry regenerated model", modelCalls.Load())
	}
	permit, err := views.Permit("researcher", grants[0].Realm, hws.ResearchViewKind, "research")
	if err != nil {
		t.Fatal(err)
	}
	views.Revoke(permit)
	if _, err = client.DownloadExport(research, &pb.DownloadRequest{Scope: wire, ExportId: "research-export", PageSize: 256}); err == nil {
		t.Fatal("submission authority survived grant revocation")
	}
}
