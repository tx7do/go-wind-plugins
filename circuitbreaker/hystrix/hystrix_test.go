package hystrix

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

	if b.cfg.errorThreshold != defaultErrorThreshold {
		t.Errorf("errorThreshold = %v, want %v", b.cfg.errorThreshold, defaultErrorThreshold)
	}
	if b.cfg.requestVolumeThreshold != defaultRequestVolumeThreshold {
		t.Errorf("requestVolumeThreshold = %d, want %d", b.cfg.requestVolumeThreshold, defaultRequestVolumeThreshold)
	}
	if b.cfg.sleepWindow != defaultSleepWindow {
		t.Errorf("sleepWindow = %v, want %v", b.cfg.sleepWindow, defaultSleepWindow)
	}
	if b.cfg.window != defaultWindow {
		t.Errorf("window = %v, want %v", b.cfg.window, defaultWindow)
	}
	if b.cfg.bucketCount != defaultBucketCount {
		t.Errorf("bucketCount = %d, want %d", b.cfg.bucketCount, defaultBucketCount)
	}
	if len(b.buckets) != defaultBucketCount {
		t.Errorf("len(buckets) = %d, want %d", len(b.buckets), defaultBucketCount)
	}
	if b.bucketDuration != defaultWindow/defaultBucketCount {
		t.Errorf("bucketDuration = %v, want %v", b.bucketDuration, defaultWindow/defaultBucketCount)
	}
}

func TestOptions(t *testing.T) {
	tests := []struct {
		name  string
		opt   Option
		check func(*testing.T, *Breaker)
	}{
		{
			name: "WithErrorThreshold valid",
			opt:  WithErrorThreshold(0.3),
			check: func(t *testing.T, b *Breaker) {
				if b.cfg.errorThreshold != 0.3 {
					t.Errorf("errorThreshold = %v, want 0.3", b.cfg.errorThreshold)
				}
			},
		},
		{
			name: "WithErrorThreshold 1.0 accepted",
			opt:  WithErrorThreshold(1.0),
			check: func(t *testing.T, b *Breaker) {
				if b.cfg.errorThreshold != 1.0 {
					t.Errorf("errorThreshold = %v, want 1.0", b.cfg.errorThreshold)
				}
			},
		},
		{
			name: "WithErrorThreshold zero ignored",
			opt:  WithErrorThreshold(0),
			check: func(t *testing.T, b *Breaker) {
				if b.cfg.errorThreshold != defaultErrorThreshold {
					t.Errorf("errorThreshold = %v, want default %v", b.cfg.errorThreshold, defaultErrorThreshold)
				}
			},
		},
		{
			name: "WithErrorThreshold above 1 ignored",
			opt:  WithErrorThreshold(1.5),
			check: func(t *testing.T, b *Breaker) {
				if b.cfg.errorThreshold != defaultErrorThreshold {
					t.Errorf("errorThreshold = %v, want default %v", b.cfg.errorThreshold, defaultErrorThreshold)
				}
			},
		},
		{
			name: "WithRequestVolumeThreshold valid",
			opt:  WithRequestVolumeThreshold(5),
			check: func(t *testing.T, b *Breaker) {
				if b.cfg.requestVolumeThreshold != 5 {
					t.Errorf("requestVolumeThreshold = %d, want 5", b.cfg.requestVolumeThreshold)
				}
			},
		},
		{
			name: "WithRequestVolumeThreshold zero ignored",
			opt:  WithRequestVolumeThreshold(0),
			check: func(t *testing.T, b *Breaker) {
				if b.cfg.requestVolumeThreshold != defaultRequestVolumeThreshold {
					t.Errorf("requestVolumeThreshold = %d, want default %d", b.cfg.requestVolumeThreshold, defaultRequestVolumeThreshold)
				}
			},
		},
		{
			name: "WithSleepWindow valid",
			opt:  WithSleepWindow(200 * time.Millisecond),
			check: func(t *testing.T, b *Breaker) {
				if b.cfg.sleepWindow != 200*time.Millisecond {
					t.Errorf("sleepWindow = %v, want 200ms", b.cfg.sleepWindow)
				}
			},
		},
		{
			name: "WithSleepWindow negative ignored",
			opt:  WithSleepWindow(-1),
			check: func(t *testing.T, b *Breaker) {
				if b.cfg.sleepWindow != defaultSleepWindow {
					t.Errorf("sleepWindow = %v, want default %v", b.cfg.sleepWindow, defaultSleepWindow)
				}
			},
		},
		{
			name: "WithWindow valid",
			opt:  WithWindow(2 * time.Second),
			check: func(t *testing.T, b *Breaker) {
				if b.cfg.window != 2*time.Second {
					t.Errorf("window = %v, want 2s", b.cfg.window)
				}
				if b.bucketDuration != 2*time.Second/time.Duration(defaultBucketCount) {
					t.Errorf("bucketDuration = %v, want %v", b.bucketDuration, 2*time.Second/time.Duration(defaultBucketCount))
				}
			},
		},
		{
			name: "WithBucketCount valid",
			opt:  WithBucketCount(4),
			check: func(t *testing.T, b *Breaker) {
				if b.cfg.bucketCount != 4 {
					t.Errorf("bucketCount = %d, want 4", b.cfg.bucketCount)
				}
				if len(b.buckets) != 4 {
					t.Errorf("len(buckets) = %d, want 4", len(b.buckets))
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := New(tt.opt)
			defer b.Close()
			tt.check(t, b)
		})
	}
}

// ---------------------------------------------------------------------------
// Initially closed
// ---------------------------------------------------------------------------

func TestAllow_InitiallyClosed(t *testing.T) {
	b := New()
	defer b.Close()

	if err := b.Allow(); err != nil {
		t.Errorf("Allow() = %v, want nil", err)
	}
	if s := b.State(); s != circuitbreaker.StateClosed {
		t.Errorf("State() = %v, want StateClosed", s)
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
// Execute
// ---------------------------------------------------------------------------

func TestExecute_Success(t *testing.T) {
	b := New()
	defer b.Close()

	err := b.Execute(context.Background(), func() error { return nil })
	if err != nil {
		t.Errorf("Execute() = %v, want nil", err)
	}
}

func TestExecute_PropagatesError(t *testing.T) {
	b := New()
	defer b.Close()

	wantErr := errors.New("boom")
	err := b.Execute(context.Background(), func() error { return wantErr })
	if !errors.Is(err, wantErr) {
		t.Errorf("Execute() = %v, want %v", err, wantErr)
	}
}

func TestExecute_RejectedDoesNotCallFn(t *testing.T) {
	b := New(WithErrorThreshold(0.5), WithRequestVolumeThreshold(2))
	defer b.Close()

	// Trip the breaker.
	_ = b.Execute(context.Background(), func() error { return errors.New("fail") })
	_ = b.Execute(context.Background(), func() error { return errors.New("fail") })

	called := false
	err := b.Execute(context.Background(), func() error {
		called = true
		return nil
	})
	if !errors.Is(err, circuitbreaker.ErrCircuitOpen) {
		t.Errorf("Execute() on open circuit = %v, want ErrCircuitOpen", err)
	}
	if called {
		t.Error("fn should not be called when the circuit is open")
	}
}

// ---------------------------------------------------------------------------
// Tripping on error rate
// ---------------------------------------------------------------------------

func TestTripsOpen_WhenErrorRateExceeded(t *testing.T) {
	b := New(WithErrorThreshold(0.5), WithRequestVolumeThreshold(4))
	defer b.Close()

	for i := 0; i < 4; i++ {
		if err := b.Allow(); err != nil {
			t.Fatalf("Allow() #%d = %v, want nil", i, err)
		}
		b.MarkFailure()
	}

	if s := b.State(); s != circuitbreaker.StateOpen {
		t.Errorf("State() after 100%% error rate = %v, want StateOpen", s)
	}
	if err := b.Allow(); !errors.Is(err, circuitbreaker.ErrCircuitOpen) {
		t.Errorf("Allow() on open circuit = %v, want ErrCircuitOpen", err)
	}
}

func TestTripsOpen_AtExactThreshold(t *testing.T) {
	b := New(WithErrorThreshold(0.5), WithRequestVolumeThreshold(4))
	defer b.Close()

	// 2 failures out of 4 → exactly 0.5 → trips.
	for i := 0; i < 2; i++ {
		_ = b.Allow()
		b.MarkFailure()
	}
	for i := 0; i < 2; i++ {
		_ = b.Allow()
		b.MarkSuccess()
	}

	if s := b.State(); s != circuitbreaker.StateOpen {
		t.Errorf("State() at exact 0.5 error rate = %v, want StateOpen", s)
	}
}

func TestBelowVolumeThreshold_DoesNotTrip(t *testing.T) {
	b := New(WithErrorThreshold(0.5), WithRequestVolumeThreshold(10))
	defer b.Close()

	// 4 failures of 4 is a 100% error rate, but below the volume threshold.
	for i := 0; i < 4; i++ {
		_ = b.Allow()
		b.MarkFailure()
	}

	if s := b.State(); s != circuitbreaker.StateClosed {
		t.Errorf("State() below volume threshold = %v, want StateClosed", s)
	}
}

func TestHealthyTraffic_DoesNotTrip(t *testing.T) {
	b := New(WithErrorThreshold(0.5), WithRequestVolumeThreshold(10))
	defer b.Close()

	// 10 successes and 1 failure → ~9% error rate.
	for i := 0; i < 10; i++ {
		_ = b.Allow()
		b.MarkSuccess()
	}
	_ = b.Allow()
	b.MarkFailure()

	if s := b.State(); s != circuitbreaker.StateClosed {
		t.Errorf("State() on healthy traffic = %v, want StateClosed", s)
	}
}

// ---------------------------------------------------------------------------
// Open → HalfOpen recovery (via State(), then a trial Allow)
// ---------------------------------------------------------------------------

func TestHalfOpen_TrialSucceedsAndRecovers(t *testing.T) {
	b := New(WithErrorThreshold(0.5), WithRequestVolumeThreshold(2), WithSleepWindow(50*time.Millisecond))
	defer b.Close()

	// Trip the breaker.
	for i := 0; i < 2; i++ {
		_ = b.Allow()
		b.MarkFailure()
	}
	if s := b.State(); s != circuitbreaker.StateOpen {
		t.Fatalf("State() before sleep window = %v, want StateOpen", s)
	}

	// Within the sleep window the circuit stays open.
	if err := b.Allow(); !errors.Is(err, circuitbreaker.ErrCircuitOpen) {
		t.Errorf("Allow() within sleep window = %v, want ErrCircuitOpen", err)
	}

	time.Sleep(100 * time.Millisecond)

	// State() performs the lazy Open → HalfOpen transition.
	if s := b.State(); s != circuitbreaker.StateHalfOpen {
		t.Fatalf("State() after sleep window = %v, want StateHalfOpen", s)
	}

	// A single trial request is admitted.
	if err := b.Allow(); err != nil {
		t.Errorf("trial Allow() = %v, want nil", err)
	}
	// Additional requests are rejected while the trial is in flight.
	if err := b.Allow(); !errors.Is(err, circuitbreaker.ErrCircuitOpen) {
		t.Errorf("second Allow() during trial = %v, want ErrCircuitOpen", err)
	}

	// Trial succeeded → recover to closed.
	b.MarkSuccess()
	if s := b.State(); s != circuitbreaker.StateClosed {
		t.Errorf("State() after successful trial = %v, want StateClosed", s)
	}

	// After recovery requests flow again.
	if err := b.Allow(); err != nil {
		t.Errorf("Allow() after recovery = %v, want nil", err)
	}
}

func TestHalfOpen_TrialFailureReopens(t *testing.T) {
	b := New(WithErrorThreshold(0.5), WithRequestVolumeThreshold(2), WithSleepWindow(50*time.Millisecond))
	defer b.Close()

	// Trip the breaker.
	for i := 0; i < 2; i++ {
		_ = b.Allow()
		b.MarkFailure()
	}

	time.Sleep(100 * time.Millisecond)

	// Transition to half-open and admit a trial.
	_ = b.State()
	if err := b.Allow(); err != nil {
		t.Fatalf("trial Allow() = %v, want nil", err)
	}

	// Trial failed → back to open.
	b.MarkFailure()
	if s := b.State(); s != circuitbreaker.StateOpen {
		t.Fatalf("State() after failed trial = %v, want StateOpen", s)
	}

	// The sleep window restarts, so an immediate Allow is rejected.
	if err := b.Allow(); !errors.Is(err, circuitbreaker.ErrCircuitOpen) {
		t.Errorf("Allow() immediately after re-open = %v, want ErrCircuitOpen", err)
	}
}

// ---------------------------------------------------------------------------
// Close
// ---------------------------------------------------------------------------

func TestClose_ReturnsNil(t *testing.T) {
	b := New()
	if err := b.Close(); err != nil {
		t.Errorf("Close() = %v, want nil", err)
	}
}

// ---------------------------------------------------------------------------
// Interface compliance
// ---------------------------------------------------------------------------

var _ circuitbreaker.CircuitBreaker = (*Breaker)(nil)

// ---------------------------------------------------------------------------
// Concurrent use
// ---------------------------------------------------------------------------

func TestConcurrentExecute(t *testing.T) {
	b := New(WithErrorThreshold(0.9), WithRequestVolumeThreshold(1000))
	defer b.Close()

	done := make(chan struct{}, 50)
	for i := 0; i < 50; i++ {
		go func(i int) {
			defer func() { done <- struct{}{} }()
			_ = b.Execute(context.Background(), func() error {
				if i%2 == 0 {
					return nil
				}
				return errors.New("fail")
			})
			_ = b.State()
		}(i)
	}
	for i := 0; i < 50; i++ {
		<-done
	}
}
