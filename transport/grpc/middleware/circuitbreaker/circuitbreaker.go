// Package circuitbreaker provides gRPC server interceptors that enforce a
// circuit-breaker policy using any implementation of
// [circuitbreaker.CircuitBreaker] (e.g. SRE, Hystrix, Vegas, Sentinel).
//
// This is the gRPC counterpart of transport/http/middleware/circuitbreaker.
//
// Before calling the handler the interceptor calls [CircuitBreaker.Allow].
// If the circuit is open, the RPC is rejected with codes.Unavailable.
// Otherwise the handler executes and the gRPC status code determines whether
// [MarkSuccess] or [MarkFailure] is called.
//
// By default any gRPC error with code >= Internal is treated as a failure.
// Customise with [WithFailureCodes].
//
// Usage:
//
//	cb, _ := sres.New(sres.WithFailureRatio(0.5))
//	srv := grpc.NewServer(grpc.ChainUnaryInterceptor(
//	    grpcCircuitBreaker.UnaryInterceptor(cb),
//	))
package circuitbreaker

import (
	"context"

	"github.com/tx7do/go-wind-plugins/circuitbreaker"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Option configures the circuit-breaker interceptor.
type Option func(*options)

type options struct {
	failureCodes map[codes.Code]bool
	skipMethods  map[string]bool
}

// WithFailureCodes sets the gRPC status codes that are treated as failures.
// By default any code >= codes.Internal is a failure (Internal, Unknown, etc.).
func WithFailureCodes(cs ...codes.Code) Option {
	return func(o *options) {
		o.failureCodes = make(map[codes.Code]bool, len(cs))
		for _, c := range cs {
			o.failureCodes[c] = true
		}
	}
}

// WithSkipMethods adds full gRPC method names that should bypass the breaker.
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

// UnaryInterceptor returns a [grpc.UnaryServerInterceptor] that enforces a
// circuit-breaker policy using the provided [circuitbreaker.CircuitBreaker].
func UnaryInterceptor(cb circuitbreaker.CircuitBreaker, opts ...Option) grpc.UnaryServerInterceptor {
	cfg := &options{}
	for _, opt := range opts {
		opt(cfg)
	}

	isFailure := func(err error) bool {
		if err == nil {
			return false
		}
		st, _ := status.FromError(err)
		if cfg.failureCodes != nil {
			return cfg.failureCodes[st.Code()]
		}
		return st.Code() >= codes.Internal
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

		if err := cb.Allow(); err != nil {
			return nil, status.Error(codes.Unavailable, "circuit breaker is open")
		}

		resp, err := handler(ctx, req)
		if isFailure(err) {
			cb.MarkFailure()
		} else {
			cb.MarkSuccess()
		}
		return resp, err
	}
}

// StreamInterceptor returns a [grpc.StreamServerInterceptor] that enforces a
// circuit-breaker policy using the provided [circuitbreaker.CircuitBreaker].
func StreamInterceptor(cb circuitbreaker.CircuitBreaker, opts ...Option) grpc.StreamServerInterceptor {
	cfg := &options{}
	for _, opt := range opts {
		opt(cfg)
	}

	isFailure := func(err error) bool {
		if err == nil {
			return false
		}
		st, _ := status.FromError(err)
		if cfg.failureCodes != nil {
			return cfg.failureCodes[st.Code()]
		}
		return st.Code() >= codes.Internal
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

		if err := cb.Allow(); err != nil {
			return status.Error(codes.Unavailable, "circuit breaker is open")
		}

		err := handler(srv, ss)
		if isFailure(err) {
			cb.MarkFailure()
		} else {
			cb.MarkSuccess()
		}
		return err
	}
}
