package otel_test

import (
	"context"

	"github.com/tx7do/go-wind-plugins/metrics/otel"
)

// ExampleNew constructs the OpenTelemetry metrics provider and records a
// counter increment, a histogram observation, and a gauge value through the
// shared metrics interface. WithEndpoint points the provider at an OTLP
// collector that receives the exported metrics; WithInsecure skips TLS for
// collectors reached over plaintext.
func ExampleNew() {
	m, err := otel.New(
		otel.WithEndpoint("localhost:4317"),
		otel.WithServiceName("my-service"),
		otel.WithInsecure(true),
	)
	if err != nil {
		return
	}

	m.Counter(context.Background(), "http_requests_total", 1,
		map[string]string{"method": "GET", "path": "/"})
	m.Histogram(context.Background(), "http_request_duration_seconds", 0.42,
		map[string]string{"method": "GET", "path": "/"})
	m.Gauge(context.Background(), "http_active_connections", 7, nil)
}
