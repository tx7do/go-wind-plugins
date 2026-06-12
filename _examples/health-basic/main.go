// health-basic demonstrates how to set up health checks with liveness and
// readiness probes suitable for Kubernetes, using built-in TCP and HTTP checkers.
package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/tx7do/go-wind-plugins/health"
)

func main() {
	// 1. Create a health aggregator.
	h := health.New(health.WithTimeout(3 * time.Second))

	// 2. Register health checkers.
	//    - TCP checker: verify a TCP endpoint is reachable.
	//    - HTTP checker: verify an HTTP endpoint returns 2xx/3xx.
	//    - PingFunc: arbitrary function returning error.
	//    - Composed checkers: All / Any.

	// Simulated "database" check using PingFunc.
	h.Register("database", health.PingFunc(func(_ context.Context) error {
		// In production, replace with: return db.PingContext(ctx)
		return nil // always up in this demo
	}))

	// Simulated "redis" check using PingFunc.
	h.Register("redis", health.PingFunc(func(_ context.Context) error {
		// In production, replace with: return redisClient.Ping(ctx).Err()
		return nil // always up in this demo
	}))

	// 3. Set up HTTP server with liveness and readiness endpoints.
	mux := http.NewServeMux()

	// Liveness probe: always returns 200 if the process is alive.
	mux.Handle("/healthz", health.NewLivenessHandler())

	// Readiness probe: runs all registered checkers.
	mux.Handle("/readyz", health.NewHandler(h))

	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprintln(w, "hello! check /healthz and /readyz")
	})

	srv := &http.Server{Addr: ":8080", Handler: mux}

	// 4. Start server.
	go func() {
		log.Println("server listening on :8080")
		log.Println("  http://localhost:8080/        — home")
		log.Println("  http://localhost:8080/healthz — liveness probe")
		log.Println("  http://localhost:8080/readyz  — readiness probe")
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server error: %v", err)
		}
	}()

	// 5. Graceful shutdown.
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("shutting down...")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer shutdownCancel()
	_ = srv.Shutdown(shutdownCtx)
}
