package s3_test

import (
	"github.com/tx7do/go-wind-plugins/oss/s3"
)

// ExampleNewStorage constructs an S3-compatible storage client from a Config
// pointing at a regional endpoint with static credentials and a default
// bucket. An application creates the storage once at startup and injects it
// wherever object uploads and downloads happen.
func ExampleNewStorage() {
	storage := s3.NewStorage(&s3.Config{
		Endpoint:  "s3.ap-southeast-1.amazonaws.com",
		Region:    "ap-southeast-1",
		Bucket:    "my-bucket",
		AccessKey: "your-access-key",
		SecretKey: "your-secret-key",
		UseSsl:    true,
	})
	_ = storage
}
