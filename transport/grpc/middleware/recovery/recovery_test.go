package recovery

import (
	"bytes"
	"context"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pluginsLog "github.com/tx7do/go-wind-plugins/log"
	"github.com/tx7do/go-wind/log"
)

// fakeServerStream provides a minimal grpc.ServerStream with a context.
type fakeServerStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (s *fakeServerStream) Context() context.Context { return s.ctx }

func newTestLogger(buf *bytes.Buffer) log.Logger {
	return pluginsLog.SlogLogger{L: slog.New(slog.NewTextHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug}))}
}

func TestUnaryInterceptor_NoPanic(t *testing.T) {
	interceptor := UnaryInterceptor()

	info := &grpc.UnaryServerInfo{FullMethod: "/pkg.Svc/GetUser"}
	handler := func(_ context.Context, _ any) (any, error) {
		return "ok", nil
	}

	resp, err := interceptor(context.Background(), nil, info, handler)
	require.NoError(t, err)
	assert.Equal(t, "ok", resp)
}

func TestUnaryInterceptor_RecoversPanic(t *testing.T) {
	var buf bytes.Buffer
	logger := newTestLogger(&buf)

	interceptor := UnaryInterceptor(WithLogger(logger))

	info := &grpc.UnaryServerInfo{FullMethod: "/pkg.Svc/GetUser"}
	handler := func(_ context.Context, _ any) (any, error) {
		panic("something went wrong")
	}

	resp, err := interceptor(context.Background(), nil, info, handler)
	require.Error(t, err)
	assert.Nil(t, resp)

	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.Internal, st.Code())

	output := buf.String()
	assert.Contains(t, output, "panic recovered")
	assert.Contains(t, output, "something went wrong")
	assert.Contains(t, output, "stack")
}

func TestUnaryInterceptor_RecoversPanic_NoStack(t *testing.T) {
	var buf bytes.Buffer
	logger := newTestLogger(&buf)

	interceptor := UnaryInterceptor(WithLogger(logger), WithStackTrace(false))

	info := &grpc.UnaryServerInfo{FullMethod: "/pkg.Svc/GetUser"}
	handler := func(_ context.Context, _ any) (any, error) {
		panic("boom")
	}

	_, err := interceptor(context.Background(), nil, info, handler)
	require.Error(t, err)

	output := buf.String()
	assert.Contains(t, output, "panic recovered")
	assert.NotContains(t, output, "goroutine") // stack trace would contain this
}

func TestStreamInterceptor_RecoversPanic(t *testing.T) {
	var buf bytes.Buffer
	logger := newTestLogger(&buf)

	interceptor := StreamInterceptor(WithLogger(logger))

	info := &grpc.StreamServerInfo{FullMethod: "/pkg.Svc/StreamData"}
	handler := func(_ any, _ grpc.ServerStream) error {
		panic("stream boom")
	}

	err := interceptor(nil, &fakeServerStream{ctx: context.Background()}, info, handler)
	require.Error(t, err)

	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.Internal, st.Code())

	assert.Contains(t, buf.String(), "stream boom")
}

func TestStreamInterceptor_NoPanic(t *testing.T) {
	interceptor := StreamInterceptor()

	info := &grpc.StreamServerInfo{FullMethod: "/pkg.Svc/StreamData"}
	handler := func(_ any, _ grpc.ServerStream) error { return nil }

	err := interceptor(nil, &fakeServerStream{ctx: context.Background()}, info, handler)
	require.NoError(t, err)
}
