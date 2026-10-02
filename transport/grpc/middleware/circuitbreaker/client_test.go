package circuitbreaker

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/tx7do/go-wind-plugins/circuitbreaker"
)

// ---------------------------------------------------------------------------
// UnaryClientInterceptor
// ---------------------------------------------------------------------------

func TestUnaryClient_Success(t *testing.T) {
	cb := &fakeBreaker{}

	var invokerCalled bool
	invoker := func(_ context.Context, _ string, _, _ any, _ *grpc.ClientConn, _ ...grpc.CallOption) error {
		invokerCalled = true
		return nil
	}

	err := UnaryClientInterceptor(cb)(context.Background(), "/pkg.Svc/Get", nil, nil, nil, invoker)
	require.NoError(t, err)
	assert.True(t, invokerCalled)
	assert.Equal(t, 1, cb.successN)
	assert.Equal(t, 0, cb.failureN)
}

func TestUnaryClient_Failure_InternalError(t *testing.T) {
	cb := &fakeBreaker{}

	invoker := func(_ context.Context, _ string, _, _ any, _ *grpc.ClientConn, _ ...grpc.CallOption) error {
		return status.Error(codes.Internal, "upstream boom")
	}

	err := UnaryClientInterceptor(cb)(context.Background(), "/pkg.Svc/Get", nil, nil, nil, invoker)
	require.Error(t, err)
	assert.Equal(t, 1, cb.failureN)
	assert.Equal(t, 0, cb.successN)
}

func TestUnaryClient_NonFailure_NotFound(t *testing.T) {
	cb := &fakeBreaker{}

	invoker := func(_ context.Context, _ string, _, _ any, _ *grpc.ClientConn, _ ...grpc.CallOption) error {
		return status.Error(codes.NotFound, "nope")
	}

	err := UnaryClientInterceptor(cb)(context.Background(), "/pkg.Svc/Get", nil, nil, nil, invoker)
	require.Error(t, err)
	// NotFound < Internal → counted as success.
	assert.Equal(t, 1, cb.successN)
	assert.Equal(t, 0, cb.failureN)
}

func TestUnaryClient_CircuitOpen(t *testing.T) {
	cb := &fakeBreaker{allowErr: circuitbreaker.ErrCircuitOpen}

	invoker := func(_ context.Context, _ string, _, _ any, _ *grpc.ClientConn, _ ...grpc.CallOption) error {
		t.Fatal("invoker should not be called")
		return nil
	}

	err := UnaryClientInterceptor(cb)(context.Background(), "/pkg.Svc/Get", nil, nil, nil, invoker)
	require.Error(t, err)
	st, _ := status.FromError(err)
	assert.Equal(t, codes.Unavailable, st.Code())
	assert.Equal(t, 0, cb.successN)
	assert.Equal(t, 0, cb.failureN)
}

func TestUnaryClient_CustomFailureCodes(t *testing.T) {
	cb := &fakeBreaker{}

	invoker := func(_ context.Context, _ string, _, _ any, _ *grpc.ClientConn, _ ...grpc.CallOption) error {
		return status.Error(codes.DeadlineExceeded, "slow")
	}

	err := UnaryClientInterceptor(cb, WithFailureCodes(codes.DeadlineExceeded))(context.Background(), "/pkg.Svc/Get", nil, nil, nil, invoker)
	require.Error(t, err)
	assert.Equal(t, 1, cb.failureN)
}

func TestUnaryClient_SkipMethods(t *testing.T) {
	cb := &fakeBreaker{allowErr: circuitbreaker.ErrCircuitOpen}

	var invokerCalled bool
	invoker := func(_ context.Context, _ string, _, _ any, _ *grpc.ClientConn, _ ...grpc.CallOption) error {
		invokerCalled = true
		return nil
	}

	err := UnaryClientInterceptor(cb, WithSkipMethods("/pkg.Svc/Public"))(context.Background(), "/pkg.Svc/Public", nil, nil, nil, invoker)
	require.NoError(t, err)
	assert.True(t, invokerCalled)
	assert.Equal(t, 0, cb.allowN)
}

func TestUnaryClient_PlainErrorCountsAsSuccess(t *testing.T) {
	cb := &fakeBreaker{}

	invoker := func(_ context.Context, _ string, _, _ any, _ *grpc.ClientConn, _ ...grpc.CallOption) error {
		return errors.New("not a status error")
	}

	// Documented behaviour: a non-status error maps to codes.Unknown (2),
	// which is NOT >= codes.Internal, so it is counted as a success.
	// NOTE: this may mask real failures — see report.
	err := UnaryClientInterceptor(cb)(context.Background(), "/pkg.Svc/Get", nil, nil, nil, invoker)
	require.Error(t, err)
	assert.Equal(t, 1, cb.successN)
	assert.Equal(t, 0, cb.failureN)
}

// ---------------------------------------------------------------------------
// StreamClientInterceptor
// ---------------------------------------------------------------------------

func TestStreamClient_Success(t *testing.T) {
	cb := &fakeBreaker{}

	var streamerCalled bool
	streamer := func(_ context.Context, _ *grpc.StreamDesc, _ *grpc.ClientConn, _ string, _ ...grpc.CallOption) (grpc.ClientStream, error) {
		streamerCalled = true
		return nil, nil
	}

	cs, err := StreamClientInterceptor(cb)(context.Background(), &grpc.StreamDesc{}, nil, "/pkg.Svc/Stream", streamer)
	require.NoError(t, err)
	assert.Nil(t, cs)
	assert.True(t, streamerCalled)
	assert.Equal(t, 1, cb.successN)
}

func TestStreamClient_Failure(t *testing.T) {
	cb := &fakeBreaker{}

	streamer := func(_ context.Context, _ *grpc.StreamDesc, _ *grpc.ClientConn, _ string, _ ...grpc.CallOption) (grpc.ClientStream, error) {
		return nil, status.Error(codes.Internal, "stream boom")
	}

	cs, err := StreamClientInterceptor(cb)(context.Background(), &grpc.StreamDesc{}, nil, "/pkg.Svc/Stream", streamer)
	require.Error(t, err)
	assert.Nil(t, cs)
	assert.Equal(t, 1, cb.failureN)
}

func TestStreamClient_CircuitOpen(t *testing.T) {
	cb := &fakeBreaker{allowErr: circuitbreaker.ErrCircuitOpen}

	streamer := func(_ context.Context, _ *grpc.StreamDesc, _ *grpc.ClientConn, _ string, _ ...grpc.CallOption) (grpc.ClientStream, error) {
		t.Fatal("streamer should not be called")
		return nil, nil
	}

	cs, err := StreamClientInterceptor(cb)(context.Background(), &grpc.StreamDesc{}, nil, "/pkg.Svc/Stream", streamer)
	require.Error(t, err)
	assert.Nil(t, cs)
	st, _ := status.FromError(err)
	assert.Equal(t, codes.Unavailable, st.Code())
}

func TestStreamClient_SkipMethods(t *testing.T) {
	cb := &fakeBreaker{allowErr: circuitbreaker.ErrCircuitOpen}

	var streamerCalled bool
	streamer := func(_ context.Context, _ *grpc.StreamDesc, _ *grpc.ClientConn, _ string, _ ...grpc.CallOption) (grpc.ClientStream, error) {
		streamerCalled = true
		return nil, nil
	}

	cs, err := StreamClientInterceptor(cb, WithSkipMethods("/pkg.Svc/Public"))(context.Background(), &grpc.StreamDesc{}, nil, "/pkg.Svc/Public", streamer)
	require.NoError(t, err)
	assert.Nil(t, cs)
	assert.True(t, streamerCalled)
	assert.Equal(t, 0, cb.allowN)
}

// ---------------------------------------------------------------------------
// StreamInterceptor — remaining parity paths
// ---------------------------------------------------------------------------

func TestStream_CustomFailureCodes(t *testing.T) {
	cb := &fakeBreaker{}
	info := &grpc.StreamServerInfo{FullMethod: "/pkg.Svc/Stream"}
	handler := func(_ any, _ grpc.ServerStream) error {
		return status.Error(codes.NotFound, "not found")
	}

	err := StreamInterceptor(cb, WithFailureCodes(codes.NotFound))(nil, &fakeServerStream{ctx: context.Background()}, info, handler)
	require.Error(t, err)
	assert.Equal(t, 1, cb.failureN)
	assert.Equal(t, 0, cb.successN)
}

func TestStream_SkipMethods(t *testing.T) {
	cb := &fakeBreaker{allowErr: circuitbreaker.ErrCircuitOpen}
	info := &grpc.StreamServerInfo{FullMethod: "/grpc.health.v1.Health/Watch"}
	handler := func(_ any, _ grpc.ServerStream) error { return nil }

	err := StreamInterceptor(cb, WithSkipMethods("/grpc.health.v1.Health/Watch"))(nil, &fakeServerStream{ctx: context.Background()}, info, handler)
	require.NoError(t, err)
	assert.Equal(t, 0, cb.allowN)
}

func TestStream_PlainErrorCountsAsSuccess(t *testing.T) {
	cb := &fakeBreaker{}
	info := &grpc.StreamServerInfo{FullMethod: "/pkg.Svc/Stream"}
	handler := func(_ any, _ grpc.ServerStream) error { return errors.New("plain") }

	// Documented behaviour: non-status errors map to codes.Unknown (2) < Internal,
	// so they count as success (same as the unary interceptor).
	err := StreamInterceptor(cb)(nil, &fakeServerStream{ctx: context.Background()}, info, handler)
	require.Error(t, err)
	assert.Equal(t, 1, cb.successN)
	assert.Equal(t, 0, cb.failureN)
}
