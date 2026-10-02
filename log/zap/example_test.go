package zap_test

import (
	"context"

	zaplog "github.com/tx7do/go-wind-plugins/log/zap"
	"go.uber.org/zap"
)

// ExampleNewZapLogger wraps a configured zap logger in the plugin's Logger
// adapter, after which zap-formatted records flow through the common
// logging interface consumed by the rest of the framework. Sync — reached
// through Close — is flushed in the application's shutdown hook.
func ExampleNewZapLogger() {
	zlog, err := zap.NewProduction()
	if err != nil {
		return
	}
	logger := zaplog.NewZapLogger(zlog)
	defer logger.Close()

	logger.Info(context.Background(), "service started", "port", 8080)
	logger.With("module", "auth").Error(context.Background(), "token expired")
}
