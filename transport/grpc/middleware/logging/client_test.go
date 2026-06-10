package logging

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

// ---------------------------------------------------------------------------
// UnaryClientInterceptor
// ---------------------------------------------------------------------------

func TestUnaryClientInterceptor_Success(t *testing.T) {
	var buf bytes.Buffer
	logger := newTestLogger(&buf)

	interceptor := UnaryClientInterceptor(WithLogger(logger))

	cc, err := grpc.NewClient("passthrough:///localhost:1234", grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	defer cc.Close()

	invoker := func(_ context.Context, _ string, _, _ any, _ *grpc.ClientConn, _ ...grpc.CallOption) error {
		return nil
	}

	err = interceptor(context.Background(), "/pkg.Svc/GetUser", "req", "reply", cc, invoker)
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "grpc unary client rpc")
	assert.Contains(t, output, "/pkg.Svc/GetUser")
	assert.Contains(t, output, "OK")
}

func TestUnaryClientInterceptor_ClientError(t *testing.T) {
	var buf bytes.Buffer
	logger := newTestLogger(&buf)

	interceptor := UnaryClientInterceptor(WithLogger(logger))

	cc, err := grpc.NewClient("passthrough:///localhost:1234", grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	defer cc.Close()

	invoker := func(_ context.Context, _ string, _, _ any, _ *grpc.ClientConn, _ ...grpc.CallOption) error {
		return status.Error(codes.NotFound, "user not found")
	}

	err = interceptor(context.Background(), "/pkg.Svc/GetUser", "req", "reply", cc, invoker)
	require.Error(t, err)

	output := buf.String()
	assert.Contains(t, output, "NotFound")
	assert.Contains(t, output, "user not found")
	// NotFound (code 5) >= NotFound → Warn level
	assert.Contains(t, output, "level=WARN")
}

func TestUnaryClientInterceptor_ServerError(t *testing.T) {
	var buf bytes.Buffer
	logger := newTestLogger(&buf)

	interceptor := UnaryClientInterceptor(WithLogger(logger))

	cc, err := grpc.NewClient("passthrough:///localhost:1234", grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	defer cc.Close()

	invoker := func(_ context.Context, _ string, _, _ any, _ *grpc.ClientConn, _ ...grpc.CallOption) error {
		return status.Error(codes.Internal, "db down")
	}

	err = interceptor(context.Background(), "/pkg.Svc/GetUser", "req", "reply", cc, invoker)
	require.Error(t, err)

	output := buf.String()
	assert.Contains(t, output, "level=ERROR")
	assert.Contains(t, output, "Internal")
}

func TestUnaryClientInterceptor_SkipMethods(t *testing.T) {
	var buf bytes.Buffer
	logger := newTestLogger(&buf)

	interceptor := UnaryClientInterceptor(
		WithLogger(logger),
		WithSkipMethods("/grpc.health.v1.Health/Check"),
	)

	cc, err := grpc.NewClient("passthrough:///localhost:1234", grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	defer cc.Close()

	invoker := func(_ context.Context, _ string, _, _ any, _ *grpc.ClientConn, _ ...grpc.CallOption) error {
		return nil
	}

	err = interceptor(context.Background(), "/grpc.health.v1.Health/Check", nil, nil, cc, invoker)
	require.NoError(t, err)

	// Should NOT be logged.
	assert.False(t, strings.Contains(buf.String(), "grpc unary client rpc"))
}

func TestUnaryClientInterceptor_DefaultLogger(t *testing.T) {
	// Without WithLogger, the interceptor should use log.GetLogger() and not panic.
	interceptor := UnaryClientInterceptor()

	cc, err := grpc.NewClient("passthrough:///localhost:1234", grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	defer cc.Close()

	invoker := func(_ context.Context, _ string, _, _ any, _ *grpc.ClientConn, _ ...grpc.CallOption) error {
		return nil
	}

	err = interceptor(context.Background(), "/pkg.Svc/GetUser", nil, nil, cc, invoker)
	require.NoError(t, err)
}

func TestUnaryClientInterceptor_TargetCaptured(t *testing.T) {
	var buf bytes.Buffer
	logger := newTestLogger(&buf)

	interceptor := UnaryClientInterceptor(WithLogger(logger))

	cc, err := grpc.NewClient("passthrough:///my-server:9999", grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	defer cc.Close()

	invoker := func(_ context.Context, _ string, _, _ any, _ *grpc.ClientConn, _ ...grpc.CallOption) error {
		return nil
	}

	err = interceptor(context.Background(), "/pkg.Svc/GetUser", nil, nil, cc, invoker)
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "target")
}

// ---------------------------------------------------------------------------
// StreamClientInterceptor
// ---------------------------------------------------------------------------

func TestStreamClientInterceptor_Success(t *testing.T) {
	var buf bytes.Buffer
	logger := newTestLogger(&buf)

	interceptor := StreamClientInterceptor(WithLogger(logger))

	cc, err := grpc.NewClient("passthrough:///localhost:1234", grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	defer cc.Close()

	streamer := func(_ context.Context, _ *grpc.StreamDesc, _ *grpc.ClientConn, _ string, _ ...grpc.CallOption) (grpc.ClientStream, error) {
		return nil, nil
	}

	stream, err := interceptor(
		context.Background(),
		&grpc.StreamDesc{ClientStreams: true, ServerStreams: false},
		cc, "/pkg.Svc/StreamData", streamer,
	)
	require.NoError(t, err)
	assert.Nil(t, stream)

	output := buf.String()
	assert.Contains(t, output, "grpc stream client rpc")
	assert.Contains(t, output, "/pkg.Svc/StreamData")
	assert.Contains(t, output, "OK")
}

func TestStreamClientInterceptor_Error(t *testing.T) {
	var buf bytes.Buffer
	logger := newTestLogger(&buf)

	interceptor := StreamClientInterceptor(WithLogger(logger))

	cc, err := grpc.NewClient("passthrough:///localhost:1234", grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	defer cc.Close()

	streamer := func(_ context.Context, _ *grpc.StreamDesc, _ *grpc.ClientConn, _ string, _ ...grpc.CallOption) (grpc.ClientStream, error) {
		return nil, status.Error(codes.Unavailable, "service unavailable")
	}

	stream, err := interceptor(
		context.Background(),
		&grpc.StreamDesc{},
		cc, "/pkg.Svc/StreamData", streamer,
	)
	require.Error(t, err)
	assert.Nil(t, stream)

	output := buf.String()
	assert.Contains(t, output, "Unavailable")
	assert.Contains(t, output, "service unavailable")
}

func TestStreamClientInterceptor_ServerError(t *testing.T) {
	var buf bytes.Buffer
	logger := newTestLogger(&buf)

	interceptor := StreamClientInterceptor(WithLogger(logger))

	cc, err := grpc.NewClient("passthrough:///localhost:1234", grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	defer cc.Close()

	streamer := func(_ context.Context, _ *grpc.StreamDesc, _ *grpc.ClientConn, _ string, _ ...grpc.CallOption) (grpc.ClientStream, error) {
		return nil, status.Error(codes.Internal, "internal error")
	}

	_, err = interceptor(
		context.Background(),
		&grpc.StreamDesc{},
		cc, "/pkg.Svc/StreamData", streamer,
	)
	require.Error(t, err)

	output := buf.String()
	assert.Contains(t, output, "level=ERROR")
	assert.Contains(t, output, "Internal")
}

func TestStreamClientInterceptor_SkipMethods(t *testing.T) {
	var buf bytes.Buffer
	logger := newTestLogger(&buf)

	interceptor := StreamClientInterceptor(
		WithLogger(logger),
		WithSkipMethods("/grpc.health.v1.Health/Watch"),
	)

	cc, err := grpc.NewClient("passthrough:///localhost:1234", grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	defer cc.Close()

	streamer := func(_ context.Context, _ *grpc.StreamDesc, _ *grpc.ClientConn, _ string, _ ...grpc.CallOption) (grpc.ClientStream, error) {
		return nil, nil
	}

	_, err = interceptor(
		context.Background(),
		&grpc.StreamDesc{},
		cc, "/grpc.health.v1.Health/Watch", streamer,
	)
	require.NoError(t, err)

	assert.False(t, strings.Contains(buf.String(), "grpc stream client rpc"))
}

func TestStreamClientInterceptor_StreamDescCaptured(t *testing.T) {
	var buf bytes.Buffer
	logger := newTestLogger(&buf)

	interceptor := StreamClientInterceptor(WithLogger(logger))

	cc, err := grpc.NewClient("passthrough:///localhost:1234", grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	defer cc.Close()

	streamer := func(_ context.Context, _ *grpc.StreamDesc, _ *grpc.ClientConn, _ string, _ ...grpc.CallOption) (grpc.ClientStream, error) {
		return nil, nil
	}

	_, err = interceptor(
		context.Background(),
		&grpc.StreamDesc{ClientStreams: true, ServerStreams: true},
		cc, "/pkg.Svc/BidiStream", streamer,
	)
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "client_stream")
	assert.Contains(t, output, "server_stream")
}
