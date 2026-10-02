package tencent_test

import (
	"context"

	"github.com/tx7do/go-wind-plugins/log/tencent"
)

// ExampleNewTencentLogger constructs a logger that ships records to a
// Tencent Cloud CLS topic through the SDK's batching producer. Records are
// buffered client-side and flushed asynchronously; Close drains the
// remaining buffer, so an application defers it in its shutdown hook before
// the process exits.
func ExampleNewTencentLogger() {
	logger, err := tencent.NewTencentLogger(
		tencent.WithEndpoint("ap-shanghai.cls.tencentcs.com"),
		tencent.WithTopicID("your-topic-id"),
		tencent.WithAccessKey("your-secret-id"),
		tencent.WithAccessSecret("your-secret-key"),
	)
	if err != nil {
		return
	}
	defer logger.Close()

	logger.Info(context.Background(), "service started", "port", 8080)
}
