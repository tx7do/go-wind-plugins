package circuitbreaker

import (
	"context"

	"github.com/tx7do/go-wind-plugins/circuitbreaker"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// UnaryClientInterceptor returns a [grpc.UnaryClientInterceptor] that enforces
// a circuit-breaker policy on outgoing unary RPCs.
//
// If the circuit is open, the RPC is rejected with codes.Unavailable without
// being sent to the server. Otherwise the RPC executes and the response error
// determines whether [MarkSuccess] or [MarkFailure] is called.
func UnaryClientInterceptor(cb circuitbreaker.CircuitBreaker, opts ...Option) grpc.UnaryClientInterceptor {
	cfg := &options{}
	for _, opt := range opts {
		opt(cfg)
	}

	return func(
		ctx context.Context,
		method string,
		req, reply any,
		cc *grpc.ClientConn,
		invoker grpc.UnaryInvoker,
		callOpts ...grpc.CallOption,
	) error {
		if cfg.skipMethods[method] {
			return invoker(ctx, method, req, reply, cc, callOpts...)
		}

		if err := cb.Allow(); err != nil {
			return status.Error(codes.Unavailable, "circuit breaker is open")
		}

		err := invoker(ctx, method, req, reply, cc, callOpts...)
		if cfg.isFailure(err) {
			cb.MarkFailure()
		} else {
			cb.MarkSuccess()
		}
		return err
	}
}

// StreamClientInterceptor returns a [grpc.StreamClientInterceptor] that enforces
// a circuit-breaker policy on outgoing streaming RPCs.
//
// Note: the breaker result is based on the stream-creation error only.
// Errors that occur during SendMsg/RecvMsg are not captured by this interceptor.
func StreamClientInterceptor(cb circuitbreaker.CircuitBreaker, opts ...Option) grpc.StreamClientInterceptor {
	cfg := &options{}
	for _, opt := range opts {
		opt(cfg)
	}

	return func(
		ctx context.Context,
		desc *grpc.StreamDesc,
		cc *grpc.ClientConn,
		method string,
		streamer grpc.Streamer,
		callOpts ...grpc.CallOption,
	) (grpc.ClientStream, error) {
		if cfg.skipMethods[method] {
			return streamer(ctx, desc, cc, method, callOpts...)
		}

		if err := cb.Allow(); err != nil {
			return nil, status.Error(codes.Unavailable, "circuit breaker is open")
		}

		stream, err := streamer(ctx, desc, cc, method, callOpts...)
		if cfg.isFailure(err) {
			cb.MarkFailure()
		} else {
			cb.MarkSuccess()
		}
		return stream, err
	}
}
