package metrics

import (
	"context"
	"testing"
)

// mockMetrics records calls made through the Metrics interface.
type mockMetrics struct {
	counters   []call
	histograms []call
	gauges     []call
}

type call struct {
	name   string
	value  float64
	labels map[string]string
}

func (m *mockMetrics) Counter(_ context.Context, name string, value float64, labels map[string]string) {
	m.counters = append(m.counters, call{name, value, labels})
}

func (m *mockMetrics) Histogram(_ context.Context, name string, value float64, labels map[string]string) {
	m.histograms = append(m.histograms, call{name, value, labels})
}

func (m *mockMetrics) Gauge(_ context.Context, name string, value float64, labels map[string]string) {
	m.gauges = append(m.gauges, call{name, value, labels})
}

// mockCloser is a minimal Closer implementation for interface verification.
type mockCloser struct{ closed bool }

func (c *mockCloser) Close() error { c.closed = true; return nil }

var (
	_ Metrics = (*mockMetrics)(nil)
	_ Closer  = (*mockCloser)(nil)
)

// ---------------------------------------------------------------------------
// Interface contract
// ---------------------------------------------------------------------------

func TestMetricsInterface_CallThrough(t *testing.T) {
	m := &mockMetrics{}
	ctx := context.Background()
	labels := map[string]string{"method": "GET"}

	m.Counter(ctx, "requests_total", 1, labels)
	m.Histogram(ctx, "request_duration_seconds", 0.042, labels)
	m.Gauge(ctx, "queue_depth", 42, labels)

	if len(m.counters) != 1 {
		t.Fatalf("len(counters) = %d, want 1", len(m.counters))
	}
	if m.counters[0].name != "requests_total" || m.counters[0].value != 1 {
		t.Errorf("counter call = %+v, want requests_total=1", m.counters[0])
	}
	if m.counters[0].labels["method"] != "GET" {
		t.Errorf("counter labels = %v, want method=GET", m.counters[0].labels)
	}

	if len(m.histograms) != 1 {
		t.Fatalf("len(histograms) = %d, want 1", len(m.histograms))
	}
	if m.histograms[0].name != "request_duration_seconds" || m.histograms[0].value != 0.042 {
		t.Errorf("histogram call = %+v, want request_duration_seconds=0.042", m.histograms[0])
	}

	if len(m.gauges) != 1 {
		t.Fatalf("len(gauges) = %d, want 1", len(m.gauges))
	}
	if m.gauges[0].name != "queue_depth" || m.gauges[0].value != 42 {
		t.Errorf("gauge call = %+v, want queue_depth=42", m.gauges[0])
	}
}

func TestMetricsInterface_ZeroValueLabels(t *testing.T) {
	m := &mockMetrics{}

	m.Counter(context.Background(), "c", 0, nil)
	if m.counters[0].labels != nil {
		t.Errorf("counter labels = %v, want nil", m.counters[0].labels)
	}
	if m.counters[0].value != 0 {
		t.Errorf("counter value = %v, want 0", m.counters[0].value)
	}
}

// ---------------------------------------------------------------------------
// Closer
// ---------------------------------------------------------------------------

func TestCloser_Close(t *testing.T) {
	c := &mockCloser{}

	if err := c.Close(); err != nil {
		t.Errorf("Close() = %v, want nil", err)
	}
	if !c.closed {
		t.Error("Close() should mark the closer as closed")
	}
}
