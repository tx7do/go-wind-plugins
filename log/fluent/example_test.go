package fluent_test

import (
	"context"

	"github.com/tx7do/go-wind-plugins/log/fluent"
)

// ExampleNewFluentLogger constructs a logger that forwards records to a
// fluentd collector. The target address selects the transport — a
// tcp://host:port endpoint or a unix:// socket path — and records are
// buffered locally and handed to the collector; Close tears the connection
// down, so an application defers it in its shutdown hook.
func ExampleNewFluentLogger() {
	logger, err := fluent.NewFluentLogger("tcp://127.0.0.1:24224")
	if err != nil {
		return
	}
	defer logger.Close()

	logger.Info(context.Background(), "service started", "port", 8080)
}
