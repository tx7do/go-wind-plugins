package circuitbreaker_test

import (
	windhttp "github.com/tx7do/go-wind-plugins/transport/http"
	"github.com/tx7do/go-wind-plugins/transport/http/middleware/circuitbreaker"
)

// ExampleMiddleware attaches the circuit-breaker middleware to a server's
// global chain. It enforces the configured circuit breaker's policy on every
// request: while the breaker is open the request is rejected with a 503
// response, otherwise the handler runs and its response status feeds the
// breaker's success or failure counters.
func ExampleMiddleware() {
	srv := windhttp.NewServer(":8080")
	srv.Use(circuitbreaker.Middleware(nil))
}
