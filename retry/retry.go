// Package retry provides a composable retry mechanism for transient failures.
//
// It implements configurable retry with:
//   - Exponential backoff with optional jitter (full or equal jitter).
//   - Maximum retry count and total timeout.
//   - Custom retry predicates (decide which errors are retryable).
//   - Context-aware cancellation.
//
// The design is transport-agnostic — it works with any function that returns
// an error, making it composable with circuit breakers, rate limiters, and
// any I/O operation.
//
// Example:
//
//	r := retry.New(
//	    retry.WithMaxAttempts(5),
//	    retry.WithBackoff(retry.ExponentialBackoff{
//	        Initial: 100 * time.Millisecond,
//	        Max:     5 * time.Second,
//	    }),
//	    retry.WithJitter(retry.FullJitter),
//	)
//
//	err := r.Do(ctx, func(ctx context.Context) error {
//	    return httpClient.Do(req)
//	})
package retry

import (
	"context"
	"errors"
	"math/rand"
	"time"
)

// ErrMaxAttempts indicates the retry exhausted all attempts.
var ErrMaxAttempts = errors.New("retry: max attempts exceeded")

// ErrTimeout indicates the retry exceeded the total timeout.
var ErrTimeout = errors.New("retry: total timeout exceeded")

// Retrier encapsulates retry configuration and executes operations with
// automatic retry on failure.
type Retrier struct {
	maxAttempts  int
	backoff      Backoff
	jitter       Jitter
	classifier   Classifier
	maxTotalWait time.Duration
	timer        func() time.Time // for testing
	rng          *rand.Rand
	rngCh        chan struct{}
}

// Option configures the [Retrier].
type Option func(*Retrier)

// WithMaxAttempts sets the maximum number of attempts (including the first).
// Default: 3. Must be >= 1.
func WithMaxAttempts(n int) Option {
	return func(r *Retrier) {
		if n >= 1 {
			r.maxAttempts = n
		}
	}
}

// WithBackoff sets the backoff strategy.
// Default: [ExponentialBackoff] with Initial=200ms, Factor=2, Max=10s.
func WithBackoff(b Backoff) Option {
	return func(r *Retrier) { r.backoff = b }
}

// WithJitter sets the jitter strategy to randomise backoff intervals and
// prevent thundering-herd effects.
// Default: [NoJitter].
func WithJitter(j Jitter) Option {
	return func(r *Retrier) { r.jitter = j }
}

// WithClassifier sets the error classifier that determines which errors are
// retryable. Only errors for which the classifier returns true are retried;
// all others cause immediate failure.
// Default: [RetryAny] (retry on any non-nil error).
func WithClassifier(c Classifier) Option {
	return func(r *Retrier) { r.classifier = c }
}

// WithMaxTotalWait sets the maximum total wall-clock duration across all
// retry attempts (including backoff sleeps). Zero means no limit.
// Default: 0 (no limit).
func WithMaxTotalWait(d time.Duration) Option {
	return func(r *Retrier) { r.maxTotalWait = d }
}

// WithRNG sets a deterministic random source (useful for testing). When set,
// jitter calculations use this source instead of the global rand.
func WithRNG(rng *rand.Rand) Option {
	return func(r *Retrier) { r.rng = rng }
}

// New creates a [Retrier] with the given options.
func New(opts ...Option) *Retrier {
	r := &Retrier{
		maxAttempts: 3,
		backoff: ExponentialBackoff{
			Initial: 200 * time.Millisecond,
			Factor:  2,
			Max:     10 * time.Second,
		},
		jitter:     NoJitter,
		classifier: RetryAny,
	}
	for _, opt := range opts {
		opt(r)
	}
	return r
}

// Do executes the given function with retry logic.
//
// It calls fn up to [Retrier.maxAttempts] times. Between attempts it sleeps
// for the backoff duration (with jitter applied). If ctx is cancelled or
// the total wait exceeds maxTotalWait, it returns the last error wrapped
// with [ErrTimeout] or the context error.
func (r *Retrier) Do(ctx context.Context, fn func(ctx context.Context) error) error {
	start := r.now()

	var lastErr error
	for attempt := 0; attempt < r.maxAttempts; attempt++ {
		// Check context before each attempt.
		if err := ctx.Err(); err != nil {
			return err
		}

		lastErr = fn(ctx)
		if lastErr == nil {
			return nil
		}

		// If the error is not retryable, return immediately.
		if !r.classifier(lastErr) {
			return lastErr
		}

		// Don't sleep after the last attempt.
		if attempt == r.maxAttempts-1 {
			break
		}

		// Calculate backoff.
		wait := r.backoff.Delay(attempt)
		wait = r.jitter(wait, r.random)

		// Check total timeout.
		if r.maxTotalWait > 0 {
			elapsed := r.now().Sub(start)
			remaining := r.maxTotalWait - elapsed
			if remaining <= 0 {
				return errors.Join(ErrTimeout, lastErr)
			}
			if wait > remaining {
				wait = remaining
			}
		}

		// Sleep with context cancellation.
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(wait):
		}
	}

	return errors.Join(ErrMaxAttempts, lastErr)
}

// now returns the current time (overridable for testing).
func (r *Retrier) now() time.Time {
	if r.timer != nil {
		return r.timer()
	}
	return time.Now()
}

// random returns a random float64 in [0, 1) (thread-safe).
func (r *Retrier) random() float64 {
	if r.rng != nil {
		// Deterministic mode for testing.
		return r.rng.Float64()
	}
	return rand.Float64()
}

// --- Backoff strategies ---

// Backoff computes the delay before the next attempt, given the current
// attempt index (0-based).
type Backoff interface {
	Delay(attempt int) time.Duration
}

// ExponentialBackoff doubles the delay after each failure.
//
//   - Initial is the delay before the 2nd attempt (attempt=0).
//   - Factor is the multiplier (typically 2.0).
//   - Max caps the delay to prevent unbounded growth.
//   - The formula is: Initial * Factor^attempt, capped at Max.
type ExponentialBackoff struct {
	Initial time.Duration
	Factor  float64
	Max     time.Duration
}

// Delay implements [Backoff].
func (b ExponentialBackoff) Delay(attempt int) time.Duration {
	d := float64(b.Initial)
	for i := 0; i < attempt; i++ {
		d *= b.Factor
		if b.Max > 0 && time.Duration(d) > b.Max {
			return b.Max
		}
	}
	if b.Max > 0 && time.Duration(d) > b.Max {
		return b.Max
	}
	return time.Duration(d)
}

// FixedBackoff returns the same delay regardless of attempt count.
type FixedBackoff time.Duration

// Delay implements [Backoff].
func (f FixedBackoff) Delay(_ int) time.Duration {
	return time.Duration(f)
}

// LinearBackoff increases the delay linearly: Initial * (attempt + 1).
type LinearBackoff struct {
	Initial time.Duration
	Step    time.Duration // increment per attempt
	Max     time.Duration
}

// Delay implements [Backoff].
func (l LinearBackoff) Delay(attempt int) time.Duration {
	d := l.Initial + l.Step*time.Duration(attempt)
	if l.Max > 0 && d > l.Max {
		return l.Max
	}
	return d
}

// --- Jitter strategies ---

// Jitter randomises a backoff delay to decorrelate retry storms.
// The function receives the computed delay and a random source, and returns
// the jittered delay.
type Jitter func(delay time.Duration, rng func() float64) time.Duration

// NoJitter returns the delay unchanged.
func NoJitter(delay time.Duration, _ func() float64) time.Duration {
	return delay
}

// FullJitter returns a random value between 0 and the delay.
// This is the recommended strategy from AWS Architecture Blog
// ("Exponential Backoff and Jitter").
func FullJitter(delay time.Duration, rng func() float64) time.Duration {
	return time.Duration(float64(delay) * rng())
}

// EqualJitter returns delay/2 + random(0, delay/2).
// This reduces variance while still providing decorrelation.
func EqualJitter(delay time.Duration, rng func() float64) time.Duration {
	half := delay / 2
	return half + time.Duration(float64(half)*rng())
}

// --- Error classifiers ---

// Classifier determines whether an error is retryable.
type Classifier func(err error) bool

// RetryAny retries on any non-nil error.
func RetryAny(err error) bool { return true }

// RetryNever never retries (fail on first error).
func RetryNever(err error) bool { return false }

// RetryIf retries only if the given predicate returns true.
func RetryIf(pred func(err error) bool) Classifier {
	return pred
}
