// retry-basic demonstrates how to use the retry package with various
// backoff strategies, jitter, and error classifiers.
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/tx7do/go-wind-plugins/retry"
)

func main() {
	ctx := context.Background()

	// --- Example 1: Default retry (3 attempts, exponential backoff) ---
	fmt.Println("=== Example 1: Default retry ===")
	r1 := retry.New()
	attempt := 0
	err := r1.Do(ctx, func(_ context.Context) error {
		attempt++
		fmt.Printf("  attempt %d\n", attempt)
		if attempt < 3 {
			return fmt.Errorf("transient error on attempt %d", attempt)
		}
		return nil
	})
	fmt.Printf("  result: %v\n\n", err)

	// --- Example 2: Custom max attempts + fixed backoff ---
	fmt.Println("=== Example 2: Fixed backoff, 5 attempts ===")
	r2 := retry.New(
		retry.WithMaxAttempts(5),
		retry.WithBackoff(retry.FixedBackoff(200*time.Millisecond)),
	)
	attempt = 0
	err = r2.Do(ctx, func(_ context.Context) error {
		attempt++
		fmt.Printf("  attempt %d\n", attempt)
		return fmt.Errorf("still failing")
	})
	fmt.Printf("  result: %v\n\n", err)

	// --- Example 3: Exponential backoff with full jitter ---
	fmt.Println("=== Example 3: Exponential backoff + full jitter ===")
	r3 := retry.New(
		retry.WithMaxAttempts(4),
		retry.WithBackoff(retry.ExponentialBackoff{
			Initial: 100 * time.Millisecond,
			Factor:  2,
			Max:     2 * time.Second,
		}),
		retry.WithJitter(retry.FullJitter),
	)
	start := time.Now()
	attempt = 0
	err = r3.Do(ctx, func(_ context.Context) error {
		attempt++
		elapsed := time.Since(start).Round(time.Millisecond)
		fmt.Printf("  attempt %d (elapsed: %s)\n", attempt, elapsed)
		if attempt < 3 {
			return fmt.Errorf("retryable error")
		}
		return nil
	})
	fmt.Printf("  result: %v\n\n", err)

	// --- Example 4: Error classifier — only retry specific errors ---
	fmt.Println("=== Example 4: Error classifier ===")
	var errTemporary = errors.New("temporary failure")
	var errFatal = errors.New("fatal failure")

	r4 := retry.New(
		retry.WithMaxAttempts(5),
		retry.WithClassifier(retry.RetryIf(func(err error) bool {
			// Only retry temporary errors; fatal errors fail immediately.
			return errors.Is(err, errTemporary)
		})),
	)

	// 4a: Temporary error — will retry and eventually succeed.
	attempt = 0
	err = r4.Do(ctx, func(_ context.Context) error {
		attempt++
		fmt.Printf("  [temporary] attempt %d\n", attempt)
		if attempt < 2 {
			return errTemporary
		}
		return nil
	})
	fmt.Printf("  temporary result: %v\n", err)

	// 4b: Fatal error — will NOT retry.
	attempt = 0
	err = r4.Do(ctx, func(_ context.Context) error {
		attempt++
		fmt.Printf("  [fatal] attempt %d\n", attempt)
		return errFatal
	})
	fmt.Printf("  fatal result: %v\n\n", err)

	// --- Example 5: Max total wait timeout ---
	fmt.Println("=== Example 5: Max total wait timeout ===")
	r5 := retry.New(
		retry.WithMaxAttempts(100), // high attempt count
		retry.WithBackoff(retry.FixedBackoff(500*time.Millisecond)),
		retry.WithMaxTotalWait(2*time.Second),
	)
	start = time.Now()
	err = r5.Do(ctx, func(_ context.Context) error {
		return fmt.Errorf("always failing")
	})
	elapsed := time.Since(start).Round(time.Millisecond)
	fmt.Printf("  stopped after %s: %v\n", elapsed, err)

	log.Println("all examples completed")
}
