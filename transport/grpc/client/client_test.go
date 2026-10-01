package client

import (
	"context"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// nopUnaryInterceptor is a no-op unary client interceptor.
func nopUnaryInterceptor(ctx context.Context, method string, req, reply any,
	cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
	return invoker(ctx, method, req, reply, cc, opts...)
}

// nopStreamInterceptor is a no-op stream client interceptor.
func nopStreamInterceptor(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn,
	method string, streamer grpc.Streamer, opts ...grpc.CallOption) (grpc.ClientStream, error) {
	return streamer(ctx, desc, cc, method, opts...)
}

// ---------------------------------------------------------------------------
// NewClient defaults
// ---------------------------------------------------------------------------

func TestNewClient_Defaults(t *testing.T) {
	c := NewClient("localhost:50051")

	if c.target != "localhost:50051" {
		t.Errorf("target = %q, want %q", c.target, "localhost:50051")
	}
	if c.conn != nil {
		t.Error("conn should be nil before Dial")
	}
	if c.hasCreds {
		t.Error("hasCreds should be false by default")
	}
	if len(c.dialOptions) != 0 {
		t.Errorf("dialOptions length = %d, want 0", len(c.dialOptions))
	}
	if len(c.middlewares) != 0 {
		t.Errorf("middlewares length = %d, want 0", len(c.middlewares))
	}
	if len(c.streamMiddlewares) != 0 {
		t.Errorf("streamMiddlewares length = %d, want 0", len(c.streamMiddlewares))
	}
}

func TestNewClient_TargetAccessor(t *testing.T) {
	c := NewClient("dns:///svc:50051")
	if got := c.Target(); got != "dns:///svc:50051" {
		t.Errorf("Target() = %q, want %q", got, "dns:///svc:50051")
	}
}

func TestNewClient_ConnNilBeforeDial(t *testing.T) {
	c := NewClient("localhost:50051")
	if got := c.Conn(); got != nil {
		t.Errorf("Conn() = %v, want nil before Dial", got)
	}
}

// ---------------------------------------------------------------------------
// Options
// ---------------------------------------------------------------------------

func TestWithDialOption(t *testing.T) {
	c := NewClient("localhost:50051",
		WithDialOption(grpc.WithReturnConnectionError()),
		WithDialOption(grpc.WithAuthority("example.com"), grpc.WithUserAgent("test-agent")),
	)

	if len(c.dialOptions) != 3 {
		t.Errorf("dialOptions length = %d, want 3", len(c.dialOptions))
	}
}

func TestWithInsecure(t *testing.T) {
	c := NewClient("localhost:50051", WithInsecure())

	if !c.hasCreds {
		t.Error("WithInsecure should set hasCreds")
	}
	if len(c.dialOptions) != 1 {
		t.Errorf("dialOptions length = %d, want 1", len(c.dialOptions))
	}
}

func TestWithTransportCredentials(t *testing.T) {
	c := NewClient("localhost:50051", WithTransportCredentials(insecure.NewCredentials()))

	if !c.hasCreds {
		t.Error("WithTransportCredentials should set hasCreds")
	}
	if len(c.dialOptions) != 1 {
		t.Errorf("dialOptions length = %d, want 1", len(c.dialOptions))
	}
}

func TestWithMiddleware(t *testing.T) {
	c := NewClient("localhost:50051", WithMiddleware(nopUnaryInterceptor))

	if len(c.middlewares) != 1 {
		t.Errorf("middlewares length = %d, want 1", len(c.middlewares))
	}
}

func TestWithMiddleware_Appends(t *testing.T) {
	c := NewClient("localhost:50051", WithMiddleware(nopUnaryInterceptor))
	c.opts()(WithMiddleware(nopUnaryInterceptor))

	if len(c.middlewares) != 2 {
		t.Errorf("middlewares length = %d, want 2 after second WithMiddleware", len(c.middlewares))
	}
}

func TestWithStreamMiddleware(t *testing.T) {
	c := NewClient("localhost:50051", WithStreamMiddleware(nopStreamInterceptor))

	if len(c.streamMiddlewares) != 1 {
		t.Errorf("streamMiddlewares length = %d, want 1", len(c.streamMiddlewares))
	}
}

func TestWithConn(t *testing.T) {
	injected, err := grpc.NewClient("localhost:50051", grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("grpc.NewClient returned error: %v", err)
	}
	defer func() { _ = injected.Close() }()

	c := NewClient("localhost:50051", WithConn(injected))

	if c.conn != injected {
		t.Error("WithConn should store the injected connection")
	}
	if got := c.Conn(); got != injected {
		t.Error("Conn() should return the injected connection")
	}
}

// opts returns an Option that applies the given options to c; a small helper
// to exercise option functions outside NewClient.
func (c *Client) opts() func(...Option) {
	return func(os ...Option) {
		for _, o := range os {
			o(c)
		}
	}
}

// ---------------------------------------------------------------------------
// buildDialOptions
// ---------------------------------------------------------------------------

func TestBuildDialOptions_DefaultInsecure(t *testing.T) {
	c := NewClient("localhost:50051")

	opts := c.buildDialOptions()
	if len(opts) != 1 {
		t.Fatalf("buildDialOptions length = %d, want 1 (default insecure)", len(opts))
	}
}

func TestBuildDialOptions_NoDuplicateCreds(t *testing.T) {
	// When credentials are explicitly set, the default insecure option
	// must not be appended again.
	c := NewClient("localhost:50051", WithInsecure())

	opts := c.buildDialOptions()
	if len(opts) != 1 {
		t.Errorf("buildDialOptions length = %d, want 1", len(opts))
	}
}

func TestBuildDialOptions_ChainsMiddlewares(t *testing.T) {
	c := NewClient("localhost:50051",
		WithMiddleware(nopUnaryInterceptor),
		WithStreamMiddleware(nopStreamInterceptor),
	)

	// 1 insecure default + 1 unary chain + 1 stream chain
	opts := c.buildDialOptions()
	if len(opts) != 3 {
		t.Errorf("buildDialOptions length = %d, want 3", len(opts))
	}
}

// ---------------------------------------------------------------------------
// Dial / Close / Conn lifecycle (grpc.NewClient is lazy, no network I/O)
// ---------------------------------------------------------------------------

func TestDial_CreatesConn(t *testing.T) {
	c := NewClient("localhost:50051", WithMiddleware(nopUnaryInterceptor))

	if err := c.Dial(context.Background()); err != nil {
		t.Fatalf("Dial returned error: %v", err)
	}
	if c.conn == nil {
		t.Fatal("Dial should create a connection")
	}
	if got := c.Conn(); got != c.conn {
		t.Error("Conn() should return the dialed connection")
	}

	if err := c.Close(); err != nil {
		t.Errorf("Close returned error: %v", err)
	}
	if c.Conn() != nil {
		t.Error("Conn() should be nil after Close")
	}
}

func TestDial_WithInjectedConnIsNoop(t *testing.T) {
	injected, err := grpc.NewClient("localhost:50051", grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("grpc.NewClient returned error: %v", err)
	}
	defer func() { _ = injected.Close() }()

	c := NewClient("localhost:50051", WithConn(injected))

	if err := c.Dial(context.Background()); err != nil {
		t.Errorf("Dial returned error: %v", err)
	}
	if c.conn != injected {
		t.Error("Dial should keep the injected connection untouched")
	}
}

func TestClose_WithoutConn(t *testing.T) {
	c := NewClient("localhost:50051")

	if err := c.Close(); err != nil {
		t.Errorf("Close without a connection should be a no-op, got error: %v", err)
	}
}

func TestDial_TimeoutContext(t *testing.T) {
	// grpc.NewClient is non-blocking, so even a tiny timeout must succeed.
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	c := NewClient("localhost:50051", WithInsecure())
	if err := c.Dial(ctx); err != nil {
		t.Errorf("Dial returned error: %v", err)
	}
	_ = c.Close()
}
