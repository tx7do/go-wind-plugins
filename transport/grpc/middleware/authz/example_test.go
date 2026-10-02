package authz_test

import (
	grpcauthz "github.com/tx7do/go-wind-plugins/transport/grpc/middleware/authz"
)

// ExampleUnaryInterceptor returns the gRPC server interceptor that enforces
// the configured authorization engine's decisions on every incoming unary
// RPC, rejecting unauthorized calls with a PermissionDenied error before the
// handler runs. It must sit after the authn interceptor in the chain, which
// is attached to a gRPC server with the WithMiddleware option of
// transport/grpc/server.
func ExampleUnaryInterceptor() {
	_ = grpcauthz.UnaryInterceptor(nil)
}

// ExampleStreamInterceptor returns the gRPC server interceptor that enforces
// the configured authorization engine's decisions on every incoming streaming
// RPC, rejecting unauthorized calls with a PermissionDenied error before the
// handler runs. It must sit after the authn interceptor in the chain, which
// is attached to a gRPC server with the WithMiddleware option of
// transport/grpc/server.
func ExampleStreamInterceptor() {
	_ = grpcauthz.StreamInterceptor(nil)
}
