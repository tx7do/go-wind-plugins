// Package authn provides a gRPC server interceptor that authenticates
// incoming RPCs via an [engine.Authenticator] and injects the resulting
// [*engine.AuthClaims] into the RPC context for downstream handlers.
//
// This is the gRPC counterpart of transport/http/middleware/authn.
//
// Usage:
//
//	import (
//	    grpcAuthn "github.com/tx7do/go-wind-plugins/transport/grpc/middleware/authn"
//	    jwtAuthn "github.com/tx7do/go-wind-plugins/security/authn/jwt"
//	)
//
//	authenticator, _ := jwtAuthn.NewAuthenticator(jwtAuthn.WithKey(secret))
//	srv := grpc.NewServer(grpc.ChainUnaryInterceptor(
//	    grpcAuthn.UnaryInterceptor(authenticator),
//	))
//
// Inside the service handler, claims are retrieved via:
//
//	claims, ok := engine.AuthClaimsFromContext(ctx)
package authn

import (
	"context"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	engine "github.com/tx7do/go-wind-plugins/security/authn"
)

// Option configures the authn interceptor.
type Option func(*options)

type options struct {
	// errorFunc allows the caller to transform or wrap authentication errors
	// before they are returned to the gRPC client. If nil, the original
	// error is returned as-is.
	errorFunc func(ctx context.Context, err error) error

	// skipMethods is a set of full method names (e.g. "/grpc.health.v1.Health/Check")
	// that should bypass authentication. Useful for health checks.
	skipMethods map[string]bool
}

// WithErrorFunc sets a custom error transformer for authentication failures.
func WithErrorFunc(fn func(ctx context.Context, err error) error) Option {
	return func(o *options) { o.errorFunc = fn }
}

// WithSkipMethods adds full gRPC method names that should bypass authentication.
// e.g. grpcAuthn.WithSkipMethods("/grpc.health.v1.Health/Check")
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

// UnaryInterceptor returns a [grpc.UnaryServerInterceptor] that authenticates
// every incoming unary RPC using the provided [engine.Authenticator].
//
// On success the resulting [*engine.AuthClaims] are injected into the
// RPC context via [engine.ContextWithAuthClaims]. Downstream handlers
// retrieve them with [engine.AuthClaimsFromContext].
//
// On failure the interceptor returns a gRPC Unauthenticated error and does
// NOT call the handler.
func UnaryInterceptor(auth engine.Authenticator, opts ...Option) grpc.UnaryServerInterceptor {
	cfg := &options{
		errorFunc: func(_ context.Context, err error) error {
			return status.Error(codes.Unauthenticated, err.Error())
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
		// Skip authentication for specified methods (e.g. health checks).
		if cfg.skipMethods[info.FullMethod] {
			return handler(ctx, req)
		}

		// Authenticate.
		claims, err := auth.Authenticate(ctx)
		if err != nil {
			return nil, cfg.errorFunc(ctx, err)
		}

		// Inject claims into the context for downstream handlers.
		ctx = engine.ContextWithAuthClaims(ctx, claims)
		return handler(ctx, req)
	}
}

// StreamInterceptor returns a [grpc.StreamServerInterceptor] that authenticates
// every incoming streaming RPC.
//
// It works identically to [UnaryInterceptor] but for streaming RPCs.
func StreamInterceptor(auth engine.Authenticator, opts ...Option) grpc.StreamServerInterceptor {
	cfg := &options{
		errorFunc: func(_ context.Context, err error) error {
			return status.Error(codes.Unauthenticated, err.Error())
		},
	}
	for _, opt := range opts {
		opt(cfg)
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
		claims, err := auth.Authenticate(ctx)
		if err != nil {
			return cfg.errorFunc(ctx, err)
		}

		ctx = engine.ContextWithAuthClaims(ctx, claims)
		wrapped := &wrappedServerStream{ServerStream: ss, ctx: ctx}
		return handler(srv, wrapped)
	}
}

// wrappedServerStream wraps a [grpc.ServerStream] to override its context,
// allowing the injected claims to flow through to the service handler.
type wrappedServerStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (w *wrappedServerStream) Context() context.Context {
	return w.ctx
}
