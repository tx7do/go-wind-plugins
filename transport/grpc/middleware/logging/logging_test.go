package logging

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// fakeServerStream provides a minimal grpc.ServerStream with a context.
type fakeServerStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (s *fakeServerStream) Context() context.Context { return s.ctx }

func newTestLogger(buf *bytes.Buffer) *slog.Logger {
	return slog.New(slog.NewTextHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
}

func TestUnaryInterceptor_Success(t *testing.T) {
	var buf bytes.Buffer
	logger := newTestLogger(&buf)

	interceptor := UnaryInterceptor(WithLogger(logger))

	info := &grpc.UnaryServerInfo{FullMethod: "/pkg.Svc/GetUser"}
	handler := func(_ context.Context, _ any) (any, error) {
		return "ok", nil
	}

	resp, err := interceptor(context.Background(), nil, info, handler)
	require.NoError(t, err)
	assert.Equal(t, "ok", resp)

	output := buf.String()
	assert.Contains(t, output, "grpc unary rpc")
	assert.Contains(t, output, "/pkg.Svc/GetUser")
	assert.Contains(t, output, "OK")
}

func TestUnaryInterceptor_Error(t *testing.T) {
	var buf bytes.Buffer
	logger := newTestLogger(&buf)

	interceptor := UnaryInterceptor(WithLogger(logger))

	info := &grpc.UnaryServerInfo{FullMethod: "/pkg.Svc/GetUser"}
	handler := func(_ context.Context, _ any) (any, error) {
		return nil, status.Error(codes.NotFound, "user not found")
	}

	_, err := interceptor(context.Background(), nil, info, handler)
	require.Error(t, err)

	output := buf.String()
	assert.Contains(t, output, "NotFound")
	assert.Contains(t, output, "user not found")
}

func TestUnaryInterceptor_ServerError(t *testing.T) {
	var buf bytes.Buffer
	logger := newTestLogger(&buf)

	interceptor := UnaryInterceptor(WithLogger(logger))

	info := &grpc.UnaryServerInfo{FullMethod: "/pkg.Svc/GetUser"}
	handler := func(_ context.Context, _ any) (any, error) {
		return nil, status.Error(codes.Internal, "db down")
	}

	_, err := interceptor(context.Background(), nil, info, handler)
	require.Error(t, err)

	output := buf.String()
	assert.Contains(t, output, "level=ERROR")
	assert.Contains(t, output, "Internal")
}

func TestUnaryInterceptor_SkipMethods(t *testing.T) {
	var buf bytes.Buffer
	logger := newTestLogger(&buf)

	interceptor := UnaryInterceptor(
		WithLogger(logger),
		WithSkipMethods("/grpc.health.v1.Health/Check"),
	)

	info := &grpc.UnaryServerInfo{FullMethod: "/grpc.health.v1.Health/Check"}
	handler := func(_ context.Context, _ any) (any, error) {
		return "ok", nil
	}

	resp, err := interceptor(context.Background(), nil, info, handler)
	require.NoError(t, err)
	assert.Equal(t, "ok", resp)

	// Should NOT be logged.
	assert.False(t, strings.Contains(buf.String(), "grpc unary rpc"))
}

func TestStreamInterceptor_Success(t *testing.T) {
	var buf bytes.Buffer
	logger := newTestLogger(&buf)

	interceptor := StreamInterceptor(WithLogger(logger))

	info := &grpc.StreamServerInfo{
		FullMethod:     "/pkg.Svc/StreamData",
		IsClientStream: false,
		IsServerStream: true,
	}
	handler := func(_ any, _ grpc.ServerStream) error { return nil }

	err := interceptor(nil, &fakeServerStream{ctx: context.Background()}, info, handler)
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "grpc stream rpc")
	assert.Contains(t, output, "/pkg.Svc/StreamData")
	assert.Contains(t, output, "OK")
}
