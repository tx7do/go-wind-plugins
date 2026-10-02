package cors_test

import (
	windhttp "github.com/tx7do/go-wind-plugins/transport/http"
	"github.com/tx7do/go-wind-plugins/transport/http/middleware/cors"
)

// ExampleMiddleware attaches the CORS middleware to a server's global chain.
// It validates each request's Origin header and, for allowed origins, sets
// the Access-Control-* response headers; preflight OPTIONS requests are
// short-circuited with a 204 No Content response. Allowed origins, methods,
// and headers are configured with the package's With* options.
func ExampleMiddleware() {
	srv := windhttp.NewServer(":8080")
	srv.Use(cors.Middleware())
}
