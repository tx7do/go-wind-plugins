package sres

import (
	"context"
	"errors"
	"testing"

	"github.com/tx7do/go-wind-plugins/circuitbreaker"
)

// ---------------------------------------------------------------------------
// New / options
// ---------------------------------------------------------------------------

func TestNew_DefaultConfig(t *testing.T) {
	b := New()
	defer b.Close()
	if b == nil {
		t.Fatal("New() returned nil")
	}
	if b.cfg.k != defaultK {
		t.Errorf("default K = %v, want %v", b.cfg.k, defaultK)
	}
	if b.cfg.window != defaultWindow {
		t.Errorf("default window = %v, want %v", b.cfg.window, defaultWindow)
	}
}

func TestNew_WithK(t *testing.T) {
	b := New(WithK(3.0))
	defer b.Close()
	if b.cfg.k != 3.0 {
		t.Errorf("K = %v, want 3.0", b.cfg.k)
	}
}

func TestNew_WithK_InvalidIgnored(t *testing.T) {
	b := New(WithK(-1))
	defer b.Close()
	if b.cfg.k != defaultK {
		t.Errorf("invalid K should be ignored, got %v, want %v", b.cfg.k, defaultK)
	}
}

func TestNew_WithWindow(t *testing.T) {
	d := defaultWindow * 2
	b := New(WithWindow(d))
	defer b.Close()
	if b.cfg.window != d {
		t.Errorf("window = %v, want %v", b.cfg.window, d)
	}
}

func TestNew_WithBucketCount(t *testing.T) {
	b := New(WithBucketCount(20))
	defer b.Close()
	if b.cfg.bucketCount != 20 {
		t.Errorf("bucketCount = %v, want 20", b.cfg.bucketCount)
	}
	if len(b.buckets) != 20 {
		t.Errorf("len(buckets) = %d, want 20", len(b.buckets))
	}
}

// ---------------------------------------------------------------------------
// Allow — initially closed (allows all)
// ---------------------------------------------------------------------------

func TestAllow_InitiallyAllows(t *testing.T) {
	b := New()
	defer b.Close()
	if err := b.Allow(); err != nil {
		t.Errorf("Allow() = %v, want nil on a fresh breaker", err)
	}
}

// ---------------------------------------------------------------------------
// Close
// ---------------------------------------------------------------------------

func TestClose_RejectsAfterClose(t *testing.T) {
	b := New()
	_ = b.Close()
	if err := b.Allow(); !errors.Is(err, circuitbreaker.ErrCircuitOpen) {
		t.Errorf("Allow() after Close = %v, want ErrCircuitOpen", err)
	}
}

// ---------------------------------------------------------------------------
// Execute — success path
// ---------------------------------------------------------------------------

func TestExecute_Success(t *testing.T) {
	b := New()
	defer b.Close()

	called := false
	err := b.Execute(context.Background(), func() error {
		called = true
		return nil
	})
	if err != nil {
		t.Errorf("Execute() = %v, want nil", err)
	}
	if !called {
		t.Error("fn was not called")
	}
}

func TestExecute_FailureRecorded(t *testing.T) {
	b := New()
	defer b.Close()

	wantErr := errors.New("downstream error")
	err := b.Execute(context.Background(), func() error {
		return wantErr
	})
	if !errors.Is(err, wantErr) {
		t.Errorf("Execute() = %v, want %v", err, wantErr)
	}
}

// ---------------------------------------------------------------------------
// Execute — high failure rate triggers throttling
// ---------------------------------------------------------------------------

func TestExecute_HighFailureRateThrottles(t *testing.T) {
	// Use K=1 so the breaker is very aggressive (trips when error rate ~100%)
	b := New(WithK(1.0), WithWindow(60*1e9), WithBucketCount(10))
	defer b.Close()

	totalReq := 200
	rejected := 0

	for i := 0; i < totalReq; i++ {
		err := b.Execute(context.Background(), func() error {
			return errors.New("always fails")
		})
		if errors.Is(err, circuitbreaker.ErrCircuitOpen) {
			rejected++
		}
	}

	if rejected == 0 {
		t.Error("expected some requests to be rejected due to high failure rate, got 0 rejections")
	}
	t.Logf("rejected %d/%d requests under sustained failures", rejected, totalReq)
}

func TestExecute_AllSuccessNotThrottled(t *testing.T) {
	b := New()
	defer b.Close()

	// The SRE breaker is probabilistic: accept = requests / (requests+1).
	// With all successes, accept approaches 1 but never reaches it,
	// so a tiny fraction of requests may still be rejected.
	// We verify that the vast majority (>90%) are allowed.
	total := 200
	allowed := 0
	for i := 0; i < total; i++ {
		err := b.Execute(context.Background(), func() error { return nil })
		if err == nil {
			allowed++
		}
	}
	if allowed < total*9/10 {
		t.Errorf("allowed %d/%d — expected at least 90%%", allowed, total)
	}
	t.Logf("allowed %d/%d under all-success", allowed, total)
}

// ---------------------------------------------------------------------------
// State
// ---------------------------------------------------------------------------

func TestState_NoDataIsClosed(t *testing.T) {
	b := New()
	defer b.Close()
	if s := b.State(); s != circuitbreaker.StateClosed {
		t.Errorf("State() = %v, want StateClosed", s)
	}
}

func TestState_AllFailuresBecomesOpen(t *testing.T) {
	b := New(WithK(1.0))
	defer b.Close()

	// Generate enough failures.
	for i := 0; i < 50; i++ {
		_ = b.Execute(context.Background(), func() error { return errors.New("fail") })
	}

	s := b.State()
	if s != circuitbreaker.StateOpen {
		t.Errorf("State() = %v, want StateOpen after sustained failures", s)
	}
}

// ---------------------------------------------------------------------------
// MarkSuccess / MarkFailure directly
// ---------------------------------------------------------------------------

func TestMarkSuccessAndFailure(t *testing.T) {
	b := New()
	defer b.Close()

	// Call Allow + MarkSuccess repeatedly. Allow may reject probabilistically,
	// so we only MarkSuccess when Allow returns nil.
	for i := 0; i < 10; i++ {
		if err := b.Allow(); err == nil {
			b.MarkSuccess()
		}
	}

	// All-success should never produce StateOpen.
	if s := b.State(); s == circuitbreaker.StateOpen {
		t.Errorf("State() = StateOpen after all successes — should not be open")
	}
}

// ---------------------------------------------------------------------------
// Concurrent access
// ---------------------------------------------------------------------------

func testConcurrentExecute(t *testing.T) {
	t.Helper()
	b := New()
	defer b.Close()

	ctx := context.Background()
	errs := make(chan error, 100)
	for i := 0; i < 100; i++ {
		go func() {
			errs <- b.Execute(ctx, func() error { return nil })
		}()
	}
	for i := 0; i < 100; i++ {
		if err := <-errs; err != nil {
			t.Errorf("concurrent Execute() = %v, want nil", err)
		}
	}
}
