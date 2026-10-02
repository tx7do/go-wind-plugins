package authz_test

import (
	windhttp "github.com/tx7do/go-wind-plugins/transport/http"
	"github.com/tx7do/go-wind-plugins/transport/http/middleware/authz"
)

// ExampleMiddleware attaches the authorization middleware to a server's
// global chain. It enforces the configured authorization engine's decision
// for every incoming request, rejecting unauthorized requests with a 403
// response before the handler runs. It must sit after the authn middleware
// in the chain, whose claims provide the subject for the decision.
func ExampleMiddleware() {
	srv := windhttp.NewServer(":8080")
	srv.Use(authz.Middleware(nil))
}
