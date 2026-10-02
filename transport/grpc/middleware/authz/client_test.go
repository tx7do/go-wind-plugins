package authz

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	authzEngine "github.com/tx7do/go-wind-plugins/security/authz"
)

// ---------------------------------------------------------------------------
// UnaryClientInterceptor
// ---------------------------------------------------------------------------

func TestUnaryClientInterceptor_Allowed(t *testing.T) {
	eng := &fakeEngine{authorized: true}

	var invokerCalled bool
	interceptor := UnaryClientInterceptor(eng, func() []Option {
		// Provide custom resolvers so we can verify what was sent to the engine.
		return []Option{
			WithClientActionResolver(func(method string) string {
				return "client-" + method
			}),
			WithClientResourceResolver(func(method string) string {
				return "client-res"
			}),
		}
	}()...)

	invoker := func(ctx context.Context, method string, _, _ any, _ *grpc.ClientConn, _ ...grpc.CallOption) error {
		invokerCalled = true
		return nil
	}

	err := interceptor(ctxWithClaims("alice"), "/pkg.Svc/Get", nil, nil, nil, invoker)
	require.NoError(t, err)
	assert.True(t, invokerCalled)
	assert.Equal(t, "alice", eng.capturedSubject)
	assert.Equal(t, "client-/pkg.Svc/Get", eng.capturedAction)
	assert.Equal(t, "client-res", eng.capturedResource)
}

func TestUnaryClientInterceptor_DefaultResolvers(t *testing.T) {
	eng := &fakeEngine{authorized: true}

	var invokerCalled bool
	interceptor := UnaryClientInterceptor(eng)

	invoker := func(_ context.Context, _ string, _, _ any, _ *grpc.ClientConn, _ ...grpc.CallOption) error {
		invokerCalled = true
		return nil
	}

	_ = interceptor(ctxWithClaims("alice"), "/pkg.UserService/GetUser", nil, nil, nil, invoker)
	require.True(t, invokerCalled)
	assert.Equal(t, "GetUser", eng.capturedAction)
	assert.Equal(t, "pkg.UserService", eng.capturedResource)
	assert.Equal(t, "", eng.capturedProject)
}

func TestUnaryClientInterceptor_Denied(t *testing.T) {
	eng := &fakeEngine{authorized: false}

	var invokerCalled bool
	interceptor := UnaryClientInterceptor(eng)

	invoker := func(_ context.Context, _ string, _, _ any, _ *grpc.ClientConn, _ ...grpc.CallOption) error {
		invokerCalled = true
		return nil
	}

	err := interceptor(ctxWithClaims("bob"), "/pkg.Svc/Get", nil, nil, nil, invoker)
	require.Error(t, err)
	assert.False(t, invokerCalled)

	st, _ := status.FromError(err)
	assert.Equal(t, codes.PermissionDenied, st.Code())
}

func TestUnaryClientInterceptor_EngineError(t *testing.T) {
	eng := &fakeEngine{err: authzEngine.ErrInvalidClaims}

	var invokerCalled bool
	interceptor := UnaryClientInterceptor(eng)

	invoker := func(_ context.Context, _ string, _, _ any, _ *grpc.ClientConn, _ ...grpc.CallOption) error {
		invokerCalled = true
		return nil
	}

	err := interceptor(ctxWithClaims("carol"), "/pkg.Svc/Get", nil, nil, nil, invoker)
	require.Error(t, err)
	assert.False(t, invokerCalled)

	st, _ := status.FromError(err)
	assert.Equal(t, codes.PermissionDenied, st.Code())
}

func TestUnaryClientInterceptor_CustomErrorFunc(t *testing.T) {
	eng := &fakeEngine{authorized: false}

	interceptor := UnaryClientInterceptor(eng, WithErrorFunc(func(_ context.Context, _ error) error {
		return status.Error(codes.Internal, "custom")
	}))

	invoker := func(_ context.Context, _ string, _, _ any, _ *grpc.ClientConn, _ ...grpc.CallOption) error {
		t.Fatal("invoker should not be called")
		return nil
	}

	err := interceptor(ctxWithClaims("alice"), "/pkg.Svc/Get", nil, nil, nil, invoker)
	require.Error(t, err)
	assert.Equal(t, codes.Internal, status.Code(err))
}

func TestUnaryClientInterceptor_SkipMethods(t *testing.T) {
	eng := &fakeEngine{authorized: false}

	var invokerCalled bool
	interceptor := UnaryClientInterceptor(eng, WithSkipMethods("/pkg.Svc/Public"))

	invoker := func(_ context.Context, _ string, _, _ any, _ *grpc.ClientConn, _ ...grpc.CallOption) error {
		invokerCalled = true
		return nil
	}

	// Engine would deny, but the method is skipped → invoker runs, engine untouched.
	err := interceptor(ctxWithClaims("alice"), "/pkg.Svc/Public", nil, nil, nil, invoker)
	require.NoError(t, err)
	assert.True(t, invokerCalled)
	assert.Equal(t, "", eng.capturedSubject)
}

func TestUnaryClientInterceptor_NoAuthnClaims(t *testing.T) {
	eng := &fakeEngine{authorized: true}

	var invokerCalled bool
	interceptor := UnaryClientInterceptor(eng)

	invoker := func(_ context.Context, _ string, _, _ any, _ *grpc.ClientConn, _ ...grpc.CallOption) error {
		invokerCalled = true
		return nil
	}

	// No authn claims in ctx → empty subject is still authorized by the fake engine.
	err := interceptor(context.Background(), "/pkg.Svc/Get", nil, nil, nil, invoker)
	require.NoError(t, err)
	assert.True(t, invokerCalled)
	assert.Equal(t, "", eng.capturedSubject)
}

// ---------------------------------------------------------------------------
// StreamClientInterceptor
// ---------------------------------------------------------------------------

func TestStreamClientInterceptor_Allowed(t *testing.T) {
	eng := &fakeEngine{authorized: true}

	var streamerCalled bool
	interceptor := StreamClientInterceptor(eng)

	streamer := func(_ context.Context, _ *grpc.StreamDesc, _ *grpc.ClientConn, _ string, _ ...grpc.CallOption) (grpc.ClientStream, error) {
		streamerCalled = true
		return nil, nil
	}

	cs, err := interceptor(ctxWithClaims("alice"), &grpc.StreamDesc{}, nil, "/pkg.DataService/StreamData", streamer)
	require.NoError(t, err)
	assert.Nil(t, cs)
	assert.True(t, streamerCalled)
	assert.Equal(t, "alice", eng.capturedSubject)
	assert.Equal(t, "StreamData", eng.capturedAction)
	assert.Equal(t, "pkg.DataService", eng.capturedResource)
}

func TestStreamClientInterceptor_Denied(t *testing.T) {
	eng := &fakeEngine{authorized: false}

	var streamerCalled bool
	interceptor := StreamClientInterceptor(eng)

	streamer := func(_ context.Context, _ *grpc.StreamDesc, _ *grpc.ClientConn, _ string, _ ...grpc.CallOption) (grpc.ClientStream, error) {
		streamerCalled = true
		return nil, nil
	}

	cs, err := interceptor(ctxWithClaims("bob"), &grpc.StreamDesc{}, nil, "/pkg.Svc/Stream", streamer)
	require.Error(t, err)
	assert.Nil(t, cs)
	assert.False(t, streamerCalled)

	st, _ := status.FromError(err)
	assert.Equal(t, codes.PermissionDenied, st.Code())
}

func TestStreamClientInterceptor_EngineError(t *testing.T) {
	eng := &fakeEngine{err: authzEngine.ErrInvalidClaims}

	interceptor := StreamClientInterceptor(eng)

	streamer := func(_ context.Context, _ *grpc.StreamDesc, _ *grpc.ClientConn, _ string, _ ...grpc.CallOption) (grpc.ClientStream, error) {
		t.Fatal("streamer should not be called")
		return nil, nil
	}

	cs, err := interceptor(ctxWithClaims("carol"), &grpc.StreamDesc{}, nil, "/pkg.Svc/Stream", streamer)
	require.Error(t, err)
	assert.Nil(t, cs)

	st, _ := status.FromError(err)
	assert.Equal(t, codes.PermissionDenied, st.Code())
}

func TestStreamClientInterceptor_SkipMethods(t *testing.T) {
	eng := &fakeEngine{authorized: false}

	var streamerCalled bool
	interceptor := StreamClientInterceptor(eng, WithSkipMethods("/pkg.Svc/Public"))

	streamer := func(_ context.Context, _ *grpc.StreamDesc, _ *grpc.ClientConn, _ string, _ ...grpc.CallOption) (grpc.ClientStream, error) {
		streamerCalled = true
		return nil, nil
	}

	_, err := interceptor(context.Background(), &grpc.StreamDesc{}, nil, "/pkg.Svc/Public", streamer)
	require.NoError(t, err)
	assert.True(t, streamerCalled)
	assert.Equal(t, "", eng.capturedSubject)
}

func TestStreamClientInterceptor_CustomErrorFunc(t *testing.T) {
	eng := &fakeEngine{authorized: false}

	interceptor := StreamClientInterceptor(eng, WithErrorFunc(func(_ context.Context, _ error) error {
		return status.Error(codes.Internal, "custom stream")
	}))

	streamer := func(_ context.Context, _ *grpc.StreamDesc, _ *grpc.ClientConn, _ string, _ ...grpc.CallOption) (grpc.ClientStream, error) {
		t.Fatal("streamer should not be called")
		return nil, nil
	}

	_, err := interceptor(ctxWithClaims("alice"), &grpc.StreamDesc{}, nil, "/pkg.Svc/Stream", streamer)
	require.Error(t, err)
	assert.Equal(t, codes.Internal, status.Code(err))
}

func TestStreamClientInterceptor_CustomResolvers(t *testing.T) {
	eng := &fakeEngine{authorized: true}

	interceptor := StreamClientInterceptor(eng,
		WithClientActionResolver(func(method string) string { return "act:" + method }),
		WithClientResourceResolver(func(_ string) string { return "res:*" }),
	)

	streamer := func(_ context.Context, _ *grpc.StreamDesc, _ *grpc.ClientConn, _ string, _ ...grpc.CallOption) (grpc.ClientStream, error) {
		return nil, nil
	}

	_, _ = interceptor(ctxWithClaims("alice"), &grpc.StreamDesc{}, nil, "/pkg.Svc/Stream", streamer)
	assert.Equal(t, "act:/pkg.Svc/Stream", eng.capturedAction)
	assert.Equal(t, "res:*", eng.capturedResource)
}

// ---------------------------------------------------------------------------
// Default resolvers — non-standard method strings
// ---------------------------------------------------------------------------

func TestUnaryClientInterceptor_DefaultResolversNoSlash(t *testing.T) {
	eng := &fakeEngine{authorized: true}

	var invokerCalled bool
	interceptor := UnaryClientInterceptor(eng)

	invoker := func(_ context.Context, _ string, _, _ any, _ *grpc.ClientConn, _ ...grpc.CallOption) error {
		invokerCalled = true
		return nil
	}

	// A method without the "/service/method" shape falls back to the raw string.
	_ = interceptor(ctxWithClaims("alice"), "baremethod", nil, nil, nil, invoker)
	require.True(t, invokerCalled)
	assert.Equal(t, "baremethod", eng.capturedAction)
	assert.Equal(t, "baremethod", eng.capturedResource)
}

// ---------------------------------------------------------------------------
// StreamInterceptor — error path parity
// ---------------------------------------------------------------------------

func TestStreamInterceptor_EngineError(t *testing.T) {
	eng := &fakeEngine{err: authzEngine.ErrInvalidClaims}

	var handlerCalled bool
	interceptor := StreamInterceptor(eng)

	info := &grpc.StreamServerInfo{FullMethod: "/pkg.Svc/Stream"}
	ss := &fakeServerStream{ctx: ctxWithClaims("carol")}
	handler := func(_ any, _ grpc.ServerStream) error {
		handlerCalled = true
		return nil
	}

	err := interceptor(nil, ss, info, handler)
	require.Error(t, err)
	assert.False(t, handlerCalled)

	st, _ := status.FromError(err)
	assert.Equal(t, codes.PermissionDenied, st.Code())
}

func TestStreamInterceptor_CustomErrorFunc(t *testing.T) {
	eng := &fakeEngine{authorized: false}

	interceptor := StreamInterceptor(eng, WithErrorFunc(func(_ context.Context, _ error) error {
		return status.Error(codes.Internal, "custom stream")
	}))

	info := &grpc.StreamServerInfo{FullMethod: "/pkg.Svc/Stream"}
	ss := &fakeServerStream{ctx: ctxWithClaims("alice")}
	handler := func(_ any, _ grpc.ServerStream) error {
		t.Fatal("handler should not be called")
		return nil
	}

	err := interceptor(nil, ss, info, handler)
	require.Error(t, err)
	assert.Equal(t, codes.Internal, status.Code(err))
}

func TestStreamInterceptor_ProjectResolver(t *testing.T) {
	eng := &fakeEngine{authorized: true}

	var capturedClaims *authzEngine.AuthClaims
	interceptor := StreamInterceptor(eng, WithProjectResolver(func(_ context.Context) string {
		return "proj-42"
	}))

	info := &grpc.StreamServerInfo{FullMethod: "/pkg.Svc/Stream"}
	ss := &fakeServerStream{ctx: ctxWithClaims("alice")}
	handler := func(_ any, stream grpc.ServerStream) error {
		capturedClaims, _ = authzEngine.AuthClaimsFromContext(stream.Context())
		return nil
	}

	err := interceptor(nil, ss, info, handler)
	require.NoError(t, err)
	assert.Equal(t, "proj-42", eng.capturedProject)
	require.NotNil(t, capturedClaims)
	require.NotNil(t, capturedClaims.Project)
	assert.Equal(t, "proj-42", string(*capturedClaims.Project))
}

func TestStreamInterceptor_AuthzClaimsInContext(t *testing.T) {
	eng := &fakeEngine{authorized: true}

	var capturedClaims *authzEngine.AuthClaims
	interceptor := StreamInterceptor(eng)

	info := &grpc.StreamServerInfo{FullMethod: "/pkg.DataService/StreamData"}
	ss := &fakeServerStream{ctx: ctxWithClaims("alice")}
	handler := func(_ any, stream grpc.ServerStream) error {
		capturedClaims, _ = authzEngine.AuthClaimsFromContext(stream.Context())
		return nil
	}

	err := interceptor(nil, ss, info, handler)
	require.NoError(t, err)
	require.NotNil(t, capturedClaims)
	require.NotNil(t, capturedClaims.Subject)
	assert.Equal(t, "alice", string(*capturedClaims.Subject))
	require.NotNil(t, capturedClaims.Action)
	assert.Equal(t, "StreamData", string(*capturedClaims.Action))
	require.NotNil(t, capturedClaims.Resource)
	assert.Equal(t, "pkg.DataService", string(*capturedClaims.Resource))
}
