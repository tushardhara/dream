package telemetry

import (
	"context"
	"github.com/tushardhara/dream/app/hws"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"sync"
	"testing"
	"time"
)

func TestBoundedConcurrentMetricsAndRedactedSpans(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	defer tp.Shutdown(context.Background())
	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	defer mp.Shutdown(context.Background())
	recorder, err := New(tp, mp)
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			for range 20 {
				recorder.Observe(hws.ProviderCall, hws.ProviderOutage, time.Millisecond)
			}
		})
	}
	wg.Wait()
	recorder.Observe(hws.CognitiveCommit, hws.BehavioralWait, -time.Second)
	recorder.Observe(hws.APIRequest, hws.SafetyDenied, 2*time.Hour)
	recorder.Observe(255, 255, time.Second)
	counts := recorder.Counts()
	if counts[hws.ProviderCall][hws.ProviderOutage] != 160 || counts[hws.CognitiveCommit][hws.BehavioralWait] != 1 || counts[hws.APIRequest][hws.SafetyDenied] != 1 {
		t.Fatal(counts)
	}
	spans := exporter.GetSpans()
	if len(spans) != 162 {
		t.Fatal(len(spans))
	}
	for _, span := range spans {
		if span.Parent.IsValid() || len(span.Attributes) != 2 || len(span.Events) != 0 || len(span.Links) != 0 {
			t.Fatal("unbounded or inherited trace data", span)
		}
		for _, attr := range span.Attributes {
			if attr.Key != "operation" && attr.Key != "outcome" {
				t.Fatal("unexpected attribute", attr)
			}
		}
	}
	var data metricdata.ResourceMetrics
	if err = reader.Collect(context.Background(), &data); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, scope := range data.ScopeMetrics {
		for _, metric := range scope.Metrics {
			if sum, ok := metric.Data.(metricdata.Sum[int64]); ok {
				if len(sum.DataPoints) != 3 {
					t.Fatal("cardinality changed", len(sum.DataPoints))
				}
				var total int64
				for _, p := range sum.DataPoints {
					if p.Attributes.Len() != 2 {
						t.Fatal("unexpected metric attributes")
					}
					total += p.Value
				}
				if total != 162 {
					t.Fatal(total)
				}
				found = true
			}
		}
	}
	if !found {
		t.Fatal("counter not exported")
	}
}
