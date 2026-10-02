package errors_test

import (
	grpcerrors "github.com/tx7do/go-wind-plugins/transport/grpc/middleware/errors"
)

// ExampleUnaryServerInterceptor returns the gRPC server interceptor that
// translates framework errors returned by unary handlers into gRPC status
// errors and passes every other error through unchanged. Interceptors are
// attached to a gRPC server with the WithMiddleware option of
// transport/grpc/server.
func ExampleUnaryServerInterceptor() {
	_ = grpcerrors.UnaryServerInterceptor()
}

// ExampleStreamServerInterceptor returns the gRPC server interceptor that
// translates framework errors returned by streaming handlers into gRPC status
// errors and passes every other error through unchanged. Interceptors are
// attached to a gRPC server with the WithMiddleware option of
// transport/grpc/server.
func ExampleStreamServerInterceptor() {
	_ = grpcerrors.StreamServerInterceptor()
}
