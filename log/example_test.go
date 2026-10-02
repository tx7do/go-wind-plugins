package log_test

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"

	"github.com/tx7do/go-wind-plugins/log"
	windlog "github.com/tx7do/go-wind/log"
)

// ExampleMultiLogger fans a single record out to every sink registered on
// the multi-logger. Both sinks here are slog text handlers writing to
// in-memory buffers so the delivery is observable; an application wires the
// same pattern to tee records into, say, a local console logger alongside a
// remote aggregation service, installing the multi-logger as its
// process-wide logger.
func ExampleMultiLogger() {
	var first, second bytes.Buffer

	ml := log.MultiLogger{Loggers: []windlog.Logger{
		log.SlogLogger{L: slog.New(slog.NewTextHandler(&first, &slog.HandlerOptions{
			Level:       slog.LevelInfo,
			ReplaceAttr: stripTimestamp,
		}))},
		log.SlogLogger{L: slog.New(slog.NewTextHandler(&second, &slog.HandlerOptions{
			Level:       slog.LevelInfo,
			ReplaceAttr: stripTimestamp,
		}))},
	}}

	ml.Info(context.Background(), "service started", "port", 8080)

	fmt.Print(first.String())
	fmt.Print(second.String())

	// Output:
	// level=INFO msg="service started" port=8080
	// level=INFO msg="service started" port=8080
}

// ExampleLevelFilter wraps an in-memory slog sink in a level filter with a
// Warn threshold. The Info call is discarded by the filter and never
// reaches the sink; the Warn call is forwarded. Applications use this to
// share a single sink while suppressing the routine noise emitted by chatty
// components.
func ExampleLevelFilter() {
	var sink bytes.Buffer

	filtered := log.LevelFilter{
		Logger: log.SlogLogger{L: slog.New(slog.NewTextHandler(&sink, &slog.HandlerOptions{
			Level:       slog.LevelInfo,
			ReplaceAttr: stripTimestamp,
		}))},
		Level: windlog.LevelWarn,
	}

	filtered.Info(context.Background(), "heartbeat")
	filtered.Warn(context.Background(), "queue backlog")

	fmt.Print(sink.String())

	// Output:
	// level=WARN msg="queue backlog"
}

// stripTimestamp drops the slog timestamp attribute so the output recorded
// by the examples above is deterministic.
func stripTimestamp(_ []string, a slog.Attr) slog.Attr {
	if a.Key == slog.TimeKey {
		return slog.Attr{}
	}
	return a
}
