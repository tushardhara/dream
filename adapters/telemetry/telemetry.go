// Package telemetry emits bounded operational signals, never domain payloads.
package telemetry

import (
	"context"
	"errors"
	"sync/atomic"
	"time"

	"github.com/tushardhara/dream/app/hws"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
)

type Recorder struct {
	counts   [hws.OperationKinds][hws.OperationalOutcomes]atomic.Uint64
	tracer   trace.Tracer
	counter  metric.Int64Counter
	duration metric.Float64Histogram
}

var operations = [...]string{"api", "provider", "cognitive", "recovery"}
var outcomes = [...]string{"success", "behavioral_wait", "provider_outage", "safety_denied", "budget_denied", "uncertain", "failure"}

// Providers are explicitly supplied by the trusted host. No environment-driven
// exporter, network destination or request-context propagation is installed.
func New(tp trace.TracerProvider, mp metric.MeterProvider) (*Recorder, error) {
	if tp == nil || mp == nil {
		return nil, errors.New("telemetry providers required")
	}
	m := mp.Meter("dream.operations.v1")
	count, err := m.Int64Counter("dream.operations")
	if err != nil {
		return nil, err
	}
	elapsed, err := m.Float64Histogram("dream.operation.seconds", metric.WithExplicitBucketBoundaries(.001, .01, .1, 1, 10, 30, 60))
	if err != nil {
		return nil, err
	}
	return &Recorder{tracer: tp.Tracer("dream.operations.v1"), counter: count, duration: elapsed}, nil
}
func (r *Recorder) Observe(kind hws.OperationKind, outcome hws.OperationalOutcome, elapsed time.Duration) {
	if r == nil || kind >= hws.OperationKinds || outcome >= hws.OperationalOutcomes {
		return
	}
	if elapsed < 0 {
		elapsed = 0
	}
	if elapsed > time.Minute {
		elapsed = time.Minute
	}
	r.counts[kind][outcome].Add(1)
	if r.tracer == nil {
		return
	} // bounded local counters remain useful without OTel export
	attrs := []attribute.KeyValue{attribute.String("operation", operations[kind]), attribute.String("outcome", outcomes[outcome])}
	ctx := context.Background() // neither arbitrary request baggage nor credentials
	r.counter.Add(ctx, 1, metric.WithAttributes(attrs...))
	r.duration.Record(ctx, elapsed.Seconds(), metric.WithAttributes(attrs...))
	ended := time.Now()
	_, span := r.tracer.Start(ctx, "dream."+operations[kind], trace.WithNewRoot(), trace.WithTimestamp(ended.Add(-elapsed)), trace.WithAttributes(attrs...))
	span.End(trace.WithTimestamp(ended))
}
func (r *Recorder) Counts() [hws.OperationKinds][hws.OperationalOutcomes]uint64 {
	var out [hws.OperationKinds][hws.OperationalOutcomes]uint64
	if r != nil {
		for i := range out {
			for j := range out[i] {
				out[i][j] = r.counts[i][j].Load()
			}
		}
	}
	return out
}
