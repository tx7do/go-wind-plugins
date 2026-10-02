package tracing_test

import (
	grpctracing "github.com/tx7do/go-wind-plugins/transport/grpc/middleware/tracing"
)

// ExampleUnaryInterceptor returns the gRPC server interceptor that creates an
// OpenTelemetry server span for each unary RPC, extracting the parent trace
// context from incoming metadata and recording gRPC semantic attributes.
// Interceptors are attached to a gRPC server with the WithMiddleware option
// of transport/grpc/server.
func ExampleUnaryInterceptor() {
	_ = grpctracing.UnaryInterceptor()
}

// ExampleStreamInterceptor returns the gRPC server interceptor that creates
// an OpenTelemetry server span for each streaming RPC, extracting the parent
// trace context from incoming metadata and recording gRPC semantic
// attributes. It is attached to a gRPC server with the WithStreamMiddleware
// option of transport/grpc/server.
func ExampleStreamInterceptor() {
	_ = grpctracing.StreamInterceptor()
}
