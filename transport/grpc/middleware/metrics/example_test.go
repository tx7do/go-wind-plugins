package metrics_test

import (
	grpcmetrics "github.com/tx7do/go-wind-plugins/transport/grpc/middleware/metrics"
)

// ExampleUnaryServerInterceptor returns the gRPC server interceptor that
// records request count, request duration, and in-flight requests for
// incoming unary RPCs through the engine-agnostic metrics interface.
// Interceptors are attached to a gRPC server with the WithMiddleware option
// of transport/grpc/server.
func ExampleUnaryServerInterceptor() {
	_ = grpcmetrics.UnaryServerInterceptor(nil)
}

// ExampleStreamServerInterceptor returns the gRPC server interceptor that
// records request count, request duration, and in-flight requests for
// incoming streaming RPCs through the engine-agnostic metrics interface. It
// is attached to a gRPC server with the WithStreamMiddleware option of
// transport/grpc/server.
func ExampleStreamServerInterceptor() {
	_ = grpcmetrics.StreamServerInterceptor(nil)
}
