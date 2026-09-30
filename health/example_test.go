package health_test

import (
	"context"
	"net/http"
	"time"

	"github.com/tx7do/go-wind-plugins/health"
)

// ExampleNew constructs a health aggregator, registers a probe, and exposes
// the Kubernetes-style liveness and readiness endpoints. The liveness handler
// answers as long as the process is alive; the readiness handler aggregates
// the registered checkers under a timeout.
func ExampleNew() {
	h := health.New(health.WithTimeout(3 * time.Second))

	h.Register("database", health.PingFunc(func(_ context.Context) error {
		// In production: return db.PingContext(ctx)
		return nil
	}))

	mux := http.NewServeMux()
	mux.Handle("/healthz", health.NewLivenessHandler())
	mux.Handle("/readyz", health.NewHandler(h))
	_ = mux
}
