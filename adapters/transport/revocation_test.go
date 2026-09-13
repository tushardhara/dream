package transport

import (
	"context"
	pb "github.com/tushardhara/dream/adapters/transport/gen/dream/v1"
	"github.com/tushardhara/dream/app/hws"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"net"
	"sync/atomic"
	"testing"
	"time"
)

// Every case must actually enter a successful handler with a live credential,
// revoke it there, and be denied after that handler returns. An already revoked
// token or an unimplemented handler would not pin return-time revalidation.
type revokingRPCBackend struct {
	pb.UnimplementedResearchServer
	auth  *Authenticator
	calls atomic.Int64
}

func (b *revokingRPCBackend) revoke(ctx context.Context) error {
	c, err := CurrentIdentity(ctx, b.auth)
	if err != nil {
		return err
	}
	b.calls.Add(1)
	b.auth.Revoke(c.ID)
	return nil
}
func (b *revokingRPCBackend) ValidateScenario(ctx context.Context, _ *pb.ScenarioRequest) (*pb.Document, error) {
	if err := b.revoke(ctx); err != nil {
		return nil, err
	}
	return &pb.Document{Schema: "fixture.v1", CanonicalJson: []byte(`{"private":"synthetic"}`)}, nil
}
func (b *revokingRPCBackend) CreateWorld(ctx context.Context, _ *pb.CreateWorldRequest) (*pb.Document, error) {
	if err := b.revoke(ctx); err != nil {
		return nil, err
	}
	return &pb.Document{Schema: "fixture.v1", CanonicalJson: []byte(`{"private":"synthetic"}`)}, nil
}
func (b *revokingRPCBackend) Control(ctx context.Context, _ *pb.ControlRequest) (*pb.Document, error) {
	if err := b.revoke(ctx); err != nil {
		return nil, err
	}
	return &pb.Document{Schema: "fixture.v1", CanonicalJson: []byte(`{"private":"synthetic"}`)}, nil
}
func (b *revokingRPCBackend) GetOperation(ctx context.Context, _ *pb.OperationRequest) (*pb.Document, error) {
	if err := b.revoke(ctx); err != nil {
		return nil, err
	}
	return &pb.Document{Schema: "fixture.v1", CanonicalJson: []byte(`{"private":"synthetic"}`)}, nil
}
func (b *revokingRPCBackend) CaptureSnapshot(ctx context.Context, _ *pb.SnapshotRequest) (*pb.SnapshotHandle, error) {
	if err := b.revoke(ctx); err != nil {
		return nil, err
	}
	return &pb.SnapshotHandle{Key: "synthetic-private"}, nil
}
func (b *revokingRPCBackend) Fork(ctx context.Context, _ *pb.ForkRequest) (*pb.Document, error) {
	if err := b.revoke(ctx); err != nil {
		return nil, err
	}
	return &pb.Document{Schema: "fixture.v1", CanonicalJson: []byte(`{"private":"synthetic"}`)}, nil
}
func (b *revokingRPCBackend) Replay(ctx context.Context, _ *pb.ReplayRequest) (*pb.Document, error) {
	if err := b.revoke(ctx); err != nil {
		return nil, err
	}
	return &pb.Document{Schema: "fixture.v1", CanonicalJson: []byte(`{"private":"synthetic"}`)}, nil
}
func (b *revokingRPCBackend) ActorView(ctx context.Context, _ *pb.Query) (*pb.Document, error) {
	if err := b.revoke(ctx); err != nil {
		return nil, err
	}
	return &pb.Document{Schema: "fixture.v1", CanonicalJson: []byte(`{"private":"synthetic"}`)}, nil
}
func (b *revokingRPCBackend) ResearchView(ctx context.Context, _ *pb.Query) (*pb.Document, error) {
	if err := b.revoke(ctx); err != nil {
		return nil, err
	}
	return &pb.Document{Schema: "fixture.v1", CanonicalJson: []byte(`{"private":"synthetic"}`)}, nil
}
func (b *revokingRPCBackend) ExternalObserve(ctx context.Context, _ *pb.ExternalRequest) (*pb.Document, error) {
	if err := b.revoke(ctx); err != nil {
		return nil, err
	}
	return &pb.Document{Schema: "fixture.v1", CanonicalJson: []byte(`{"private":"synthetic"}`)}, nil
}
func (b *revokingRPCBackend) ExternalAct(ctx context.Context, _ *pb.ControlRequest) (*pb.Document, error) {
	if err := b.revoke(ctx); err != nil {
		return nil, err
	}
	return &pb.Document{Schema: "fixture.v1", CanonicalJson: []byte(`{"private":"synthetic"}`)}, nil
}
func (b *revokingRPCBackend) SubmitExport(ctx context.Context, _ *pb.ExportRequest) (*pb.Document, error) {
	if err := b.revoke(ctx); err != nil {
		return nil, err
	}
	return &pb.Document{Schema: "fixture.v1", CanonicalJson: []byte(`{"private":"synthetic"}`)}, nil
}
func (b *revokingRPCBackend) DownloadExport(ctx context.Context, _ *pb.DownloadRequest) (*pb.ExportPage, error) {
	if err := b.revoke(ctx); err != nil {
		return nil, err
	}
	return &pb.ExportPage{Page: []byte("synthetic-private")}, nil
}
func TestEveryGRPCReturnRevalidatesMidFlightRevocation(t *testing.T) {
	for _, test := range []struct {
		name     string
		role     hws.ViewKind
		request  func(*pb.Scope) proto.Message
		response func() proto.Message
	}{
		{"ValidateScenario", hws.ResearchViewKind, func(scope *pb.Scope) proto.Message { return &pb.ScenarioRequest{Scope: scope} }, func() proto.Message { return &pb.Document{} }},
		{"CreateWorld", hws.ResearchViewKind, func(scope *pb.Scope) proto.Message { return &pb.CreateWorldRequest{Scope: scope} }, func() proto.Message { return &pb.Document{} }},
		{"Control", hws.ResearchViewKind, func(scope *pb.Scope) proto.Message { return &pb.ControlRequest{Scope: scope} }, func() proto.Message { return &pb.Document{} }},
		{"GetOperation", hws.ResearchViewKind, func(scope *pb.Scope) proto.Message { return &pb.OperationRequest{Scope: scope} }, func() proto.Message { return &pb.Document{} }},
		{"CaptureSnapshot", hws.ResearchViewKind, func(scope *pb.Scope) proto.Message { return &pb.SnapshotRequest{Scope: scope} }, func() proto.Message { return &pb.SnapshotHandle{} }},
		{"Fork", hws.ResearchViewKind, func(scope *pb.Scope) proto.Message {
			return &pb.ForkRequest{Source: &pb.SnapshotHandle{Scope: scope, Key: "snapshot", Hash: "hash"}}
		}, func() proto.Message { return &pb.Document{} }},
		{"Replay", hws.ResearchViewKind, func(scope *pb.Scope) proto.Message {
			return &pb.ReplayRequest{Source: &pb.SnapshotHandle{Scope: scope, Key: "snapshot", Hash: "hash"}}
		}, func() proto.Message { return &pb.Document{} }},
		{"ActorView", hws.ActorViewKind, func(scope *pb.Scope) proto.Message { return &pb.Query{Scope: scope} }, func() proto.Message { return &pb.Document{} }},
		{"ResearchView", hws.ResearchViewKind, func(scope *pb.Scope) proto.Message { return &pb.Query{Scope: scope} }, func() proto.Message { return &pb.Document{} }},
		{"ExternalObserve", hws.ExternalViewKind, func(scope *pb.Scope) proto.Message { return &pb.ExternalRequest{Scope: scope} }, func() proto.Message { return &pb.Document{} }},
		{"ExternalAct", hws.ExternalViewKind, func(scope *pb.Scope) proto.Message { return &pb.ControlRequest{Scope: scope} }, func() proto.Message { return &pb.Document{} }},
		{"SubmitExport", hws.ResearchViewKind, func(scope *pb.Scope) proto.Message { return &pb.ExportRequest{Scope: scope} }, func() proto.Message { return &pb.Document{} }},
		{"DownloadExport", hws.ResearchViewKind, func(scope *pb.Scope) proto.Message { return &pb.DownloadRequest{Scope: scope} }, func() proto.Message { return &pb.ExportPage{} }},
	} {
		t.Run(test.name, func(t *testing.T) {
			c := credentialFixture()
			c.Role = test.role
			if test.role == hws.ActorViewKind {
				c.Caller = c.Principal
				c.Purpose = "simulation"
			}
			if test.role == hws.ExternalViewKind {
				c.Caller = "external"
				c.Purpose = "simulation"
			}
			auth, err := NewAuthenticator([]Credential{c}, func() time.Time { return time.Unix(100, 0) })
			if err != nil {
				t.Fatal(err)
			}
			backend := &revokingRPCBackend{auth: auth}
			server, err := NewServers(context.Background(), ServerConfig{Auth: auth, Backend: backend, Development: true, RuntimeReady: func(context.Context) error { return nil }, Admission: func(context.Context, Credential) error { return nil }})
			if err != nil {
				t.Fatal(err)
			}
			defer server.Close()
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			go func() { _ = server.ServeGRPC(listener) }()
			connection, err := grpc.NewClient(listener.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
			if err != nil {
				t.Fatal(err)
			}
			defer connection.Close()
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			ctx = metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+testSecret)
			response := test.response()
			err = connection.Invoke(ctx, "/dream.v1.Research/"+test.name, test.request(wireScope(c)), response)
			if backend.calls.Load() != 1 {
				t.Fatal("request did not enter the authenticated handler", err)
			}
			if status.Code(err) != codes.PermissionDenied {
				t.Fatalf("gRPC returned data after mid-flight revocation: %v (%v)", response, err)
			}
		})
	}
}
