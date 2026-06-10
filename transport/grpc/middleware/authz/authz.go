// Package authz provides a gRPC server interceptor that enforces authorization
// decisions using an [engine.Engine] (e.g. casbin, opa, acl, rbac).
//
// This is the gRPC counterpart of transport/http/middleware/authz.
//
// The interceptor expects authentication claims to already be in the RPC
// context — typically injected by the authn interceptor. It extracts the
// subject from the authn claims, then calls the authz engine's
// [engine.IsAuthorized] to decide whether to allow or deny the request.
//
// Usage:
//
//	import (
//	    grpcAuthn "github.com/tx7do/go-wind-plugins/transport/grpc/middleware/authn"
//	    grpcAuthz "github.com/tx7do/go-wind-plugins/transport/grpc/middleware/authz"
//	    authnEngine "github.com/tx7do/go-wind-plugins/security/authn"
//	    "github.com/tx7do/go-wind-plugins/security/authz/acl"
//	)
//
//	authzEng, _ := acl.NewEngine(ctx,
//	    acl.WithRule("alice", "GetUser", "UserService"),
//	)
//	srv := grpc.NewServer(grpc.ChainUnaryInterceptor(
//	    grpcAuthn.UnaryInterceptor(authenticator),
//	    grpcAuthz.UnaryInterceptor(authzEng),
//	))
//
// On failure the interceptor returns a gRPC PermissionDenied error and does
// NOT call the handler.
package authz

import (
	"context"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	authnEngine "github.com/tx7do/go-wind-plugins/security/authn"
	authzEngine "github.com/tx7do/go-wind-plugins/security/authz"
)

// Option configures the authz interceptor.
type Option func(*options)

// ResolverFunc extracts the action or resource string from gRPC method info.
type ResolverFunc func(info *grpc.UnaryServerInfo) string

// StreamResolverFunc is the streaming counterpart of ResolverFunc.
type StreamResolverFunc func(info *grpc.StreamServerInfo) string

type options struct {
	// actionResolver maps a unary RPC to an authorization action.
	// If nil, the gRPC method name is used (e.g. "GetUser").
	actionResolver ResolverFunc

	// resourceResolver maps a unary RPC to an authorization resource.
	// If nil, the gRPC service name is used (e.g. "pkg.UserService").
	resourceResolver ResolverFunc

	// subjectResolver extracts the subject from the RPC context.
	// If nil, the authn claims subject is used.
	subjectResolver func(ctx context.Context) string

	// projectResolver extracts the project from the RPC context.
	// If nil, no project is passed (empty string).
	projectResolver func(ctx context.Context) string

	// errorFunc allows the caller to transform authorization errors.
	errorFunc func(ctx context.Context, err error) error

	// skipMethods bypasses authorization for specified methods.
	skipMethods map[string]bool

	// streamActionResolver is the streaming counterpart of actionResolver.
	streamActionResolver StreamResolverFunc

	// streamResourceResolver is the streaming counterpart of resourceResolver.
	streamResourceResolver StreamResolverFunc
}

// WithActionResolver sets a custom function to derive the authorization
// action from unary RPC info.
func WithActionResolver(fn ResolverFunc) Option {
	return func(o *options) { o.actionResolver = fn }
}

// WithResourceResolver sets a custom function to derive the authorization
// resource from unary RPC info.
func WithResourceResolver(fn ResolverFunc) Option {
	return func(o *options) { o.resourceResolver = fn }
}

// WithStreamActionResolver sets a custom action resolver for streaming RPCs.
func WithStreamActionResolver(fn StreamResolverFunc) Option {
	return func(o *options) { o.streamActionResolver = fn }
}

// WithStreamResourceResolver sets a custom resource resolver for streaming RPCs.
func WithStreamResourceResolver(fn StreamResolverFunc) Option {
	return func(o *options) { o.streamResourceResolver = fn }
}

// WithSubjectResolver sets a custom function to extract the subject
// from the context.
func WithSubjectResolver(fn func(ctx context.Context) string) Option {
	return func(o *options) { o.subjectResolver = fn }
}

// WithProjectResolver sets a custom function to extract the project
// from the context.
func WithProjectResolver(fn func(ctx context.Context) string) Option {
	return func(o *options) { o.projectResolver = fn }
}

// WithErrorFunc sets a custom error transformer for authorization failures.
func WithErrorFunc(fn func(ctx context.Context, err error) error) Option {
	return func(o *options) { o.errorFunc = fn }
}

// WithSkipMethods adds full gRPC method names that should bypass authorization.
func WithSkipMethods(methods ...string) Option {
	return func(o *options) {
		if o.skipMethods == nil {
			o.skipMethods = make(map[string]bool)
		}
		for _, m := range methods {
			o.skipMethods[m] = true
		}
	}
}

// UnaryInterceptor returns a [grpc.UnaryServerInterceptor] that enforces
// authorization on every incoming unary RPC.
//
// It should be placed AFTER the authn interceptor in the chain, as it
// relies on authn claims being in the RPC context.
func UnaryInterceptor(eng authzEngine.Engine, opts ...Option) grpc.UnaryServerInterceptor {
	cfg := &options{
		actionResolver:   defaultUnaryActionResolver,
		resourceResolver: defaultUnaryResourceResolver,
		errorFunc: func(_ context.Context, _ error) error {
			return status.Error(codes.PermissionDenied, "permission denied")
		},
	}
	for _, opt := range opts {
		opt(cfg)
	}

	return func(
		ctx context.Context,
		req any,
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (any, error) {
		if cfg.skipMethods[info.FullMethod] {
			return handler(ctx, req)
		}

		subject := extractSubject(ctx, cfg)
		action := cfg.actionResolver(info)
		resource := cfg.resourceResolver(info)
		project := extractProject(ctx, cfg)

		authorized, err := eng.IsAuthorized(ctx,
			authzEngine.Subject(subject),
			authzEngine.Action(action),
			authzEngine.Resource(resource),
			authzEngine.Project(project),
		)
		if err != nil {
			return nil, cfg.errorFunc(ctx, err)
		}
		if !authorized {
			return nil, cfg.errorFunc(ctx, authzEngine.ErrMissingAuthClaims)
		}

		ctx = authzEngine.ContextWithAuthClaims(ctx, &authzEngine.AuthClaims{
			Subject:  (*authzEngine.Subject)(&subject),
			Action:   (*authzEngine.Action)(&action),
			Resource: (*authzEngine.Resource)(&resource),
			Project:  (*authzEngine.Project)(&project),
		})
		return handler(ctx, req)
	}
}

// StreamInterceptor returns a [grpc.StreamServerInterceptor] that enforces
// authorization on every incoming streaming RPC.
func StreamInterceptor(eng authzEngine.Engine, opts ...Option) grpc.StreamServerInterceptor {
	// Apply options once at setup time.
	cfg := &options{
		actionResolver:   defaultUnaryActionResolver,
		resourceResolver: defaultUnaryResourceResolver,
		errorFunc: func(_ context.Context, _ error) error {
			return status.Error(codes.PermissionDenied, "permission denied")
		},
	}
	for _, opt := range opts {
		opt(cfg)
	}

	// Build stream resolvers, falling back to unary resolvers that read
	// FullMethod from the stream info.
	streamAction := cfg.streamActionResolver
	if streamAction == nil {
		streamAction = func(info *grpc.StreamServerInfo) string {
			return defaultActionFromMethod(info.FullMethod)
		}
	}
	streamResource := cfg.streamResourceResolver
	if streamResource == nil {
		streamResource = func(info *grpc.StreamServerInfo) string {
			return defaultResourceFromMethod(info.FullMethod)
		}
	}

	return func(
		srv any,
		ss grpc.ServerStream,
		info *grpc.StreamServerInfo,
		handler grpc.StreamHandler,
	) error {
		if cfg.skipMethods[info.FullMethod] {
			return handler(srv, ss)
		}

		ctx := ss.Context()
		subject := extractSubject(ctx, cfg)
		action := streamAction(info)
		resource := streamResource(info)
		project := extractProject(ctx, cfg)

		authorized, err := eng.IsAuthorized(ctx,
			authzEngine.Subject(subject),
			authzEngine.Action(action),
			authzEngine.Resource(resource),
			authzEngine.Project(project),
		)
		if err != nil {
			return cfg.errorFunc(ctx, err)
		}
		if !authorized {
			return cfg.errorFunc(ctx, authzEngine.ErrMissingAuthClaims)
		}

		ctx = authzEngine.ContextWithAuthClaims(ctx, &authzEngine.AuthClaims{
			Subject:  (*authzEngine.Subject)(&subject),
			Action:   (*authzEngine.Action)(&action),
			Resource: (*authzEngine.Resource)(&resource),
			Project:  (*authzEngine.Project)(&project),
		})
		wrapped := &wrappedServerStream{ServerStream: ss, ctx: ctx}
		return handler(srv, wrapped)
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func extractSubject(ctx context.Context, cfg *options) string {
	if cfg.subjectResolver != nil {
		return cfg.subjectResolver(ctx)
	}
	if claims, ok := authnEngine.AuthClaimsFromContext(ctx); ok {
		sub, _ := claims.GetSubject()
		return sub
	}
	return ""
}

func extractProject(ctx context.Context, cfg *options) string {
	if cfg.projectResolver != nil {
		return cfg.projectResolver(ctx)
	}
	return ""
}

// defaultUnaryActionResolver extracts the method name from FullMethod.
// FullMethod format: "/pkg.ServiceName/MethodName" → "MethodName"
func defaultUnaryActionResolver(info *grpc.UnaryServerInfo) string {
	return defaultActionFromMethod(info.FullMethod)
}

// defaultUnaryResourceResolver extracts the service name from FullMethod.
// FullMethod format: "/pkg.ServiceName/MethodName" → "pkg.ServiceName"
func defaultUnaryResourceResolver(info *grpc.UnaryServerInfo) string {
	return defaultResourceFromMethod(info.FullMethod)
}

// defaultActionFromMethod returns the method portion of a FullMethod string.
func defaultActionFromMethod(fullMethod string) string {
	parts := strings.Split(fullMethod, "/")
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}
	return fullMethod
}

// defaultResourceFromMethod returns the service portion of a FullMethod string.
func defaultResourceFromMethod(fullMethod string) string {
	parts := strings.Split(fullMethod, "/")
	if len(parts) >= 2 {
		return parts[len(parts)-2]
	}
	return fullMethod
}

// wrappedServerStream wraps a [grpc.ServerStream] to override its context.
type wrappedServerStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (w *wrappedServerStream) Context() context.Context {
	return w.ctx
}
