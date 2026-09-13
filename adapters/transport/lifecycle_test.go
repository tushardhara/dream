package transport

import (
	"bytes"
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	pb "github.com/tushardhara/dream/adapters/transport/gen/dream/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/encoding/protojson"
)

type drainingBackend struct {
	pb.UnimplementedResearchServer
	entered chan struct{}
	release chan struct{}
}

func (b *drainingBackend) ResearchView(ctx context.Context, _ *pb.Query) (*pb.Document, error) {
	b.entered <- struct{}{}
	select {
	case <-b.release:
		return &pb.Document{Schema: "synthetic.v1"}, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}
func TestConcurrentDrainCompletesAdmittedRequests(t *testing.T) {
	for _, protocol := range []string{"grpc", "http"} {
		t.Run(protocol, func(t *testing.T) {
			c := credentialFixture()
			a, _ := NewAuthenticator([]Credential{c}, func() time.Time { return time.Unix(100, 0) })
			b := &drainingBackend{entered: make(chan struct{}, 1), release: make(chan struct{})}
			s, err := NewServers(context.Background(), ServerConfig{Auth: a, Backend: b, Development: true, RuntimeReady: func(context.Context) error { return nil }, Admission: func(context.Context, Credential) error { return nil }})
			if err != nil {
				t.Fatal(err)
			}
			defer s.Close()
			l, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			result := make(chan error, 1)
			if protocol == "grpc" {
				go func() { _ = s.ServeGRPC(l) }()
				conn, err := grpc.NewClient(l.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
				if err != nil {
					t.Fatal(err)
				}
				defer conn.Close()
				go func() {
					_, err := pb.NewResearchClient(conn).ResearchView(metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+testSecret), &pb.Query{Scope: wireScope(c)})
					result <- err
				}()
			} else {
				go func() { _ = s.ServeHTTP(l) }()
				go func() {
					raw, _ := protojson.Marshal(&pb.Query{Scope: wireScope(c)})
					r, _ := http.NewRequestWithContext(ctx, http.MethodPost, "http://"+l.Addr().String()+"/dream.v1.Research/ResearchView", bytes.NewReader(raw))
					r.Header.Set("Authorization", "Bearer "+testSecret)
					r.Header.Set("Content-Type", "application/json")
					res, err := http.DefaultClient.Do(r)
					if err == nil {
						res.Body.Close()
						if res.StatusCode != 200 {
							err = errors.New("admitted request lost")
						}
					}
					result <- err
				}()
			}
			select {
			case <-b.entered:
			case <-ctx.Done():
				t.Fatal("handler not reached")
			}
			var wg sync.WaitGroup
			errs := make(chan error, 8)
			for range 8 {
				wg.Go(func() { errs <- s.Shutdown(ctx) })
			}
			for !s.life.draining.Load() {
				select {
				case <-ctx.Done():
					t.Fatal("drain not started")
				default:
					time.Sleep(time.Millisecond)
				}
			}
			select {
			case err := <-result:
				t.Fatal("drain interrupted admitted request", err)
			default:
			}
			close(b.release)
			if err := <-result; err != nil {
				t.Fatal(err)
			}
			wg.Wait()
			close(errs)
			for err := range errs {
				if err != nil {
					t.Fatal(err)
				}
			}
			w := httptest.NewRecorder()
			s.health(w, httptest.NewRequest("GET", "/readyz", nil))
			if w.Code != 503 {
				t.Fatal("ready during drain")
			}
		})
	}
}
func TestReadinessOutageAndBoundedProbe(t *testing.T) {
	c := credentialFixture()
	a, _ := NewAuthenticator([]Credential{c}, func() time.Time { return time.Unix(100, 0) })
	var outage, hold atomic.Bool
	entered := make(chan struct{}, 1)
	release := make(chan struct{})
	s, err := NewServers(context.Background(), ServerConfig{Auth: a, Backend: &boundaryBackend{auth: a}, Development: true, Admission: func(context.Context, Credential) error { return nil }, RuntimeReady: func(ctx context.Context) error {
		if outage.Load() {
			return errors.New("private-dsn-canary")
		}
		if hold.Load() {
			entered <- struct{}{}
			select {
			case <-release:
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		return nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	probe := func(path string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		s.http.Handler.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		return w
	}
	if probe("/readyz").Code != 204 {
		t.Fatal("not ready")
	}
	outage.Store(true)
	w := probe("/readyz")
	if w.Code != 503 || w.Body.Len() != 0 {
		t.Fatal("outage hidden or exposed")
	}
	if probe("/healthz").Code != 204 {
		t.Fatal("database outage mistaken for process loss")
	}
	outage.Store(false)
	hold.Store(true)
	done := make(chan struct{})
	go func() { probe("/readyz"); close(done) }()
	<-entered
	if probe("/readyz").Code != 503 {
		t.Fatal("unbounded simultaneous probes")
	}
	close(release)
	<-done
}
func TestDrainDeadlineForcesStuckRequest(t *testing.T) {
	c := credentialFixture()
	a, _ := NewAuthenticator([]Credential{c}, func() time.Time { return time.Unix(100, 0) })
	b := &drainingBackend{entered: make(chan struct{}, 1), release: make(chan struct{})}
	s, err := NewServers(context.Background(), ServerConfig{Auth: a, Backend: b, Development: true, Admission: func(context.Context, Credential) error { return nil }, RuntimeReady: func(context.Context) error { return nil }})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = s.ServeGRPC(l) }()
	conn, err := grpc.NewClient(l.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	result := make(chan error, 1)
	go func() {
		_, e := pb.NewResearchClient(conn).ResearchView(metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+testSecret), &pb.Query{Scope: wireScope(c)})
		result <- e
	}()
	select {
	case <-b.entered:
	case <-ctx.Done():
		t.Fatal("handler not reached")
	}
	expired, stop := context.WithCancel(ctx)
	stop()
	if s.Shutdown(expired) == nil {
		t.Fatal("deadline ignored")
	}
	select {
	case err := <-result:
		if err == nil {
			t.Fatal("stuck request succeeded")
		}
	case <-ctx.Done():
		t.Fatal("force stop did not cancel request")
	}
}
