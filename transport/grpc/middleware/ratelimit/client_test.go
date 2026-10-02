package ratelimit

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/tx7do/go-wind-plugins/ratelimit"
)

// ---------------------------------------------------------------------------
// UnaryInterceptor — remaining paths
// ---------------------------------------------------------------------------

func TestUnary_WaitMode_WaitError(t *testing.T) {
	lim := &fakeLimiter{waitErr: context.Canceled}
	info := &grpc.UnaryServerInfo{FullMethod: "/pkg.Svc/Get"}
	handler := func(_ context.Context, _ any) (any, error) {
		t.Fatal("handler should not be called")
		return nil, nil
	}

	resp, err := UnaryInterceptor(lim, WithWait())(context.Background(), nil, info, handler)
	assert.Error(t, err)
	assert.Nil(t, resp)

	st, _ := status.FromError(err)
	assert.Equal(t, codes.ResourceExhausted, st.Code())
	assert.Equal(t, 1, lim.waitN)
	assert.Equal(t, 0, lim.allowN)
}

func TestUnary_AllowError(t *testing.T) {
	lim := &fakeLimiter{allowOk: true, allowErr: errors.New("limiter backend down")}
	info := &grpc.UnaryServerInfo{FullMethod: "/pkg.Svc/Get"}
	handler := func(_ context.Context, _ any) (any, error) {
		t.Fatal("handler should not be called")
		return nil, nil
	}

	// ok=true but err!=nil → still rejected.
	_, err := UnaryInterceptor(lim)(context.Background(), nil, info, handler)
	require.Error(t, err)
	st, _ := status.FromError(err)
	assert.Equal(t, codes.ResourceExhausted, st.Code())
}

func TestUnary_RejectWithoutError(t *testing.T) {
	lim := &fakeLimiter{allowOk: false}
	info := &grpc.UnaryServerInfo{FullMethod: "/pkg.Svc/Get"}
	handler := func(_ context.Context, _ any) (any, error) {
		t.Fatal("handler should not be called")
		return nil, nil
	}

	// ok=false with nil err → rejected.
	_, err := UnaryInterceptor(lim)(context.Background(), nil, info, handler)
	require.Error(t, err)
	st, _ := status.FromError(err)
	assert.Equal(t, codes.ResourceExhausted, st.Code())
}

// ---------------------------------------------------------------------------
// StreamInterceptor — remaining paths
// ---------------------------------------------------------------------------

func TestStream_WaitMode(t *testing.T) {
	lim := &fakeLimiter{}
	info := &grpc.StreamServerInfo{FullMethod: "/pkg.Svc/Stream"}
	handler := func(_ any, _ grpc.ServerStream) error { return nil }

	err := StreamInterceptor(lim, WithWait())(nil, &fakeServerStream{ctx: context.Background()}, info, handler)
	require.NoError(t, err)
	assert.Equal(t, 1, lim.waitN)
	assert.Equal(t, 0, lim.allowN)
}

func TestStream_WaitMode_WaitError(t *testing.T) {
	lim := &fakeLimiter{waitErr: context.Canceled}
	info := &grpc.StreamServerInfo{FullMethod: "/pkg.Svc/Stream"}
	handler := func(_ any, _ grpc.ServerStream) error {
		t.Fatal("handler should not be called")
		return nil
	}

	err := StreamInterceptor(lim, WithWait())(nil, &fakeServerStream{ctx: context.Background()}, info, handler)
	require.Error(t, err)
	st, _ := status.FromError(err)
	assert.Equal(t, codes.ResourceExhausted, st.Code())
	assert.Equal(t, 1, lim.waitN)
}

func TestStream_AllowError(t *testing.T) {
	lim := &fakeLimiter{allowOk: true, allowErr: ratelimit.ErrLimited}
	info := &grpc.StreamServerInfo{FullMethod: "/pkg.Svc/Stream"}
	handler := func(_ any, _ grpc.ServerStream) error {
		t.Fatal("handler should not be called")
		return nil
	}

	err := StreamInterceptor(lim)(nil, &fakeServerStream{ctx: context.Background()}, info, handler)
	require.Error(t, err)
	st, _ := status.FromError(err)
	assert.Equal(t, codes.ResourceExhausted, st.Code())
}

// ---------------------------------------------------------------------------
// UnaryClientInterceptor
// ---------------------------------------------------------------------------

func TestUnaryClient_Allow(t *testing.T) {
	lim := &fakeLimiter{allowOk: true}

	var invokerCalled bool
	invoker := func(_ context.Context, _ string, _, _ any, _ *grpc.ClientConn, _ ...grpc.CallOption) error {
		invokerCalled = true
		return nil
	}

	err := UnaryClientInterceptor(lim)(context.Background(), "/pkg.Svc/Get", nil, nil, nil, invoker)
	require.NoError(t, err)
	assert.True(t, invokerCalled)
	assert.Equal(t, 1, lim.allowN)
}

func TestUnaryClient_Reject(t *testing.T) {
	lim := &fakeLimiter{allowOk: false, allowErr: ratelimit.ErrLimited}

	invoker := func(_ context.Context, _ string, _, _ any, _ *grpc.ClientConn, _ ...grpc.CallOption) error {
		t.Fatal("invoker should not be called")
		return nil
	}

	err := UnaryClientInterceptor(lim)(context.Background(), "/pkg.Svc/Get", nil, nil, nil, invoker)
	require.Error(t, err)
	st, _ := status.FromError(err)
	assert.Equal(t, codes.ResourceExhausted, st.Code())
}

func TestUnaryClient_WaitMode(t *testing.T) {
	lim := &fakeLimiter{}

	var invokerCalled bool
	invoker := func(_ context.Context, _ string, _, _ any, _ *grpc.ClientConn, _ ...grpc.CallOption) error {
		invokerCalled = true
		return nil
	}

	err := UnaryClientInterceptor(lim, WithWait())(context.Background(), "/pkg.Svc/Get", nil, nil, nil, invoker)
	require.NoError(t, err)
	assert.True(t, invokerCalled)
	assert.Equal(t, 1, lim.waitN)
	assert.Equal(t, 0, lim.allowN)
}

func TestUnaryClient_WaitError(t *testing.T) {
	lim := &fakeLimiter{waitErr: context.DeadlineExceeded}

	invoker := func(_ context.Context, _ string, _, _ any, _ *grpc.ClientConn, _ ...grpc.CallOption) error {
		t.Fatal("invoker should not be called")
		return nil
	}

	err := UnaryClientInterceptor(lim, WithWait())(context.Background(), "/pkg.Svc/Get", nil, nil, nil, invoker)
	require.Error(t, err)
	st, _ := status.FromError(err)
	assert.Equal(t, codes.ResourceExhausted, st.Code())
}

func TestUnaryClient_SkipMethods(t *testing.T) {
	lim := &fakeLimiter{allowOk: false, allowErr: ratelimit.ErrLimited}

	var invokerCalled bool
	invoker := func(_ context.Context, _ string, _, _ any, _ *grpc.ClientConn, _ ...grpc.CallOption) error {
		invokerCalled = true
		return nil
	}

	err := UnaryClientInterceptor(lim, WithSkipMethods("/pkg.Svc/Public"))(context.Background(), "/pkg.Svc/Public", nil, nil, nil, invoker)
	require.NoError(t, err)
	assert.True(t, invokerCalled)
	assert.Equal(t, 0, lim.allowN)
}

// ---------------------------------------------------------------------------
// StreamClientInterceptor
// ---------------------------------------------------------------------------

func TestStreamClient_Allow(t *testing.T) {
	lim := &fakeLimiter{allowOk: true}

	var streamerCalled bool
	streamer := func(_ context.Context, _ *grpc.StreamDesc, _ *grpc.ClientConn, _ string, _ ...grpc.CallOption) (grpc.ClientStream, error) {
		streamerCalled = true
		return nil, nil
	}

	cs, err := StreamClientInterceptor(lim)(context.Background(), &grpc.StreamDesc{}, nil, "/pkg.Svc/Stream", streamer)
	require.NoError(t, err)
	assert.Nil(t, cs)
	assert.True(t, streamerCalled)
	assert.Equal(t, 1, lim.allowN)
}

func TestStreamClient_Reject(t *testing.T) {
	lim := &fakeLimiter{allowOk: false, allowErr: ratelimit.ErrLimited}

	streamer := func(_ context.Context, _ *grpc.StreamDesc, _ *grpc.ClientConn, _ string, _ ...grpc.CallOption) (grpc.ClientStream, error) {
		t.Fatal("streamer should not be called")
		return nil, nil
	}

	cs, err := StreamClientInterceptor(lim)(context.Background(), &grpc.StreamDesc{}, nil, "/pkg.Svc/Stream", streamer)
	require.Error(t, err)
	assert.Nil(t, cs)
	st, _ := status.FromError(err)
	assert.Equal(t, codes.ResourceExhausted, st.Code())
}

func TestStreamClient_WaitMode(t *testing.T) {
	lim := &fakeLimiter{}

	var streamerCalled bool
	streamer := func(_ context.Context, _ *grpc.StreamDesc, _ *grpc.ClientConn, _ string, _ ...grpc.CallOption) (grpc.ClientStream, error) {
		streamerCalled = true
		return nil, nil
	}

	cs, err := StreamClientInterceptor(lim, WithWait())(context.Background(), &grpc.StreamDesc{}, nil, "/pkg.Svc/Stream", streamer)
	require.NoError(t, err)
	assert.Nil(t, cs)
	assert.True(t, streamerCalled)
	assert.Equal(t, 1, lim.waitN)
	assert.Equal(t, 0, lim.allowN)
}

func TestStreamClient_WaitError(t *testing.T) {
	lim := &fakeLimiter{waitErr: context.DeadlineExceeded}

	streamer := func(_ context.Context, _ *grpc.StreamDesc, _ *grpc.ClientConn, _ string, _ ...grpc.CallOption) (grpc.ClientStream, error) {
		t.Fatal("streamer should not be called")
		return nil, nil
	}

	cs, err := StreamClientInterceptor(lim, WithWait())(context.Background(), &grpc.StreamDesc{}, nil, "/pkg.Svc/Stream", streamer)
	require.Error(t, err)
	assert.Nil(t, cs)
	st, _ := status.FromError(err)
	assert.Equal(t, codes.ResourceExhausted, st.Code())
}

func TestStreamClient_SkipMethods(t *testing.T) {
	lim := &fakeLimiter{allowOk: false, allowErr: ratelimit.ErrLimited}

	var streamerCalled bool
	streamer := func(_ context.Context, _ *grpc.StreamDesc, _ *grpc.ClientConn, _ string, _ ...grpc.CallOption) (grpc.ClientStream, error) {
		streamerCalled = true
		return nil, nil
	}

	cs, err := StreamClientInterceptor(lim, WithSkipMethods("/pkg.Svc/Public"))(context.Background(), &grpc.StreamDesc{}, nil, "/pkg.Svc/Public", streamer)
	require.NoError(t, err)
	assert.Nil(t, cs)
	assert.True(t, streamerCalled)
	assert.Equal(t, 0, lim.allowN)
}
