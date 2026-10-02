package aliyun_test

import (
	"context"

	"github.com/tx7do/go-wind-plugins/log/aliyun"
)

// ExampleNewAliyunLogger constructs a logger that ships records to an
// Aliyun SLS logstore through the SDK's batching producer. Records are
// buffered client-side and flushed asynchronously; Close drains the
// remaining buffer, so an application defers it in its shutdown hook before
// the process exits. The credentials identify the producer to a single
// project and logstore and are supplied per deployment.
func ExampleNewAliyunLogger() {
	logger, err := aliyun.NewAliyunLogger(
		aliyun.WithEndpoint("cn-hangzhou.log.aliyuncs.com"),
		aliyun.WithProject("my-project"),
		aliyun.WithLogstore("app"),
		aliyun.WithAccessKey("your-access-key-id"),
		aliyun.WithAccessSecret("your-access-key-secret"),
	)
	if err != nil {
		return
	}
	defer logger.Close()

	logger.Info(context.Background(), "service started", "port", 8080)
}
