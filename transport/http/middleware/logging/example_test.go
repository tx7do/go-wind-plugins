package logging_test

import (
	windhttp "github.com/tx7do/go-wind-plugins/transport/http"
	"github.com/tx7do/go-wind-plugins/transport/http/middleware/logging"
)

// ExampleMiddleware attaches the request-logging middleware to a server's
// global chain. It emits one structured log record per request through the
// configured logger.
func ExampleMiddleware() {
	srv := windhttp.NewServer(":8080")
	srv.Use(logging.Middleware())
}
