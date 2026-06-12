package thrift

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/apache/thrift/lib/go/thrift"

	windlog "github.com/tx7do/go-wind/log"

	"github.com/tx7do/go-wind-plugins/metrics"
)

// ---------------------------------------------------------------------------
// testProcessor: implements thrift.TProcessor for testing.
// ---------------------------------------------------------------------------

type testProcessor struct {
	mu       sync.Mutex
	called   bool
	err      thrift.TException
	panicVal any
}

func (p *testProcessor) Process(_ context.Context, _, _ thrift.TProtocol) (bool, thrift.TException) {
	p.mu.Lock()
	p.called = true
	p.mu.Unlock()
	if p.panicVal != nil {
		panic(p.panicVal)
	}
	return true, p.err
}

func (p *testProcessor) ProcessorMap() map[string]thrift.TProcessorFunction  { return nil }
func (p *testProcessor) AddToProcessorMap(string, thrift.TProcessorFunction) {}

func (p *testProcessor) wasCalled() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.called
}

// ---------------------------------------------------------------------------
// mock Logger
// ---------------------------------------------------------------------------

type mockLogger struct {
	mu    sync.Mutex
	calls []string
}

func (l *mockLogger) Debug(_ context.Context, msg string, _ ...any) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.calls = append(l.calls, "debug:"+msg)
}
func (l *mockLogger) Info(_ context.Context, msg string, _ ...any) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.calls = append(l.calls, "info:"+msg)
}
func (l *mockLogger) Warn(_ context.Context, msg string, _ ...any) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.calls = append(l.calls, "warn:"+msg)
}
func (l *mockLogger) Error(_ context.Context, msg string, _ ...any) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.calls = append(l.calls, "error:"+msg)
}
func (l *mockLogger) Enabled(_ windlog.Level) bool { return true }
func (l *mockLogger) With(_ ...any) windlog.Logger { return l }

func (l *mockLogger) hasCall(prefix string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, c := range l.calls {
		if strings.HasPrefix(c, prefix) {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// mock Metrics
// ---------------------------------------------------------------------------

type mockMetrics struct {
	mu       sync.Mutex
	counters []string
	hists    []string
}

func (m *mockMetrics) Counter(_ context.Context, name string, _ float64, _ map[string]string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.counters = append(m.counters, name)
}
func (m *mockMetrics) Histogram(_ context.Context, name string, _ float64, _ map[string]string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.hists = append(m.hists, name)
}
func (m *mockMetrics) Gauge(_ context.Context, _ string, _ float64, _ map[string]string) {}

func (m *mockMetrics) hasCounter(name string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, c := range m.counters {
		if c == name {
			return true
		}
	}
	return false
}
func (m *mockMetrics) hasHistogram(name string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, h := range m.hists {
		if h == name {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// Tests: LoggingWrapper
// ---------------------------------------------------------------------------

func TestLoggingWrapper_Success(t *testing.T) {
	logger := &mockLogger{}
	inner := &testProcessor{}
	wrapped := LoggingWrapper(logger)(inner)

	ok, err := wrapped.Process(context.Background(), nil, nil)
	if !ok || err != nil {
		t.Fatalf("expected ok=true, err=nil; got ok=%v, err=%v", ok, err)
	}
	if !inner.wasCalled() {
		t.Error("inner processor was not called")
	}
	if !logger.hasCall("info:thrift rpc") {
		t.Error("expected info log for successful RPC")
	}
}

func TestLoggingWrapper_Error(t *testing.T) {
	logger := &mockLogger{}
	inner := &testProcessor{err: thrift.NewTApplicationException(thrift.INTERNAL_ERROR, "boom")}
	wrapped := LoggingWrapper(logger)(inner)

	ok, _ := wrapped.Process(context.Background(), nil, nil)
	// The inner processor returns ok=true with an error.
	// LoggingWrapper should log at error level but preserve the return values.
	if !ok {
		t.Error("expected ok=true (wrapper preserves inner ok)")
	}
	if !logger.hasCall("error:thrift rpc") {
		t.Error("expected error log for failed RPC")
	}
}

// ---------------------------------------------------------------------------
// Tests: RecoveryWrapper
// ---------------------------------------------------------------------------

func TestRecoveryWrapper_NoPanic(t *testing.T) {
	logger := &mockLogger{}
	inner := &testProcessor{}
	wrapped := RecoveryWrapper(logger)(inner)

	ok, err := wrapped.Process(context.Background(), nil, nil)
	if !ok || err != nil {
		t.Fatalf("expected ok=true, err=nil; got ok=%v, err=%v", ok, err)
	}
}

func TestRecoveryWrapper_Panic(t *testing.T) {
	logger := &mockLogger{}
	inner := &testProcessor{panicVal: "something broke"}
	wrapped := RecoveryWrapper(logger)(inner)

	ok, err := wrapped.Process(context.Background(), nil, nil)
	if ok {
		t.Error("expected ok=false after panic")
	}
	if err == nil {
		t.Fatal("expected TException from recovered panic")
	}
	if !logger.hasCall("error:thrift rpc panic recovered") {
		t.Error("expected error log for recovered panic")
	}
}

// ---------------------------------------------------------------------------
// Tests: MetricsWrapper
// ---------------------------------------------------------------------------

func TestMetricsWrapper_Success(t *testing.T) {
	m := &mockMetrics{}
	inner := &testProcessor{}
	wrapped := MetricsWrapper(m)(inner)

	ok, err := wrapped.Process(context.Background(), nil, nil)
	if !ok || err != nil {
		t.Fatalf("expected ok=true, err=nil; got ok=%v, err=%v", ok, err)
	}
	if !m.hasCounter("thrift_rpc_calls_total") {
		t.Error("expected counter metric")
	}
	if !m.hasHistogram("thrift_rpc_duration_ms") {
		t.Error("expected histogram metric")
	}
}

// ---------------------------------------------------------------------------
// Tests: WithLogging / WithRecovery / WithMetrics Options
// ---------------------------------------------------------------------------

func TestWithLogging(t *testing.T) {
	srv := NewServer(":0",
		WithProcessor(thrift.NewTMultiplexedProcessor()),
		WithLogging(nil),
	)
	if len(srv.wrappers) != 1 {
		t.Fatalf("expected 1 wrapper, got %d", len(srv.wrappers))
	}
}

func TestWithRecovery(t *testing.T) {
	srv := NewServer(":0",
		WithProcessor(thrift.NewTMultiplexedProcessor()),
		WithRecovery(nil),
	)
	if len(srv.wrappers) != 1 {
		t.Fatalf("expected 1 wrapper, got %d", len(srv.wrappers))
	}
}

func TestWithMetrics(t *testing.T) {
	m := &mockMetrics{}
	srv := NewServer(":0",
		WithProcessor(thrift.NewTMultiplexedProcessor()),
		WithMetrics(m),
	)
	if len(srv.wrappers) != 1 {
		t.Fatalf("expected 1 wrapper, got %d", len(srv.wrappers))
	}
}

// ---------------------------------------------------------------------------
// Tests: Chained wrappers via WithProcessorWrappers
// ---------------------------------------------------------------------------

func TestWithProcessorWrappers_ChainOrder(t *testing.T) {
	var order []int

	inner := &testProcessor{}

	w1 := func(next thrift.TProcessor) thrift.TProcessor {
		return &testProcessorWrap{fn: func(ctx context.Context) (bool, thrift.TException) {
			order = append(order, 1)
			return next.Process(ctx, nil, nil)
		}}
	}
	w2 := func(next thrift.TProcessor) thrift.TProcessor {
		return &testProcessorWrap{fn: func(ctx context.Context) (bool, thrift.TException) {
			order = append(order, 2)
			return next.Process(ctx, nil, nil)
		}}
	}

	// Build the chain manually (same logic as NewServer).
	proc := thrift.TProcessor(inner)
	for i := 1; i >= 0; i-- {
		wrappers := []ProcessorWrapper{w1, w2}
		proc = wrappers[i](proc)
	}

	proc.Process(context.Background(), nil, nil)

	if len(order) != 2 {
		t.Fatalf("expected 2 calls, got %d", len(order))
	}
	// w1 should be outermost (called first)
	if order[0] != 1 || order[1] != 2 {
		t.Errorf("expected order [1,2], got %v", order)
	}
}

// testProcessorWrap is a thin wrapper for testing chain order.
type testProcessorWrap struct {
	fn func(ctx context.Context) (bool, thrift.TException)
}

func (p *testProcessorWrap) Process(ctx context.Context, _, _ thrift.TProtocol) (bool, thrift.TException) {
	return p.fn(ctx)
}
func (p *testProcessorWrap) ProcessorMap() map[string]thrift.TProcessorFunction  { return nil }
func (p *testProcessorWrap) AddToProcessorMap(string, thrift.TProcessorFunction) {}

// ---------------------------------------------------------------------------
// Compile-time checks
// ---------------------------------------------------------------------------

var _ thrift.TProcessor = (*testProcessor)(nil)
var _ thrift.TProcessor = (*testProcessorWrap)(nil)
var _ metrics.Metrics = (*mockMetrics)(nil)
