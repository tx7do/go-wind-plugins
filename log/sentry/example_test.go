package sentry_test

import (
	"context"

	"github.com/tx7do/go-wind-plugins/log/sentry"
)

// ExampleNewLogger constructs a logger that reports to a Sentry project.
// Error-level records are captured as Sentry events while lower-level
// records are recorded as breadcrumbs attached to the events that follow.
// Close flushes pending events to the Sentry server, so an application
// defers it in its shutdown hook.
func ExampleNewLogger() {
	logger, err := sentry.NewLogger(
		sentry.WithDSN("https://xxx@sentry.io/123"),
		sentry.WithEnvironment("production"),
	)
	if err != nil {
		return
	}
	defer logger.Close()

	logger.Error(context.Background(), "upstream dependency timeout")
}
