package recovery_test

import (
	windhttp "github.com/tx7do/go-wind-plugins/transport/http"
	"github.com/tx7do/go-wind-plugins/transport/http/middleware/recovery"
)

// ExampleMiddleware attaches the recovery middleware to a server's global
// chain. It converts panics raised in downstream handlers into HTTP 500
// responses and therefore belongs at the outermost position of the chain.
func ExampleMiddleware() {
	srv := windhttp.NewServer(":8080")
	srv.Use(recovery.Middleware())
}
