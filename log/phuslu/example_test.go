package phuslu_test

import (
	"context"

	"github.com/tx7do/go-wind-plugins/log/phuslu"
)

// ExampleNewLogger constructs the adapter around a default phuslu logger —
// JSON records on stderr at INFO level. Callers that already own a
// configured phuslu logger can wrap it with NewLoggerWith instead.
func ExampleNewLogger() {
	logger := phuslu.NewLogger()
	defer logger.Close()

	logger.Info(context.Background(), "service started", "port", 8080)
}
