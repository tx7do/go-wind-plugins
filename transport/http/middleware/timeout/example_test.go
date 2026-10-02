package timeout_test

import (
	"time"

	windhttp "github.com/tx7do/go-wind-plugins/transport/http"
	"github.com/tx7do/go-wind-plugins/transport/http/middleware/timeout"
)

// ExampleMiddleware attaches the timeout middleware to a server's global
// chain. Every request is executed under a deadline: when the deadline elapses
// before the handler finishes, the request context is cancelled and the
// default 503 Service Unavailable response is sent instead; the status and
// message of that response can be changed with WithStatus and WithMessage,
// and WithTimeoutFunc varies the budget per request. A runnable end-to-end
// setup additionally needs a driver (see transport/http/driver/std).
func ExampleMiddleware() {
	srv := windhttp.NewServer(":8080")
	srv.Use(timeout.Middleware(30 * time.Second))
}
