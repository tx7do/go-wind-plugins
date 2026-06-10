package metadata

import (
	"context"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

func TestUnaryServerInterceptor_ExtractsKeys(t *testing.T) {
	intc := UnaryServerInterceptor(WithKeys("x-tenant-id", "x-user-id"))

	var tenantID, userID string
	handler := func(ctx context.Context, req any) (any, error) {
		tenantID = FromContext(ctx, "x-tenant-id")
		userID = FromContext(ctx, "x-user-id")
		return "ok", nil
	}

	ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs(
		"x-tenant-id", "acme",
		"x-user-id", "user-123",
	))
	intc(ctx, nil, &grpc.UnaryServerInfo{FullMethod: "/test.Svc/Method"}, handler)

	if tenantID != "acme" {
		t.Fatalf("expected tenant 'acme', got %q", tenantID)
	}
	if userID != "user-123" {
		t.Fatalf("expected user 'user-123', got %q", userID)
	}
}

func TestUnaryServerInterceptor_MissingKey(t *testing.T) {
	intc := UnaryServerInterceptor(WithKeys("x-tenant-id"))

	var tenantID string
	handler := func(ctx context.Context, req any) (any, error) {
		tenantID = FromContext(ctx, "x-tenant-id")
		return "ok", nil
	}

	// No metadata at all.
	intc(context.Background(), nil, &grpc.UnaryServerInfo{FullMethod: "/test.Svc/Method"}, handler)

	if tenantID != "" {
		t.Fatalf("expected empty for missing key, got %q", tenantID)
	}
}

func TestUnaryServerInterceptor_PartialKeys(t *testing.T) {
	intc := UnaryServerInterceptor(WithKeys("x-tenant-id", "x-user-id"))

	var tenantID, userID string
	handler := func(ctx context.Context, req any) (any, error) {
		tenantID = FromContext(ctx, "x-tenant-id")
		userID = FromContext(ctx, "x-user-id")
		return "ok", nil
	}

	// Only tenant-id is present.
	ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs("x-tenant-id", "acme"))
	intc(ctx, nil, &grpc.UnaryServerInfo{FullMethod: "/test.Svc/Method"}, handler)

	if tenantID != "acme" {
		t.Fatalf("expected 'acme', got %q", tenantID)
	}
	if userID != "" {
		t.Fatalf("expected empty for missing user-id, got %q", userID)
	}
}

func TestUnaryClientInterceptor_InjectsFromContext(t *testing.T) {
	intc := UnaryClientInterceptor(WithKeys("x-tenant-id"))

	var capturedMD metadata.MD
	invoker := func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, callOpts ...grpc.CallOption) error {
		capturedMD, _ = metadata.FromOutgoingContext(ctx)
		return nil
	}

	ctx := WithMetadata(context.Background(), "x-tenant-id", "acme")
	intc(ctx, "/test.Svc/Method", nil, nil, nil, invoker)

	values := capturedMD.Get("x-tenant-id")
	if len(values) == 0 || values[0] != "acme" {
		t.Fatalf("expected outgoing metadata 'acme', got %v", values)
	}
}

func TestUnaryClientInterceptor_ConstantMetadata(t *testing.T) {
	intc := UnaryClientInterceptor(
		WithConstant(map[string]string{"x-service": "orders-api", "x-version": "v2"}),
	)

	var capturedMD metadata.MD
	invoker := func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, callOpts ...grpc.CallOption) error {
		capturedMD, _ = metadata.FromOutgoingContext(ctx)
		return nil
	}

	intc(context.Background(), "/test.Svc/Method", nil, nil, nil, invoker)

	if v := capturedMD.Get("x-service"); len(v) == 0 || v[0] != "orders-api" {
		t.Fatalf("expected constant metadata 'orders-api', got %v", v)
	}
	if v := capturedMD.Get("x-version"); len(v) == 0 || v[0] != "v2" {
		t.Fatalf("expected constant metadata 'v2', got %v", v)
	}
}

func TestUnaryClientInterceptor_ContextAndConstant(t *testing.T) {
	intc := UnaryClientInterceptor(
		WithKeys("x-tenant-id"),
		WithConstant(map[string]string{"x-service": "api"}),
	)

	var capturedMD metadata.MD
	invoker := func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, callOpts ...grpc.CallOption) error {
		capturedMD, _ = metadata.FromOutgoingContext(ctx)
		return nil
	}

	ctx := WithMetadata(context.Background(), "x-tenant-id", "acme")
	intc(ctx, "/test.Svc/Method", nil, nil, nil, invoker)

	if v := capturedMD.Get("x-tenant-id"); len(v) == 0 || v[0] != "acme" {
		t.Fatalf("expected context metadata 'acme', got %v", v)
	}
	if v := capturedMD.Get("x-service"); len(v) == 0 || v[0] != "api" {
		t.Fatalf("expected constant metadata 'api', got %v", v)
	}
}

func TestStreamServerInterceptor(t *testing.T) {
	intc := StreamServerInterceptor(WithKeys("x-tenant-id"))

	var tenantID string
	handler := func(srv any, ss grpc.ServerStream) error {
		tenantID = FromContext(ss.Context(), "x-tenant-id")
		return nil
	}

	ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs("x-tenant-id", "acme"))
	ss := &fakeServerStream{ctx: ctx}

	err := intc(nil, ss, &grpc.StreamServerInfo{FullMethod: "/test.Svc/Stream"}, handler)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tenantID != "acme" {
		t.Fatalf("expected 'acme', got %q", tenantID)
	}
}

func TestStreamClientInterceptor(t *testing.T) {
	intc := StreamClientInterceptor(WithKeys("x-trace-id"))

	var capturedMD metadata.MD
	streamer := func(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn, method string, callOpts ...grpc.CallOption) (grpc.ClientStream, error) {
		capturedMD, _ = metadata.FromOutgoingContext(ctx)
		return nil, nil
	}

	ctx := WithMetadata(context.Background(), "x-trace-id", "trace-789")
	intc(ctx, &grpc.StreamDesc{}, nil, "/test.Svc/Stream", streamer)

	if v := capturedMD.Get("x-trace-id"); len(v) == 0 || v[0] != "trace-789" {
		t.Fatalf("expected 'trace-789', got %v", v)
	}
}

func TestFromContext_NilCtx(t *testing.T) {
	if v := FromContext(nil, "x-key"); v != "" {
		t.Fatalf("expected empty, got %q", v)
	}
}

func TestFromContext_NotPresent(t *testing.T) {
	if v := FromContext(context.Background(), "x-key"); v != "" {
		t.Fatalf("expected empty, got %q", v)
	}
}

// fakeServerStream for stream testing.
type fakeServerStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (f *fakeServerStream) Context() context.Context { return f.ctx }
