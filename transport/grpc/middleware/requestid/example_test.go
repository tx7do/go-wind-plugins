package requestid_test

import (
	grpcrequestid "github.com/tx7do/go-wind-plugins/transport/grpc/middleware/requestid"
)

// ExampleUnaryServerInterceptor returns the gRPC server interceptor that
// extracts the request ID from incoming metadata and stores it in the request
// context for downstream handlers and logs. Interceptors are attached to a
// gRPC server with the WithMiddleware option of transport/grpc/server.
func ExampleUnaryServerInterceptor() {
	_ = grpcrequestid.UnaryServerInterceptor()
}

// ExampleStreamServerInterceptor returns the gRPC server interceptor that
// extracts the request ID from incoming metadata of a streaming RPC and
// stores it in the stream context for downstream handlers and logs. It is
// attached to a gRPC server with the WithStreamMiddleware option of
// transport/grpc/server.
func ExampleStreamServerInterceptor() {
	_ = grpcrequestid.StreamServerInterceptor()
}
