package client_test

import (
	grpcclient "github.com/tx7do/go-wind-plugins/transport/grpc/client"
)

// ExampleNewClient constructs a gRPC client for a target address with the
// plaintext transport option, matching a development server without TLS. In
// an application the client is dialed with Dial, the *grpc.ClientConn
// returned by Conn is passed to the service stubs generated alongside the
// protobuf definitions, and the target points at an endpoint served by
// transport/grpc/server; Close releases the connection on shutdown.
func ExampleNewClient() {
	_ = grpcclient.NewClient("localhost:50051", grpcclient.WithInsecure())
}
