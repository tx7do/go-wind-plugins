package metrics

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	coreMetrics "github.com/tx7do/go-wind-plugins/metrics"
)

// fakeMetrics is a thread-safe test implementation of metrics.Metrics.
type fakeMetrics struct {
	mu         sync.Mutex
	counters   map[string]float64
	histograms map[string][]float64
	gauges     map[string]float64
	lastLabels map[string]string
}

func newFakeMetrics() *fakeMetrics {
	return &fakeMetrics{
		counters:   make(map[string]float64),
		histograms: make(map[string][]float64),
		gauges:     make(map[string]float64),
	}
}

func (f *fakeMetrics) Counter(_ context.Context, name string, value float64, labels map[string]string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.counters[name] += value
	f.lastLabels = labels
}

func (f *fakeMetrics) Histogram(_ context.Context, name string, value float64, labels map[string]string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.histograms[name] = append(f.histograms[name], value)
	f.lastLabels = labels
}

func (f *fakeMetrics) Gauge(_ context.Context, name string, value float64, labels map[string]string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.gauges[name] = value
}

func TestMiddleware_BasicRequest(t *testing.T) {
	fm := newFakeMetrics()
	mw := Middleware(fm)

	called := false
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/users", nil)
	h.ServeHTTP(rec, req)

	if !called {
		t.Fatal("handler was not called")
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	fm.mu.Lock()
	defer fm.mu.Unlock()
	if fm.counters[defaultRequestCounter] != 1 {
		t.Fatalf("expected counter=1, got %v", fm.counters[defaultRequestCounter])
	}
	if len(fm.histograms[defaultLatencyHistogram]) != 1 {
		t.Fatalf("expected 1 histogram observation, got %d", len(fm.histograms[defaultLatencyHistogram]))
	}
	if fm.lastLabels["method"] != http.MethodGet {
		t.Fatalf("expected method=GET, got %q", fm.lastLabels["method"])
	}
	if fm.lastLabels["path"] != "/api/users" {
		t.Fatalf("expected path=/api/users, got %q", fm.lastLabels["path"])
	}
	if fm.lastLabels["status"] != "200" {
		t.Fatalf("expected status=200, got %q", fm.lastLabels["status"])
	}
}

func TestMiddleware_InFlightGauge(t *testing.T) {
	fm := newFakeMetrics()
	mw := Middleware(fm)

	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
	}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/items", nil)
	h.ServeHTTP(rec, req)

	fm.mu.Lock()
	defer fm.mu.Unlock()
	// After request completes, gauge should be 0.
	if fm.gauges[defaultInFlightGauge] != 0 {
		t.Fatalf("expected gauge=0 after completion, got %v", fm.gauges[defaultInFlightGauge])
	}
}

func TestMiddleware_SkipFunc(t *testing.T) {
	fm := newFakeMetrics()
	mw := Middleware(fm, WithSkipFunc(func(r *http.Request) bool {
		return r.URL.Path == "/healthz"
	}))

	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	h.ServeHTTP(rec, req)

	fm.mu.Lock()
	defer fm.mu.Unlock()
	if len(fm.counters) != 0 {
		t.Fatalf("expected no metrics for skipped path, got %v", fm.counters)
	}
}

func TestMiddleware_CustomNames(t *testing.T) {
	fm := newFakeMetrics()
	mw := Middleware(fm,
		WithRequestCounterName("my_requests"),
		WithLatencyHistogramName("my_latency"),
		WithInFlightGaugeName("my_inflight"),
	)

	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	h.ServeHTTP(rec, req)

	fm.mu.Lock()
	defer fm.mu.Unlock()
	if fm.counters["my_requests"] != 1 {
		t.Fatalf("expected custom counter, got %v", fm.counters)
	}
	if len(fm.histograms["my_latency"]) != 1 {
		t.Fatalf("expected custom histogram, got %v", fm.histograms)
	}
	if _, ok := fm.gauges["my_inflight"]; !ok {
		t.Fatal("expected custom gauge to be set")
	}
}

func TestMiddleware_CustomLabelFunc(t *testing.T) {
	fm := newFakeMetrics()
	mw := Middleware(fm, WithLabelFunc(func(r *http.Request, status int) map[string]string {
		return map[string]string{
			"route": "custom_route",
		}
	}))

	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	h.ServeHTTP(rec, req)

	fm.mu.Lock()
	defer fm.mu.Unlock()
	if fm.lastLabels["route"] != "custom_route" {
		t.Fatalf("expected custom label, got %v", fm.lastLabels)
	}
}

// Ensure fakeMetrics satisfies the interface at compile time.
var _ coreMetrics.Metrics = (*fakeMetrics)(nil)
