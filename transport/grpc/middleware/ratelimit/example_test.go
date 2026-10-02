package ratelimit_test

import (
	grpcratelimit "github.com/tx7do/go-wind-plugins/transport/grpc/middleware/ratelimit"
)

// ExampleUnaryInterceptor returns the gRPC server interceptor that enforces
// rate-limiting on incoming unary RPCs through the supplied limiter
// implementation, rejecting over-limit calls with ResourceExhausted.
// Interceptors are attached to a gRPC server with the WithMiddleware option
// of transport/grpc/server.
func ExampleUnaryInterceptor() {
	_ = grpcratelimit.UnaryInterceptor(nil)
}

// ExampleStreamInterceptor returns the gRPC server interceptor that enforces
// rate-limiting on incoming streaming RPCs through the supplied limiter
// implementation, rejecting over-limit calls with ResourceExhausted. It is
// attached to a gRPC server with the WithStreamMiddleware option of
// transport/grpc/server.
func ExampleStreamInterceptor() {
	_ = grpcratelimit.StreamInterceptor(nil)
}
