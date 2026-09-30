package tracing_test

import (
	windhttp "github.com/tx7do/go-wind-plugins/transport/http"
	"github.com/tx7do/go-wind-plugins/transport/http/middleware/tracing"
)

// ExampleMiddleware attaches the OpenTelemetry tracing middleware. It opens a
// server span for every request — marked as an error span when the handler
// fails — using the globally registered TracerProvider.
func ExampleMiddleware() {
	srv := windhttp.NewServer(":8080")
	srv.Use(tracing.Middleware())
}
