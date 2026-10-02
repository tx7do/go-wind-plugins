package loki_test

import (
	"context"

	"github.com/tx7do/go-wind-plugins/log/loki"
)

// ExampleNewLogger constructs a logger that pushes records to a Grafana
// Loki ingestion endpoint. The labels identify the emitting service on
// every stream, records are buffered and pushed in batches, and Close
// drains the remaining buffer — an application defers it in its shutdown
// hook so buffered records are not lost on exit.
func ExampleNewLogger() {
	logger, err := loki.NewLogger(
		loki.WithEndpoint("http://loki:3100/loki/api/v1/push"),
		loki.WithLabel("app", "my-service"),
		loki.WithLabel("env", "production"),
	)
	if err != nil {
		return
	}
	defer logger.Close()

	logger.Info(context.Background(), "service started", "port", 8080)
}
