package datadog_test

import (
	"context"

	"github.com/tx7do/go-wind-plugins/metrics/datadog"
)

// ExampleNew constructs the Datadog metrics provider and records a counter
// increment, a histogram observation, and a gauge value through the shared
// metrics interface. WithAddress points the provider at the local DogStatsD
// agent that relays metrics to Datadog's backend; WithNamespace prefixes all
// metric names sent upstream.
func ExampleNew() {
	m, err := datadog.New(
		datadog.WithAddress("127.0.0.1:8125"),
		datadog.WithNamespace("myapp"),
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
