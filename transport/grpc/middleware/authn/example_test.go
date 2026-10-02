package authn_test

import (
	grpcauthn "github.com/tx7do/go-wind-plugins/transport/grpc/middleware/authn"
)

// ExampleUnaryInterceptor returns the gRPC server interceptor that
// authenticates every incoming unary RPC with the configured authenticator
// and injects the resulting auth claims into the RPC context for downstream
// handlers. Interceptors are attached to a gRPC server with the
// WithMiddleware option of transport/grpc/server.
func ExampleUnaryInterceptor() {
	_ = grpcauthn.UnaryInterceptor(nil)
}

// ExampleStreamInterceptor returns the gRPC server interceptor that
// authenticates every incoming streaming RPC with the configured
// authenticator and injects the resulting auth claims into the RPC context
// for downstream handlers. Interceptors are attached to a gRPC server with
// the WithMiddleware option of transport/grpc/server.
func ExampleStreamInterceptor() {
	_ = grpcauthn.StreamInterceptor(nil)
}
