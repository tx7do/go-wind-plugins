// Package requestid provides gRPC interceptors (both server and client side)
// that extract, generate, and propagate a unique request ID through gRPC
// metadata.
//
// The request ID is carried in the "x-request-id" gRPC metadata key. On the
// server side it is extracted from incoming metadata and stored in the
// context. On the client side it is injected from the context (or generated)
// into outgoing metadata.
//
// Usage (server):
//
//	srv := grpc.NewServer(grpc.ChainUnaryInterceptor(
//	    grpcRequestID.UnaryServerInterceptor(),
//	))
//
// Usage (client):
//
//	conn, _ := grpc.NewClient(addr,
//	    grpc.WithUnaryInterceptor(grpcRequestID.UnaryClientInterceptor()),
//	)
//
// To retrieve the request ID inside a handler:
//
//	id := grpcRequestID.FromContext(ctx)
package requestid

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

// MetadataKey is the gRPC metadata key for the request ID.
const MetadataKey = "x-request-id"

// ctxKey is an unexported type for context keys in this package.
type ctxKey struct{}

// Option configures the requestid interceptors.
type Option func(*options)

type options struct {
	idGenerator func() string
}

// WithIDGenerator sets a custom function for generating new request IDs.
// Default: a 16-byte hex-encoded random ID.
func WithIDGenerator(fn func() string) Option {
	return func(o *options) { o.idGenerator = fn }
}

// ---------------------------------------------------------------------------
// Server interceptors
// ---------------------------------------------------------------------------

// UnaryServerInterceptor returns a [grpc.UnaryServerInterceptor] that extracts
// the request ID from incoming metadata and stores it in the context.
func UnaryServerInterceptor(opts ...Option) grpc.UnaryServerInterceptor {
	cfg := newConfig(opts)

	return func(
		ctx context.Context,
		req any,
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (any, error) {
		id := extractFromIncoming(ctx)
		if id == "" {
			id = cfg.idGenerator()
		}
		return handler(WithRequestID(ctx, id), req)
	}
}

// StreamServerInterceptor returns a [grpc.StreamServerInterceptor] that
// extracts the request ID from incoming metadata.
func StreamServerInterceptor(opts ...Option) grpc.StreamServerInterceptor {
	cfg := newConfig(opts)

	return func(
		srv any,
		ss grpc.ServerStream,
		info *grpc.StreamServerInfo,
		handler grpc.StreamHandler,
	) error {
		id := extractFromIncoming(ss.Context())
		if id == "" {
			id = cfg.idGenerator()
		}
		wrapped := &wrappedServerStream{ServerStream: ss, ctx: WithRequestID(ss.Context(), id)}
		return handler(srv, wrapped)
	}
}

// ---------------------------------------------------------------------------
// Client interceptors
// ---------------------------------------------------------------------------

// UnaryClientInterceptor returns a [grpc.UnaryClientInterceptor] that injects
// the request ID from the context into outgoing metadata.
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
		return invoker(injectOutgoing(ctx, cfg), method, req, reply, cc, callOpts...)
	}
}

// StreamClientInterceptor returns a [grpc.StreamClientInterceptor] that injects
// the request ID from the context into outgoing metadata.
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
		return streamer(injectOutgoing(ctx, cfg), desc, cc, method, callOpts...)
	}
}

// ---------------------------------------------------------------------------
// Context helpers
// ---------------------------------------------------------------------------

// WithRequestID stores a request ID in the context.
func WithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, ctxKey{}, id)
}

// FromContext extracts the request ID from the context.
// Returns an empty string if no ID is present.
func FromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	id, _ := ctx.Value(ctxKey{}).(string)
	return id
}

// ---------------------------------------------------------------------------
// Internal helpers
// ---------------------------------------------------------------------------

func newConfig(opts []Option) *options {
	cfg := &options{
		idGenerator: defaultIDGenerator,
	}
	for _, opt := range opts {
		opt(cfg)
	}
	return cfg
}

// extractFromIncoming reads the request ID from incoming gRPC metadata.
func extractFromIncoming(ctx context.Context) string {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return ""
	}
	values := md.Get(MetadataKey)
	if len(values) == 0 {
		return ""
	}
	return strings.TrimSpace(values[0])
}

// injectOutgoing injects the request ID into outgoing metadata. If the context
// already has an ID it is used; otherwise a new one is generated.
func injectOutgoing(ctx context.Context, cfg *options) context.Context {
	id := FromContext(ctx)
	if id == "" {
		id = cfg.idGenerator()
	}
	return metadata.AppendToOutgoingContext(ctx, MetadataKey, id)
}

// defaultIDGenerator generates a 16-byte hex-encoded random ID.
func defaultIDGenerator() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// wrappedServerStream wraps a [grpc.ServerStream] to override its context.
type wrappedServerStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (w *wrappedServerStream) Context() context.Context {
	return w.ctx
}
