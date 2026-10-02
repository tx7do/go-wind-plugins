package prometheus

import (
	"context"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	dto "github.com/prometheus/client_model/go"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// gatherFamily returns the metric family with the given fully qualified name
// from the provider's own registry, or fails the test when it is missing.
func gatherFamily(t *testing.T, p *Provider, name string) *dto.MetricFamily {
	t.Helper()
	families, err := p.Registry().Gather()
	if err != nil {
		t.Fatalf("registry.Gather() error = %v", err)
	}
	for _, mf := range families {
		if mf.GetName() == name {
			return mf
		}
	}
	t.Fatalf("metric family %q not found in registry; got %d families", name, len(families))
	return nil
}

// ---------------------------------------------------------------------------
// Construction and options
// ---------------------------------------------------------------------------

func TestNew_Defaults(t *testing.T) {
	p, err := New()
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if p.cfg.namespace != "" {
		t.Errorf("namespace = %q, want empty by default", p.cfg.namespace)
	}
	if p.cfg.subsystem != "" {
		t.Errorf("subsystem = %q, want empty by default", p.cfg.subsystem)
	}
	if p.cfg.registry == nil {
		t.Error("registry = nil, want a fresh registry")
	}
	// New always installs its own registry, so Registry() must hand back
	// exactly that registry.
	if reg, ok := p.Registry().(*prometheus.Registry); !ok || reg != p.cfg.registry.(*prometheus.Registry) {
		t.Error("Registry() did not return the provider's own registry")
	}
}

func TestNew_NamespaceAndSubsystemOptions(t *testing.T) {
	p, err := New(WithNamespace("myapp"), WithSubsystem("gateway"))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if p.cfg.namespace != "myapp" {
		t.Errorf("WithNamespace: got %q, want myapp", p.cfg.namespace)
	}
	if p.cfg.subsystem != "gateway" {
		t.Errorf("WithSubsystem: got %q, want gateway", p.cfg.subsystem)
	}

	p.Counter(context.Background(), "requests_total", 1, nil)
	// Namespace and subsystem must prefix the exported metric name.
	gatherFamily(t, p, "myapp_gateway_requests_total")
}

// New() must honour a registry supplied via WithRegistry; without an explicit
// registry it installs its own fresh one.
func TestNew_HonoursWithRegistry(t *testing.T) {
	custom := prometheus.NewRegistry()
	p, err := New(WithRegistry(custom))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if p.cfg.registry != prometheus.Registerer(custom) {
		t.Error("New() did not use the registry supplied via WithRegistry")
	}

	// Metrics must land in the custom registry only, never in the default one.
	p.Counter(context.Background(), "new_custom_counter", 3, nil)
	if got := testutil.CollectAndCount(custom, "new_custom_counter"); got != 1 {
		t.Errorf("custom registry has %d new_custom_counter series, want 1", got)
	}
}

// Without WithRegistry, New() still installs its own fresh registry.
func TestNew_InstallsFreshRegistryWithoutOption(t *testing.T) {
	p, err := New()
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if _, ok := p.cfg.registry.(*prometheus.Registry); !ok {
		t.Fatalf("registry = %T, want *prometheus.Registry", p.cfg.registry)
	}
	p.Counter(context.Background(), "fresh_counter", 1, nil)
	families, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		t.Fatalf("DefaultGatherer.Gather() error = %v", err)
	}
	for _, f := range families {
		if f.GetName() == "fresh_counter" {
			t.Error("default registry contains fresh_counter; New must not leak into the default registry")
		}
	}
}

func TestNewWithDefaultRegistry_HonoursWithRegistry(t *testing.T) {
	custom := prometheus.NewRegistry()
	p, err := NewWithDefaultRegistry(WithRegistry(custom))
	if err != nil {
		t.Fatalf("NewWithDefaultRegistry() error = %v", err)
	}
	if p.cfg.registry != prometheus.Registerer(custom) {
		t.Error("NewWithDefaultRegistry() did not use the registry from WithRegistry")
	}

	// Metrics must land in the custom registry only.
	p.Counter(context.Background(), "custom_counter", 2, nil)
	if got := testutil.CollectAndCount(custom, "custom_counter"); got != 1 {
		t.Errorf("custom registry has %d custom_counter series, want 1", got)
	}
}

// registryOnly is a Registerer that is not a *prometheus.Registry, forcing
// Registry() down its fallback path.
type registryOnly struct {
	prometheus.Registerer
}

func TestRegistry_FallsBackToDefaultGatherer(t *testing.T) {
	p := &Provider{cfg: &config{registry: &registryOnly{}}}
	if p.Registry() != prometheus.DefaultGatherer {
		t.Error("Registry() = custom value, want prometheus.DefaultGatherer for a non-registry registerer")
	}
}

// ---------------------------------------------------------------------------
// Counters
// ---------------------------------------------------------------------------

func TestCounter_NoLabels_AccumulatesAndCaches(t *testing.T) {
	p, _ := New(WithNamespace("ns"))
	ctx := context.Background()

	p.Counter(ctx, "jobs_total", 1.5, nil)
	p.Counter(ctx, "jobs_total", 2.5, nil)

	mf := gatherFamily(t, p, "ns_jobs_total")
	if mf.GetType() != dto.MetricType_COUNTER {
		t.Errorf("metric type = %v, want COUNTER", mf.GetType())
	}
	if len(mf.Metric) != 1 {
		t.Fatalf("family has %d metrics, want 1", len(mf.Metric))
	}
	if got := mf.Metric[0].Counter.GetValue(); got != 4 {
		t.Errorf("counter value = %v, want 4 (1.5 + 2.5)", got)
	}

	// The same instrument must be reused, not re-registered.
	if _, ok := p.counters["jobs_total"]; !ok {
		t.Error("counter was not cached in p.counters")
	}
}

func TestCounter_WithLabels(t *testing.T) {
	p, _ := New()
	ctx := context.Background()

	p.Counter(ctx, "http_requests_total", 1, map[string]string{"method": "GET", "path": "/a"})
	p.Counter(ctx, "http_requests_total", 2, map[string]string{"method": "POST", "path": "/b"})
	p.Counter(ctx, "http_requests_total", 3, map[string]string{"method": "GET", "path": "/a"})

	mf := gatherFamily(t, p, "http_requests_total")
	if len(mf.Metric) != 2 {
		t.Fatalf("family has %d metrics, want 2 (label dimensions)", len(mf.Metric))
	}
	for _, m := range mf.Metric {
		labels := labelMap(m.GetLabel())
		if labels["method"] == "GET" {
			if got := m.Counter.GetValue(); got != 4 {
				t.Errorf("GET counter = %v, want 4 (1 + 3)", got)
			}
		}
		if labels["method"] == "POST" {
			if got := m.Counter.GetValue(); got != 2 {
				t.Errorf("POST counter = %v, want 2", got)
			}
		}
	}
}

// ---------------------------------------------------------------------------
// Histograms
// ---------------------------------------------------------------------------

func TestHistogram_NoLabels(t *testing.T) {
	p, _ := New()
	ctx := context.Background()

	p.Histogram(ctx, "duration_seconds", 1, nil)
	p.Histogram(ctx, "duration_seconds", 2, nil)

	mf := gatherFamily(t, p, "duration_seconds")
	if mf.GetType() != dto.MetricType_HISTOGRAM {
		t.Errorf("metric type = %v, want HISTOGRAM", mf.GetType())
	}
	h := mf.Metric[0].Histogram
	if got := h.GetSampleCount(); got != 2 {
		t.Errorf("sample count = %d, want 2", got)
	}
	if got := h.GetSampleSum(); got != 3 {
		t.Errorf("sample sum = %v, want 3", got)
	}
	if len(h.GetBucket()) == 0 {
		t.Error("histogram has no buckets, want the default bucket layout")
	}
}

func TestHistogram_WithLabels(t *testing.T) {
	p, _ := New()
	ctx := context.Background()

	p.Histogram(ctx, "latency_seconds", 0.1, map[string]string{"route": "/fast"})
	p.Histogram(ctx, "latency_seconds", 0.9, map[string]string{"route": "/slow"})

	mf := gatherFamily(t, p, "latency_seconds")
	if len(mf.Metric) != 2 {
		t.Fatalf("family has %d metrics, want 2", len(mf.Metric))
	}
	for _, m := range mf.Metric {
		labels := labelMap(m.GetLabel())
		wantSum := 0.1
		if labels["route"] == "/slow" {
			wantSum = 0.9
		}
		if got := m.Histogram.GetSampleSum(); got != wantSum {
			t.Errorf("route %s sample sum = %v, want %v", labels["route"], got, wantSum)
		}
	}
}

// ---------------------------------------------------------------------------
// Gauges
// ---------------------------------------------------------------------------

func TestGauge_NoLabels_SetSemantics(t *testing.T) {
	p, _ := New()
	ctx := context.Background()

	p.Gauge(ctx, "active_connections", 5, nil)
	p.Gauge(ctx, "active_connections", 9, nil)

	mf := gatherFamily(t, p, "active_connections")
	if mf.GetType() != dto.MetricType_GAUGE {
		t.Errorf("metric type = %v, want GAUGE", mf.GetType())
	}
	if got := mf.Metric[0].Gauge.GetValue(); got != 9 {
		t.Errorf("gauge = %v, want 9 (last Set wins)", got)
	}
}

func TestGauge_WithLabels(t *testing.T) {
	p, _ := New()
	ctx := context.Background()

	p.Gauge(ctx, "queue_depth", 3, map[string]string{"queue": "orders"})
	p.Gauge(ctx, "queue_depth", 8, map[string]string{"queue": "payments"})

	mf := gatherFamily(t, p, "queue_depth")
	if len(mf.Metric) != 2 {
		t.Fatalf("family has %d metrics, want 2", len(mf.Metric))
	}
	for _, m := range mf.Metric {
		labels := labelMap(m.GetLabel())
		want := 3.0
		if labels["queue"] == "payments" {
			want = 8
		}
		if got := m.Gauge.GetValue(); got != want {
			t.Errorf("queue %s gauge = %v, want %v", labels["queue"], got, want)
		}
	}
}

// ---------------------------------------------------------------------------
// Label handling
// ---------------------------------------------------------------------------

// The label key order is cached on first use so that later calls with the
// same metric write values positionally into the same vector children.
func TestCachedLabelKeys_StableAcrossCalls(t *testing.T) {
	p, _ := New()
	ctx := context.Background()

	p.Counter(ctx, "events_total", 1, map[string]string{"kind": "a", "source": "s"})
	p.Counter(ctx, "events_total", 1, map[string]string{"source": "s", "kind": "a"})

	keys := p.labelNames["events_total"]
	if len(keys) != 2 {
		t.Fatalf("cached label keys = %v, want 2 entries", keys)
	}

	// Both calls must have fed values in the cached key order; the resulting
	// child must carry both label values.
	mf := gatherFamily(t, p, "events_total")
	if len(mf.Metric) != 1 {
		t.Fatalf("family has %d metrics, want 1 (identical label set)", len(mf.Metric))
	}
	labels := labelMap(mf.Metric[0].GetLabel())
	if labels["kind"] != "a" || labels["source"] != "s" {
		t.Errorf("labels = %v, want kind=a source=s", labels)
	}
}

func TestLabelKeysAndValues(t *testing.T) {
	p, _ := New()
	labels := map[string]string{"b": "2", "a": "1", "c": "3"}

	keys := p.labelKeys(labels)
	if len(keys) != 3 {
		t.Errorf("labelKeys() = %v, want 3 keys", keys)
	}

	vals := p.labelValues(labels, []string{"a", "b", "c"})
	if vals[0] != "1" || vals[1] != "2" || vals[2] != "3" {
		t.Errorf("labelValues() = %v, want [1 2 3] in key order", vals)
	}

	// A missing key yields an empty value, not a panic.
	vals = p.labelValues(labels, []string{"missing"})
	if len(vals) != 1 || vals[0] != "" {
		t.Errorf("labelValues(missing) = %v, want one empty string", vals)
	}
}

// ---------------------------------------------------------------------------
// Misc
// ---------------------------------------------------------------------------

func labelMap(pairs []*dto.LabelPair) map[string]string {
	out := make(map[string]string, len(pairs))
	for _, pair := range pairs {
		out[pair.GetName()] = pair.GetValue()
	}
	return out
}
