package transport

import (
	"github.com/tushardhara/dream/app/hws"
	"google.golang.org/grpc/codes"
	"net/http"
	"time"
)

func observeAPI(o hws.OperationalObserver, c codes.Code, d time.Duration) {
	if o == nil {
		return
	}
	outcome := hws.OperationalFailure
	switch c {
	case codes.OK:
		outcome = hws.OperationalSuccess
	case codes.PermissionDenied, codes.Unauthenticated:
		outcome = hws.SafetyDenied
	case codes.ResourceExhausted:
		outcome = hws.BudgetDenied
	}
	o.Observe(hws.APIRequest, outcome, d)
}
func observeHTTP(o hws.OperationalObserver, c int, d time.Duration) {
	code := codes.Internal
	switch {
	case c >= 200 && c < 300:
		code = codes.OK
	case c == 401 || c == 403:
		code = codes.PermissionDenied
	case c == 429:
		code = codes.ResourceExhausted
	case c == 503:
		code = codes.Unavailable
	}
	observeAPI(o, code, d)
}

type observedWriter struct {
	http.ResponseWriter
	code  int
	wrote bool
}

func (w *observedWriter) WriteHeader(code int) {
	if !w.wrote {
		w.code = code
		w.wrote = true
		w.ResponseWriter.WriteHeader(code)
	}
}
func (w *observedWriter) Write(p []byte) (int, error) {
	if !w.wrote {
		w.WriteHeader(200)
	}
	return w.ResponseWriter.Write(p)
}
