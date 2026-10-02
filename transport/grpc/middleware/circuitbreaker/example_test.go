package circuitbreaker_test

import (
	grpccircuitbreaker "github.com/tx7do/go-wind-plugins/transport/grpc/middleware/circuitbreaker"
)

// ExampleUnaryInterceptor returns the gRPC server interceptor that enforces
// the configured circuit breaker's policy on every incoming unary RPC,
// rejecting the call with an Unavailable error while the breaker is open.
// Interceptors are attached to a gRPC server with the WithMiddleware option
// of transport/grpc/server.
func ExampleUnaryInterceptor() {
	_ = grpccircuitbreaker.UnaryInterceptor(nil)
}

// ExampleStreamInterceptor returns the gRPC server interceptor that enforces
// the configured circuit breaker's policy on every incoming streaming RPC,
// rejecting the call with an Unavailable error while the breaker is open.
// Interceptors are attached to a gRPC server with the WithMiddleware option
// of transport/grpc/server.
func ExampleStreamInterceptor() {
	_ = grpccircuitbreaker.StreamInterceptor(nil)
}
