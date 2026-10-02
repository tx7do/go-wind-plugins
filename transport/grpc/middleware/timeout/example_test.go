package timeout_test

import (
	grpctimeout "github.com/tx7do/go-wind-plugins/transport/grpc/middleware/timeout"
)

// ExampleUnaryServerInterceptor returns the gRPC server interceptor that
// enforces a default deadline on incoming unary RPCs when the caller supplied
// none. Interceptors are attached to a gRPC server with the WithMiddleware
// option of transport/grpc/server.
func ExampleUnaryServerInterceptor() {
	_ = grpctimeout.UnaryServerInterceptor(0)
}

// ExampleStreamServerInterceptor returns the gRPC server interceptor that
// enforces a default deadline on incoming streaming RPCs when the caller
// supplied none. It is attached to a gRPC server with the
// WithStreamMiddleware option of transport/grpc/server.
func ExampleStreamServerInterceptor() {
	_ = grpctimeout.StreamServerInterceptor(0)
}
