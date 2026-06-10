// Package timeout provides gRPC interceptors (both server and client side)
// that enforce deadlines on RPC execution.
//
// Server-side: if the incoming context has no deadline, a default deadline is
// applied. If the handler exceeds the deadline, it is cancelled and a
// DeadlineExceeded status is returned.
//
// Client-side: if the outgoing context has no deadline, a default deadline is
// applied before the RPC is sent.
//
// Usage (server):
//
//	srv := grpc.NewServer(grpc.ChainUnaryInterceptor(
//	    grpcTimeout.UnaryServerInterceptor(30 * time.Second),
//	))
//
// Usage (client):
//
//	conn, _ := grpc.NewClient(addr,
//	    grpc.WithUnaryInterceptor(grpcTimeout.UnaryClientInterceptor(10 * time.Second)),
//	)
package timeout

import (
	"context"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Option configures the timeout interceptors.
type Option func(*options)

type options struct {
	skipFunc func(method string) bool
}

// WithSkipFunc sets a function that returns true for methods to bypass the
// timeout (e.g. streaming or long-running RPCs).
func WithSkipFunc(fn func(method string) bool) Option {
	return func(o *options) { o.skipFunc = fn }
}

// ---------------------------------------------------------------------------
// Server interceptors
// ---------------------------------------------------------------------------

// UnaryServerInterceptor returns a [grpc.UnaryServerInterceptor] that enforces
// a default deadline on incoming unary RPCs when none is present.
func UnaryServerInterceptor(defaultTimeout time.Duration, opts ...Option) grpc.UnaryServerInterceptor {
	cfg := newConfig(opts)

	return func(
		ctx context.Context,
		req any,
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (any, error) {
		if cfg.skipFunc != nil && cfg.skipFunc(info.FullMethod) {
			return handler(ctx, req)
		}

		if _, ok := ctx.Deadline(); !ok && defaultTimeout > 0 {
			var cancel context.CancelFunc
			ctx, cancel = context.WithTimeout(ctx, defaultTimeout)
			defer cancel()
		}

		resp, err := handler(ctx, req)
		if err != nil {
			return resp, err
		}
		if ctx.Err() != nil {
			return nil, status.Error(codes.DeadlineExceeded, "handler exceeded deadline")
		}
		return resp, nil
	}
}

// StreamServerInterceptor returns a [grpc.StreamServerInterceptor] that enforces
// a default deadline on incoming streaming RPCs when none is present.
func StreamServerInterceptor(defaultTimeout time.Duration, opts ...Option) grpc.StreamServerInterceptor {
	cfg := newConfig(opts)

	return func(
		srv any,
		ss grpc.ServerStream,
		info *grpc.StreamServerInfo,
		handler grpc.StreamHandler,
	) error {
		if cfg.skipFunc != nil && cfg.skipFunc(info.FullMethod) {
			return handler(srv, ss)
		}

		ctx := ss.Context()
		if _, ok := ctx.Deadline(); !ok && defaultTimeout > 0 {
			var cancel context.CancelFunc
			ctx, cancel = context.WithTimeout(ctx, defaultTimeout)
			defer cancel()
			ss = &wrappedServerStream{ServerStream: ss, ctx: ctx}
		}

		return handler(srv, ss)
	}
}

// ---------------------------------------------------------------------------
// Client interceptors
// ---------------------------------------------------------------------------

// UnaryClientInterceptor returns a [grpc.UnaryClientInterceptor] that applies
// a default deadline on outgoing unary RPCs when none is present.
func UnaryClientInterceptor(defaultTimeout time.Duration, opts ...Option) grpc.UnaryClientInterceptor {
	cfg := newConfig(opts)

	return func(
		ctx context.Context,
		method string,
		req, reply any,
		cc *grpc.ClientConn,
		invoker grpc.UnaryInvoker,
		callOpts ...grpc.CallOption,
	) error {
		if cfg.skipFunc != nil && cfg.skipFunc(method) {
			return invoker(ctx, method, req, reply, cc, callOpts...)
		}

		if _, ok := ctx.Deadline(); !ok && defaultTimeout > 0 {
			var cancel context.CancelFunc
			ctx, cancel = context.WithTimeout(ctx, defaultTimeout)
			defer cancel()
		}

		return invoker(ctx, method, req, reply, cc, callOpts...)
	}
}

// StreamClientInterceptor returns a [grpc.StreamClientInterceptor] that applies
// a default deadline on outgoing streaming RPCs when none is present.
func StreamClientInterceptor(defaultTimeout time.Duration, opts ...Option) grpc.StreamClientInterceptor {
	cfg := newConfig(opts)

	return func(
		ctx context.Context,
		desc *grpc.StreamDesc,
		cc *grpc.ClientConn,
		method string,
		streamer grpc.Streamer,
		callOpts ...grpc.CallOption,
	) (grpc.ClientStream, error) {
		if cfg.skipFunc != nil && cfg.skipFunc(method) {
			return streamer(ctx, desc, cc, method, callOpts...)
		}

		if _, ok := ctx.Deadline(); !ok && defaultTimeout > 0 {
			var cancel context.CancelFunc
			ctx, cancel = context.WithTimeout(ctx, defaultTimeout)
			defer cancel()
		}

		return streamer(ctx, desc, cc, method, callOpts...)
	}
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

// wrappedServerStream wraps a [grpc.ServerStream] to override its context.
type wrappedServerStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (w *wrappedServerStream) Context() context.Context {
	return w.ctx
}
