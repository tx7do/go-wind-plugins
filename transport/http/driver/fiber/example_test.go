package fiber_test

import (
	"fmt"
	"net/http"

	windhttp "github.com/tx7do/go-wind-plugins/transport/http"
	"github.com/tx7do/go-wind-plugins/transport/http/driver/fiber"
)

// ExampleNewDriver shows the canonical HTTP server setup with the fiber
// driver: the driver plugged into the server with WithDriver, followed by
// route registration. Handle, GET, POST, ... and Start all require a driver
// to be set first.
//
// Routes registered through the server are served through the driver's
// net/http compatibility adapter and pass through the server middleware
// chain like any other driver. Handlers built around the native *fiber.Ctx
// are a separate registration path on the driver itself and bypass that
// chain.
func ExampleNewDriver() {
	srv := windhttp.NewServer(":8080", windhttp.WithDriver(fiber.NewDriver()))

	srv.GET("/hello", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		fmt.Fprintln(w, "Hello, GoWind!")
	})

	fmt.Println(srv.Endpoint())
}
