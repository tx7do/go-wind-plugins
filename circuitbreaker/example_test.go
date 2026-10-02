package circuitbreaker_test

import (
	"fmt"

	"github.com/tx7do/go-wind-plugins/circuitbreaker"
)

// ExampleState prints the name each circuit-breaker state reports through
// the State type's String method. A concrete breaker surfaces these values
// from its State method; a health endpoint or metrics exporter can relay
// the names to an operations dashboard so a tripped circuit is visible
// without sending probe traffic.
func ExampleState() {
	fmt.Println(circuitbreaker.StateClosed)
	fmt.Println(circuitbreaker.StateOpen)
	fmt.Println(circuitbreaker.StateHalfOpen)
}
