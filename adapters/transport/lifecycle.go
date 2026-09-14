package transport

import (
	"context"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

// lifecycle is shared by both transports. Draining denies new admissions while
// already admitted handlers finish through their usual authorization checks.
type lifecycle struct {
	draining atomic.Bool
	probe    atomic.Bool
	once     sync.Once
	done     chan struct{}
}

func (s *Servers) Shutdown(ctx context.Context) error {
	if s == nil {
		return nil
	}
	s.life.draining.Store(true)
	s.life.once.Do(func() {
		go func() {
			// A host cannot accidentally leave drain unbounded by omitting a deadline.
			bounded, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			rpcDone := make(chan struct{})
			go func() { s.grpc.GracefulStop(); close(rpcDone) }()
			if s.http.Shutdown(bounded) != nil {
				_ = s.http.Close()
			}
			select {
			case <-rpcDone:
			case <-bounded.Done():
				s.grpc.Stop()
				<-rpcDone
			}
			close(s.life.done)
		}()
	})
	select {
	case <-s.life.done:
		return nil
	case <-ctx.Done():
		s.Close()
		return ctx.Err()
	}
}

func (s *Servers) health(w http.ResponseWriter, r *http.Request) {
	// Only process liveness/readiness, never database diagnostics or scope data.
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if r.URL.Path == "/healthz" {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if s.life.draining.Load() || !s.life.probe.CompareAndSwap(false, true) {
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	defer s.life.probe.Store(false)
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if s.config.RuntimeReady(ctx) != nil || s.life.draining.Load() {
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
