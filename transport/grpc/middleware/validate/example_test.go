package validate_test

import (
	grpcvalidate "github.com/tx7do/go-wind-plugins/transport/grpc/middleware/validate"
)

// ExampleUnaryServerInterceptor returns the gRPC server interceptor that
// validates incoming unary request messages through the Validator interface
// before the handler is invoked, rejecting invalid requests with
// InvalidArgument. Interceptors are attached to a gRPC server with the
// WithMiddleware option of transport/grpc/server.
func ExampleUnaryServerInterceptor() {
	_ = grpcvalidate.UnaryServerInterceptor()
}

// ExampleStreamServerInterceptor returns the gRPC server interceptor that
// validates each message received through an incoming stream before it is
// forwarded to the handler, rejecting invalid messages with InvalidArgument.
// It is attached to a gRPC server with the WithStreamMiddleware option of
// transport/grpc/server.
func ExampleStreamServerInterceptor() {
	_ = grpcvalidate.StreamServerInterceptor()
}
