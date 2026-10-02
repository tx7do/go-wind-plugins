package logrus_test

import (
	"context"

	"github.com/sirupsen/logrus"

	logruslog "github.com/tx7do/go-wind-plugins/log/logrus"
)

// ExampleNewLogrusLogger wraps a configured logrus logger in the plugin's
// Logger adapter, after which logrus-formatted records flow through the
// common logging interface consumed by the rest of the framework. Fields
// attached through With persist on the derived logger only.
func ExampleNewLogrusLogger() {
	l := logrus.New()
	logger := logruslog.NewLogrusLogger(l)

	logger.Info(context.Background(), "service started", "port", 8080)
}
