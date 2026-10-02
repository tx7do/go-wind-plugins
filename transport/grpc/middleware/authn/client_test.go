package authn

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	engine "github.com/tx7do/go-wind-plugins/security/authn"
)

// outgoingAuthHeader returns the value of the outgoing Authorization header
// attached to ctx (empty string when absent).
func outgoingAuthHeader(t *testing.T, ctx context.Context) string {
	t.Helper()
	md, ok := metadata.FromOutgoingContext(ctx)
	if !ok {
		return ""
	}
	vals := md.Get(engine.HeaderAuthorize)
	if len(vals) == 0 {
		return ""
	}
	return vals[0]
}

// ---------------------------------------------------------------------------
// UnaryClientInterceptor
// ---------------------------------------------------------------------------

func TestUnaryClientInterceptor_InjectsBearerToken(t *testing.T) {
	interceptor := UnaryClientInterceptor(WithTokenProvider(func(_ context.Context) (string, error) {
		return "tok-123", nil
	}))

	var capturedCtx context.Context
	invoker := func(ctx context.Context, _ string, _, _ any, _ *grpc.ClientConn, _ ...grpc.CallOption) error {
		capturedCtx = ctx
		return nil
	}

	err := interceptor(context.Background(), "/pkg.Svc/Get", nil, nil, nil, invoker)
	require.NoError(t, err)
	assert.Equal(t, "Bearer tok-123", outgoingAuthHeader(t, capturedCtx))
}

func TestUnaryClientInterceptor_DefaultSchemeIsBearer(t *testing.T) {
	interceptor := UnaryClientInterceptor(WithTokenProvider(func(_ context.Context) (string, error) {
		return "tok", nil
	}))

	var capturedCtx context.Context
	invoker := func(ctx context.Context, _ string, _, _ any, _ *grpc.ClientConn, _ ...grpc.CallOption) error {
		capturedCtx = ctx
		return nil
	}

	_ = interceptor(context.Background(), "/pkg.Svc/Get", nil, nil, nil, invoker)
	assert.Equal(t, engine.BearerWord+" tok", outgoingAuthHeader(t, capturedCtx))
}

func TestUnaryClientInterceptor_CustomScheme(t *testing.T) {
	interceptor := UnaryClientInterceptor(
		WithTokenProvider(func(_ context.Context) (string, error) { return "tok", nil }),
		WithScheme("ApiKey"),
	)

	var capturedCtx context.Context
	invoker := func(ctx context.Context, _ string, _, _ any, _ *grpc.ClientConn, _ ...grpc.CallOption) error {
		capturedCtx = ctx
		return nil
	}

	_ = interceptor(context.Background(), "/pkg.Svc/Get", nil, nil, nil, invoker)
	assert.Equal(t, "ApiKey tok", outgoingAuthHeader(t, capturedCtx))
}

func TestUnaryClientInterceptor_NoProviderIsNoop(t *testing.T) {
	interceptor := UnaryClientInterceptor()

	var capturedCtx context.Context
	invoker := func(ctx context.Context, _ string, _, _ any, _ *grpc.ClientConn, _ ...grpc.CallOption) error {
		capturedCtx = ctx
		return nil
	}

	err := interceptor(context.Background(), "/pkg.Svc/Get", nil, nil, nil, invoker)
	require.NoError(t, err)
	assert.Empty(t, outgoingAuthHeader(t, capturedCtx))
}

func TestUnaryClientInterceptor_ProviderErrorSkipsInjection(t *testing.T) {
	interceptor := UnaryClientInterceptor(WithTokenProvider(func(_ context.Context) (string, error) {
		return "", errors.New("vault unavailable")
	}))

	var capturedCtx context.Context
	invoker := func(ctx context.Context, _ string, _, _ any, _ *grpc.ClientConn, _ ...grpc.CallOption) error {
		capturedCtx = ctx
		return nil
	}

	// Provider failure must not fail the RPC — it just skips injection.
	err := interceptor(context.Background(), "/pkg.Svc/Get", nil, nil, nil, invoker)
	require.NoError(t, err)
	assert.Empty(t, outgoingAuthHeader(t, capturedCtx))
}

func TestUnaryClientInterceptor_EmptyTokenSkipsInjection(t *testing.T) {
	interceptor := UnaryClientInterceptor(WithTokenProvider(func(_ context.Context) (string, error) {
		return "", nil
	}))

	var capturedCtx context.Context
	invoker := func(ctx context.Context, _ string, _, _ any, _ *grpc.ClientConn, _ ...grpc.CallOption) error {
		capturedCtx = ctx
		return nil
	}

	err := interceptor(context.Background(), "/pkg.Svc/Get", nil, nil, nil, invoker)
	require.NoError(t, err)
	assert.Empty(t, outgoingAuthHeader(t, capturedCtx))
}

func TestUnaryClientInterceptor_InvokerErrorPropagates(t *testing.T) {
	interceptor := UnaryClientInterceptor(WithTokenProvider(func(_ context.Context) (string, error) {
		return "tok", nil
	}))

	wantErr := status.Error(codes.NotFound, "nope")
	invoker := func(_ context.Context, _ string, _, _ any, _ *grpc.ClientConn, _ ...grpc.CallOption) error {
		return wantErr
	}

	err := interceptor(context.Background(), "/pkg.Svc/Get", nil, nil, nil, invoker)
	require.Error(t, err)
	assert.Equal(t, codes.NotFound, status.Code(err))
}

// ---------------------------------------------------------------------------
// StreamClientInterceptor
// ---------------------------------------------------------------------------

func TestStreamClientInterceptor_InjectsBearerToken(t *testing.T) {
	interceptor := StreamClientInterceptor(WithTokenProvider(func(_ context.Context) (string, error) {
		return "stream-tok", nil
	}))

	var capturedCtx context.Context
	streamer := func(ctx context.Context, _ *grpc.StreamDesc, _ *grpc.ClientConn, _ string, _ ...grpc.CallOption) (grpc.ClientStream, error) {
		capturedCtx = ctx
		return nil, nil
	}

	_, err := interceptor(context.Background(), &grpc.StreamDesc{}, nil, "/pkg.Svc/Stream", streamer)
	require.NoError(t, err)
	assert.Equal(t, "Bearer stream-tok", outgoingAuthHeader(t, capturedCtx))
}

func TestStreamClientInterceptor_NoProviderIsNoop(t *testing.T) {
	interceptor := StreamClientInterceptor()

	var capturedCtx context.Context
	streamer := func(ctx context.Context, _ *grpc.StreamDesc, _ *grpc.ClientConn, _ string, _ ...grpc.CallOption) (grpc.ClientStream, error) {
		capturedCtx = ctx
		return nil, nil
	}

	_, err := interceptor(context.Background(), &grpc.StreamDesc{}, nil, "/pkg.Svc/Stream", streamer)
	require.NoError(t, err)
	assert.Empty(t, outgoingAuthHeader(t, capturedCtx))
}

func TestStreamClientInterceptor_ProviderErrorSkipsInjection(t *testing.T) {
	interceptor := StreamClientInterceptor(WithTokenProvider(func(_ context.Context) (string, error) {
		return "", errors.New("boom")
	}))

	var capturedCtx context.Context
	streamer := func(ctx context.Context, _ *grpc.StreamDesc, _ *grpc.ClientConn, _ string, _ ...grpc.CallOption) (grpc.ClientStream, error) {
		capturedCtx = ctx
		return nil, nil
	}

	cs, err := interceptor(context.Background(), &grpc.StreamDesc{}, nil, "/pkg.Svc/Stream", streamer)
	require.NoError(t, err)
	assert.Nil(t, cs)
	assert.Empty(t, outgoingAuthHeader(t, capturedCtx))
}

func TestStreamClientInterceptor_CustomScheme(t *testing.T) {
	interceptor := StreamClientInterceptor(
		WithTokenProvider(func(_ context.Context) (string, error) { return "t", nil }),
		WithScheme("Token"),
	)

	var capturedCtx context.Context
	streamer := func(ctx context.Context, _ *grpc.StreamDesc, _ *grpc.ClientConn, _ string, _ ...grpc.CallOption) (grpc.ClientStream, error) {
		capturedCtx = ctx
		return nil, nil
	}

	_, _ = interceptor(context.Background(), &grpc.StreamDesc{}, nil, "/pkg.Svc/Stream", streamer)
	assert.Equal(t, "Token t", outgoingAuthHeader(t, capturedCtx))
}

// ---------------------------------------------------------------------------
// StreamInterceptor — error-func parity with unary
// ---------------------------------------------------------------------------

func TestStreamInterceptor_CustomErrorFunc(t *testing.T) {
	auth := &fakeAuthenticator{token: "valid-token"}

	interceptor := StreamInterceptor(auth, WithErrorFunc(func(_ context.Context, err error) error {
		return status.Error(codes.Internal, "wrapped: "+err.Error())
	}))

	info := &grpc.StreamServerInfo{FullMethod: "/pkg.Svc/StreamData"}
	ss := &fakeServerStream{ctx: context.Background()}
	handler := func(_ any, _ grpc.ServerStream) error {
		t.Fatal("handler should not be called")
		return nil
	}

	err := interceptor(nil, ss, info, handler)
	require.Error(t, err)

	st, _ := status.FromError(err)
	assert.Equal(t, codes.Internal, st.Code())
	assert.Contains(t, st.Message(), "wrapped:")
}
