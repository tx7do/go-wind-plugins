package sentinel

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	sentinelapi "github.com/alibaba/sentinel-golang/api"
	"github.com/alibaba/sentinel-golang/core/base"
	scb "github.com/alibaba/sentinel-golang/core/circuitbreaker"
	sentinelconfig "github.com/alibaba/sentinel-golang/core/config"

	windcb "github.com/tx7do/go-wind-plugins/circuitbreaker"
)

// TestMain initializes Sentinel with a minimal in-memory configuration: no
// metric log task, no system-statistic collectors, and a throwaway log
// directory. No dashboard or external service is contacted.
func TestMain(m *testing.M) {
	logDir, err := os.MkdirTemp("", "sentinel-cb-test-logs")
	if err != nil {
		logDir = os.TempDir()
	}

	conf := sentinelconfig.NewDefaultConfig()
	conf.Sentinel.Log.Dir = logDir
	conf.Sentinel.Log.Metric.FlushIntervalSec = 0                 // disable metric log task
	conf.Sentinel.Stat.System = sentinelconfig.SystemStatConfig{} // disable system collectors

	_ = sentinelapi.InitWithConfig(conf)

	code := m.Run()

	_ = os.RemoveAll(logDir)
	os.Exit(code)
}

// loadErrorCountRule installs an ErrorCount circuit-breaking rule that trips
// after a single failed request. retryTimeout controls how long the circuit
// stays open before allowing a half-open probe.
func loadErrorCountRule(t *testing.T, resource string, retryTimeout time.Duration) {
	t.Helper()
	ok, err := scb.LoadRules([]*scb.Rule{{
		Resource:         resource,
		Strategy:         scb.ErrorCount,
		Threshold:        1, // a single error trips the breaker
		RetryTimeoutMs:   uint32(retryTimeout.Milliseconds()),
		MinRequestAmount: 1,
		StatIntervalMs:   1000,
	}})
	if err != nil || !ok {
		t.Fatalf("scb.LoadRules() = (%v, %v), want (true, nil)", ok, err)
	}
	t.Cleanup(func() { _ = scb.ClearRulesOfResource(resource) })
}

// ---------------------------------------------------------------------------
// New / options
// ---------------------------------------------------------------------------

func TestNew_Defaults(t *testing.T) {
	b := New("test-cb-defaults")
	defer func() { _ = b.Close() }()

	if b.resource != "test-cb-defaults" {
		t.Errorf("resource = %q, want %q", b.resource, "test-cb-defaults")
	}
	if b.cfg.trafficType != base.Outbound {
		t.Errorf("trafficType = %v, want Outbound", b.cfg.trafficType)
	}
	if len(b.cfg.entryOpts) != 0 {
		t.Errorf("len(entryOpts) = %d, want 0", len(b.cfg.entryOpts))
	}
}

func TestWithTrafficType(t *testing.T) {
	b := New("r", WithTrafficType(base.Inbound))
	defer func() { _ = b.Close() }()

	if b.cfg.trafficType != base.Inbound {
		t.Errorf("trafficType = %v, want Inbound", b.cfg.trafficType)
	}
}

func TestWithEntryOptions(t *testing.T) {
	b := New("r", WithEntryOptions(sentinelapi.WithAcquireCount(2)))
	defer func() { _ = b.Close() }()

	if len(b.cfg.entryOpts) != 1 {
		t.Fatalf("len(entryOpts) = %d, want 1", len(b.cfg.entryOpts))
	}
}

// ---------------------------------------------------------------------------
// Allow / MarkSuccess / MarkFailure — no rules configured
// ---------------------------------------------------------------------------

func TestAllow_NoRulesAlwaysAdmits(t *testing.T) {
	b := New("test-cb-no-rules")
	defer func() { _ = b.Close() }()

	for i := 0; i < 5; i++ {
		if err := b.Allow(); err != nil {
			t.Fatalf("Allow() #%d = %v, want nil", i, err)
		}
		// Each Allow must be closed by exactly one Mark* call.
		b.MarkSuccess()
	}
}

func TestMarkSuccess_MarkFailure_WithoutAllowAreNoOps(t *testing.T) {
	b := New("test-cb-mark-no-allow")
	defer func() { _ = b.Close() }()

	// Neither call has an entry to close — both must be safe no-ops.
	b.MarkSuccess()
	b.MarkFailure()
}

func TestState_NoRulesClosed(t *testing.T) {
	b := New("test-cb-state-no-rules")
	defer func() { _ = b.Close() }()

	if s := b.State(); s != windcb.StateClosed {
		t.Errorf("State() = %v, want StateClosed", s)
	}
}

// ---------------------------------------------------------------------------
// Execute — happy path and error propagation
// ---------------------------------------------------------------------------

func TestExecute_Success(t *testing.T) {
	b := New("test-cb-execute-ok")
	defer func() { _ = b.Close() }()

	err := b.Execute(context.Background(), func() error { return nil })
	if err != nil {
		t.Errorf("Execute() = %v, want nil", err)
	}
}

func TestExecute_PropagatesError(t *testing.T) {
	b := New("test-cb-execute-error")
	defer func() { _ = b.Close() }()

	wantErr := errors.New("boom")
	err := b.Execute(context.Background(), func() error { return wantErr })
	if !errors.Is(err, wantErr) {
		t.Errorf("Execute() = %v, want %v", err, wantErr)
	}
}

// ---------------------------------------------------------------------------
// Tripping via Execute (ErrorCount rule)
// ---------------------------------------------------------------------------

func TestExecute_TripsAfterErrorCountExceeded(t *testing.T) {
	const res = "test-cb-trip-execute"
	loadErrorCountRule(t, res, 10*time.Minute)

	b := New(res)
	defer func() { _ = b.Close() }()

	// The single failing request is admitted and its own error is returned.
	wantErr := errors.New("downstream failure")
	err := b.Execute(context.Background(), func() error { return wantErr })
	if !errors.Is(err, wantErr) {
		t.Fatalf("Execute() = %v, want %v", err, wantErr)
	}

	// The breaker has tripped: subsequent requests are rejected.
	if s := b.State(); s != windcb.StateOpen {
		t.Errorf("State() after one error = %v, want StateOpen", s)
	}
	if err := b.Allow(); !errors.Is(err, windcb.ErrCircuitOpen) {
		t.Errorf("Allow() on open circuit = %v, want ErrCircuitOpen", err)
	}

	// Execute must short-circuit without calling fn.
	called := false
	err = b.Execute(context.Background(), func() error {
		called = true
		return nil
	})
	if !errors.Is(err, windcb.ErrCircuitOpen) {
		t.Errorf("Execute() on open circuit = %v, want ErrCircuitOpen", err)
	}
	if called {
		t.Error("fn should not be called when the circuit is open")
	}
}

// ---------------------------------------------------------------------------
// Tripping via Allow + MarkFailure
// ---------------------------------------------------------------------------

func TestAllow_MarkFailureTripsCircuit(t *testing.T) {
	const res = "test-cb-trip-mark"
	loadErrorCountRule(t, res, 10*time.Minute)

	b := New(res)
	defer func() { _ = b.Close() }()

	if err := b.Allow(); err != nil {
		t.Fatalf("Allow() = %v, want nil", err)
	}
	b.MarkFailure()

	if s := b.State(); s != windcb.StateOpen {
		t.Errorf("State() after MarkFailure = %v, want StateOpen", s)
	}
	if err := b.Allow(); !errors.Is(err, windcb.ErrCircuitOpen) {
		t.Errorf("Allow() on open circuit = %v, want ErrCircuitOpen", err)
	}
}

// ---------------------------------------------------------------------------
// Half-open probe: failure re-opens, success recovers
// ---------------------------------------------------------------------------

func TestHalfOpen_FailedProbeReopens(t *testing.T) {
	const res = "test-cb-halfopen-fail"
	loadErrorCountRule(t, res, 50*time.Millisecond)

	b := New(res)
	defer func() { _ = b.Close() }()

	// Trip the breaker.
	if err := b.Allow(); err != nil {
		t.Fatalf("Allow() = %v, want nil", err)
	}
	b.MarkFailure()
	if s := b.State(); s != windcb.StateOpen {
		t.Fatalf("State() after trip = %v, want StateOpen", s)
	}

	// After the retry timeout the probe request is admitted.
	time.Sleep(150 * time.Millisecond)
	if err := b.Allow(); err != nil {
		t.Fatalf("probe Allow() = %v, want nil", err)
	}
	// While the probe is in flight further requests are rejected.
	if err := b.Allow(); !errors.Is(err, windcb.ErrCircuitOpen) {
		t.Errorf("Allow() during probe = %v, want ErrCircuitOpen", err)
	}

	// A failed probe re-opens the breaker.
	b.MarkFailure()
	if err := b.Allow(); !errors.Is(err, windcb.ErrCircuitOpen) {
		t.Errorf("Allow() after failed probe = %v, want ErrCircuitOpen", err)
	}
}

func TestHalfOpen_SuccessfulProbeRecovers(t *testing.T) {
	const res = "test-cb-halfopen-success"
	loadErrorCountRule(t, res, 50*time.Millisecond)

	b := New(res)
	defer func() { _ = b.Close() }()

	// Trip the breaker.
	if err := b.Allow(); err != nil {
		t.Fatalf("Allow() = %v, want nil", err)
	}
	b.MarkFailure()

	// After the retry timeout the probe request is admitted.
	time.Sleep(150 * time.Millisecond)
	if err := b.Allow(); err != nil {
		t.Fatalf("probe Allow() = %v, want nil", err)
	}

	// A successful probe closes the breaker again.
	b.MarkSuccess()
	if s := b.State(); s != windcb.StateClosed {
		t.Errorf("State() after successful probe = %v, want StateClosed", s)
	}
	if err := b.Allow(); err != nil {
		t.Errorf("Allow() after recovery = %v, want nil", err)
	}
}

// ---------------------------------------------------------------------------
// Close
// ---------------------------------------------------------------------------

func TestClose_IsNoOp(t *testing.T) {
	b := New("test-cb-close")
	if err := b.Close(); err != nil {
		t.Errorf("Close() = %v, want nil", err)
	}
}

// ---------------------------------------------------------------------------
// Interface compliance
// ---------------------------------------------------------------------------

var _ windcb.CircuitBreaker = (*Breaker)(nil)

// ---------------------------------------------------------------------------
// Concurrent use
// ---------------------------------------------------------------------------

func TestConcurrentExecute(t *testing.T) {
	const res = "test-cb-concurrent"
	// High threshold: most requests pass regardless of scheduling.
	ok, err := scb.LoadRules([]*scb.Rule{{
		Resource:         res,
		Strategy:         scb.ErrorCount,
		Threshold:        1000,
		RetryTimeoutMs:   600000,
		MinRequestAmount: 1000,
		StatIntervalMs:   1000,
	}})
	if err != nil || !ok {
		t.Fatalf("scb.LoadRules() = (%v, %v), want (true, nil)", ok, err)
	}
	t.Cleanup(func() { _ = scb.ClearRulesOfResource(res) })

	b := New(res)

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_ = b.Execute(context.Background(), func() error {
				if i%2 == 0 {
					return nil
				}
				return errors.New("fail")
			})
			_ = b.State()
		}(i)
	}
	wg.Wait()
}
