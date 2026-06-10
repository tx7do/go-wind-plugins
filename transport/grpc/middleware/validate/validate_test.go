package validate

import (
	"context"
	"errors"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// testValidMsg implements the Validator interface.
type testValidMsg struct {
	name string
}

func (m *testValidMsg) Validate() error {
	if m.name == "" {
		return errors.New("name is required")
	}
	return nil
}

// testPlainMsg does NOT implement Validator.
type testPlainMsg struct {
	value int
}

func TestUnaryServerInterceptor_ValidMessage(t *testing.T) {
	intc := UnaryServerInterceptor()

	handlerCalled := false
	handler := func(ctx context.Context, req any) (any, error) {
		handlerCalled = true
		return "ok", nil
	}

	resp, err := intc(
		context.Background(),
		&testValidMsg{name: "test"},
		&grpc.UnaryServerInfo{FullMethod: "/test.Svc/Method"},
		handler,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !handlerCalled {
		t.Fatal("handler was not called")
	}
	if resp != "ok" {
		t.Fatalf("expected 'ok', got %v", resp)
	}
}

func TestUnaryServerInterceptor_InvalidMessage(t *testing.T) {
	intc := UnaryServerInterceptor()

	handler := func(ctx context.Context, req any) (any, error) {
		t.Fatal("handler should not be called for invalid message")
		return nil, nil
	}

	resp, err := intc(
		context.Background(),
		&testValidMsg{name: ""},
		&grpc.UnaryServerInfo{FullMethod: "/test.Svc/Method"},
		handler,
	)
	if err == nil {
		t.Fatal("expected error for invalid message")
	}
	if resp != nil {
		t.Fatalf("expected nil resp, got %v", resp)
	}
	st, _ := status.FromError(err)
	if st.Code() != codes.InvalidArgument {
		t.Fatalf("expected InvalidArgument, got %v", st.Code())
	}
}

func TestUnaryServerInterceptor_NonValidatorMessage(t *testing.T) {
	intc := UnaryServerInterceptor()

	handlerCalled := false
	handler := func(ctx context.Context, req any) (any, error) {
		handlerCalled = true
		return "ok", nil
	}

	resp, err := intc(
		context.Background(),
		&testPlainMsg{value: 42},
		&grpc.UnaryServerInfo{FullMethod: "/test.Svc/Method"},
		handler,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !handlerCalled {
		t.Fatal("handler was not called for non-validator message")
	}
	if resp != "ok" {
		t.Fatalf("expected 'ok', got %v", resp)
	}
}

func TestUnaryServerInterceptor_SkipMethod(t *testing.T) {
	intc := UnaryServerInterceptor(WithSkipMethod("/test.Svc/SkipMe"))

	handlerCalled := false
	handler := func(ctx context.Context, req any) (any, error) {
		handlerCalled = true
		return "ok", nil
	}

	// Even with an invalid message, skipped method should pass through.
	intc(
		context.Background(),
		&testValidMsg{name: ""},
		&grpc.UnaryServerInfo{FullMethod: "/test.Svc/SkipMe"},
		handler,
	)

	if !handlerCalled {
		t.Fatal("handler should be called for skipped method even with invalid message")
	}
}

func TestUnaryServerInterceptor_CustomValidator(t *testing.T) {
	intc := UnaryServerInterceptor(
		WithValidator("*validate.testPlainMsg", func(req any) error {
			m, ok := req.(*testPlainMsg)
			if !ok {
				return errors.New("type assertion failed")
			}
			if m.value < 0 {
				return errors.New("value must be non-negative")
			}
			return nil
		}),
	)

	// Valid case
	handler := func(ctx context.Context, req any) (any, error) {
		return "ok", nil
	}
	_, err := intc(
		context.Background(),
		&testPlainMsg{value: 10},
		&grpc.UnaryServerInfo{FullMethod: "/test.Svc/Method"},
		handler,
	)
	if err != nil {
		t.Fatalf("unexpected error for valid custom: %v", err)
	}

	// Invalid case
	_, err = intc(
		context.Background(),
		&testPlainMsg{value: -1},
		&grpc.UnaryServerInfo{FullMethod: "/test.Svc/Method"},
		handler,
	)
	if err == nil {
		t.Fatal("expected error for negative value")
	}
	st, _ := status.FromError(err)
	if st.Code() != codes.InvalidArgument {
		t.Fatalf("expected InvalidArgument, got %v", st.Code())
	}
}

func TestTypeName(t *testing.T) {
	if got := typeName(nil); got != "" {
		t.Fatalf("expected empty for nil, got %q", got)
	}
	if got := typeName(&testPlainMsg{}); got != "*validate.testPlainMsg" {
		t.Fatalf("expected '*validate.testPlainMsg', got %q", got)
	}
}

// fakeServerStream for stream testing.
type fakeServerStream struct {
	grpc.ServerStream
	recvMsgs []any
	recvIdx  int
}

func (s *fakeServerStream) RecvMsg(m any) error {
	// In real gRPC, m is a pointer that we populate. For testing, we just
	// simulate the type assertion and validation flow.
	if s.recvIdx >= len(s.recvMsgs) {
		return errors.New("EOF")
	}
	s.recvIdx++
	return nil
}

func TestStreamServerInterceptor(t *testing.T) {
	intc := StreamServerInterceptor()

	handler := func(srv any, ss grpc.ServerStream) error {
		return nil
	}

	err := intc(
		nil,
		&fakeServerStream{},
		&grpc.StreamServerInfo{FullMethod: "/test.Svc/StreamMethod"},
		handler,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
