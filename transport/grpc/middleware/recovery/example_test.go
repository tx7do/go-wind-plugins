package recovery_test

import (
	grpcmiddleware "github.com/tx7do/go-wind-plugins/transport/grpc/middleware/recovery"
)

// ExampleUnaryInterceptor returns the gRPC server interceptor that converts
// panics in downstream handlers into gRPC Internal errors. It belongs at the
// outermost position of the interceptor chain, which is attached to a gRPC
// server with the WithMiddleware option of transport/grpc/server.
func ExampleUnaryInterceptor() {
	_ = grpcmiddleware.UnaryInterceptor()
}
