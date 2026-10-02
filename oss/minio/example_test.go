package minio_test

import (
	"github.com/tx7do/go-wind-plugins/oss/minio"
)

// ExampleNewStorage constructs a MinIO-backed storage client from a Config
// holding the server endpoint and static credentials. An application
// creates the storage once at startup and injects it wherever object
// uploads and downloads happen; buckets are named on each call.
func ExampleNewStorage() {
	storage := minio.NewStorage(&minio.Config{
		Endpoint:  "127.0.0.1:9000",
		AccessKey: "your-access-key",
		SecretKey: "your-secret-key",
		UseSsl:    false,
	})
	_ = storage
}
