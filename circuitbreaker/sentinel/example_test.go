package sentinel_test

import (
	"context"
	"fmt"

	sentinelapi "github.com/alibaba/sentinel-golang/api"
	"github.com/alibaba/sentinel-golang/core/base"
	"github.com/tx7do/go-wind-plugins/circuitbreaker"
	"github.com/tx7do/go-wind-plugins/circuitbreaker/sentinel"
)

// ExampleNew constructs a Sentinel-backed circuit breaker for a named
// resource. Execute marks the outcome automatically from the wrapped
// function's error; the manual path is Allow followed by MarkSuccess or
// MarkFailure. Breaker rules for the resource are configured separately
// through Sentinel's rule API; once one trips, Allow and Execute fail fast
// with circuitbreaker.ErrCircuitOpen.
func ExampleNew() {
	cb := sentinel.New("my-api",
		sentinel.WithTrafficType(base.Inbound),                     // traffic type of the wrapped entry
		sentinel.WithEntryOptions(sentinelapi.WithAcquireCount(2)), // raw Sentinel entry options
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
