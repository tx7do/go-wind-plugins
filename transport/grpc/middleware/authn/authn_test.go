package authn

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	engine "github.com/tx7do/go-wind-plugins/security/authn"
)

// fakeAuthenticator is a minimal [engine.Authenticator] for testing.
type fakeAuthenticator struct {
	token    string
	claims   *engine.AuthClaims
	failWith error
}

func (f *fakeAuthenticator) Authenticate(ctx context.Context) (*engine.AuthClaims, error) {
	// Simulate real behavior: extract token from metadata.
	token, err := engine.AuthFromMD(ctx, engine.BearerWord)
	if err != nil {
		return nil, engine.ErrMissingBearerToken
	}
	if f.failWith != nil {
		return nil, f.failWith
	}
	if token != f.token {
		return nil, engine.ErrInvalidToken
	}
	return f.claims, nil
}

func (f *fakeAuthenticator) AuthenticateToken(token string) (*engine.AuthClaims, error) {
	if token != f.token {
		return nil, engine.ErrInvalidToken
	}
	return f.claims, nil
}

func (f *fakeAuthenticator) CreateIdentityWithContext(ctx context.Context, _ engine.AuthClaims) (context.Context, error) {
	return ctx, nil
}

func (f *fakeAuthenticator) CreateIdentity(_ engine.AuthClaims) (string, error) {
	return f.token, nil
}

func (f *fakeAuthenticator) Close() {}

// ctxWithMD creates a context with incoming metadata carrying a bearer token.
func ctxWithMD(token string) context.Context {
	md := metadata.Pairs(engine.HeaderAuthorize, engine.BearerWord+" "+token)
	return metadata.NewIncomingContext(context.Background(), md)
}

// ---------------------------------------------------------------------------
// UnaryInterceptor
// ---------------------------------------------------------------------------

func TestUnaryInterceptor_Success(t *testing.T) {
	claims := &engine.AuthClaims{engine.ClaimFieldSubject: "alice"}
	auth := &fakeAuthenticator{token: "valid-token", claims: claims}

	var capturedClaims *engine.AuthClaims
	interceptor := UnaryInterceptor(auth)

	info := &grpc.UnaryServerInfo{FullMethod: "/pkg.Svc/GetUser"}
	handler := func(ctx context.Context, _ any) (any, error) {
		capturedClaims, _ = engine.AuthClaimsFromContext(ctx)
		return "ok", nil
	}

	resp, err := interceptor(ctxWithMD("valid-token"), nil, info, handler)
	require.NoError(t, err)
	assert.Equal(t, "ok", resp)
	require.NotNil(t, capturedClaims)
	sub, _ := capturedClaims.GetSubject()
	assert.Equal(t, "alice", sub)
}

func TestUnaryInterceptor_NoMetadata(t *testing.T) {
	auth := &fakeAuthenticator{token: "valid-token"}

	var handlerCalled bool
	interceptor := UnaryInterceptor(auth)

	info := &grpc.UnaryServerInfo{FullMethod: "/pkg.Svc/GetUser"}
	handler := func(_ context.Context, _ any) (any, error) {
		handlerCalled = true
		return nil, nil
	}

	resp, err := interceptor(context.Background(), nil, info, handler)
	assert.Error(t, err)
	assert.Nil(t, resp)
	assert.False(t, handlerCalled)

	st, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.Unauthenticated, st.Code())
}

func TestUnaryInterceptor_InvalidToken(t *testing.T) {
	auth := &fakeAuthenticator{token: "valid-token"}

	interceptor := UnaryInterceptor(auth)

	info := &grpc.UnaryServerInfo{FullMethod: "/pkg.Svc/GetUser"}
	handler := func(_ context.Context, _ any) (any, error) {
		t.Fatal("handler should not be called")
		return nil, nil
	}

	resp, err := interceptor(ctxWithMD("bad-token"), nil, info, handler)
	assert.Error(t, err)
	assert.Nil(t, resp)

	st, _ := status.FromError(err)
	assert.Equal(t, codes.Unauthenticated, st.Code())
}

func TestUnaryInterceptor_SkipMethods(t *testing.T) {
	auth := &fakeAuthenticator{token: "valid-token"}

	var handlerCalled bool
	interceptor := UnaryInterceptor(auth, WithSkipMethods("/grpc.health.v1.Health/Check"))

	info := &grpc.UnaryServerInfo{FullMethod: "/grpc.health.v1.Health/Check"}
	handler := func(_ context.Context, _ any) (any, error) {
		handlerCalled = true
		return "ok", nil
	}

	// No metadata, but should skip auth.
	resp, err := interceptor(context.Background(), nil, info, handler)
	require.NoError(t, err)
	assert.Equal(t, "ok", resp)
	assert.True(t, handlerCalled)
}

func TestUnaryInterceptor_CustomErrorFunc(t *testing.T) {
	auth := &fakeAuthenticator{token: "valid-token"}

	interceptor := UnaryInterceptor(auth, WithErrorFunc(func(_ context.Context, err error) error {
		return status.Error(codes.Internal, "wrapped: "+err.Error())
	}))

	info := &grpc.UnaryServerInfo{FullMethod: "/pkg.Svc/GetUser"}
	handler := func(_ context.Context, _ any) (any, error) {
		t.Fatal("handler should not be called")
		return nil, nil
	}

	_, err := interceptor(context.Background(), nil, info, handler)
	require.Error(t, err)

	st, _ := status.FromError(err)
	assert.Equal(t, codes.Internal, st.Code())
	assert.Contains(t, st.Message(), "wrapped:")
}

// ---------------------------------------------------------------------------
// StreamInterceptor
// ---------------------------------------------------------------------------

type fakeServerStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (s *fakeServerStream) Context() context.Context { return s.ctx }

func TestStreamInterceptor_Success(t *testing.T) {
	claims := &engine.AuthClaims{engine.ClaimFieldSubject: "bob"}
	auth := &fakeAuthenticator{token: "valid-token", claims: claims}

	var capturedClaims *engine.AuthClaims
	interceptor := StreamInterceptor(auth)

	info := &grpc.StreamServerInfo{FullMethod: "/pkg.Svc/StreamData"}
	ss := &fakeServerStream{ctx: ctxWithMD("valid-token")}
	handler := func(_ any, stream grpc.ServerStream) error {
		capturedClaims, _ = engine.AuthClaimsFromContext(stream.Context())
		return nil
	}

	err := interceptor(nil, ss, info, handler)
	require.NoError(t, err)
	require.NotNil(t, capturedClaims)
	sub, _ := capturedClaims.GetSubject()
	assert.Equal(t, "bob", sub)
}

func TestStreamInterceptor_NoMetadata(t *testing.T) {
	auth := &fakeAuthenticator{token: "valid-token"}

	interceptor := StreamInterceptor(auth)

	info := &grpc.StreamServerInfo{FullMethod: "/pkg.Svc/StreamData"}
	ss := &fakeServerStream{ctx: context.Background()}
	handler := func(_ any, _ grpc.ServerStream) error {
		t.Fatal("handler should not be called")
		return nil
	}

	err := interceptor(nil, ss, info, handler)
	require.Error(t, err)

	st, _ := status.FromError(err)
	assert.Equal(t, codes.Unauthenticated, st.Code())
}

func TestStreamInterceptor_SkipMethods(t *testing.T) {
	auth := &fakeAuthenticator{token: "valid-token"}

	var handlerCalled bool
	interceptor := StreamInterceptor(auth, WithSkipMethods("/grpc.health.v1.Health/Watch"))

	info := &grpc.StreamServerInfo{FullMethod: "/grpc.health.v1.Health/Watch"}
	ss := &fakeServerStream{ctx: context.Background()}
	handler := func(_ any, _ grpc.ServerStream) error {
		handlerCalled = true
		return nil
	}

	err := interceptor(nil, ss, info, handler)
	require.NoError(t, err)
	assert.True(t, handlerCalled)
}
