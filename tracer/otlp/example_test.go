package otlp_test

import (
	"context"

	"github.com/tx7do/go-wind-plugins/tracer/otlp"
)

// ExampleNew constructs the OTLP-backed OpenTelemetry TracerProvider with a
// collector endpoint and a service identity, and registers it as the global
// provider together with the W3C trace-context propagator; from here on,
// spans are recorded through the standard OpenTelemetry API. Shutdown on
// process exit flushes any spans still buffered in the batch processor.
func ExampleNew() {
	tp, err := otlp.New(
		otlp.WithEndpoint("localhost:4317"),
		otlp.WithServiceName("my-service"),
		otlp.WithServiceVersion("v1.0.0"),
		otlp.WithSampleRatio(1.0),
		otlp.WithInsecure(true),
	)
	if err != nil {
		return
	}
	defer tp.Shutdown(context.Background())
}
