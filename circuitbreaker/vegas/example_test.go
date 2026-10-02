package vegas_test

import (
	"context"
	"fmt"
	"time"

	"github.com/tx7do/go-wind-plugins/circuitbreaker"
	"github.com/tx7do/go-wind-plugins/circuitbreaker/vegas"
)

// ExampleNew constructs a Vegas breaker, which degrades on latency
// inflation rather than error counts. Execute times the wrapped call and
// feeds the latency to RecordLatency automatically; the manual path is
// Allow followed by RecordLatency or MarkFailure. BaseRTT, CurrentRTT and
// Inflation expose the breaker's internal estimates for observability.
// Once the breaker trips, Allow and Execute fail fast with
// circuitbreaker.ErrCircuitOpen.
func ExampleNew() {
	cb := vegas.New(
		vegas.WithAlpha(0.5),        // degrade when RTT inflation > 50%
		vegas.WithBeta(0.3),         // heal when RTT inflation < 30%
		vegas.WithWarmupSamples(10), // samples before evaluation begins
	)
	defer cb.Close()

	_ = cb.Execute(context.Background(), func() error {
		// Returning non-nil here marks the call as a failure.
		return nil
	})

	if cb.Allow() == nil {
		// The operation ran; its latency feeds RecordLatency.
		// MarkFailure() would be used instead if it had failed.
		cb.RecordLatency(10 * time.Millisecond)
	}

	_ = cb.BaseRTT()
	_ = cb.CurrentRTT()
	_ = cb.Inflation()

	fmt.Println(cb.State(), circuitbreaker.ErrCircuitOpen)
}
