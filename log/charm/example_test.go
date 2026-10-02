package charm_test

import (
	"context"

	"github.com/tx7do/go-wind-plugins/log/charm"
)

// ExampleNewLogger constructs the adapter around a default charm log
// logger — colored, human-friendly console output on stderr at INFO level,
// the usual choice for local development where operators read the terminal
// directly. Callers that already own a configured charm log logger can wrap
// it with NewLoggerWith instead.
func ExampleNewLogger() {
	logger := charm.NewLogger()

	logger.Info(context.Background(), "service started", "port", 8080)
}
