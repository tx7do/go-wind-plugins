package server_test

import (
	"context"

	grpcserver "github.com/tx7do/go-wind-plugins/transport/grpc/server"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"
)

type demoService struct{}

// Ping implements the demo service.
func (d *demoService) Ping(_ context.Context, e *emptypb.Empty) (*emptypb.Empty, error) {
	return e, nil
}

// demoServer is the service interface; grpc.ServiceDesc.HandlerType must be a
// pointer to an interface type (as in generated code) — a pointer to a struct
// type makes RegisterService panic with "non-interface type".
type demoServer interface {
	Ping(context.Context, *emptypb.Empty) (*emptypb.Empty, error)
}

var demoServiceDesc = grpc.ServiceDesc{
	ServiceName: "demo.DemoService",
	HandlerType: (*demoServer)(nil),
	Methods: []grpc.MethodDesc{
		{
			MethodName: "Ping",
			Handler: func(srv any, ctx context.Context, dec func(any) error, interceptor grpc.UnaryServerInterceptor) (any, error) {
				in := new(emptypb.Empty)
				if err := dec(in); err != nil {
					return nil, err
				}
				if interceptor == nil {
					return srv.(*demoService).Ping(ctx, in)
				}
				info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/demo.DemoService/Ping"}
				return interceptor(ctx, in, info, func(ctx context.Context, req any) (any, error) {
					return srv.(*demoService).Ping(ctx, req.(*emptypb.Empty))
				})
			},
		},
	},
}

// Example registers a service implementation. Real services use the
// registration helper generated alongside their gRPC stubs — e.g.
// foo.RegisterFooServer(srv.Server(), impl) — which delegates to
// RegisterService with the generated ServiceDesc.
//
// The empty Output section makes this example run as a test: registering a
// ServiceDesc whose HandlerType is not a pointer to an interface would panic
// here ("grpc: ServiceDesc.HandlerType needs to be a pointer to an interface
// type").
func Example() {
	srv := grpcserver.NewServer(":9000")

	srv.RegisterService(&demoServiceDesc, &demoService{})

	// Output:
}
