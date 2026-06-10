// Package main demonstrates the go-wind [App] abstraction by running an HTTP
// server and a gRPC server simultaneously under a single lifecycle manager.
//
// The [wind.App] handles:
//   - Concurrent server startup (errgroup)
//   - OS signal trapping (SIGINT/SIGTERM/SIGQUIT) → graceful shutdown
//   - Lifecycle hooks: BeforeStop → Server.Stop → AfterStop
//   - Startup banner with instance metadata
//
// Compare this to http-basic and grpc-basic which each manage signals manually.
// With App, there is zero signal-handling boilerplate in user code.
//
// Run:
//
//	go run ./examples/app-multi-server
//
// Test:
//
//	# HTTP
//	curl http://localhost:8080/hello
//
//	# gRPC
//	grpcurl -plaintext -d '{"message":"hi"}' localhost:9000 echo.EchoService/Say
package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/tx7do/go-wind"
	"github.com/tx7do/go-wind-plugins/transport/grpc/middleware/logging"
	grpcRecovery "github.com/tx7do/go-wind-plugins/transport/grpc/middleware/recovery"
	grpcServer "github.com/tx7do/go-wind-plugins/transport/grpc/server"
	httpServer "github.com/tx7do/go-wind-plugins/transport/http"
	"github.com/tx7do/go-wind-plugins/transport/http/driver/std"
	"github.com/tx7do/go-wind-plugins/transport/http/middleware/recovery"
	"github.com/tx7do/go-wind-plugins/transport/http/middleware/requestid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// ---------------------------------------------------------------------------
// gRPC Echo service (same as grpc-basic example, kept self-contained)
// ---------------------------------------------------------------------------

type echoRequest struct {
	Message string `protobuf:"bytes,1,opt,name=message,proto3" json:"message,omitempty"`
}

func (m *echoRequest) Reset()         { *m = echoRequest{} }
func (m *echoRequest) String() string { return fmt.Sprintf("{message:%q}", m.Message) }
func (m *echoRequest) ProtoMessage()  {}

type echoResponse struct {
	Message string `protobuf:"bytes,1,opt,name=message,proto3" json:"message,omitempty"`
}

func (m *echoResponse) Reset()         { *m = echoResponse{} }
func (m *echoResponse) String() string { return fmt.Sprintf("{message:%q}", m.Message) }
func (m *echoResponse) ProtoMessage()  {}

type echoServiceImpl struct{}

func (s *echoServiceImpl) Say(_ context.Context, req *echoRequest) (*echoResponse, error) {
	if req.Message == "" {
		return nil, status.Error(codes.InvalidArgument, "message is required")
	}
	return &echoResponse{Message: "echo: " + req.Message}, nil
}

func _echoSayHandler(srv any, _ context.Context, dec func(any) error, _ grpc.UnaryServerInterceptor) (any, error) {
	req := &echoRequest{}
	if err := dec(req); err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	return srv.(*echoServiceImpl).Say(context.Background(), req)
}

var echoServiceDesc = grpc.ServiceDesc{
	ServiceName: "echo.EchoService",
	HandlerType: (*echoServiceImpl)(nil),
	Methods: []grpc.MethodDesc{
		{MethodName: "Say", Handler: _echoSayHandler},
	},
	Streams:  []grpc.StreamDesc{},
	Metadata: "echo.proto",
}

// ---------------------------------------------------------------------------
// main
// ---------------------------------------------------------------------------

func main() {
	// 1. HTTP server (port 8080)
	httpSrv := httpServer.NewServer(":8080",
		httpServer.WithDriver(std.NewDriver()),
	)
	httpSrv.Use(
		recovery.Middleware(),
		requestid.Middleware(),
	)
	httpSrv.GET("/hello", func(w http.ResponseWriter, r *http.Request) {
		id := requestid.FromContext(r.Context())
		fmt.Fprintf(w, "Hello from multi-server app! (request-id: %s)\n", id)
	})

	// 2. gRPC server (port 9000)
	grpcSrv := grpcServer.NewServer(":9000",
		grpcServer.WithMiddleware(
			grpcRecovery.UnaryInterceptor(),
			logging.UnaryInterceptor(),
		),
	)
	grpcSrv.RegisterService(&echoServiceDesc, &echoServiceImpl{})

	// 3. Assemble the App — one place to manage all servers.
	app := wind.New(
		wind.WithID("multi-server-demo"),
		wind.WithName("multi-server-app"),
		wind.WithVersion("1.0.0"),
		wind.WithServer(httpSrv, grpcSrv),
		wind.WithStopTimeout(15*time.Second),
		// BeforeStop: runs BEFORE any server stops.
		// Typical use: deregister from service registry, drain request queue.
		wind.WithBeforeStop(func(_ context.Context) error {
			fmt.Println("[hook] beforeStop: preparing for graceful shutdown...")
			return nil
		}),
		// AfterStop: runs AFTER all servers have stopped.
		// Typical use: close database connections, flush log buffers.
		wind.WithAfterStop(func(_ context.Context) error {
			fmt.Println("[hook] afterStop: cleanup complete, exiting.")
			return nil
		}),
	)

	fmt.Println("Starting multi-server application...")
	fmt.Printf("  HTTP:  %s\n", httpSrv.Endpoint())
	fmt.Printf("  gRPC:  %s\n", grpcSrv.Endpoint())
	fmt.Println()

	// App.Run blocks until SIGINT/SIGTERM/SIGQUIT or context cancellation.
	// No manual signal.NotifyContext needed!
	if err := app.Run(context.Background()); err != nil {
		fmt.Fprintf(os.Stderr, "application exited with error: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("application stopped gracefully")
}
