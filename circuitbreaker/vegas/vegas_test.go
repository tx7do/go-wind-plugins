package vegas

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/tx7do/go-wind-plugins/circuitbreaker"
)

// ---------------------------------------------------------------------------
// New / options
// ---------------------------------------------------------------------------

func TestNew_Defaults(t *testing.T) {
	b := New()
	defer b.Close()
	if b.cfg.alpha != defaultAlpha {
		t.Errorf("alpha = %v, want %v", b.cfg.alpha, defaultAlpha)
	}
	if b.cfg.beta != defaultBeta {
		t.Errorf("beta = %v, want %v", b.cfg.beta, defaultBeta)
	}
	if b.cfg.warmupSamples != defaultWarmupSamples {
		t.Errorf("warmupSamples = %v, want %v", b.cfg.warmupSamples, defaultWarmupSamples)
	}
}

func TestNew_WithAlpha(t *testing.T) {
	b := New(WithAlpha(0.8))
	defer b.Close()
	if b.cfg.alpha != 0.8 {
		t.Errorf("alpha = %v, want 0.8", b.cfg.alpha)
	}
}

func TestNew_WithAlpha_InvalidIgnored(t *testing.T) {
	b := New(WithAlpha(-1))
	defer b.Close()
	if b.cfg.alpha != defaultAlpha {
		t.Errorf("invalid alpha should be ignored, got %v, want %v", b.cfg.alpha, defaultAlpha)
	}
}

func TestNew_WithBeta(t *testing.T) {
	b := New(WithBeta(0.2))
	defer b.Close()
	if b.cfg.beta != 0.2 {
		t.Errorf("beta = %v, want 0.2", b.cfg.beta)
	}
}

func TestNew_WithWarmupSamples(t *testing.T) {
	b := New(WithWarmupSamples(5))
	defer b.Close()
	if b.cfg.warmupSamples != 5 {
		t.Errorf("warmupSamples = %v, want 5", b.cfg.warmupSamples)
	}
}

// ---------------------------------------------------------------------------
// Allow — initially closed
// ---------------------------------------------------------------------------

func TestAllow_InitiallyClosed(t *testing.T) {
	b := New()
	defer b.Close()
	if err := b.Allow(); err != nil {
		t.Errorf("Allow() = %v, want nil", err)
	}
}

func TestAllow_AfterCloseRejected(t *testing.T) {
	b := New()
	_ = b.Close()
	if err := b.Allow(); !errors.Is(err, circuitbreaker.ErrCircuitOpen) {
		t.Errorf("Allow() after Close = %v, want ErrCircuitOpen", err)
	}
}

// ---------------------------------------------------------------------------
// State
// ---------------------------------------------------------------------------

func TestState_InitiallyClosed(t *testing.T) {
	b := New()
	defer b.Close()
	if s := b.State(); s != circuitbreaker.StateClosed {
		t.Errorf("State() = %v, want StateClosed", s)
	}
}

// ---------------------------------------------------------------------------
// RecordLatency — baseline establishment
// ---------------------------------------------------------------------------

func TestRecordLatency_EstablishesBaseline(t *testing.T) {
	b := New(WithWarmupSamples(3))
	defer b.Close()

	b.RecordLatency(10 * time.Millisecond)
	b.RecordLatency(10 * time.Millisecond)
	b.RecordLatency(10 * time.Millisecond)

	if b.BaseRTT() != 10*time.Millisecond {
		t.Errorf("BaseRTT = %v, want 10ms", b.BaseRTT())
	}
}

func TestRecordLatency_OutlierFiltered(t *testing.T) {
	b := New()
	defer b.Close()

	// minRTT is 1ms, so anything below should be filtered.
	b.RecordLatency(0) // below minRTT — filtered
	b.RecordLatency(5 * time.Millisecond)

	if b.BaseRTT() != 5*time.Millisecond {
		t.Errorf("BaseRTT = %v, want 5ms (0 should be filtered)", b.BaseRTT())
	}
}

// ---------------------------------------------------------------------------
// Latency degradation → state transitions
// ---------------------------------------------------------------------------

func TestLatencyInflation_OpensCircuit(t *testing.T) {
	b := New(
		WithAlpha(0.5),
		WithBeta(0.2),
		WithWarmupSamples(5),
	)
	defer b.Close()

	// Establish baseline at 10ms
	for i := 0; i < 10; i++ {
		b.RecordLatency(10 * time.Millisecond)
	}
	if s := b.State(); s != circuitbreaker.StateClosed {
		t.Fatalf("expected StateClosed after baseline, got %v", s)
	}

	// Inject high latency to trigger degradation (>50% inflation)
	for i := 0; i < 20; i++ {
		b.RecordLatency(50 * time.Millisecond) // 400% inflation
	}

	if s := b.State(); s != circuitbreaker.StateOpen {
		t.Errorf("expected StateOpen after latency inflation, got %v (inflation=%.2f)", s, b.Inflation())
	}
}

func TestLatencyRecovery_ClosesCircuit(t *testing.T) {
	b := New(
		WithAlpha(0.5),
		WithBeta(0.2),
		WithWarmupSamples(5),
	)
	defer b.Close()

	// Establish baseline
	for i := 0; i < 10; i++ {
		b.RecordLatency(10 * time.Millisecond)
	}

	// Degrade to open
	for i := 0; i < 30; i++ {
		b.RecordLatency(50 * time.Millisecond)
	}
	if b.State() != circuitbreaker.StateOpen {
		t.Fatalf("expected StateOpen before recovery, got %v", b.State())
	}

	// Recover back to low latency
	for i := 0; i < 30; i++ {
		b.RecordLatency(10 * time.Millisecond)
	}

	s := b.State()
	if s != circuitbreaker.StateClosed && s != circuitbreaker.StateHalfOpen {
		t.Errorf("expected StateClosed or StateHalfOpen after recovery, got %v", s)
	}
}

// ---------------------------------------------------------------------------
// MarkFailure — treats as high latency
// ---------------------------------------------------------------------------

func TestMarkFailure_DegradesToOpen(t *testing.T) {
	b := New(WithAlpha(0.5), WithWarmupSamples(3))
	defer b.Close()

	// Establish baseline
	for i := 0; i < 5; i++ {
		b.RecordLatency(5 * time.Millisecond)
	}

	// Repeated failures inject maxRTT
	for i := 0; i < 30; i++ {
		b.MarkFailure()
	}

	if s := b.State(); s != circuitbreaker.StateOpen {
		t.Errorf("expected StateOpen after repeated failures, got %v", s)
	}
}

// ---------------------------------------------------------------------------
// Execute — success records latency
// ---------------------------------------------------------------------------

func TestExecute_Success(t *testing.T) {
	b := New()
	defer b.Close()

	err := b.Execute(context.Background(), func() error {
		time.Sleep(time.Millisecond)
		return nil
	})
	if err != nil {
		t.Errorf("Execute() = %v, want nil", err)
	}
	if b.CurrentRTT() == 0 {
		t.Error("expected CurrentRTT to be set after Execute")
	}
}

func TestExecute_Failure(t *testing.T) {
	b := New()
	defer b.Close()

	wantErr := errors.New("boom")
	err := b.Execute(context.Background(), func() error {
		return wantErr
	})
	if !errors.Is(err, wantErr) {
		t.Errorf("Execute() = %v, want %v", err, wantErr)
	}
}

// ---------------------------------------------------------------------------
// Inflation — observability
// ---------------------------------------------------------------------------

func TestInflation_ZeroBeforeBaseline(t *testing.T) {
	b := New()
	defer b.Close()
	if inf := b.Inflation(); inf != 0 {
		t.Errorf("Inflation() before baseline = %v, want 0", inf)
	}
}

func TestInflation_PositiveAfterDegradation(t *testing.T) {
	b := New(WithWarmupSamples(3))
	defer b.Close()

	b.RecordLatency(10 * time.Millisecond)
	b.RecordLatency(10 * time.Millisecond)
	b.RecordLatency(10 * time.Millisecond)
	b.RecordLatency(30 * time.Millisecond)
	b.RecordLatency(30 * time.Millisecond)

	if inf := b.Inflation(); inf <= 0 {
		t.Errorf("Inflation() after degradation = %v, want > 0", inf)
	}
}

// ---------------------------------------------------------------------------
// Concurrent access
// ---------------------------------------------------------------------------

func TestConcurrentRecordLatency(t *testing.T) {
	b := New(WithWarmupSamples(1))
	defer b.Close()

	done := make(chan struct{}, 50)
	for i := 0; i < 50; i++ {
		go func() {
			b.RecordLatency(5 * time.Millisecond)
			_ = b.State()
			_ = b.Inflation()
			done <- struct{}{}
		}()
	}
	for i := 0; i < 50; i++ {
		<-done
	}
}
