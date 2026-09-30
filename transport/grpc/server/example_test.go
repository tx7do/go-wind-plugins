package server_test

import (
	grpcserver "github.com/tx7do/go-wind-plugins/transport/grpc/server"
	"google.golang.org/grpc"
)

type demoService struct{}

var demoServiceDesc = grpc.ServiceDesc{
	ServiceName: "demo.DemoService",
	HandlerType: (*demoService)(nil),
}

// Example registers a service implementation. Real services use the
// registration helper generated alongside their gRPC stubs — e.g.
// foo.RegisterFooServer(srv.Server(), impl) — which delegates to
// RegisterService with the generated ServiceDesc.
func Example() {
	srv := grpcserver.NewServer(":9000")

	srv.RegisterService(&demoServiceDesc, &demoService{})
}
