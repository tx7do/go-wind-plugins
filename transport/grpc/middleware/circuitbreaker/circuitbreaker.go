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
// By default only server-side fault codes are treated as failures (Unknown,
// DeadlineExceeded, Internal, Unavailable, DataLoss — see [defaultFailureCodes]).
// Caller-fault codes (InvalidArgument, NotFound, PermissionDenied, ...) count
// as successes so a flood of bad client requests cannot open the circuit.
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

// defaultFailureCodes lists the gRPC status codes treated as failures when no
// explicit [WithFailureCodes] option is supplied: the server-side faults, i.e.
// codes that mean the request reached the service but the service (or one of
// its dependencies) failed to serve it.
//
//   - Unknown:           non-gRPC errors (plain error values) and status-less
//     failures are mapped here by status.FromError, so an error returned by a
//     handler that never built a gRPC status still trips the breaker
//   - DeadlineExceeded:  the operation overran its deadline — a latency /
//     overload signal of the service itself (the module docs and tests give no
//     justification for treating it as a caller fault, so it counts as failure)
//   - Internal:          unexpected server-side error
//   - Unavailable:       service or dependency down / overloaded
//   - DataLoss:          unrecoverable data loss server-side
//
// The remaining codes are caller faults and count as successes: OK, Canceled,
// InvalidArgument, NotFound, AlreadyExists, PermissionDenied, ResourceExhausted,
// FailedPrecondition, Aborted, OutOfRange, Unimplemented, Unauthenticated —
// clients sending bad requests must not open the circuit.
var defaultFailureCodes = map[codes.Code]bool{
	codes.Unknown:          true,
	codes.DeadlineExceeded: true,
	codes.Internal:         true,
	codes.Unavailable:      true,
	codes.DataLoss:         true,
}

// isFailure reports whether err should trip the circuit breaker. A nil error is
// always a success. With an explicit [WithFailureCodes] table, only the listed
// codes count as failures; otherwise the [defaultFailureCodes] table applies.
func (o *options) isFailure(err error) bool {
	if err == nil {
		return false
	}
	st, _ := status.FromError(err)
	if o.failureCodes != nil {
		return o.failureCodes[st.Code()]
	}
	return defaultFailureCodes[st.Code()]
}

// WithFailureCodes sets the gRPC status codes that are treated as failures.
// By default the server-side fault codes are failures (Unknown, DeadlineExceeded,
// Internal, Unavailable, DataLoss — see [defaultFailureCodes]); pass an explicit
// table here to override that choice entirely.
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
		if cfg.isFailure(err) {
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
		if cfg.isFailure(err) {
			cb.MarkFailure()
		} else {
			cb.MarkSuccess()
		}
		return err
	}
}
