package timeout

import (
	"context"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestUnaryServerInterceptor_NoExistingDeadline(t *testing.T) {
	intc := UnaryServerInterceptor(100 * time.Millisecond)

	handlerCalled := false
	handler := func(ctx context.Context, req any) (any, error) {
		handlerCalled = true
		_, ok := ctx.Deadline()
		if !ok {
			t.Error("expected deadline in context")
		}
		return "ok", nil
	}

	resp, err := intc(context.Background(), nil, &grpc.UnaryServerInfo{FullMethod: "/test.Svc/Method"}, handler)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !handlerCalled {
		t.Fatal("handler not called")
	}
	if resp != "ok" {
		t.Fatalf("expected 'ok', got %v", resp)
	}
}

func TestUnaryServerInterceptor_ExistingDeadlinePreserved(t *testing.T) {
	intc := UnaryServerInterceptor(100 * time.Millisecond)

	handler := func(ctx context.Context, req any) (any, error) {
		dl, ok := ctx.Deadline()
		if !ok {
			t.Error("expected deadline")
		}
		expected := time.Now().Add(5 * time.Second)
		// The original 5s deadline should be preserved, not the 100ms one.
		if dl.Sub(expected) > 100*time.Millisecond {
			t.Fatalf("expected original deadline preserved, got %v", dl)
		}
		return "ok", nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	intc(ctx, nil, &grpc.UnaryServerInfo{FullMethod: "/test.Svc/Method"}, handler)
}

func TestUnaryServerInterceptor_HandlerTimesOut(t *testing.T) {
	intc := UnaryServerInterceptor(50 * time.Millisecond)

	handler := func(ctx context.Context, req any) (any, error) {
		<-ctx.Done()
		return "ok", nil
	}

	resp, err := intc(context.Background(), nil, &grpc.UnaryServerInfo{FullMethod: "/test.Svc/Method"}, handler)
	if err == nil {
		t.Fatal("expected DeadlineExceeded error")
	}
	if resp != nil {
		t.Fatalf("expected nil resp, got %v", resp)
	}
	st, _ := status.FromError(err)
	if st.Code() != codes.DeadlineExceeded {
		t.Fatalf("expected DeadlineExceeded, got %v", st.Code())
	}
}

func TestUnaryServerInterceptor_SkipFunc(t *testing.T) {
	intc := UnaryServerInterceptor(50*time.Millisecond,
		WithSkipFunc(func(method string) bool { return method == "/test.Svc/Long" }),
	)

	handler := func(ctx context.Context, req any) (any, error) {
		_, ok := ctx.Deadline()
		if ok {
			t.Error("expected no deadline for skipped method")
		}
		return "ok", nil
	}

	intc(context.Background(), nil, &grpc.UnaryServerInfo{FullMethod: "/test.Svc/Long"}, handler)
}

func TestUnaryServerInterceptor_ZeroTimeout(t *testing.T) {
	intc := UnaryServerInterceptor(0)

	handler := func(ctx context.Context, req any) (any, error) {
		_, ok := ctx.Deadline()
		if ok {
			t.Error("expected no deadline when timeout is 0")
		}
		return "ok", nil
	}

	intc(context.Background(), nil, &grpc.UnaryServerInfo{FullMethod: "/test.Svc/Method"}, handler)
}

func TestUnaryClientInterceptor_NoExistingDeadline(t *testing.T) {
	intc := UnaryClientInterceptor(100 * time.Millisecond)

	var capturedHasDeadline bool
	invoker := func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, callOpts ...grpc.CallOption) error {
		_, ok := ctx.Deadline()
		capturedHasDeadline = ok
		return nil
	}

	err := intc(context.Background(), "/test.Svc/Method", nil, nil, nil, invoker)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !capturedHasDeadline {
		t.Fatal("expected deadline in outgoing context")
	}
}

func TestUnaryClientInterceptor_PreservesExistingDeadline(t *testing.T) {
	intc := UnaryClientInterceptor(100 * time.Millisecond)

	var capturedDL time.Time
	invoker := func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, callOpts ...grpc.CallOption) error {
		dl, ok := ctx.Deadline()
		if !ok {
			t.Error("expected deadline")
		}
		capturedDL = dl
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	intc(ctx, "/test.Svc/Method", nil, nil, nil, invoker)

	// The 5s deadline should be preserved, not the 100ms one.
	if time.Until(capturedDL) < 4*time.Second {
		t.Fatalf("expected original deadline preserved, got %v", time.Until(capturedDL))
	}
}

func TestStreamServerInterceptor_AddsDeadline(t *testing.T) {
	intc := StreamServerInterceptor(100 * time.Millisecond)

	var hasDeadline bool
	handler := func(srv any, ss grpc.ServerStream) error {
		_, ok := ss.Context().Deadline()
		hasDeadline = ok
		return nil
	}

	err := intc(nil, &fakeServerStream{ctx: context.Background()}, &grpc.StreamServerInfo{FullMethod: "/test.Svc/StreamMethod"}, handler)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !hasDeadline {
		t.Fatal("expected deadline in stream context")
	}
}

// fakeServerStream is a minimal grpc.ServerStream for testing.
type fakeServerStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (f *fakeServerStream) Context() context.Context { return f.ctx }
