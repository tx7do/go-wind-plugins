// metrics-basic demonstrates how to use the Prometheus metrics provider
// to record counters, histograms, and gauges, and expose them via /metrics.
package main

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/tx7do/go-wind-plugins/metrics"
	prom "github.com/tx7do/go-wind-plugins/metrics/prometheus"
)

func main() {
	// 1. Create a Prometheus metrics provider.
	m, err := prom.New(prom.WithNamespace("myapp"))
	if err != nil {
		log.Fatalf("failed to create metrics provider: %v", err)
	}
	_ = m

	// 2. Set up HTTP server with /metrics endpoint.
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.HandlerFor(m.Registry(), promhttp.HandlerOpts{}))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// Record some demo metrics on each request.
		labels := map[string]string{"method": r.Method, "path": r.URL.Path}
		m.Counter(r.Context(), "http_requests_total", 1, labels)
		m.Histogram(r.Context(), "http_request_duration_seconds", rand.Float64(), labels)
		m.Gauge(r.Context(), "http_active_connections", float64(rand.Intn(100)), nil)
		fmt.Fprintf(w, "metrics recorded! visit /metrics to see them.\n")
	})

	srv := &http.Server{Addr: ":8080", Handler: mux}

	// 3. Background goroutine to record periodic metrics.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go recordPeriodicMetrics(ctx, m)

	// 4. Start server.
	go func() {
		log.Println("server listening on :8080")
		log.Println("  http://localhost:8080/       — trigger metric recording")
		log.Println("  http://localhost:8080/metrics — prometheus metrics")
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

// recordPeriodicMetrics demonstrates recording metrics in a background loop.
func recordPeriodicMetrics(ctx context.Context, m metrics.Metrics) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.Gauge(ctx, "uptime_gauge", float64(time.Now().Unix()), nil)
			m.Counter(ctx, "background_ticks_total", 1, nil)
		}
	}
}
