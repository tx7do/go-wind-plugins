package circuitbreaker

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/tx7do/go-wind-plugins/circuitbreaker"
)

type fakeBreaker struct {
	allowErr error
	allowN   int
	successN int
	failureN int
}

func (f *fakeBreaker) Allow() error                                { f.allowN++; return f.allowErr }
func (f *fakeBreaker) MarkSuccess()                                { f.successN++ }
func (f *fakeBreaker) MarkFailure()                                { f.failureN++ }
func (f *fakeBreaker) State() circuitbreaker.State                 { return circuitbreaker.StateClosed }
func (f *fakeBreaker) Execute(context.Context, func() error) error { return nil }
func (f *fakeBreaker) Close() error                                { return nil }

type fakeServerStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (s *fakeServerStream) Context() context.Context { return s.ctx }

// --- Unary ---

func TestUnary_Success(t *testing.T) {
	cb := &fakeBreaker{}
	info := &grpc.UnaryServerInfo{FullMethod: "/pkg.Svc/Get"}
	handler := func(_ context.Context, _ any) (any, error) { return "ok", nil }

	resp, err := UnaryInterceptor(cb)(context.Background(), nil, info, handler)
	require.NoError(t, err)
	assert.Equal(t, "ok", resp)
	assert.Equal(t, 1, cb.successN)
	assert.Equal(t, 0, cb.failureN)
}

func TestUnary_Failure_InternalError(t *testing.T) {
	cb := &fakeBreaker{}
	info := &grpc.UnaryServerInfo{FullMethod: "/pkg.Svc/Get"}
	handler := func(_ context.Context, _ any) (any, error) {
		return nil, status.Error(codes.Internal, "db error")
	}

	_, err := UnaryInterceptor(cb)(context.Background(), nil, info, handler)
	require.Error(t, err)
	assert.Equal(t, 1, cb.failureN)
	assert.Equal(t, 0, cb.successN)
}

func TestUnary_NonFailure_NotFound(t *testing.T) {
	cb := &fakeBreaker{}
	info := &grpc.UnaryServerInfo{FullMethod: "/pkg.Svc/Get"}
	handler := func(_ context.Context, _ any) (any, error) {
		return nil, status.Error(codes.NotFound, "not found")
	}

	_, err := UnaryInterceptor(cb)(context.Background(), nil, info, handler)
	require.Error(t, err)
	// NotFound is a caller fault — not in the default failure table, so it
	// counts as success.
	assert.Equal(t, 1, cb.successN)
	assert.Equal(t, 0, cb.failureN)
}

func TestUnary_CircuitOpen(t *testing.T) {
	cb := &fakeBreaker{allowErr: circuitbreaker.ErrCircuitOpen}
	info := &grpc.UnaryServerInfo{FullMethod: "/pkg.Svc/Get"}
	handler := func(_ context.Context, _ any) (any, error) { t.Fatal("should not be called"); return nil, nil }

	_, err := UnaryInterceptor(cb)(context.Background(), nil, info, handler)
	require.Error(t, err)
	st, _ := status.FromError(err)
	assert.Equal(t, codes.Unavailable, st.Code())
	assert.Equal(t, 0, cb.successN)
	assert.Equal(t, 0, cb.failureN)
}

func TestUnary_CustomFailureCodes(t *testing.T) {
	cb := &fakeBreaker{}
	info := &grpc.UnaryServerInfo{FullMethod: "/pkg.Svc/Get"}
	handler := func(_ context.Context, _ any) (any, error) {
		return nil, status.Error(codes.NotFound, "not found")
	}

	// Treat NotFound as failure.
	_, err := UnaryInterceptor(cb, WithFailureCodes(codes.NotFound))(context.Background(), nil, info, handler)
	require.Error(t, err)
	assert.Equal(t, 1, cb.failureN)
	assert.Equal(t, 0, cb.successN)
}

func TestUnary_SkipMethods(t *testing.T) {
	cb := &fakeBreaker{allowErr: circuitbreaker.ErrCircuitOpen}
	info := &grpc.UnaryServerInfo{FullMethod: "/grpc.health.v1.Health/Check"}
	handler := func(_ context.Context, _ any) (any, error) { return "ok", nil }

	resp, err := UnaryInterceptor(cb, WithSkipMethods("/grpc.health.v1.Health/Check"))(context.Background(), nil, info, handler)
	require.NoError(t, err)
	assert.Equal(t, "ok", resp)
	assert.Equal(t, 0, cb.allowN)
}

// --- Stream ---

func TestStream_Success(t *testing.T) {
	cb := &fakeBreaker{}
	info := &grpc.StreamServerInfo{FullMethod: "/pkg.Svc/Stream"}
	handler := func(_ any, _ grpc.ServerStream) error { return nil }

	err := StreamInterceptor(cb)(nil, &fakeServerStream{ctx: context.Background()}, info, handler)
	require.NoError(t, err)
	assert.Equal(t, 1, cb.successN)
}

func TestStream_CircuitOpen(t *testing.T) {
	cb := &fakeBreaker{allowErr: circuitbreaker.ErrCircuitOpen}
	info := &grpc.StreamServerInfo{FullMethod: "/pkg.Svc/Stream"}
	handler := func(_ any, _ grpc.ServerStream) error { t.Fatal("should not be called"); return nil }

	err := StreamInterceptor(cb)(nil, &fakeServerStream{ctx: context.Background()}, info, handler)
	require.Error(t, err)
	st, _ := status.FromError(err)
	assert.Equal(t, codes.Unavailable, st.Code())
}

func TestStream_Failure(t *testing.T) {
	cb := &fakeBreaker{}
	info := &grpc.StreamServerInfo{FullMethod: "/pkg.Svc/Stream"}
	handler := func(_ any, _ grpc.ServerStream) error {
		return status.Error(codes.Internal, "boom")
	}

	err := StreamInterceptor(cb)(nil, &fakeServerStream{ctx: context.Background()}, info, handler)
	require.Error(t, err)
	assert.Equal(t, 1, cb.failureN)
}

// TestUnary_FailureCodeTable drives the server interceptor with every gRPC
// code and pins the default failure/success classification (see
// defaultFailureCodes: Unknown, DeadlineExceeded, Internal, Unavailable and
// DataLoss are server-side faults; everything else counts as success).
func TestUnary_FailureCodeTable(t *testing.T) {
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
			info := &grpc.UnaryServerInfo{FullMethod: "/pkg.Svc/Get"}
			handler := func(_ context.Context, _ any) (any, error) {
				return nil, status.Error(code, code.String())
			}

			_, err := UnaryInterceptor(cb)(context.Background(), nil, info, handler)
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
