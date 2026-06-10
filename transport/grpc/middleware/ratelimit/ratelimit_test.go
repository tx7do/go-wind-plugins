package ratelimit

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/tx7do/go-wind-plugins/ratelimit"
)

type fakeLimiter struct {
	allowOk  bool
	allowErr error
	waitErr  error
	allowN   int
	waitN    int
}

func (f *fakeLimiter) Allow() (bool, error)       { f.allowN++; return f.allowOk, f.allowErr }
func (f *fakeLimiter) Wait(context.Context) error { f.waitN++; return f.waitErr }
func (f *fakeLimiter) Close() error               { return nil }

type fakeServerStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (s *fakeServerStream) Context() context.Context { return s.ctx }

// --- Unary ---

func TestUnary_Allow(t *testing.T) {
	lim := &fakeLimiter{allowOk: true}
	info := &grpc.UnaryServerInfo{FullMethod: "/pkg.Svc/Get"}
	handler := func(_ context.Context, _ any) (any, error) { return "ok", nil }

	resp, err := UnaryInterceptor(lim)(context.Background(), nil, info, handler)
	require.NoError(t, err)
	assert.Equal(t, "ok", resp)
	assert.Equal(t, 1, lim.allowN)
}

func TestUnary_Reject(t *testing.T) {
	lim := &fakeLimiter{allowOk: false, allowErr: ratelimit.ErrLimited}
	info := &grpc.UnaryServerInfo{FullMethod: "/pkg.Svc/Get"}
	handler := func(_ context.Context, _ any) (any, error) { t.Fatal("should not be called"); return nil, nil }

	_, err := UnaryInterceptor(lim)(context.Background(), nil, info, handler)
	require.Error(t, err)
	st, _ := status.FromError(err)
	assert.Equal(t, codes.ResourceExhausted, st.Code())
}

func TestUnary_WaitMode(t *testing.T) {
	lim := &fakeLimiter{waitErr: nil}
	info := &grpc.UnaryServerInfo{FullMethod: "/pkg.Svc/Get"}
	handler := func(_ context.Context, _ any) (any, error) { return "ok", nil }

	resp, err := UnaryInterceptor(lim, WithWait())(context.Background(), nil, info, handler)
	require.NoError(t, err)
	assert.Equal(t, "ok", resp)
	assert.Equal(t, 1, lim.waitN)
	assert.Equal(t, 0, lim.allowN)
}

func TestUnary_SkipMethods(t *testing.T) {
	lim := &fakeLimiter{allowOk: false, allowErr: ratelimit.ErrLimited}
	info := &grpc.UnaryServerInfo{FullMethod: "/grpc.health.v1.Health/Check"}
	handler := func(_ context.Context, _ any) (any, error) { return "ok", nil }

	resp, err := UnaryInterceptor(lim, WithSkipMethods("/grpc.health.v1.Health/Check"))(context.Background(), nil, info, handler)
	require.NoError(t, err)
	assert.Equal(t, "ok", resp)
	assert.Equal(t, 0, lim.allowN) // bypassed
}

// --- Stream ---

func TestStream_Allow(t *testing.T) {
	lim := &fakeLimiter{allowOk: true}
	info := &grpc.StreamServerInfo{FullMethod: "/pkg.Svc/Stream"}
	handler := func(_ any, _ grpc.ServerStream) error { return nil }

	err := StreamInterceptor(lim)(nil, &fakeServerStream{ctx: context.Background()}, info, handler)
	require.NoError(t, err)
	assert.Equal(t, 1, lim.allowN)
}

func TestStream_Reject(t *testing.T) {
	lim := &fakeLimiter{allowOk: false, allowErr: ratelimit.ErrLimited}
	info := &grpc.StreamServerInfo{FullMethod: "/pkg.Svc/Stream"}
	handler := func(_ any, _ grpc.ServerStream) error { t.Fatal("should not be called"); return nil }

	err := StreamInterceptor(lim)(nil, &fakeServerStream{ctx: context.Background()}, info, handler)
	require.Error(t, err)
	st, _ := status.FromError(err)
	assert.Equal(t, codes.ResourceExhausted, st.Code())
}

func TestStream_SkipMethods(t *testing.T) {
	lim := &fakeLimiter{allowOk: false, allowErr: ratelimit.ErrLimited}
	info := &grpc.StreamServerInfo{FullMethod: "/pkg.Svc/Stream"}
	handler := func(_ any, _ grpc.ServerStream) error { return nil }

	err := StreamInterceptor(lim, WithSkipMethods("/pkg.Svc/Stream"))(nil, &fakeServerStream{ctx: context.Background()}, info, handler)
	require.NoError(t, err)
	assert.Equal(t, 0, lim.allowN)
}
