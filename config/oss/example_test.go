package oss_test

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/aws"
	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/tx7do/go-wind-plugins/config/oss"
)

// ExampleNew constructs an S3-backed configuration source. Load downloads the
// object at the configured key from the configured bucket; WatchValue polls
// the object's ETag and delivers the new content on the returned channel
// whenever it changes.
func ExampleNew() {
	client := awss3.NewFromConfig(aws.Config{
		Region: "us-east-1",
	})

	src, err := oss.New(client,
		oss.WithBucket("my-config-bucket"),
		oss.WithKey("myapp/config.yaml"),
	)
	if err != nil {
		return
	}

	raw, err := src.Load(context.Background(), "")
	if err != nil {
		return
	}
	_ = raw

	ch, err := src.WatchValue(context.Background(), "")
	if err != nil {
		return
	}
	_ = ch
}
