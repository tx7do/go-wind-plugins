package requestid_test

import (
	"fmt"
	"net/http"

	windhttp "github.com/tx7do/go-wind-plugins/transport/http"
	"github.com/tx7do/go-wind-plugins/transport/http/middleware/requestid"
)

// ExampleMiddleware attaches the request-id middleware, which tags every
// request with a unique id and exposes it to handlers via FromContext. The
// same id can be echoed into downstream calls and log records so a request can
// be traced across service boundaries.
func ExampleMiddleware() {
	srv := windhttp.NewServer(":8080")
	srv.Use(requestid.Middleware())

	var handler http.HandlerFunc = func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "request-id: %s\n", requestid.FromContext(r.Context()))
	}
	_ = handler
}
