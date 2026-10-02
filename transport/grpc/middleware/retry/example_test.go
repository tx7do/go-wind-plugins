package retry_test

import (
	grpcretry "github.com/tx7do/go-wind-plugins/transport/grpc/middleware/retry"
)

// ExampleUnaryInterceptor returns the gRPC server interceptor that retries
// idempotent unary RPCs through the supplied retrier when the handler returns
// a transient failure. Interceptors are attached to a gRPC server with the
// WithMiddleware option of transport/grpc/server.
func ExampleUnaryInterceptor() {
	_ = grpcretry.UnaryInterceptor(nil)
}

// ExampleStreamInterceptor returns the gRPC server interceptor that retries
// idempotent streaming RPCs through the supplied retrier when the handler
// returns a transient failure. It is attached to a gRPC server with the
// WithStreamMiddleware option of transport/grpc/server.
func ExampleStreamInterceptor() {
	_ = grpcretry.StreamInterceptor(nil)
}
