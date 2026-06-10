package requestid

import (
	"context"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

func TestUnaryServerInterceptor_ExtractsFromMetadata(t *testing.T) {
	intc := UnaryServerInterceptor()

	var capturedID string
	handler := func(ctx context.Context, req any) (any, error) {
		capturedID = FromContext(ctx)
		return "ok", nil
	}

	ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs(MetadataKey, "req-123"))
	intc(ctx, nil, &grpc.UnaryServerInfo{FullMethod: "/test.Svc/Method"}, handler)

	if capturedID != "req-123" {
		t.Fatalf("expected 'req-123', got %q", capturedID)
	}
}

func TestUnaryServerInterceptor_GeneratesID(t *testing.T) {
	intc := UnaryServerInterceptor()

	var capturedID string
	handler := func(ctx context.Context, req any) (any, error) {
		capturedID = FromContext(ctx)
		return "ok", nil
	}

	intc(context.Background(), nil, &grpc.UnaryServerInfo{FullMethod: "/test.Svc/Method"}, handler)

	if capturedID == "" {
		t.Fatal("expected a generated ID, got empty string")
	}
	if len(capturedID) != 32 {
		t.Fatalf("expected 32-char hex ID, got %d chars: %q", len(capturedID), capturedID)
	}
}

func TestUnaryServerInterceptor_CustomGenerator(t *testing.T) {
	intc := UnaryServerInterceptor(WithIDGenerator(func() string { return "custom-id" }))

	var capturedID string
	handler := func(ctx context.Context, req any) (any, error) {
		capturedID = FromContext(ctx)
		return "ok", nil
	}

	intc(context.Background(), nil, &grpc.UnaryServerInfo{FullMethod: "/test.Svc/Method"}, handler)

	if capturedID != "custom-id" {
		t.Fatalf("expected 'custom-id', got %q", capturedID)
	}
}

func TestUnaryClientInterceptor_InjectsFromContext(t *testing.T) {
	intc := UnaryClientInterceptor()

	var capturedMD metadata.MD
	invoker := func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, callOpts ...grpc.CallOption) error {
		capturedMD, _ = metadata.FromOutgoingContext(ctx)
		return nil
	}

	ctx := WithRequestID(context.Background(), "outbound-id")
	intc(ctx, "/test.Svc/Method", nil, nil, nil, invoker)

	values := capturedMD.Get(MetadataKey)
	if len(values) == 0 || values[0] != "outbound-id" {
		t.Fatalf("expected outgoing metadata with 'outbound-id', got %v", values)
	}
}

func TestUnaryClientInterceptor_GeneratesIfMissing(t *testing.T) {
	intc := UnaryClientInterceptor()

	var capturedMD metadata.MD
	invoker := func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, callOpts ...grpc.CallOption) error {
		capturedMD, _ = metadata.FromOutgoingContext(ctx)
		return nil
	}

	intc(context.Background(), "/test.Svc/Method", nil, nil, nil, invoker)

	values := capturedMD.Get(MetadataKey)
	if len(values) == 0 || values[0] == "" {
		t.Fatalf("expected a generated ID in outgoing metadata, got %v", values)
	}
}

func TestStreamServerInterceptor_ExtractsID(t *testing.T) {
	intc := StreamServerInterceptor()

	var capturedID string
	handler := func(srv any, ss grpc.ServerStream) error {
		capturedID = FromContext(ss.Context())
		return nil
	}

	ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs(MetadataKey, "stream-req-id"))
	ss := &fakeServerStream{ctx: ctx}

	err := intc(nil, ss, &grpc.StreamServerInfo{FullMethod: "/test.Svc/StreamMethod"}, handler)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedID != "stream-req-id" {
		t.Fatalf("expected 'stream-req-id', got %q", capturedID)
	}
}

func TestFromContext_Empty(t *testing.T) {
	if id := FromContext(context.Background()); id != "" {
		t.Fatalf("expected empty, got %q", id)
	}
}

func TestStreamClientInterceptor_InjectsID(t *testing.T) {
	intc := StreamClientInterceptor()

	var capturedMD metadata.MD
	streamer := func(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn, method string, callOpts ...grpc.CallOption) (grpc.ClientStream, error) {
		capturedMD, _ = metadata.FromOutgoingContext(ctx)
		return nil, nil
	}

	ctx := WithRequestID(context.Background(), "client-stream-id")
	intc(ctx, &grpc.StreamDesc{}, nil, "/test.Svc/StreamMethod", streamer)

	values := capturedMD.Get(MetadataKey)
	if len(values) == 0 || values[0] != "client-stream-id" {
		t.Fatalf("expected 'client-stream-id', got %v", values)
	}
}

// fakeServerStream is a minimal grpc.ServerStream for testing.
type fakeServerStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (f *fakeServerStream) Context() context.Context { return f.ctx }
