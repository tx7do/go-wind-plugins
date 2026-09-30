package logging_test

import (
	grpclogging "github.com/tx7do/go-wind-plugins/transport/grpc/middleware/logging"
)

// ExampleUnaryInterceptor returns the gRPC server interceptor that logs every
// unary RPC call with its duration through the configured logger. Interceptors
// are attached to a gRPC server with the WithMiddleware option of
// transport/grpc/server.
func ExampleUnaryInterceptor() {
	_ = grpclogging.UnaryInterceptor()
}
