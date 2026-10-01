package otel

import (
	"context"
	"testing"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	"github.com/tx7do/go-wind-plugins/metrics"
)

// ---------------------------------------------------------------------------
// defaultConfig
// ---------------------------------------------------------------------------

func TestDefaultConfig(t *testing.T) {
	cfg := defaultConfig()

	if cfg.endpoint != "localhost:4317" {
		t.Errorf("endpoint = %q, want %q", cfg.endpoint, "localhost:4317")
	}
	if cfg.serviceName != "go-wind-service" {
		t.Errorf("serviceName = %q, want %q", cfg.serviceName, "go-wind-service")
	}
	if cfg.serviceVersion != "v0.0.1" {
		t.Errorf("serviceVersion = %q, want %q", cfg.serviceVersion, "v0.0.1")
	}
	if cfg.insecure {
		t.Error("insecure = true, want false")
	}
	if cfg.useHTTP {
		t.Error("useHTTP = true, want false")
	}
	if cfg.exportInterval != 60*time.Second {
		t.Errorf("exportInterval = %v, want 60s", cfg.exportInterval)
	}
}

// ---------------------------------------------------------------------------
// Options
// ---------------------------------------------------------------------------

func TestOptions(t *testing.T) {
	tests := []struct {
		name  string
		opt   Option
		check func(*testing.T, *config)
	}{
		{
			name: "WithEndpoint",
			opt:  WithEndpoint("collector:4317"),
			check: func(t *testing.T, c *config) {
				if c.endpoint != "collector:4317" {
					t.Errorf("endpoint = %q, want %q", c.endpoint, "collector:4317")
				}
			},
		},
		{
			name: "WithServiceName",
			opt:  WithServiceName("my-service"),
			check: func(t *testing.T, c *config) {
				if c.serviceName != "my-service" {
					t.Errorf("serviceName = %q, want %q", c.serviceName, "my-service")
				}
			},
		},
		{
			name: "WithServiceVersion",
			opt:  WithServiceVersion("v1.2.3"),
			check: func(t *testing.T, c *config) {
				if c.serviceVersion != "v1.2.3" {
					t.Errorf("serviceVersion = %q, want %q", c.serviceVersion, "v1.2.3")
				}
			},
		},
		{
			name: "WithInsecure true",
			opt:  WithInsecure(true),
			check: func(t *testing.T, c *config) {
				if !c.insecure {
					t.Error("insecure = false, want true")
				}
			},
		},
		{
			name: "WithHTTP true",
			opt:  WithHTTP(true),
			check: func(t *testing.T, c *config) {
				if !c.useHTTP {
					t.Error("useHTTP = false, want true")
				}
			},
		},
		{
			name: "WithExportInterval",
			opt:  WithExportInterval(5 * time.Second),
			check: func(t *testing.T, c *config) {
				if c.exportInterval != 5*time.Second {
					t.Errorf("exportInterval = %v, want 5s", c.exportInterval)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := defaultConfig()
			tt.opt(cfg)
			tt.check(t, cfg)
		})
	}
}

// ---------------------------------------------------------------------------
// toAttrs
// ---------------------------------------------------------------------------

func TestToAttrs(t *testing.T) {
	tests := []struct {
		name   string
		labels map[string]string
		want   map[string]string
	}{
		{"nil labels", nil, map[string]string{}},
		{"empty labels", map[string]string{}, map[string]string{}},
		{"single label", map[string]string{"method": "GET"}, map[string]string{"method": "GET"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := toAttrs(tt.labels)
			if len(got) != len(tt.want) {
				t.Fatalf("toAttrs() length = %d, want %d", len(got), len(tt.want))
			}
			seen := map[string]string{}
			for _, kv := range got {
				seen[string(kv.Key)] = kv.Value.AsString()
			}
			for k, want := range tt.want {
				if seen[k] != want {
					t.Errorf("toAttrs()[%q] = %q, want %q", k, seen[k], want)
				}
			}
		})
	}
}

func TestToAttrs_MultipleLabels(t *testing.T) {
	// Map iteration order is random, so only check membership.
	got := toAttrs(map[string]string{"k1": "v1", "k2": "v2"})
	if len(got) != 2 {
		t.Fatalf("toAttrs() length = %d, want 2", len(got))
	}
	seen := map[string]string{}
	for _, kv := range got {
		seen[string(kv.Key)] = kv.Value.AsString()
	}
	if seen["k1"] != "v1" || seen["k2"] != "v2" {
		t.Errorf("toAttrs() = %v, want it to contain k1=v1 and k2=v2", seen)
	}
}

// ---------------------------------------------------------------------------
// Provider recording — verified through an in-memory ManualReader, so no
// network is involved anywhere in these tests.
// ---------------------------------------------------------------------------

// newManualProvider builds a Provider wired to a ManualReader instead of an
// OTLP exporter. Recording and collection stay entirely in memory.
func newManualProvider(t *testing.T) (*Provider, *sdkmetric.ManualReader) {
	t.Helper()

	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	t.Cleanup(func() { _ = mp.Shutdown(context.Background()) })

	return &Provider{
		meter:      mp.Meter("otel-test"),
		mp:         mp,
		counters:   make(map[string]metric.Float64Counter),
		histograms: make(map[string]metric.Float64Histogram),
		gauges:     make(map[string]metric.Float64UpDownCounter),
	}, reader
}

// collectMetric collects from the reader and returns the metric with the
// given name, if it was reported.
func collectMetric(t *testing.T, reader *sdkmetric.ManualReader, name string) (metricdata.Metrics, bool) {
	t.Helper()

	var rm metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &rm); err != nil {
		t.Fatalf("reader.Collect() error = %v", err)
	}
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name == name {
				return m, true
			}
		}
	}
	return metricdata.Metrics{}, false
}

func TestCounter_RecordsAndAggregates(t *testing.T) {
	p, reader := newManualProvider(t)

	p.Counter(context.Background(), "test_requests_total", 1.5, map[string]string{"method": "GET"})
	p.Counter(context.Background(), "test_requests_total", 2.5, map[string]string{"method": "GET"})

	m, ok := collectMetric(t, reader, "test_requests_total")
	if !ok {
		t.Fatal("counter was not reported by the reader")
	}

	sum, ok := m.Data.(metricdata.Sum[float64])
	if !ok {
		t.Fatalf("counter data type = %T, want metricdata.Sum[float64]", m.Data)
	}
	if !sum.IsMonotonic {
		t.Error("counter Sum.IsMonotonic = false, want true")
	}
	if len(sum.DataPoints) != 1 {
		t.Fatalf("len(DataPoints) = %d, want 1", len(sum.DataPoints))
	}
	if got := sum.DataPoints[0].Value; got != 4 {
		t.Errorf("counter sum = %v, want 4", got)
	}
	if v, ok := sum.DataPoints[0].Attributes.Value(attribute.Key("method")); !ok || v.AsString() != "GET" {
		t.Errorf("counter attributes = %v, want method=GET", sum.DataPoints[0].Attributes.ToSlice())
	}
}

func TestHistogram_RecordsObservation(t *testing.T) {
	p, reader := newManualProvider(t)

	p.Histogram(context.Background(), "test_duration_seconds", 0.5, nil)
	p.Histogram(context.Background(), "test_duration_seconds", 1.0, nil)

	m, ok := collectMetric(t, reader, "test_duration_seconds")
	if !ok {
		t.Fatal("histogram was not reported by the reader")
	}

	hist, ok := m.Data.(metricdata.Histogram[float64])
	if !ok {
		t.Fatalf("histogram data type = %T, want metricdata.Histogram[float64]", m.Data)
	}
	if len(hist.DataPoints) != 1 {
		t.Fatalf("len(DataPoints) = %d, want 1", len(hist.DataPoints))
	}
	dp := hist.DataPoints[0]
	if dp.Count != 2 {
		t.Errorf("histogram Count = %d, want 2", dp.Count)
	}
	if dp.Sum != 1.5 {
		t.Errorf("histogram Sum = %v, want 1.5", dp.Sum)
	}
}

func TestGauge_UpDownCounterAccumulates(t *testing.T) {
	p, reader := newManualProvider(t)

	// Gauge is implemented as a Float64UpDownCounter, so repeated
	// observations accumulate rather than replace.
	p.Gauge(context.Background(), "test_queue_depth", 10, nil)
	p.Gauge(context.Background(), "test_queue_depth", 4, nil)

	m, ok := collectMetric(t, reader, "test_queue_depth")
	if !ok {
		t.Fatal("gauge was not reported by the reader")
	}

	sum, ok := m.Data.(metricdata.Sum[float64])
	if !ok {
		t.Fatalf("gauge data type = %T, want metricdata.Sum[float64]", m.Data)
	}
	if sum.IsMonotonic {
		t.Error("gauge Sum.IsMonotonic = true, want false (UpDownCounter)")
	}
	if len(sum.DataPoints) != 1 {
		t.Fatalf("len(DataPoints) = %d, want 1", len(sum.DataPoints))
	}
	if got := sum.DataPoints[0].Value; got != 14 {
		t.Errorf("gauge accumulated value = %v, want 14", got)
	}
}

func TestInstrumentsAreCachedPerName(t *testing.T) {
	p, _ := newManualProvider(t)

	p.Counter(context.Background(), "c1", 1, nil)
	p.Counter(context.Background(), "c1", 1, nil)
	p.Histogram(context.Background(), "h1", 1, nil)
	p.Histogram(context.Background(), "h1", 1, nil)
	p.Gauge(context.Background(), "g1", 1, nil)
	p.Gauge(context.Background(), "g1", 1, nil)

	if got := len(p.counters); got != 1 {
		t.Errorf("len(counters) = %d, want 1 (instrument reused across calls)", got)
	}
	if got := len(p.histograms); got != 1 {
		t.Errorf("len(histograms) = %d, want 1 (instrument reused across calls)", got)
	}
	if got := len(p.gauges); got != 1 {
		t.Errorf("len(gauges) = %d, want 1 (instrument reused across calls)", got)
	}
}

func TestProviderClose_ShutsDownMeterProvider(t *testing.T) {
	p, _ := newManualProvider(t)

	if err := p.Close(); err != nil {
		t.Errorf("Close() = %v, want nil", err)
	}
}

// ---------------------------------------------------------------------------
// New — construction smoke tests for both export protocols.
//
// OTLP exporters are created lazily: no connection is dialed at construction
// time, so these tests never touch the network.
// ---------------------------------------------------------------------------

func TestNew_GrpcExporter(t *testing.T) {
	p, err := New(
		WithEndpoint("127.0.0.1:4317"),
		WithServiceName("test-svc"),
		WithInsecure(true),
	)
	if err != nil {
		t.Fatalf("New() error = %v, want nil", err)
	}

	if p.meter == nil {
		t.Error("meter is nil, want non-nil")
	}
	if p.mp == nil {
		t.Error("mp is nil, want non-nil")
	}
	if p.exporter == nil {
		t.Error("exporter is nil, want non-nil")
	}
	if p.counters == nil || p.histograms == nil || p.gauges == nil {
		t.Error("instrument caches must be initialized by New()")
	}
}

func TestNew_HttpExporter(t *testing.T) {
	p, err := New(
		WithEndpoint("127.0.0.1:4318"),
		WithServiceName("test-svc"),
		WithHTTP(true),
		WithInsecure(true),
	)
	if err != nil {
		t.Fatalf("New() error = %v, want nil", err)
	}

	if p.mp == nil {
		t.Error("mp is nil, want non-nil")
	}
	if p.exporter == nil {
		t.Error("exporter is nil, want non-nil")
	}
}

// ---------------------------------------------------------------------------
// Interface compliance
// ---------------------------------------------------------------------------

var _ metrics.Metrics = (*Provider)(nil)
var _ metrics.Closer = (*Provider)(nil)
