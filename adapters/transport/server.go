package transport

import (
	"context"
	"crypto/tls"
	"net"
	"net/http"
	"strings"
	"time"

	gateway "github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	pb "github.com/tushardhara/dream/adapters/transport/gen/dream/v1"
	"github.com/tushardhara/dream/app/hws"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

type sessionKey struct{}

// CurrentIdentity is deliberately revalidated rather than a stored role value.
func CurrentIdentity(ctx context.Context, auth *Authenticator) (Credential, error) {
	session, ok := ctx.Value(sessionKey{}).(Session)
	if !ok {
		return Credential{}, ErrDenied
	}
	return auth.Revalidate(session)
}

type ServerConfig struct {
	Auth    *Authenticator
	Backend pb.ResearchServer
	// RuntimeReady must verify the actual runtime database connection/role.
	RuntimeReady func(context.Context) error
	// Admission is an atomic durable ledger; the in-process limiter is additional
	// burst protection and cannot replace the restart-stable total budget.
	Admission func(context.Context, Credential) error
	// Development permits cleartext only on a verified loopback listener.
	Development bool
	TLS         *tls.Config
}
type Servers struct {
	grpc   *grpc.Server
	http   *http.Server
	config ServerConfig
}

func NewServers(ctx context.Context, c ServerConfig) (*Servers, error) {
	if c.Auth == nil || c.Backend == nil || c.RuntimeReady == nil || c.Admission == nil {
		return nil, ErrDenied
	}
	if !c.Development && (c.TLS == nil || len(c.TLS.Certificates) == 0 || c.TLS.MinVersion < tls.VersionTLS12) {
		return nil, ErrDenied
	}
	if err := c.RuntimeReady(ctx); err != nil {
		return nil, ErrDenied
	}
	auth := func(ctx context.Context, values []string) (context.Context, error) {
		if len(values) != 1 {
			return nil, status.Error(codes.Unauthenticated, "authentication required")
		}
		s, err := c.Auth.Authenticate(values[0])
		if err != nil {
			return nil, sanitized(err)
		}
		identity, err := c.Auth.Revalidate(s)
		if err != nil {
			return nil, sanitized(err)
		}
		if err = c.Admission(ctx, identity); err != nil {
			return nil, sanitized(err)
		}
		return context.WithValue(ctx, sessionKey{}, s), nil
	}
	interceptor := func(ctx context.Context, request any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
		defer cancel()
		md, _ := metadata.FromIncomingContext(ctx)
		trusted, err := auth(ctx, md.Get("authorization"))
		if err != nil {
			return nil, err
		}
		if err = authorizeWire(trusted, c.Auth, info.FullMethod, request); err != nil {
			return nil, sanitized(err)
		}
		response, err := handler(trusted, request)
		if err != nil {
			return nil, sanitized(err)
		}
		if _, err = CurrentIdentity(trusted, c.Auth); err != nil {
			return nil, sanitized(err)
		}
		return response, nil
	}
	opts := []grpc.ServerOption{grpc.UnaryInterceptor(interceptor), grpc.MaxRecvMsgSize(2 << 20), grpc.MaxSendMsgSize(34 << 20), grpc.MaxConcurrentStreams(32)}
	if c.TLS != nil {
		opts = append(opts, grpc.Creds(credentials.NewTLS(c.TLS.Clone())))
	}
	rpc := grpc.NewServer(opts...)
	pb.RegisterResearchServer(rpc, c.Backend)
	// Register through an in-process authenticated wrapper. The generated direct
	// gateway does not run gRPC interceptors, so every method is guarded here too.
	mux := gateway.NewServeMux(gateway.WithMarshalerOption(gateway.MIMEWildcard, &gateway.JSONPb{}))
	if err := pb.RegisterResearchHandlerServer(ctx, mux, &guardedBackend{backend: c.Backend, auth: c.Auth}); err != nil {
		return nil, err
	}
	httpHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		bounded, cancel := context.WithTimeout(r.Context(), 20*time.Second)
		defer cancel()
		r = r.WithContext(bounded)
		// Never accept proxy identity metadata as a substitute or second credential.
		if r.Header.Get("Grpc-Metadata-Authorization") != "" {
			http.Error(w, "authentication required", http.StatusUnauthorized)
			return
		}
		trusted, err := auth(r.Context(), r.Header.Values("Authorization"))
		if err != nil {
			code := http.StatusUnauthorized
			if status.Code(err) == codes.ResourceExhausted {
				code = http.StatusTooManyRequests
			}
			http.Error(w, "request rejected", code)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 2<<20)
		mux.ServeHTTP(w, r.WithContext(trusted))
	})
	return &Servers{grpc: rpc, http: &http.Server{Handler: httpHandler, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 20 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 16 << 10, TLSConfig: c.TLS}, config: c}, nil
}
func (s *Servers) validateListener(l net.Listener) error {
	if s == nil || l == nil {
		return ErrDenied
	}
	addr, ok := l.Addr().(*net.TCPAddr)
	if !ok {
		return ErrDenied
	}
	if s.config.Development && !addr.IP.IsLoopback() {
		return ErrDenied
	}
	return nil
}
func (s *Servers) ServeGRPC(l net.Listener) error {
	if err := s.validateListener(l); err != nil {
		return err
	}
	return s.grpc.Serve(l)
}
func (s *Servers) ServeHTTP(l net.Listener) error {
	if err := s.validateListener(l); err != nil {
		return err
	}
	if s.config.TLS != nil {
		return s.http.ServeTLS(l, "", "")
	}
	return s.http.Serve(l)
}
func sanitized(err error) error {
	if err == nil {
		return nil
	}
	if err == ErrLimited || err == hws.ErrAdmission {
		return status.Error(codes.ResourceExhausted, "request budget exhausted")
	}
	if err == ErrDenied || err == hws.ErrViewDenied {
		return status.Error(codes.PermissionDenied, "request not authorized")
	}
	return status.Error(codes.FailedPrecondition, "request unavailable")
}
func authorizeWire(ctx context.Context, a *Authenticator, method string, request any) error {
	c, err := CurrentIdentity(ctx, a)
	if err != nil {
		return err
	}
	method = method[strings.LastIndex(method, "/")+1:]
	var scope *pb.Scope
	switch r := request.(type) {
	case interface{ GetScope() *pb.Scope }:
		scope = r.GetScope()
	case *pb.ReplayRequest:
		scope = r.GetSource().GetScope()
	case *pb.ForkRequest:
		scope = r.GetSource().GetScope()
	default:
		return ErrDenied
	}
	if scope == nil || scope.Operator != string(c.Scope.Actor) || scope.Namespace != string(c.Scope.Namespace) || scope.World != string(c.Scope.World) || scope.Branch != string(c.Scope.Branch) || scope.Run != string(c.Scope.Run) {
		return ErrDenied
	}
	switch method {
	case "ActorView":
		if c.Role != hws.ActorViewKind {
			return ErrDenied
		}
	case "ExternalObserve", "ExternalAct":
		if c.Role != hws.ExternalViewKind {
			return ErrDenied
		}
	case "SubmitExport", "DownloadExport":
		if c.Role != hws.ActorViewKind && c.Role != hws.ResearchViewKind {
			return ErrDenied
		}
	default:
		if c.Role != hws.ResearchViewKind {
			return ErrDenied
		}
	}
	return nil
}

func (s *Servers) Close() {
	if s != nil {
		s.grpc.Stop()
		_ = s.http.Close()
	}
}
