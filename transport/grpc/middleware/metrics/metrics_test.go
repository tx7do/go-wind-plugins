package metrics

import (
	"context"
	"sync"
	"testing"

	coreMetrics "github.com/tx7do/go-wind-plugins/metrics"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

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

var _ coreMetrics.Metrics = (*fakeMetrics)(nil)

func TestUnaryServerInterceptor_Success(t *testing.T) {
	fm := newFakeMetrics()
	intc := UnaryServerInterceptor(fm)

	handler := func(ctx context.Context, req any) (any, error) {
		return "ok", nil
	}

	resp, err := intc(context.Background(), nil, &grpc.UnaryServerInfo{FullMethod: "/test.Svc/Method"}, handler)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp != "ok" {
		t.Fatalf("expected 'ok', got %v", resp)
	}

	fm.mu.Lock()
	defer fm.mu.Unlock()
	if fm.counters[defaultRequestCounter] != 1 {
		t.Fatalf("expected counter=1, got %v", fm.counters)
	}
	if len(fm.histograms[defaultLatencyHistogram]) != 1 {
		t.Fatalf("expected 1 histogram, got %d", len(fm.histograms[defaultLatencyHistogram]))
	}
	if fm.lastLabels["code"] != codes.OK.String() {
		t.Fatalf("expected code=OK, got %q", fm.lastLabels["code"])
	}
}

func TestUnaryServerInterceptor_Error(t *testing.T) {
	fm := newFakeMetrics()
	intc := UnaryServerInterceptor(fm)

	handlerErr := status.Error(codes.NotFound, "not found")
	handler := func(ctx context.Context, req any) (any, error) {
		return nil, handlerErr
	}

	_, err := intc(context.Background(), nil, &grpc.UnaryServerInfo{FullMethod: "/test.Svc/Method"}, handler)
	if err == nil {
		t.Fatal("expected error")
	}

	fm.mu.Lock()
	defer fm.mu.Unlock()
	if fm.lastLabels["code"] != codes.NotFound.String() {
		t.Fatalf("expected code=NotFound, got %q", fm.lastLabels["code"])
	}
}

func TestUnaryServerInterceptor_SkipFunc(t *testing.T) {
	fm := newFakeMetrics()
	intc := UnaryServerInterceptor(fm, WithSkipFunc(func(method string) bool {
		return method == "/grpc.health.v1.Health/Check"
	}))

	handler := func(ctx context.Context, req any) (any, error) {
		return "ok", nil
	}

	intc(context.Background(), nil, &grpc.UnaryServerInfo{FullMethod: "/grpc.health.v1.Health/Check"}, handler)

	fm.mu.Lock()
	defer fm.mu.Unlock()
	if len(fm.counters) != 0 {
		t.Fatalf("expected no metrics for skipped method, got %v", fm.counters)
	}
}

func TestUnaryClientInterceptor_Success(t *testing.T) {
	fm := newFakeMetrics()
	intc := UnaryClientInterceptor(fm)

	invoker := func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, callOpts ...grpc.CallOption) error {
		return nil
	}

	err := intc(context.Background(), "/test.Svc/Method", nil, nil, nil, invoker)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	fm.mu.Lock()
	defer fm.mu.Unlock()
	if fm.counters[defaultRequestCounter] != 1 {
		t.Fatalf("expected counter=1, got %v", fm.counters)
	}
	if fm.gauges[defaultInFlightGauge] != 0 {
		t.Fatalf("expected gauge=0 after completion, got %v", fm.gauges)
	}
}

func TestUnaryClientInterceptor_Error(t *testing.T) {
	fm := newFakeMetrics()
	intc := UnaryClientInterceptor(fm)

	invoker := func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, callOpts ...grpc.CallOption) error {
		return status.Error(codes.DeadlineExceeded, "timeout")
	}

	err := intc(context.Background(), "/test.Svc/Method", nil, nil, nil, invoker)
	if err == nil {
		t.Fatal("expected error")
	}

	fm.mu.Lock()
	defer fm.mu.Unlock()
	if fm.lastLabels["code"] != codes.DeadlineExceeded.String() {
		t.Fatalf("expected code=DeadlineExceeded, got %q", fm.lastLabels["code"])
	}
}

func TestStreamServerInterceptor_Success(t *testing.T) {
	fm := newFakeMetrics()
	intc := StreamServerInterceptor(fm)

	handler := func(srv any, ss grpc.ServerStream) error {
		return nil
	}

	err := intc(nil, &fakeServerStream{}, &grpc.StreamServerInfo{FullMethod: "/test.Svc/StreamMethod"}, handler)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	fm.mu.Lock()
	defer fm.mu.Unlock()
	if fm.counters[defaultRequestCounter] != 1 {
		t.Fatalf("expected counter=1, got %v", fm.counters)
	}
}

func TestCustomMetricNames(t *testing.T) {
	fm := newFakeMetrics()
	intc := UnaryServerInterceptor(fm,
		WithRequestCounterName("rpc_count"),
		WithLatencyHistogramName("rpc_latency"),
		WithInFlightGaugeName("rpc_inflight"),
	)

	handler := func(ctx context.Context, req any) (any, error) {
		return "ok", nil
	}

	intc(context.Background(), nil, &grpc.UnaryServerInfo{FullMethod: "/test.Svc/Method"}, handler)

	fm.mu.Lock()
	defer fm.mu.Unlock()
	if fm.counters["rpc_count"] != 1 {
		t.Fatalf("expected custom counter, got %v", fm.counters)
	}
	if len(fm.histograms["rpc_latency"]) != 1 {
		t.Fatalf("expected custom histogram, got %v", fm.histograms)
	}
	if _, ok := fm.gauges["rpc_inflight"]; !ok {
		t.Fatal("expected custom gauge")
	}
}

func TestDefaultLabels_ServiceName(t *testing.T) {
	labels := defaultLabels("/myapp.UserService/GetUser", nil)
	if labels["service"] != "myapp.UserService" {
		t.Fatalf("expected service=myapp.UserService, got %q", labels["service"])
	}
	if labels["method"] != "/myapp.UserService/GetUser" {
		t.Fatalf("expected method, got %q", labels["method"])
	}
	if labels["code"] != codes.OK.String() {
		t.Fatalf("expected code=OK, got %q", labels["code"])
	}
}

// fakeServerStream is a minimal grpc.ServerStream for testing.
type fakeServerStream struct {
	grpc.ServerStream
}

func (f *fakeServerStream) Context() context.Context {
	return context.Background()
}
