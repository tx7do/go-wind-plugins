package server

import (
	"context"
	"errors"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/emptypb"
)

// ---------------------------------------------------------------------------
// Test service: a hand-written ServiceDesc whose Ping behavior is injectable.
// ---------------------------------------------------------------------------

type demoService struct {
	ping func(ctx context.Context, e *emptypb.Empty) (*emptypb.Empty, error)
}

func (d *demoService) Ping(ctx context.Context, e *emptypb.Empty) (*emptypb.Empty, error) {
	return d.ping(ctx, e)
}

// demoServer is the service interface; grpc.ServiceDesc.HandlerType must be
// a pointer to an interface type (as in generated code).
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

// startTestServer spins a Server on 127.0.0.1 with a random port and returns
// it together with the actual endpoint (grpc://host:port). Bound by 3s.
func startTestServer(t *testing.T, s *Server) string {
	t.Helper()

	errChan := make(chan error, 1)
	go func() { errChan <- s.Start(context.Background()) }()
	t.Cleanup(func() {
		stopCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = s.Stop(stopCtx)
		select {
		case <-errChan:
		case <-time.After(3 * time.Second):
			t.Error("Start did not return within 3s after Stop")
		}
	})

	deadline := time.Now().Add(3 * time.Second)
	for {
		if s.listener != nil {
			return s.Endpoint()
		}
		if time.Now().After(deadline) {
			t.Fatal("server did not start listening within 3s")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// dial connects a client to the endpoint, retrying until the server accepts
// RPCs (bounded by 3s). A direct context dialer is used to bypass any proxy
// configured via environment variables.
func dial(t *testing.T, endpoint string) *grpc.ClientConn {
	t.Helper()

	// Endpoint() carries the informational "grpc://" scheme; the client
	// wants the bare host:port target.
	cc, err := grpc.NewClient(strings.TrimPrefix(endpoint, "grpc://"),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithContextDialer(func(ctx context.Context, addr string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, "tcp", addr)
		}),
	)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	t.Cleanup(func() { _ = cc.Close() })

	deadline := time.Now().Add(3 * time.Second)
	for {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		err := cc.Invoke(ctx, "/demo.DemoService/Ping", &emptypb.Empty{}, &emptypb.Empty{})
		cancel()
		if err == nil {
			return cc
		}
		if time.Now().After(deadline) {
			t.Fatalf("server not reachable within 3s: %v", err)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// ---------------------------------------------------------------------------
// Construction, options, simple accessors
// ---------------------------------------------------------------------------

func TestNewServer_Defaults(t *testing.T) {
	s := NewServer(":9000")
	if s.Addr() != ":9000" {
		t.Errorf("Addr() = %q, want :9000", s.Addr())
	}
	if s.Server() != nil {
		t.Error("underlying grpc.Server should be lazy until needed")
	}
	if got, want := s.Endpoint(), "grpc://localhost:9000"; got != want {
		t.Errorf("Endpoint() = %q, want %q", got, want)
	}
}

func TestEndpoint_AddressVariants(t *testing.T) {
	cases := []struct{ addr, want string }{
		{":9000", "grpc://localhost:9000"},
		{"0.0.0.0:80", "grpc://localhost:80"},
		{"127.0.0.1:9090", "grpc://127.0.0.1:9090"},
		{"[::1]:9090", "grpc://[::1]:9090"},
		{"no-port-here", "no-port-here"}, // unparseable: returned as-is
	}
	for _, tc := range cases {
		if got := NewServer(tc.addr).Endpoint(); got != tc.want {
			t.Errorf("NewServer(%q).Endpoint() = %q, want %q", tc.addr, got, tc.want)
		}
	}
}

func TestOptions(t *testing.T) {
	custom := grpc.NewServer()
	mw := func(ctx context.Context, req any, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		return handler(ctx, req)
	}
	smw := func(srv any, ss grpc.ServerStream, _ *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		return handler(srv, ss)
	}

	s := NewServer(":9000",
		WithServer(custom),
		WithMiddleware(mw),
		WithMiddleware(mw, mw),
		WithStreamMiddleware(smw),
		WithTimeout(7*time.Second),
	)

	if s.Server() != custom {
		t.Error("WithServer should wire the provided *grpc.Server")
	}
	if len(s.middlewares) != 3 {
		t.Errorf("middlewares = %d, want 3", len(s.middlewares))
	}
	if len(s.streamMiddleware) != 1 {
		t.Errorf("stream middlewares = %d, want 1", len(s.streamMiddleware))
	}
	if s.timeout != 7*time.Second {
		t.Errorf("timeout = %v, want 7s", s.timeout)
	}
}

func TestFormatEndpoint(t *testing.T) {
	if got, want := FormatEndpoint("127.0.0.1", 9000), "grpc://127.0.0.1:9000"; got != want {
		t.Errorf("FormatEndpoint = %q, want %q", got, want)
	}
}

// ---------------------------------------------------------------------------
// RegisterService lazily creates the grpc.Server
// ---------------------------------------------------------------------------

func TestRegisterService_LazilyCreatesServer(t *testing.T) {
	s := NewServer(":0")
	if s.Server() != nil {
		t.Fatal("Server() should be nil before registration")
	}
	s.RegisterService(&demoServiceDesc, &demoService{})
	if s.Server() == nil {
		t.Fatal("RegisterService should create the underlying grpc.Server")
	}
}

// ---------------------------------------------------------------------------
// Chain
// ---------------------------------------------------------------------------

func TestChain_EmptyReturnsNil(t *testing.T) {
	if got := Chain(); got != nil {
		t.Errorf("Chain() = %v, want nil", got)
	}
}

func TestChain_SingleReturnsSame(t *testing.T) {
	called := false
	mw := func(ctx context.Context, req any, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		called = true
		return handler(ctx, req)
	}
	got := Chain(mw)
	if got == nil {
		t.Fatal("Chain(mw) should return mw itself")
	}
	_, err := got(context.Background(), "req", &grpc.UnaryServerInfo{}, func(ctx context.Context, req any) (any, error) {
		return "resp", nil
	})
	if err != nil {
		t.Fatalf("interceptor error: %v", err)
	}
	if !called {
		t.Error("single middleware should be invoked unchanged")
	}
}

func TestChain_MultipleRunsInOrder(t *testing.T) {
	var order []string
	mk := func(name string) Middleware {
		return func(ctx context.Context, req any, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
			order = append(order, name+"-before")
			resp, err := handler(ctx, req)
			order = append(order, name+"-after")
			return resp, err
		}
	}

	got := Chain(mk("a"), mk("b"), mk("c"))
	resp, err := got(context.Background(), "req", &grpc.UnaryServerInfo{}, func(ctx context.Context, req any) (any, error) {
		order = append(order, "handler")
		return req, nil
	})
	if err != nil {
		t.Fatalf("interceptor error: %v", err)
	}
	if resp != "req" {
		t.Errorf("resp = %v, want req", resp)
	}
	want := []string{"a-before", "b-before", "c-before", "handler", "c-after", "b-after", "a-after"}
	if len(order) != len(want) {
		t.Fatalf("order = %v, want %v", order, want)
	}
	for i := range want {
		if order[i] != want[i] {
			t.Fatalf("order = %v, want %v", order, want)
		}
	}
}

// ---------------------------------------------------------------------------
// Start / Stop: happy path with a real unary RPC over 127.0.0.1
// ---------------------------------------------------------------------------

func TestStartServeStop_HappyPath(t *testing.T) {
	s := NewServer("127.0.0.1:0")
	s.RegisterService(&demoServiceDesc, &demoService{
		ping: func(ctx context.Context, e *emptypb.Empty) (*emptypb.Empty, error) { return e, nil },
	})

	endpoint := startTestServer(t, s)
	// Random port resolved in the advertised endpoint.
	if endpoint == "grpc://localhost:0" || endpoint == "grpc://127.0.0.1:0" {
		t.Fatalf("Endpoint() = %q, want resolved random port", endpoint)
	}

	cc := dial(t, endpoint)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := cc.Invoke(ctx, "/demo.DemoService/Ping", &emptypb.Empty{}, &emptypb.Empty{}); err != nil {
		t.Errorf("Ping RPC failed: %v", err)
	}

	// Graceful stop with no in-flight work returns nil and unblocks Start.
	stopCtx, stopCancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer stopCancel()
	if err := s.Stop(stopCtx); err != nil {
		t.Errorf("Stop = %v, want nil", err)
	}
}

func TestStart_ListenError(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	s := NewServer(ln.Addr().String())
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := s.Start(ctx); err == nil {
		t.Error("Start on an occupied address should fail")
	}
}

func TestStop_NilServerReturnsNil(t *testing.T) {
	s := NewServer(":9000")
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := s.Stop(ctx); err != nil {
		t.Errorf("Stop without a grpc.Server = %v, want nil", err)
	}
}

// Stop with an expired context must force-stop the in-flight RPC and return
// the context error (covers the GracefulStop-timeout branch).
func TestStop_ContextTimeoutForceStops(t *testing.T) {
	release := make(chan struct{})
	entered := make(chan struct{})
	var once sync.Once

	s := NewServer("127.0.0.1:0")
	s.RegisterService(&demoServiceDesc, &demoService{
		ping: func(ctx context.Context, e *emptypb.Empty) (*emptypb.Empty, error) {
			once.Do(func() { close(entered) })
			<-release
			return e, nil
		},
	})

	// Bring the server up without issuing any RPC: the Ping handler blocks,
	// so the probe-based dial helper must not be used here.
	errChan := make(chan error, 1)
	go func() { errChan <- s.Start(context.Background()) }()
	deadline := time.Now().Add(3 * time.Second)
	for s.listener == nil {
		if time.Now().After(deadline) {
			t.Fatal("server did not start listening within 3s")
		}
		time.Sleep(5 * time.Millisecond)
	}

	cc, err := grpc.NewClient(strings.TrimPrefix(s.Endpoint(), "grpc://"),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithContextDialer(func(ctx context.Context, addr string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, "tcp", addr)
		}),
	)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	t.Cleanup(func() {
		_ = cc.Close()
		close(release)
		select {
		case <-errChan:
		case <-time.After(3 * time.Second):
			t.Error("Start did not return within 3s")
		}
	})

	// Fire one RPC that blocks inside the handler.
	rpcDone := make(chan error, 1)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		rpcDone <- cc.Invoke(ctx, "/demo.DemoService/Ping", &emptypb.Empty{}, &emptypb.Empty{})
	}()
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("handler not entered within 3s")
	}

	// GracefulStop cannot finish while the handler blocks -> ctx timeout.
	stopCtx, stopCancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer stopCancel()
	err = s.Stop(stopCtx)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("Stop with expired ctx = %v, want DeadlineExceeded", err)
	}

	// The force stop unblocks the RPC; its result is irrelevant.
	select {
	case <-rpcDone:
	case <-time.After(3 * time.Second):
		t.Error("RPC did not finish within 3s after force stop")
	}
}
