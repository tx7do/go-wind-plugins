package metadata_test

import (
	grpcmetadata "github.com/tx7do/go-wind-plugins/transport/grpc/middleware/metadata"
)

// ExampleUnaryServerInterceptor returns the gRPC server interceptor that
// extracts the configured keys from incoming unary RPC metadata and stores
// them in the request context for downstream handlers. Interceptors are
// attached to a gRPC server with the WithMiddleware option of
// transport/grpc/server.
func ExampleUnaryServerInterceptor() {
	_ = grpcmetadata.UnaryServerInterceptor()
}

// ExampleStreamServerInterceptor returns the gRPC server interceptor that
// extracts the configured keys from incoming streaming RPC metadata and
// stores them in the request context for downstream handlers. Interceptors
// are attached to a gRPC server with the WithMiddleware option of
// transport/grpc/server.
func ExampleStreamServerInterceptor() {
	_ = grpcmetadata.StreamServerInterceptor()
}
