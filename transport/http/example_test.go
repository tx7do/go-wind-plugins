package http_test

import (
	"fmt"
	"net/http"

	windhttp "github.com/tx7do/go-wind-plugins/transport/http"
)

// ExampleNewServer shows the server-side basics: constructing a server bound
// to an address, attaching middleware to the global chain with Use, and
// reading the advertised endpoint address.
//
// Middleware registered with Use only wraps routes registered afterwards, so
// application middleware must be attached before any route registration.
// Route registration (GET/POST/.../Handle/HandlePrefix) and Start additionally
// require a driver set with WithDriver — see the example in
// transport/http/driver/std for the full setup.
func ExampleNewServer() {
	srv := windhttp.NewServer(":8080")

	// A trivial middleware: tag every response with a server header.
	srv.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-Server", "go-wind")
			next.ServeHTTP(w, r)
		})
	})

	fmt.Println(srv.Endpoint())
}
