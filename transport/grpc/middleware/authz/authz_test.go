package authz

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	authnEngine "github.com/tx7do/go-wind-plugins/security/authn"
	authzEngine "github.com/tx7do/go-wind-plugins/security/authz"
)

// fakeEngine is a minimal [engine.Engine] for testing.
type fakeEngine struct {
	authorized bool
	err        error

	capturedSubject  string
	capturedAction   string
	capturedResource string
	capturedProject  string
}

func (e *fakeEngine) Name() string { return "fake" }

func (e *fakeEngine) ProjectsAuthorized(_ context.Context, _ authzEngine.Subjects, _ authzEngine.Action, _ authzEngine.Resource, _ authzEngine.Projects) (authzEngine.Projects, error) {
	return nil, e.err
}

func (e *fakeEngine) FilterAuthorizedPairs(_ context.Context, _ authzEngine.Subjects, _ authzEngine.Pairs) (authzEngine.Pairs, error) {
	return nil, e.err
}

func (e *fakeEngine) FilterAuthorizedProjects(_ context.Context, _ authzEngine.Subjects) (authzEngine.Projects, error) {
	return nil, e.err
}

func (e *fakeEngine) IsAuthorized(_ context.Context, subject authzEngine.Subject, action authzEngine.Action, resource authzEngine.Resource, project authzEngine.Project) (bool, error) {
	e.capturedSubject = string(subject)
	e.capturedAction = string(action)
	e.capturedResource = string(resource)
	e.capturedProject = string(project)
	return e.authorized, e.err
}

func (e *fakeEngine) SetPolicies(_ context.Context, _ authzEngine.PolicyMap, _ authzEngine.RoleMap) error {
	return nil
}

// ctxWithClaims creates a context carrying authn claims, simulating
// the authn interceptor having already run.
func ctxWithClaims(subject string) context.Context {
	claims := &authnEngine.AuthClaims{authnEngine.ClaimFieldSubject: subject}
	return authnEngine.ContextWithAuthClaims(context.Background(), claims)
}

// ---------------------------------------------------------------------------
// UnaryInterceptor — basic authorize / deny
// ---------------------------------------------------------------------------

func TestUnaryInterceptor_Authorized(t *testing.T) {
	eng := &fakeEngine{authorized: true}

	var handlerCalled bool
	interceptor := UnaryInterceptor(eng)

	info := &grpc.UnaryServerInfo{FullMethod: "/pkg.UserService/GetUser"}
	handler := func(_ context.Context, _ any) (any, error) {
		handlerCalled = true
		return "ok", nil
	}

	resp, err := interceptor(ctxWithClaims("alice"), nil, info, handler)
	require.NoError(t, err)
	assert.Equal(t, "ok", resp)
	assert.True(t, handlerCalled)
	assert.Equal(t, "alice", eng.capturedSubject)
}

func TestUnaryInterceptor_Denied(t *testing.T) {
	eng := &fakeEngine{authorized: false}

	var handlerCalled bool
	interceptor := UnaryInterceptor(eng)

	info := &grpc.UnaryServerInfo{FullMethod: "/pkg.UserService/GetUser"}
	handler := func(_ context.Context, _ any) (any, error) {
		handlerCalled = true
		return nil, nil
	}

	resp, err := interceptor(ctxWithClaims("bob"), nil, info, handler)
	assert.Error(t, err)
	assert.Nil(t, resp)
	assert.False(t, handlerCalled)

	st, _ := status.FromError(err)
	assert.Equal(t, codes.PermissionDenied, st.Code())
}

func TestUnaryInterceptor_EngineError(t *testing.T) {
	eng := &fakeEngine{err: authzEngine.ErrInvalidClaims}

	interceptor := UnaryInterceptor(eng)

	info := &grpc.UnaryServerInfo{FullMethod: "/pkg.UserService/GetUser"}
	handler := func(_ context.Context, _ any) (any, error) {
		t.Fatal("handler should not be called")
		return nil, nil
	}

	_, err := interceptor(ctxWithClaims("carol"), nil, info, handler)
	require.Error(t, err)

	st, _ := status.FromError(err)
	assert.Equal(t, codes.PermissionDenied, st.Code())
}

// ---------------------------------------------------------------------------
// UnaryInterceptor — default resolvers
// ---------------------------------------------------------------------------

func TestUnaryInterceptor_DefaultActionResolver(t *testing.T) {
	eng := &fakeEngine{authorized: true}
	interceptor := UnaryInterceptor(eng)

	info := &grpc.UnaryServerInfo{FullMethod: "/pkg.UserService/GetUser"}
	handler := func(_ context.Context, _ any) (any, error) { return nil, nil }

	_, _ = interceptor(ctxWithClaims("alice"), nil, info, handler)

	// Default: method name extracted from FullMethod
	assert.Equal(t, "GetUser", eng.capturedAction)
}

func TestUnaryInterceptor_DefaultResourceResolver(t *testing.T) {
	eng := &fakeEngine{authorized: true}
	interceptor := UnaryInterceptor(eng)

	info := &grpc.UnaryServerInfo{FullMethod: "/pkg.UserService/DeleteUser"}
	handler := func(_ context.Context, _ any) (any, error) { return nil, nil }

	_, _ = interceptor(ctxWithClaims("alice"), nil, info, handler)

	// Default: service name extracted from FullMethod
	assert.Equal(t, "pkg.UserService", eng.capturedResource)
}

// ---------------------------------------------------------------------------
// UnaryInterceptor — custom resolvers
// ---------------------------------------------------------------------------

func TestUnaryInterceptor_CustomActionResolver(t *testing.T) {
	eng := &fakeEngine{authorized: true}
	interceptor := UnaryInterceptor(eng, WithActionResolver(func(info *grpc.UnaryServerInfo) string {
		if strings.Contains(info.FullMethod, "Get") {
			return "read"
		}
		return "write"
	}))

	info := &grpc.UnaryServerInfo{FullMethod: "/pkg.UserService/GetUser"}
	handler := func(_ context.Context, _ any) (any, error) { return nil, nil }

	_, _ = interceptor(ctxWithClaims("alice"), nil, info, handler)
	assert.Equal(t, "read", eng.capturedAction)
}

func TestUnaryInterceptor_CustomResourceResolver(t *testing.T) {
	eng := &fakeEngine{authorized: true}
	interceptor := UnaryInterceptor(eng, WithResourceResolver(func(_ *grpc.UnaryServerInfo) string {
		return "*"
	}))

	info := &grpc.UnaryServerInfo{FullMethod: "/pkg.UserService/GetUser"}
	handler := func(_ context.Context, _ any) (any, error) { return nil, nil }

	_, _ = interceptor(ctxWithClaims("alice"), nil, info, handler)
	assert.Equal(t, "*", eng.capturedResource)
}

func TestUnaryInterceptor_CustomSubjectResolver(t *testing.T) {
	eng := &fakeEngine{authorized: true}
	interceptor := UnaryInterceptor(eng, WithSubjectResolver(func(ctx context.Context) string {
		return "from-ctx"
	}))

	info := &grpc.UnaryServerInfo{FullMethod: "/pkg.Svc/Method"}
	handler := func(_ context.Context, _ any) (any, error) { return nil, nil }

	_, _ = interceptor(context.Background(), nil, info, handler)
	assert.Equal(t, "from-ctx", eng.capturedSubject)
}

func TestUnaryInterceptor_CustomProjectResolver(t *testing.T) {
	eng := &fakeEngine{authorized: true}

	interceptor := UnaryInterceptor(eng, WithProjectResolver(func(_ context.Context) string {
		return "proj-001"
	}))

	info := &grpc.UnaryServerInfo{FullMethod: "/pkg.Svc/Method"}
	handler := func(_ context.Context, _ any) (any, error) { return nil, nil }

	_, _ = interceptor(ctxWithClaims("alice"), nil, info, handler)
	assert.Equal(t, "proj-001", eng.capturedProject)
}

func TestUnaryInterceptor_NoAuthnClaims(t *testing.T) {
	eng := &fakeEngine{authorized: true}
	interceptor := UnaryInterceptor(eng)

	info := &grpc.UnaryServerInfo{FullMethod: "/pkg.Svc/Method"}
	handler := func(_ context.Context, _ any) (any, error) { return "ok", nil }

	// No authn claims → subject is empty, but engine still called.
	resp, err := interceptor(context.Background(), nil, info, handler)
	require.NoError(t, err)
	assert.Equal(t, "ok", resp)
	assert.Equal(t, "", eng.capturedSubject)
}

// ---------------------------------------------------------------------------
// UnaryInterceptor — skip methods
// ---------------------------------------------------------------------------

func TestUnaryInterceptor_SkipMethods(t *testing.T) {
	eng := &fakeEngine{authorized: false}

	var handlerCalled bool
	interceptor := UnaryInterceptor(eng, WithSkipMethods("/grpc.health.v1.Health/Check"))

	info := &grpc.UnaryServerInfo{FullMethod: "/grpc.health.v1.Health/Check"}
	handler := func(_ context.Context, _ any) (any, error) {
		handlerCalled = true
		return "ok", nil
	}

	// Even though engine denies, skipped method passes through.
	resp, err := interceptor(context.Background(), nil, info, handler)
	require.NoError(t, err)
	assert.Equal(t, "ok", resp)
	assert.True(t, handlerCalled)
}

// ---------------------------------------------------------------------------
// UnaryInterceptor — custom error func
// ---------------------------------------------------------------------------

func TestUnaryInterceptor_CustomErrorFunc(t *testing.T) {
	eng := &fakeEngine{authorized: false}

	interceptor := UnaryInterceptor(eng, WithErrorFunc(func(_ context.Context, _ error) error {
		return status.Error(codes.Internal, "custom error")
	}))

	info := &grpc.UnaryServerInfo{FullMethod: "/pkg.Svc/Method"}
	handler := func(_ context.Context, _ any) (any, error) {
		t.Fatal("handler should not be called")
		return nil, nil
	}

	_, err := interceptor(ctxWithClaims("alice"), nil, info, handler)
	require.Error(t, err)

	st, _ := status.FromError(err)
	assert.Equal(t, codes.Internal, st.Code())
}

// ---------------------------------------------------------------------------
// UnaryInterceptor — authz claims injected into context
// ---------------------------------------------------------------------------

func TestUnaryInterceptor_AuthzClaimsInContext(t *testing.T) {
	eng := &fakeEngine{authorized: true}

	var capturedClaims *authzEngine.AuthClaims
	interceptor := UnaryInterceptor(eng)

	info := &grpc.UnaryServerInfo{FullMethod: "/pkg.UserService/GetUser"}
	handler := func(ctx context.Context, _ any) (any, error) {
		capturedClaims, _ = authzEngine.AuthClaimsFromContext(ctx)
		return nil, nil
	}

	_, _ = interceptor(ctxWithClaims("alice"), nil, info, handler)
	require.NotNil(t, capturedClaims)
	require.NotNil(t, capturedClaims.Subject)
	assert.Equal(t, "alice", string(*capturedClaims.Subject))
	require.NotNil(t, capturedClaims.Action)
	assert.Equal(t, "GetUser", string(*capturedClaims.Action))
	require.NotNil(t, capturedClaims.Resource)
	assert.Equal(t, "pkg.UserService", string(*capturedClaims.Resource))
}

// ---------------------------------------------------------------------------
// StreamInterceptor
// ---------------------------------------------------------------------------

type fakeServerStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (s *fakeServerStream) Context() context.Context { return s.ctx }

func TestStreamInterceptor_Authorized(t *testing.T) {
	eng := &fakeEngine{authorized: true}

	var handlerCalled bool
	interceptor := StreamInterceptor(eng)

	info := &grpc.StreamServerInfo{FullMethod: "/pkg.DataService/StreamData"}
	ss := &fakeServerStream{ctx: ctxWithClaims("alice")}
	handler := func(_ any, _ grpc.ServerStream) error {
		handlerCalled = true
		return nil
	}

	err := interceptor(nil, ss, info, handler)
	require.NoError(t, err)
	assert.True(t, handlerCalled)
	assert.Equal(t, "alice", eng.capturedSubject)
	assert.Equal(t, "StreamData", eng.capturedAction)
	assert.Equal(t, "pkg.DataService", eng.capturedResource)
}

func TestStreamInterceptor_Denied(t *testing.T) {
	eng := &fakeEngine{authorized: false}

	var handlerCalled bool
	interceptor := StreamInterceptor(eng)

	info := &grpc.StreamServerInfo{FullMethod: "/pkg.DataService/StreamData"}
	ss := &fakeServerStream{ctx: ctxWithClaims("bob")}
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

func TestStreamInterceptor_SkipMethods(t *testing.T) {
	eng := &fakeEngine{authorized: false}

	var handlerCalled bool
	interceptor := StreamInterceptor(eng, WithSkipMethods("/pkg.Svc/PubSub"))

	info := &grpc.StreamServerInfo{FullMethod: "/pkg.Svc/PubSub"}
	ss := &fakeServerStream{ctx: context.Background()}
	handler := func(_ any, _ grpc.ServerStream) error {
		handlerCalled = true
		return nil
	}

	err := interceptor(nil, ss, info, handler)
	require.NoError(t, err)
	assert.True(t, handlerCalled)
}

func TestStreamInterceptor_CustomStreamResolvers(t *testing.T) {
	eng := &fakeEngine{authorized: true}

	interceptor := StreamInterceptor(eng,
		WithStreamActionResolver(func(_ *grpc.StreamServerInfo) string {
			return "stream-read"
		}),
		WithStreamResourceResolver(func(_ *grpc.StreamServerInfo) string {
			return "stream-resource"
		}),
	)

	info := &grpc.StreamServerInfo{FullMethod: "/pkg.Svc/Watch"}
	ss := &fakeServerStream{ctx: ctxWithClaims("alice")}
	handler := func(_ any, _ grpc.ServerStream) error { return nil }

	_ = interceptor(nil, ss, info, handler)
	assert.Equal(t, "stream-read", eng.capturedAction)
	assert.Equal(t, "stream-resource", eng.capturedResource)
}
