package prometheus_test

import (
	"context"

	"github.com/tx7do/go-wind-plugins/metrics/prometheus"
)

// ExampleNew constructs the Prometheus metrics provider and records a counter
// increment, a histogram observation, and a gauge value through the shared
// metrics interface. Registry returns the underlying prometheus.Registry,
// which is what a /metrics endpoint is mounted on with
// promhttp.HandlerFor.
func ExampleNew() {
	m, err := prometheus.New(prometheus.WithNamespace("myapp"))
	if err != nil {
		return
	}

	m.Counter(context.Background(), "http_requests_total", 1,
		map[string]string{"method": "GET", "path": "/"})
	m.Histogram(context.Background(), "http_request_duration_seconds", 0.42,
		map[string]string{"method": "GET", "path": "/"})
	m.Gauge(context.Background(), "http_active_connections", 7, nil)
	_ = m.Registry()

	// Output:
}
