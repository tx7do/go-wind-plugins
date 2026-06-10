// Package main demonstrates a production-grade HTTP server with a middleware
// chain: recovery, request-id propagation, structured logging, and automatic
// content negotiation (JSON/XML).
//
// Run:
//
//	go run ./examples/http-middleware
//
// Test:
//
//	curl http://localhost:8080/hello
//	curl -H "Content-Type: application/json" -d '{"name":"alice"}' http://localhost:8080/echo
//	curl -H "Accept: application/xml" http://localhost:8080/hello
package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	_ "github.com/tx7do/go-wind-plugins/encoding/json" // side-effect: register JSON codec
	_ "github.com/tx7do/go-wind-plugins/encoding/xml"  // side-effect: register XML codec
	httpServer "github.com/tx7do/go-wind-plugins/transport/http"
	"github.com/tx7do/go-wind-plugins/transport/http/driver/std"
	"github.com/tx7do/go-wind-plugins/transport/http/middleware/codec"
	"github.com/tx7do/go-wind-plugins/transport/http/middleware/logging"
	"github.com/tx7do/go-wind-plugins/transport/http/middleware/recovery"
	"github.com/tx7do/go-wind-plugins/transport/http/middleware/requestid"
)

// greetRequest / greetResponse are simple structs for the /echo endpoint.
type greetRequest struct {
	Name string `json:"name" xml:"name"`
}

type greetResponse struct {
	Message string `json:"message" xml:"message"`
}

func main() {
	srv := httpServer.NewServer(":8080", httpServer.WithDriver(std.NewDriver()))

	// Middleware chain — order matters!
	// recovery must be outermost to catch panics from everything below.
	srv.Use(
		recovery.Middleware(),
		requestid.Middleware(),
		logging.Middleware(),
		codec.Middleware(),
	)

	// Plain text endpoint.
	srv.GET("/hello", func(w http.ResponseWriter, r *http.Request) {
		id := requestid.FromContext(r.Context())
		fmt.Fprintf(w, "Hello! (request-id: %s)\n", id)
	})

	// Endpoint that uses codec middleware for automatic JSON/XML negotiation.
	srv.POST("/echo", func(w http.ResponseWriter, r *http.Request) {
		var req greetRequest
		if err := codec.ReadBody(r, &req); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		// Respond uses the codec from the context (based on Content-Type).
		codec.Respond(w, r, http.StatusOK, &greetResponse{
			Message: "Hello, " + req.Name + "!",
		})
	})

	// Graceful shutdown.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	fmt.Printf("HTTP server listening on %s\n", srv.Endpoint())
	if err := srv.Start(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "server error: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("server stopped")
}
