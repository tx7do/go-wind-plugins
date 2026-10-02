package hystrix_test

import (
	"context"
	"fmt"
	"time"

	"github.com/tx7do/go-wind-plugins/circuitbreaker"
	"github.com/tx7do/go-wind-plugins/circuitbreaker/hystrix"
)

// ExampleNew constructs a Hystrix circuit breaker. Execute marks the outcome
// automatically from the wrapped function's error; the manual path is
// Allow followed by MarkSuccess or MarkFailure. Once the breaker trips,
// Allow and Execute fail fast with circuitbreaker.ErrCircuitOpen.
func ExampleNew() {
	cb := hystrix.New(
		hystrix.WithErrorThreshold(0.5),        // error rate that trips the breaker
		hystrix.WithRequestVolumeThreshold(20), // minimum requests before evaluation
		hystrix.WithSleepWindow(5*time.Second), // Open -> HalfOpen transition delay
		hystrix.WithWindow(10*time.Second),     // statistical window
		hystrix.WithBucketCount(10),            // buckets within window
	)
	defer cb.Close()

	_ = cb.Execute(context.Background(), func() error {
		// Returning non-nil here marks the call as a failure.
		return nil
	})

	if cb.Allow() == nil {
		// The operation ran; MarkFailure() would be used instead
		// if it had failed.
		cb.MarkSuccess()
	}

	fmt.Println(cb.State(), circuitbreaker.ErrCircuitOpen)
}
