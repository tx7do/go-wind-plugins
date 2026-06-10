package retry

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// ExponentialBackoff
// ---------------------------------------------------------------------------

func TestExponentialBackoff_Delay(t *testing.T) {
	b := ExponentialBackoff{Initial: 100 * time.Millisecond, Factor: 2, Max: 10 * time.Second}

	tests := []struct {
		attempt int
		want    time.Duration
	}{
		{0, 100 * time.Millisecond},
		{1, 200 * time.Millisecond},
		{2, 400 * time.Millisecond},
		{3, 800 * time.Millisecond},
		{4, 1600 * time.Millisecond},
	}
	for _, tt := range tests {
		got := b.Delay(tt.attempt)
		if got != tt.want {
			t.Errorf("Delay(%d) = %v, want %v", tt.attempt, got, tt.want)
		}
	}
}

func TestExponentialBackoff_MaxCap(t *testing.T) {
	b := ExponentialBackoff{Initial: 1 * time.Second, Factor: 2, Max: 3 * time.Second}
	// attempt=2 → 1*2^2 = 4s → capped to 3s
	if got := b.Delay(2); got != 3*time.Second {
		t.Errorf("Delay(2) = %v, want 3s (capped)", got)
	}
}

func TestFixedBackoff(t *testing.T) {
	b := FixedBackoff(500 * time.Millisecond)
	for i := 0; i < 5; i++ {
		if got := b.Delay(i); got != 500*time.Millisecond {
			t.Errorf("FixedBackoff.Delay(%d) = %v, want 500ms", i, got)
		}
	}
}

func TestLinearBackoff(t *testing.T) {
	b := LinearBackoff{Initial: 100 * time.Millisecond, Step: 100 * time.Millisecond, Max: 1 * time.Second}
	// attempt=0 → 100ms, attempt=1 → 200ms, attempt=2 → 300ms
	if got := b.Delay(0); got != 100*time.Millisecond {
		t.Errorf("Linear.Delay(0) = %v, want 100ms", got)
	}
	if got := b.Delay(2); got != 300*time.Millisecond {
		t.Errorf("Linear.Delay(2) = %v, want 300ms", got)
	}
	// attempt=9 → 100+100*9 = 1000ms → at max
	if got := b.Delay(9); got != 1*time.Second {
		t.Errorf("Linear.Delay(9) = %v, want 1s", got)
	}
	// attempt=10 → would be 1100ms → capped to 1s
	if got := b.Delay(10); got != 1*time.Second {
		t.Errorf("Linear.Delay(10) = %v, want 1s (capped)", got)
	}
}

// ---------------------------------------------------------------------------
// Jitter
// ---------------------------------------------------------------------------

func TestNoJitter(t *testing.T) {
	d := 200 * time.Millisecond
	rng := func() float64 { return 0.5 }
	if got := NoJitter(d, rng); got != d {
		t.Errorf("NoJitter = %v, want %v", got, d)
	}
}

func TestFullJitter(t *testing.T) {
	d := 200 * time.Millisecond
	// rng = 0 → 0ms
	if got := FullJitter(d, func() float64 { return 0 }); got != 0 {
		t.Errorf("FullJitter(rng=0) = %v, want 0", got)
	}
	// rng = 1 → 200ms (but rng is [0,1), so 0.999... ≈ 200ms; test with 0.5)
	if got := FullJitter(d, func() float64 { return 0.5 }); got != 100*time.Millisecond {
		t.Errorf("FullJitter(rng=0.5) = %v, want 100ms", got)
	}
}

func TestEqualJitter(t *testing.T) {
	d := 200 * time.Millisecond
	// half = 100ms; rng=0 → 100ms, rng=0.5 → 150ms, rng=1 (exclusive) → ~200ms
	if got := EqualJitter(d, func() float64 { return 0 }); got != 100*time.Millisecond {
		t.Errorf("EqualJitter(rng=0) = %v, want 100ms", got)
	}
	if got := EqualJitter(d, func() float64 { return 0.5 }); got != 150*time.Millisecond {
		t.Errorf("EqualJitter(rng=0.5) = %v, want 150ms", got)
	}
}

// ---------------------------------------------------------------------------
// Classifiers
// ---------------------------------------------------------------------------

func TestRetryAny(t *testing.T) {
	if !RetryAny(errors.New("x")) {
		t.Error("RetryAny should return true for any error")
	}
}

func TestRetryNever(t *testing.T) {
	if RetryNever(errors.New("x")) {
		t.Error("RetryNever should return false")
	}
}

func TestRetryIf(t *testing.T) {
	sentinel := errors.New("retryable")
	pred := RetryIf(func(err error) bool {
		return errors.Is(err, sentinel)
	})
	if !pred(sentinel) {
		t.Error("RetryIf should return true for sentinel")
	}
	if pred(errors.New("other")) {
		t.Error("RetryIf should return false for non-sentinel")
	}
}

// ---------------------------------------------------------------------------
// Retrier.Do
// ---------------------------------------------------------------------------

func TestDo_SuccessOnFirstAttempt(t *testing.T) {
	r := New(WithMaxAttempts(3))
	calls := 0
	err := r.Do(context.Background(), func(_ context.Context) error {
		calls++
		return nil
	})
	if err != nil {
		t.Fatalf("Do error = %v", err)
	}
	if calls != 1 {
		t.Errorf("calls = %d, want 1", calls)
	}
}

func TestDo_SuccessAfterRetries(t *testing.T) {
	r := New(
		WithMaxAttempts(4),
		WithBackoff(FixedBackoff(1*time.Millisecond)),
	)
	calls := 0
	err := r.Do(context.Background(), func(_ context.Context) error {
		calls++
		if calls < 3 {
			return errors.New("transient")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Do error = %v", err)
	}
	if calls != 3 {
		t.Errorf("calls = %d, want 3", calls)
	}
}

func TestDo_MaxAttemptsExceeded(t *testing.T) {
	r := New(
		WithMaxAttempts(3),
		WithBackoff(FixedBackoff(1*time.Millisecond)),
	)
	calls := 0
	err := r.Do(context.Background(), func(_ context.Context) error {
		calls++
		return errors.New("always fails")
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, ErrMaxAttempts) {
		t.Errorf("err = %v, want ErrMaxAttempts", err)
	}
	if calls != 3 {
		t.Errorf("calls = %d, want 3", calls)
	}
}

func TestDo_ClassifierStopsImmediately(t *testing.T) {
	sentinel := errors.New("non-retryable")
	r := New(
		WithMaxAttempts(5),
		WithBackoff(FixedBackoff(1*time.Millisecond)),
		WithClassifier(RetryIf(func(err error) bool {
			return !errors.Is(err, sentinel)
		})),
	)
	calls := 0
	err := r.Do(context.Background(), func(_ context.Context) error {
		calls++
		return sentinel
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, sentinel) {
		t.Errorf("err = %v, want sentinel", err)
	}
	if calls != 1 {
		t.Errorf("calls = %d, want 1 (should stop immediately)", calls)
	}
}

func TestDo_ContextCanceled(t *testing.T) {
	r := New(
		WithMaxAttempts(10),
		WithBackoff(FixedBackoff(100*time.Millisecond)),
	)
	ctx, cancel := context.WithCancel(context.Background())
	calls := 0

	// Cancel after the first call
	go func() {
		time.Sleep(10 * time.Millisecond)
		cancel()
	}()

	err := r.Do(ctx, func(_ context.Context) error {
		calls++
		return errors.New("fail")
	})
	if err == nil {
		t.Fatal("expected error")
	}
	// Should be cancelled before all 10 attempts
	if calls >= 10 {
		t.Errorf("calls = %d, expected fewer than 10 (cancelled)", calls)
	}
}

func TestDo_MaxTotalWait(t *testing.T) {
	r := New(
		WithMaxAttempts(100),
		WithBackoff(FixedBackoff(50*time.Millisecond)),
		WithMaxTotalWait(100*time.Millisecond),
	)
	start := time.Now()
	err := r.Do(context.Background(), func(_ context.Context) error {
		return errors.New("always fails")
	})
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, ErrTimeout) {
		t.Errorf("err = %v, want ErrTimeout", err)
	}
	// Should have stopped well before 100 attempts * 50ms = 5s
	if elapsed > 500*time.Millisecond {
		t.Errorf("elapsed = %v, expected < 500ms", elapsed)
	}
}

func TestDo_NilFunction(t *testing.T) {
	r := New(WithMaxAttempts(2))
	// fn that returns nil immediately
	err := r.Do(context.Background(), func(_ context.Context) error {
		return nil
	})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestWithMaxAttempts_LessThanOne(t *testing.T) {
	r := New(WithMaxAttempts(0))
	// Should default to 3 (0 is ignored)
	if r.maxAttempts != 3 {
		t.Errorf("maxAttempts = %d, want 3 (default)", r.maxAttempts)
	}
}

// ---------------------------------------------------------------------------
// Retrier with Jitter integration
// ---------------------------------------------------------------------------

func TestDo_WithJitter(t *testing.T) {
	r := New(
		WithMaxAttempts(3),
		WithBackoff(ExponentialBackoff{Initial: 10 * time.Millisecond, Factor: 2, Max: 1 * time.Second}),
		WithJitter(FullJitter),
		WithRNG(rand.New(rand.NewSource(42))), // deterministic
	)
	calls := 0
	err := r.Do(context.Background(), func(_ context.Context) error {
		calls++
		if calls < 3 {
			return errors.New("retry")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Do error = %v", err)
	}
	if calls != 3 {
		t.Errorf("calls = %d, want 3", calls)
	}
}

// ---------------------------------------------------------------------------
// WithRNG option
// ---------------------------------------------------------------------------

func TestWithRNG(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	r := New(WithRNG(rng))
	if r.rng == nil {
		t.Error("WithRNG should set r.rng")
	}
	// Should produce deterministic values
	v1 := r.random()
	r2 := New(WithRNG(rand.New(rand.NewSource(42))))
	v2 := r2.random()
	if v1 != v2 {
		t.Errorf("deterministic RNG mismatch: %v vs %v", v1, v2)
	}
}

// ---------------------------------------------------------------------------
// Example end-to-end
// ---------------------------------------------------------------------------

func TestExample_RetryWithBackoff(t *testing.T) {
	var attempts int
	r := New(
		WithMaxAttempts(5),
		WithBackoff(ExponentialBackoff{
			Initial: 1 * time.Millisecond,
			Factor:  2,
			Max:     10 * time.Millisecond,
		}),
		WithJitter(EqualJitter),
		WithRNG(rand.New(rand.NewSource(1))),
	)

	op := func(_ context.Context) error {
		attempts++
		if attempts < 4 {
			return fmt.Errorf("attempt %d failed", attempts)
		}
		return nil
	}

	if err := r.Do(context.Background(), op); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if attempts != 4 {
		t.Errorf("attempts = %d, want 4", attempts)
	}
}
