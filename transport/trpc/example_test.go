package trpc_test

import (
	trpctransport "github.com/tx7do/go-wind-plugins/transport/trpc"
)

type demoServiceImpl struct{}

// ExampleNewServer constructs a tRPC server and registers a service
// implementation under its service name. The registration must happen before
// the server starts; register the server with the application's transport
// lifecycle so the service is served once it starts.
func ExampleNewServer() {
	srv := trpctransport.NewServer(
		trpctransport.WithAddress("localhost:8972"),
		trpctransport.WithNamespace("Development"),
		trpctransport.WithEnvName("dev"),
		trpctransport.WithServiceName("trpc.example.demo"),
	)

	// The descriptor argument is the service descriptor emitted by the tRPC
	// code generator; nil is used here as a placeholder.
	if err := srv.RegisterService("trpc.example.demo", nil, &demoServiceImpl{}); err != nil {
		return
	}
}
