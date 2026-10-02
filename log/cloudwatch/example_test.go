package cloudwatch_test

import (
	"context"

	"github.com/tx7do/go-wind-plugins/log/cloudwatch"
)

// ExampleNewCloudWatchLogger constructs a logger that batches records into
// an AWS CloudWatch Logs stream. The log group and stream are created on
// first use, records are buffered and flushed in the background or when the
// batch threshold is reached, and Close drains the remainder — an
// application defers it in its shutdown hook so buffered records are not
// lost on exit. Credentials are resolved from the ambient AWS
// configuration of the deployment.
func ExampleNewCloudWatchLogger() {
	logger, err := cloudwatch.NewCloudWatchLogger(
		context.Background(),
		cloudwatch.WithRegion("us-east-1"),
		cloudwatch.WithLogGroup("my-app"),
		cloudwatch.WithLogStream("api-server"),
	)
	if err != nil {
		return
	}
	defer logger.Close()

	logger.Info(context.Background(), "service started", "port", 8080)
}
