// Package metadata provides gRPC interceptors (both server and client side)
// and an HTTP middleware that propagate arbitrary key-value metadata between
// services.
//
// On the server side, metadata is extracted from incoming gRPC metadata
// (or HTTP headers) and stored in the request context for downstream handlers
// to consume.
//
// On the client side, metadata stored in the context is injected into outgoing
// gRPC metadata (or HTTP headers) before the RPC is sent.
//
// Unlike the dedicated requestid middleware (which handles a single key),
// this middleware handles an arbitrary set of keys, making it suitable for
// propagating tenant IDs, correlation IDs, feature flags, user locale, and
// other cross-cutting concerns.
//
// Usage (gRPC server):
//
//	srv := grpc.NewServer(grpc.ChainUnaryInterceptor(
//	    grpcMeta.UnaryServerInterceptor("x-tenant-id", "x-user-id"),
//	))
//
// Usage (gRPC client):
//
//	conn, _ := grpc.NewClient(addr,
//	    grpc.WithUnaryInterceptor(grpcMeta.UnaryClientInterceptor("x-tenant-id")),
//	)
//
// To read/write metadata in a handler:
//
//	tenantID := grpcMeta.FromContext(ctx, "x-tenant-id")
//	ctx = grpcMeta.WithMetadata(ctx, "x-tenant-id", "acme")
package metadata

import (
	"context"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

// ctxKey is an unexported type for context keys in this package.
type ctxKey struct {
	key string
}

// Option configures the metadata interceptors/middleware.
type Option func(*options)

type options struct {
	// keys are the metadata keys to propagate.
	keys []string

	// constantMD is metadata that is always injected on the client side,
	// regardless of what's in the context.
	constantMD map[string]string
}

// WithKeys sets the metadata keys to propagate between services.
func WithKeys(keys ...string) Option {
	return func(o *options) { o.keys = append(o.keys, keys...) }
}

// WithConstant sets metadata key-value pairs that are always injected into
// outgoing requests on the client side. This is useful for service-level
// metadata like service name or version.
func WithConstant(kv map[string]string) Option {
	return func(o *options) {
		if o.constantMD == nil {
			o.constantMD = make(map[string]string)
		}
		for k, v := range kv {
			o.constantMD[k] = v
		}
	}
}

// ---------------------------------------------------------------------------
// gRPC Server interceptors
// ---------------------------------------------------------------------------

// UnaryServerInterceptor returns a [grpc.UnaryServerInterceptor] that extracts
// the configured keys from incoming gRPC metadata and stores them in the
// request context.
func UnaryServerInterceptor(opts ...Option) grpc.UnaryServerInterceptor {
	cfg := newConfig(opts)
	return func(
		ctx context.Context,
		req any,
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (any, error) {
		ctx = extractToContext(ctx, cfg.keys)
		return handler(ctx, req)
	}
}

// StreamServerInterceptor returns a [grpc.StreamServerInterceptor] that
// extracts the configured keys from incoming gRPC metadata.
func StreamServerInterceptor(opts ...Option) grpc.StreamServerInterceptor {
	cfg := newConfig(opts)
	return func(
		srv any,
		ss grpc.ServerStream,
		info *grpc.StreamServerInfo,
		handler grpc.StreamHandler,
	) error {
		ctx := extractToContext(ss.Context(), cfg.keys)
		wrapped := &wrappedServerStream{ServerStream: ss, ctx: ctx}
		return handler(srv, wrapped)
	}
}

// ---------------------------------------------------------------------------
// gRPC Client interceptors
// ---------------------------------------------------------------------------

// UnaryClientInterceptor returns a [grpc.UnaryClientInterceptor] that injects
// metadata from the context into outgoing gRPC metadata.
func UnaryClientInterceptor(opts ...Option) grpc.UnaryClientInterceptor {
	cfg := newConfig(opts)
	return func(
		ctx context.Context,
		method string,
		req, reply any,
		cc *grpc.ClientConn,
		invoker grpc.UnaryInvoker,
		callOpts ...grpc.CallOption,
	) error {
		ctx = injectFromContext(ctx, cfg.keys, cfg)
		return invoker(ctx, method, req, reply, cc, callOpts...)
	}
}

// StreamClientInterceptor returns a [grpc.StreamClientInterceptor] that injects
// metadata from the context into outgoing gRPC metadata.
func StreamClientInterceptor(opts ...Option) grpc.StreamClientInterceptor {
	cfg := newConfig(opts)
	return func(
		ctx context.Context,
		desc *grpc.StreamDesc,
		cc *grpc.ClientConn,
		method string,
		streamer grpc.Streamer,
		callOpts ...grpc.CallOption,
	) (grpc.ClientStream, error) {
		ctx = injectFromContext(ctx, cfg.keys, cfg)
		return streamer(ctx, desc, cc, method, callOpts...)
	}
}

// ---------------------------------------------------------------------------
// Context helpers
// ---------------------------------------------------------------------------

// WithMetadata stores a metadata value in the context under the given key.
func WithMetadata(ctx context.Context, key, value string) context.Context {
	return context.WithValue(ctx, ctxKey{key: key}, value)
}

// FromContext retrieves a metadata value from the context.
// Returns an empty string if the key is not present.
func FromContext(ctx context.Context, key string) string {
	if ctx == nil {
		return ""
	}
	v, _ := ctx.Value(ctxKey{key: key}).(string)
	return v
}

// ---------------------------------------------------------------------------
// Internal helpers
// ---------------------------------------------------------------------------

func newConfig(opts []Option) *options {
	cfg := &options{}
	for _, opt := range opts {
		opt(cfg)
	}
	return cfg
}

// extractToContext reads the specified keys from incoming gRPC metadata and
// stores them in a new context.
func extractToContext(ctx context.Context, keys []string) context.Context {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return ctx
	}
	for _, key := range keys {
		values := md.Get(key)
		if len(values) > 0 {
			ctx = WithMetadata(ctx, key, values[0])
		}
	}
	return ctx
}

// injectFromContext reads the specified keys from the context and appends
// them to outgoing gRPC metadata. Constant metadata from options is also
// injected.
func injectFromContext(ctx context.Context, keys []string, cfg *options) context.Context {
	pairs := make([]string, 0, (len(keys)+len(cfg.constantMD))*2)

	// Inject context metadata.
	for _, key := range keys {
		if val := FromContext(ctx, key); val != "" {
			pairs = append(pairs, key, val)
		}
	}

	// Inject constant metadata.
	for k, v := range cfg.constantMD {
		pairs = append(pairs, k, v)
	}

	if len(pairs) > 0 {
		ctx = metadata.AppendToOutgoingContext(ctx, pairs...)
	}
	return ctx
}

// wrappedServerStream wraps a [grpc.ServerStream] to override its context.
type wrappedServerStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (w *wrappedServerStream) Context() context.Context {
	return w.ctx
}
