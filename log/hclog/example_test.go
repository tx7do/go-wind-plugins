package hclog_test

import (
	"context"

	"github.com/tx7do/go-wind-plugins/log/hclog"
)

// ExampleNewLogger constructs the adapter around a default hclog logger —
// JSON records on stderr at INFO level. Callers that already own a
// configured hclog logger can wrap it with NewLoggerWith instead.
func ExampleNewLogger() {
	logger := hclog.NewLogger()

	logger.Info(context.Background(), "service started", "port", 8080)
}
