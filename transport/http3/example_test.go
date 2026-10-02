package http3_test

import (
	"net/http"

	http3transport "github.com/tx7do/go-wind-plugins/transport/http3"
)

// ExampleNewServer constructs an HTTP/3 server, creates a router scoped to an
// API prefix, and registers a GET route with a path parameter on that router.
// Register the server with the application's transport lifecycle; the routes
// are then served over QUIC once the server starts.
func ExampleNewServer() {
	srv := http3transport.NewServer(
		http3transport.WithAddress("localhost:8443"),
	)

	router := srv.Route("/api")
	router.GET("/hello/{name}", func(ctx http3transport.Context) error {
		return ctx.String(http.StatusOK, "Hello "+ctx.Vars().Get("name"))
	})
}
