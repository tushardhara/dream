package transport

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	pb "github.com/tushardhara/dream/adapters/transport/gen/dream/v1"
	"github.com/tushardhara/dream/app/hws"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

type boundaryBackend struct {
	pb.UnimplementedResearchServer
	auth   *Authenticator
	calls  atomic.Int64
	revoke atomic.Bool
	fail   atomic.Bool
}

func (b *boundaryBackend) ResearchView(ctx context.Context, _ *pb.Query) (*pb.Document, error) {
	b.calls.Add(1)
	c, err := CurrentIdentity(ctx, b.auth)
	if err != nil {
		return nil, err
	}
	if b.revoke.Load() {
		b.auth.Revoke(c.ID)
	}
	if b.fail.Load() {
		return nil, errors.New("synthetic-sensitive-database-connection")
	}
	raw, _ := json.Marshal(struct {
		Caller    string
		Principal string
	}{string(c.Caller), string(c.Principal)})
	return &pb.Document{Schema: "fixture.v1", CanonicalJson: raw}, nil
}
func wireScope(c Credential) *pb.Scope {
	return &pb.Scope{Operator: string(c.Scope.Actor), Namespace: string(c.Scope.Namespace), World: string(c.Scope.World), Branch: string(c.Scope.Branch), Run: string(c.Scope.Run)}
}
func TestRealGRPCAndHTTPAuthScopeAndEquivalence(t *testing.T) {
	c := credentialFixture()
	c.RequestsPerMinute = 100
	c.TotalRequests = 100
	a, _ := NewAuthenticator([]Credential{c}, func() time.Time { return time.Unix(100, 0) })
	backend := &boundaryBackend{auth: a}
	var admitted atomic.Int64
	servers, err := NewServers(context.Background(), ServerConfig{Auth: a, Backend: backend, Development: true, RuntimeReady: func(context.Context) error { return nil }, Admission: func(context.Context, Credential) error { admitted.Add(1); return nil }})
	if err != nil {
		t.Fatal(err)
	}
	defer servers.Close()
	rpcListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	httpListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = servers.ServeGRPC(rpcListener) }()
	go func() { _ = servers.ServeHTTP(httpListener) }()
	conn, err := grpc.NewClient(rpcListener.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	client := pb.NewResearchClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	trusted := metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+testSecret)
	query := &pb.Query{Scope: wireScope(c)}
	expected, err := client.ResearchView(trusted, query)
	if err != nil {
		t.Fatal(err)
	}
	requestHTTP := func(query *pb.Query, headers http.Header) (int, []byte) {
		t.Helper()
		body, _ := protojson.Marshal(query)
		req, _ := http.NewRequestWithContext(ctx, http.MethodPost, "http://"+httpListener.Addr().String()+"/dream.v1.Research/ResearchView", bytes.NewReader(body))
		req.Header = headers
		req.Header.Set("Content-Type", "application/json")
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		raw, _ := io.ReadAll(res.Body)
		return res.StatusCode, raw
	}
	statusCode, raw := requestHTTP(query, http.Header{"Authorization": {"Bearer " + testSecret}})
	var got pb.Document
	if statusCode != 200 || protojson.Unmarshal(raw, &got) != nil || !proto.Equal(expected, &got) {
		t.Fatalf("gateway drift: %d %s", statusCode, raw)
	}
	for _, md := range [][]string{{"x-actor", "researcher", "x-role", "research"}, {"authorization", "Bearer wrong-secret-but-long-enough-to-parse"}, {"authorization", "Bearer " + testSecret, "authorization", "Bearer " + testSecret}} {
		if _, err = client.ResearchView(metadata.AppendToOutgoingContext(ctx, md...), query); err == nil {
			t.Fatal("gRPC auth bypass")
		}
	}
	for _, headers := range []http.Header{{"X-Actor": {"researcher"}, "X-Role": {"research"}}, {"Grpc-Metadata-Authorization": {"Bearer " + testSecret}}, {"Authorization": {"Bearer " + testSecret, "Bearer " + testSecret}}, {"Authorization": {"Bearer " + testSecret}, "Grpc-Metadata-Authorization": {"Bearer " + testSecret}}} {
		if code, _ := requestHTTP(query, headers); code == 200 {
			t.Fatal("HTTP proxy/header auth bypass")
		}
	}
	for _, mutate := range []func(*pb.Scope){func(s *pb.Scope) { s.Operator = "other" }, func(s *pb.Scope) { s.Namespace = "other" }, func(s *pb.Scope) { s.World = "other" }, func(s *pb.Scope) { s.Branch = "other" }, func(s *pb.Scope) { s.Run = "other" }} {
		bad := proto.Clone(query).(*pb.Query)
		mutate(bad.Scope)
		if _, err = client.ResearchView(trusted, bad); err == nil {
			t.Fatal("gRPC IDOR")
		}
		if code, _ := requestHTTP(bad, http.Header{"Authorization": {"Bearer " + testSecret}}); code == 200 {
			t.Fatal("HTTP IDOR")
		}
	}
	if backend.calls.Load() != 2 {
		t.Fatal("rejected request reached backend", backend.calls.Load())
	}
	backend.fail.Store(true)
	if _, err = client.ResearchView(trusted, query); err == nil || bytes.Contains([]byte(err.Error()), []byte("synthetic-sensitive")) {
		t.Fatal("backend error exposed", err)
	}
	backend.fail.Store(false)
	backend.revoke.Store(true)
	if code, _ := requestHTTP(query, http.Header{"Authorization": {"Bearer " + testSecret}}); code == 200 {
		t.Fatal("revoke at return not enforced")
	}
	if admitted.Load() < 2 {
		t.Fatal("durable admission not invoked")
	}
}
func TestRoleSeparationAndDirectGatewayDenial(t *testing.T) {
	for _, role := range []hws.ViewKind{hws.ActorViewKind, hws.ExternalViewKind} {
		c := credentialFixture()
		c.Caller = c.Principal
		c.Role = role
		a, _ := NewAuthenticator([]Credential{c}, func() time.Time { return time.Unix(100, 0) })
		session, _ := a.Authenticate("Bearer " + testSecret)
		ctx := context.WithValue(context.Background(), sessionKey{}, session)
		for _, method := range []string{"ResearchView", "Control", "CreateWorld", "Replay", "CaptureSnapshot", "GetOperation"} {
			if err := authorizeWire(ctx, a, method, &pb.Query{Scope: wireScope(c)}); err == nil {
				t.Fatal("role escalation", role, method)
			}
		}
		backend := &boundaryBackend{auth: a}
		guard := &guardedBackend{backend: backend, auth: a}
		if _, err := guard.ResearchView(context.Background(), &pb.Query{Scope: wireScope(c)}); err == nil || backend.calls.Load() != 0 {
			t.Fatal("direct gateway skipped auth")
		}
	}
}

type fakeListener struct{ net.Listener }

func (fakeListener) Addr() net.Addr { return &net.TCPAddr{IP: net.IPv4zero, Port: 1} }
func TestStartupAndListenerFailClosed(t *testing.T) {
	c := credentialFixture()
	a, _ := NewAuthenticator([]Credential{c}, func() time.Time { return time.Unix(100, 0) })
	valid := ServerConfig{Auth: a, Backend: &boundaryBackend{auth: a}, Development: true, RuntimeReady: func(context.Context) error { return nil }, Admission: func(context.Context, Credential) error { return nil }}
	for _, mutate := range []func(*ServerConfig){func(c *ServerConfig) { c.Auth = nil }, func(c *ServerConfig) { c.Backend = nil }, func(c *ServerConfig) { c.Development = false }, func(c *ServerConfig) { c.RuntimeReady = nil }, func(c *ServerConfig) { c.Admission = nil }, func(c *ServerConfig) {
		c.RuntimeReady = func(context.Context) error { return errors.New("owner role") }
	}} {
		cfg := valid
		mutate(&cfg)
		if s, err := NewServers(context.Background(), cfg); err == nil {
			s.Close()
			t.Fatal("unsafe startup accepted")
		}
	}
	s, err := NewServers(context.Background(), valid)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err = s.ServeGRPC(fakeListener{}); err == nil {
		t.Fatal("non-loopback development gRPC")
	}
	if err = s.ServeHTTP(fakeListener{}); err == nil {
		t.Fatal("non-loopback development HTTP")
	}
}
