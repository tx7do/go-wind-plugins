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
	// NotFound is a caller fault — not in the default failure table, so it
	// counts as success.
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

func TestUnaryClient_PlainErrorCountsAsFailure(t *testing.T) {
	cb := &fakeBreaker{}

	invoker := func(_ context.Context, _ string, _, _ any, _ *grpc.ClientConn, _ ...grpc.CallOption) error {
		return errors.New("not a status error")
	}

	// A non-status error maps to codes.Unknown (2), which is one of the
	// default server-side fault codes, so it is counted as a failure and
	// trips the breaker.
	err := UnaryClientInterceptor(cb)(context.Background(), "/pkg.Svc/Get", nil, nil, nil, invoker)
	require.Error(t, err)
	assert.Equal(t, 1, cb.failureN)
	assert.Equal(t, 0, cb.successN)
}

// TestUnaryClient_FailureCodeTable drives the client interceptor with every
// gRPC code and pins the default failure/success classification.
func TestUnaryClient_FailureCodeTable(t *testing.T) {
	allCodes := []codes.Code{
		codes.OK, codes.Canceled, codes.Unknown, codes.InvalidArgument,
		codes.DeadlineExceeded, codes.NotFound, codes.AlreadyExists,
		codes.PermissionDenied, codes.ResourceExhausted, codes.FailedPrecondition,
		codes.Aborted, codes.OutOfRange, codes.Unimplemented, codes.Internal,
		codes.Unavailable, codes.DataLoss, codes.Unauthenticated,
	}
	for _, code := range allCodes {
		wantFailure := defaultFailureCodes[code]

		t.Run(code.String(), func(t *testing.T) {
			cb := &fakeBreaker{}

			invoker := func(_ context.Context, _ string, _, _ any, _ *grpc.ClientConn, _ ...grpc.CallOption) error {
				return status.Error(code, code.String())
			}

			err := UnaryClientInterceptor(cb)(context.Background(), "/pkg.Svc/Get", nil, nil, nil, invoker)
			if code == codes.OK {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}

			if wantFailure {
				assert.Equal(t, 1, cb.failureN, "code %s must count as failure", code)
				assert.Equal(t, 0, cb.successN)
			} else {
				assert.Equal(t, 1, cb.successN, "code %s must count as success", code)
				assert.Equal(t, 0, cb.failureN)
			}
		})
	}
}

// TestUnaryClient_ExplicitTableOverridesDefault pins the override semantics of
// WithFailureCodes: the explicit table replaces the default one entirely.
func TestUnaryClient_ExplicitTableOverridesDefault(t *testing.T) {
	cb := &fakeBreaker{}

	invoker := func(_ context.Context, _ string, _, _ any, _ *grpc.ClientConn, _ ...grpc.CallOption) error {
		return status.Error(codes.Internal, "boom")
	}

	// Internal alone must not fail: only PermissionDenied is listed.
	err := UnaryClientInterceptor(cb, WithFailureCodes(codes.PermissionDenied))(context.Background(), "/pkg.Svc/Get", nil, nil, nil, invoker)
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

func TestStream_PlainErrorCountsAsFailure(t *testing.T) {
	cb := &fakeBreaker{}
	info := &grpc.StreamServerInfo{FullMethod: "/pkg.Svc/Stream"}
	handler := func(_ any, _ grpc.ServerStream) error { return errors.New("plain") }

	// Non-status errors map to codes.Unknown, one of the default server-side
	// fault codes, so they trip the breaker (same as the unary interceptor).
	err := StreamInterceptor(cb)(nil, &fakeServerStream{ctx: context.Background()}, info, handler)
	require.Error(t, err)
	assert.Equal(t, 1, cb.failureN)
	assert.Equal(t, 0, cb.successN)
}

// TestStream_FailureCodeTable drives the stream server interceptor with every
// gRPC code and pins the default failure/success classification.
func TestStream_FailureCodeTable(t *testing.T) {
	allCodes := []codes.Code{
		codes.OK, codes.Canceled, codes.Unknown, codes.InvalidArgument,
		codes.DeadlineExceeded, codes.NotFound, codes.AlreadyExists,
		codes.PermissionDenied, codes.ResourceExhausted, codes.FailedPrecondition,
		codes.Aborted, codes.OutOfRange, codes.Unimplemented, codes.Internal,
		codes.Unavailable, codes.DataLoss, codes.Unauthenticated,
	}
	for _, code := range allCodes {
		wantFailure := defaultFailureCodes[code]

		t.Run(code.String(), func(t *testing.T) {
			cb := &fakeBreaker{}
			info := &grpc.StreamServerInfo{FullMethod: "/pkg.Svc/Stream"}
			handler := func(_ any, _ grpc.ServerStream) error {
				return status.Error(code, code.String())
			}

			err := StreamInterceptor(cb)(nil, &fakeServerStream{ctx: context.Background()}, info, handler)
			if code == codes.OK {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}

			if wantFailure {
				assert.Equal(t, 1, cb.failureN, "code %s must count as failure", code)
				assert.Equal(t, 0, cb.successN)
			} else {
				assert.Equal(t, 1, cb.successN, "code %s must count as success", code)
				assert.Equal(t, 0, cb.failureN)
			}
		})
	}
}
