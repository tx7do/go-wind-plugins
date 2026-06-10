package retry

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	coreRetry "github.com/tx7do/go-wind-plugins/retry"
)

func fastRetrier(attempts int) *coreRetry.Retrier {
	return coreRetry.New(
		coreRetry.WithMaxAttempts(attempts),
		coreRetry.WithBackoff(coreRetry.FixedBackoff(1*time.Millisecond)),
	)
}

type fakeServerStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (s *fakeServerStream) Context() context.Context { return s.ctx }

// --- isIdempotent ---

func TestIsIdempotent(t *testing.T) {
	prefixes := []string{"Get", "List", "Search"}
	assert.True(t, isIdempotent("/pkg.UserService/GetUser", prefixes))
	assert.True(t, isIdempotent("/pkg.UserService/ListUsers", prefixes))
	assert.True(t, isIdempotent("/pkg.UserService/SearchUsers", prefixes))
	assert.False(t, isIdempotent("/pkg.UserService/CreateUser", prefixes))
	assert.False(t, isIdempotent("/pkg.UserService/DeleteUser", prefixes))
}

// --- Unary ---

func TestUnary_SuccessFirstTry(t *testing.T) {
	var calls int32
	handler := func(_ context.Context, _ any) (any, error) {
		atomic.AddInt32(&calls, 1)
		return "ok", nil
	}

	info := &grpc.UnaryServerInfo{FullMethod: "/pkg.Svc/GetUser"}
	resp, err := UnaryInterceptor(fastRetrier(3))(context.Background(), nil, info, handler)
	require.NoError(t, err)
	assert.Equal(t, "ok", resp)
	assert.Equal(t, int32(1), atomic.LoadInt32(&calls))
}

func TestUnary_RetriesUntilSuccess(t *testing.T) {
	var calls int32
	handler := func(_ context.Context, _ any) (any, error) {
		n := atomic.AddInt32(&calls, 1)
		if n < 3 {
			return nil, status.Error(codes.Unavailable, "try again")
		}
		return "ok", nil
	}

	info := &grpc.UnaryServerInfo{FullMethod: "/pkg.Svc/GetUser"}
	resp, err := UnaryInterceptor(fastRetrier(3))(context.Background(), nil, info, handler)
	require.NoError(t, err)
	assert.Equal(t, "ok", resp)
	assert.Equal(t, int32(3), atomic.LoadInt32(&calls))
}

func TestUnary_AllRetriesFail(t *testing.T) {
	var calls int32
	handler := func(_ context.Context, _ any) (any, error) {
		atomic.AddInt32(&calls, 1)
		return nil, status.Error(codes.Unavailable, "down")
	}

	info := &grpc.UnaryServerInfo{FullMethod: "/pkg.Svc/GetUser"}
	_, err := UnaryInterceptor(fastRetrier(3))(context.Background(), nil, info, handler)
	require.Error(t, err)
	st, _ := status.FromError(err)
	assert.Equal(t, codes.Unavailable, st.Code())
	assert.Equal(t, int32(3), atomic.LoadInt32(&calls))
}

func TestUnary_NonIdempotent_NotRetried(t *testing.T) {
	var calls int32
	handler := func(_ context.Context, _ any) (any, error) {
		atomic.AddInt32(&calls, 1)
		return nil, status.Error(codes.Unavailable, "down")
	}

	// CreateUser is not idempotent.
	info := &grpc.UnaryServerInfo{FullMethod: "/pkg.Svc/CreateUser"}
	_, err := UnaryInterceptor(fastRetrier(5))(context.Background(), nil, info, handler)
	require.Error(t, err)
	assert.Equal(t, int32(1), atomic.LoadInt32(&calls))
}

func TestUnary_NonRetryableCode_NotRetried(t *testing.T) {
	var calls int32
	handler := func(_ context.Context, _ any) (any, error) {
		atomic.AddInt32(&calls, 1)
		return nil, status.Error(codes.NotFound, "missing")
	}

	info := &grpc.UnaryServerInfo{FullMethod: "/pkg.Svc/GetUser"}
	_, err := UnaryInterceptor(fastRetrier(5))(context.Background(), nil, info, handler)
	require.Error(t, err)
	st, _ := status.FromError(err)
	assert.Equal(t, codes.NotFound, st.Code())
	assert.Equal(t, int32(1), atomic.LoadInt32(&calls))
}

func TestUnary_CustomRetryCodes(t *testing.T) {
	var calls int32
	handler := func(_ context.Context, _ any) (any, error) {
		n := atomic.AddInt32(&calls, 1)
		if n < 2 {
			return nil, status.Error(codes.DeadlineExceeded, "timeout")
		}
		return "ok", nil
	}

	info := &grpc.UnaryServerInfo{FullMethod: "/pkg.Svc/GetUser"}
	resp, err := UnaryInterceptor(fastRetrier(3), WithRetryCodes(codes.DeadlineExceeded))(context.Background(), nil, info, handler)
	require.NoError(t, err)
	assert.Equal(t, "ok", resp)
	assert.Equal(t, int32(2), atomic.LoadInt32(&calls))
}

func TestUnary_CustomPrefixes(t *testing.T) {
	var calls int32
	handler := func(_ context.Context, _ any) (any, error) {
		n := atomic.AddInt32(&calls, 1)
		if n < 2 {
			return nil, status.Error(codes.Unavailable, "down")
		}
		return "ok", nil
	}

	// "Read" is not in the default prefixes, but we add it.
	info := &grpc.UnaryServerInfo{FullMethod: "/pkg.Svc/ReadData"}
	resp, err := UnaryInterceptor(fastRetrier(3), WithIdempotentPrefixes("Read"))(context.Background(), nil, info, handler)
	require.NoError(t, err)
	assert.Equal(t, "ok", resp)
	assert.Equal(t, int32(2), atomic.LoadInt32(&calls))
}

func TestUnary_SkipMethods(t *testing.T) {
	var calls int32
	handler := func(_ context.Context, _ any) (any, error) {
		atomic.AddInt32(&calls, 1)
		return "skipped", nil
	}

	info := &grpc.UnaryServerInfo{FullMethod: "/grpc.health.v1.Health/Check"}
	resp, err := UnaryInterceptor(fastRetrier(3), WithSkipMethods("/grpc.health.v1.Health/Check"))(context.Background(), nil, info, handler)
	require.NoError(t, err)
	assert.Equal(t, "skipped", resp)
	assert.Equal(t, int32(1), atomic.LoadInt32(&calls))
}

// --- Stream ---

func TestStream_Success(t *testing.T) {
	var calls int32
	handler := func(_ any, _ grpc.ServerStream) error {
		atomic.AddInt32(&calls, 1)
		return nil
	}

	info := &grpc.StreamServerInfo{FullMethod: "/pkg.Svc/ListUsers"}
	err := StreamInterceptor(fastRetrier(3))(nil, &fakeServerStream{ctx: context.Background()}, info, handler)
	require.NoError(t, err)
	assert.Equal(t, int32(1), atomic.LoadInt32(&calls))
}

func TestStream_RetriesUntilSuccess(t *testing.T) {
	var calls int32
	handler := func(_ any, _ grpc.ServerStream) error {
		n := atomic.AddInt32(&calls, 1)
		if n < 3 {
			return status.Error(codes.Unavailable, "down")
		}
		return nil
	}

	info := &grpc.StreamServerInfo{FullMethod: "/pkg.Svc/ListUsers"}
	err := StreamInterceptor(fastRetrier(3))(nil, &fakeServerStream{ctx: context.Background()}, info, handler)
	require.NoError(t, err)
	assert.Equal(t, int32(3), atomic.LoadInt32(&calls))
}

func TestStream_NonIdempotent(t *testing.T) {
	var calls int32
	handler := func(_ any, _ grpc.ServerStream) error {
		atomic.AddInt32(&calls, 1)
		return status.Error(codes.Unavailable, "down")
	}

	info := &grpc.StreamServerInfo{FullMethod: "/pkg.Svc/CreateUser"}
	err := StreamInterceptor(fastRetrier(5))(nil, &fakeServerStream{ctx: context.Background()}, info, handler)
	require.Error(t, err)
	assert.Equal(t, int32(1), atomic.LoadInt32(&calls))
}
