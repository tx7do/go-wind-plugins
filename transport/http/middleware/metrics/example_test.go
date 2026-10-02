package metrics_test

import (
	windhttp "github.com/tx7do/go-wind-plugins/transport/http"
	"github.com/tx7do/go-wind-plugins/transport/http/middleware/metrics"
)

// ExampleMiddleware attaches the metrics middleware to a server's global
// chain. It records request count, request duration, and in-flight requests
// for every request through the engine-agnostic metrics interface. Metric
// names and label derivation are configurable with the package's With*
// options.
func ExampleMiddleware() {
	srv := windhttp.NewServer(":8080")
	srv.Use(metrics.Middleware(nil))
}
