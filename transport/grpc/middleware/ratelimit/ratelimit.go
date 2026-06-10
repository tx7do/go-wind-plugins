// Package ratelimit provides gRPC server interceptors that enforce rate-limiting
// using any implementation of [ratelimit.Limiter] (e.g. token-bucket, BBR,
// Sentinel).
//
// This is the gRPC counterpart of transport/http/middleware/ratelimit.
//
// Usage:
//
//	limiter, _ := tokenbucket.New(100, 200)
//	srv := grpc.NewServer(grpc.ChainUnaryInterceptor(
//	    grpcRatelimit.UnaryInterceptor(limiter),
//	))
package ratelimit

import (
	"context"

	"github.com/tx7do/go-wind-plugins/ratelimit"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Option configures the rate-limit interceptor.
type Option func(*options)

type options struct {
	waitMode    bool
	skipMethods map[string]bool
}

// WithWait enables wait mode: instead of rejecting immediately, the interceptor
// blocks until the limiter allows the request or the context is cancelled.
func WithWait() Option {
	return func(o *options) { o.waitMode = true }
}

// WithSkipMethods adds full gRPC method names that should bypass rate-limiting.
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
// rate-limiting using the provided [ratelimit.Limiter].
func UnaryInterceptor(limiter ratelimit.Limiter, opts ...Option) grpc.UnaryServerInterceptor {
	cfg := &options{}
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

		if cfg.waitMode {
			if err := limiter.Wait(ctx); err != nil {
				return nil, status.Error(codes.ResourceExhausted, "rate limit exceeded")
			}
		} else {
			ok, err := limiter.Allow()
			if !ok || err != nil {
				return nil, status.Error(codes.ResourceExhausted, "rate limit exceeded")
			}
		}

		return handler(ctx, req)
	}
}

// StreamInterceptor returns a [grpc.StreamServerInterceptor] that enforces
// rate-limiting using the provided [ratelimit.Limiter].
func StreamInterceptor(limiter ratelimit.Limiter, opts ...Option) grpc.StreamServerInterceptor {
	cfg := &options{}
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

		if cfg.waitMode {
			if err := limiter.Wait(ss.Context()); err != nil {
				return status.Error(codes.ResourceExhausted, "rate limit exceeded")
			}
		} else {
			ok, err := limiter.Allow()
			if !ok || err != nil {
				return status.Error(codes.ResourceExhausted, "rate limit exceeded")
			}
		}

		return handler(srv, ss)
	}
}
