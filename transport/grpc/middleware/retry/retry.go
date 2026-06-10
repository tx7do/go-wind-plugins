// Package retry provides gRPC server interceptors that retry idempotent RPCs
// when the handler returns a transient failure.
//
// This is the gRPC counterpart of transport/http/middleware/retry.
//
// The interceptor wraps the handler in a [retry.Retrier] loop. An RPC is
// considered retryable when:
//   - The gRPC method is in the idempotent set (default: methods whose name
//     starts with "Get", "List", or "Search"). Customise with
//     [WithIdempotentPrefixes].
//   - The handler returns an error with a retryable gRPC code (default:
//     Unavailable). Customise with [WithRetryCodes].
//
// Usage:
//
//	r := retry.New(
//	    retry.WithMaxAttempts(3),
//	    retry.WithBackoff(retry.ExponentialBackoff{
//	        Initial: 200 * time.Millisecond,
//	        Factor:  2,
//	        Max:     5 * time.Second,
//	    }),
//	)
//	srv := grpc.NewServer(grpc.ChainUnaryInterceptor(
//	    grpcRetry.UnaryInterceptor(r),
//	))
package retry

import (
	"context"
	"strings"

	coreRetry "github.com/tx7do/go-wind-plugins/retry"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

var defaultIdempotentPrefixes = []string{"Get", "List", "Search"}

var defaultRetryCodes = map[codes.Code]bool{
	codes.Unavailable: true,
}

// Option configures the retry interceptor.
type Option func(*options)

type options struct {
	idempotentPrefixes []string
	retryCodes         map[codes.Code]bool
	skipMethods        map[string]bool
}

// WithIdempotentPrefixes sets the method-name prefixes that identify
// idempotent RPCs. Default: "Get", "List", "Search".
func WithIdempotentPrefixes(prefixes ...string) Option {
	return func(o *options) { o.idempotentPrefixes = prefixes }
}

// WithRetryCodes sets the gRPC status codes that should trigger a retry.
// Default: codes.Unavailable.
func WithRetryCodes(cs ...codes.Code) Option {
	return func(o *options) {
		o.retryCodes = make(map[codes.Code]bool, len(cs))
		for _, c := range cs {
			o.retryCodes[c] = true
		}
	}
}

// WithSkipMethods adds full gRPC method names that should bypass retrying.
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

// UnaryInterceptor returns a [grpc.UnaryServerInterceptor] that retries
// idempotent RPCs using the provided [retry.Retrier].
func UnaryInterceptor(r *coreRetry.Retrier, opts ...Option) grpc.UnaryServerInterceptor {
	cfg := &options{
		idempotentPrefixes: defaultIdempotentPrefixes,
		retryCodes:         defaultRetryCodes,
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
		if cfg.skipMethods[info.FullMethod] || !isIdempotent(info.FullMethod, cfg.idempotentPrefixes) {
			return handler(ctx, req)
		}

		var lastResp any
		var lastErr error

		_ = r.Do(ctx, func(attemptCtx context.Context) error {
			resp, err := handler(attemptCtx, req)
			lastResp = resp
			lastErr = err

			if err == nil {
				return nil
			}

			st, _ := status.FromError(err)
			if cfg.retryCodes[st.Code()] {
				return err // retryable
			}
			return nil // not retryable — stop the loop, lastErr is preserved
		})

		return lastResp, lastErr
	}
}

// StreamInterceptor returns a [grpc.StreamServerInterceptor] that retries
// idempotent streaming RPCs using the provided [retry.Retrier].
func StreamInterceptor(r *coreRetry.Retrier, opts ...Option) grpc.StreamServerInterceptor {
	cfg := &options{
		idempotentPrefixes: defaultIdempotentPrefixes,
		retryCodes:         defaultRetryCodes,
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
		if cfg.skipMethods[info.FullMethod] || !isIdempotent(info.FullMethod, cfg.idempotentPrefixes) {
			return handler(srv, ss)
		}

		var lastErr error

		_ = r.Do(ss.Context(), func(attemptCtx context.Context) error {
			err := handler(srv, &wrappedServerStream{ServerStream: ss, ctx: attemptCtx})
			lastErr = err

			if err == nil {
				return nil
			}

			st, _ := status.FromError(err)
			if cfg.retryCodes[st.Code()] {
				return err
			}
			return nil // not retryable — stop the loop, lastErr is preserved
		})

		return lastErr
	}
}

// isIdempotent checks whether the gRPC full method matches any of the
// idempotent prefixes.
//
// FullMethod format: "/pkg.UserService/GetUser" → extracts "GetUser".
func isIdempotent(fullMethod string, prefixes []string) bool {
	// Extract the method name from FullMethod.
	methodName := fullMethod
	if idx := strings.LastIndex(fullMethod, "/"); idx >= 0 {
		methodName = fullMethod[idx+1:]
	}

	for _, prefix := range prefixes {
		if strings.HasPrefix(methodName, prefix) {
			return true
		}
	}
	return false
}

// wrappedServerStream wraps a [grpc.ServerStream] to override its context,
// allowing the retry's per-attempt context to flow through to the handler.
type wrappedServerStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (w *wrappedServerStream) Context() context.Context {
	return w.ctx
}
