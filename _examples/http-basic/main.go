// Package main demonstrates a minimal HTTP server using the go-wind framework.
//
// This example shows the simplest possible setup: one route, no middleware,
// using the default net/http driver.
//
// Run:
//
//	go run ./examples/http-basic
//
// Test:
//
//	curl http://localhost:8080/hello
package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	httpServer "github.com/tx7do/go-wind-plugins/transport/http"
	"github.com/tx7do/go-wind-plugins/transport/http/driver/std"
)

func main() {
	// Create an HTTP server listening on :8080.
	srv := httpServer.NewServer(":8080", httpServer.WithDriver(std.NewDriver()))

	// Register a simple route.
	srv.GET("/hello", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		fmt.Fprintln(w, "Hello, GoWind!")
	})

	// Graceful shutdown on SIGINT / SIGTERM.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	fmt.Printf("HTTP server listening on %s\n", srv.Endpoint())
	if err := srv.Start(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "server error: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("server stopped")
}
