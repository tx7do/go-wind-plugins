package zerolog_test

import (
	"context"

	"github.com/rs/zerolog"

	zerologlog "github.com/tx7do/go-wind-plugins/log/zerolog"
)

// ExampleNewZerologLogger builds a destination with the package's writer
// factory, binds a zerolog logger to it, and wraps that logger in the
// plugin's Logger adapter so zerolog-formatted records flow through the
// common logging interface. The factory can also produce console writers,
// rotating lumberjack files, and multi-writer tees feeding the same
// adapter.
func ExampleNewZerologLogger() {
	writer := zerologlog.NewStdoutWriter()
	z := zerolog.New(writer)
	logger := zerologlog.NewZerologLogger(&z)

	logger.Info(context.Background(), "service started", "port", 8080)
}
