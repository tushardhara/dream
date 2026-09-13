package postgres

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	api "github.com/tushardhara/dream/adapters/transport"
	pb "github.com/tushardhara/dream/adapters/transport/gen/dream/v1"
	"github.com/tushardhara/dream/app/hws"
	"github.com/tushardhara/dream/core"
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
	secrets := []string{"synthetic-research-token-longer-than-thirty-two", "synthetic-actor-token-longer-than-thirty-two"}
	configs := []api.Credential{}
	for i, role := range []hws.ViewKind{hws.ResearchViewKind, hws.ActorViewKind} {
		sum := sha256.Sum256([]byte(secrets[i]))
		caller := core.ID("researcher")
		purpose := core.ID("research")
		if role == hws.ActorViewKind {
			caller = "a"
			purpose = "simulation"
		}
		configs = append(configs, api.Credential{ID: caller, TokenSHA256: hex.EncodeToString(sum[:]), Caller: caller, Role: role, Scope: manifest.Scope, Principal: "a", Purpose: purpose, Expires: time.Now().Add(time.Hour), RequestsPerMinute: 10000, TotalRequests: 10000})
	}
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
	backend := &api.Backend{Auth: auth, Store: store, Views: views, Handler: runtimeFake{}}
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
	permit, err := views.Permit("researcher", grants[0].Realm, hws.ResearchViewKind, "research")
	if err != nil {
		t.Fatal(err)
	}
	views.Revoke(permit)
	if _, err = client.DownloadExport(research, &pb.DownloadRequest{Scope: wire, ExportId: "research-export", PageSize: 256}); err == nil {
		t.Fatal("submission authority survived grant revocation")
	}
}
