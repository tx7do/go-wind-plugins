package std_test

import (
	"fmt"
	"net/http"

	windhttp "github.com/tx7do/go-wind-plugins/transport/http"
	"github.com/tx7do/go-wind-plugins/transport/http/driver/std"
)

// ExampleNewDriver shows the canonical HTTP server setup: the net/http driver
// plugged into the server with WithDriver, followed by route registration.
// Handle, GET, POST, ... and Start all require a driver to be set first.
func ExampleNewDriver() {
	srv := windhttp.NewServer(":8080", windhttp.WithDriver(std.NewDriver()))

	srv.GET("/hello", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		fmt.Fprintln(w, "Hello, GoWind!")
	})

	fmt.Println(srv.Endpoint())
}
