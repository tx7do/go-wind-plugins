// Package recovery provides a gRPC server interceptor that recovers from
// panics in downstream handlers, logs the panic with a stack trace, and
// returns an Internal error. It should typically be the outermost
// interceptor so that panics in any subsequent handler are caught.
//
// This is the gRPC counterpart of transport/http/middleware/recovery.
//
// Usage:
//
//	srv := grpc.NewServer(grpc.ChainUnaryInterceptor(
//	    grpcRecovery.UnaryInterceptor(),
//	))
package recovery

import (
	"context"
	"fmt"
	"log/slog"
	"runtime/debug"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Option configures the recovery interceptor.
type Option func(*options)

type options struct {
	logStack bool
	logger   *slog.Logger
}

// WithStackTrace enables or disables stack trace logging.
// Defaults to true.
func WithStackTrace(enabled bool) Option {
	return func(o *options) { o.logStack = enabled }
}

// WithLogger sets a custom [slog.Logger] for panic logging.
// Defaults to [slog.Default].
func WithLogger(l *slog.Logger) Option {
	return func(o *options) { o.logger = l }
}

// UnaryInterceptor returns a [grpc.UnaryServerInterceptor] that recovers from
// panics in downstream handlers, logs them, and returns an Internal error.
func UnaryInterceptor(opts ...Option) grpc.UnaryServerInterceptor {
	cfg := &options{
		logStack: true,
		logger:   slog.Default(),
	}
	for _, opt := range opts {
		opt(cfg)
	}

	return func(
		ctx context.Context,
		req any,
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (resp any, err error) {
		defer func() {
			if rvr := recover(); rvr != nil {
				args := []any{
					slog.String("error", fmt.Sprint(rvr)),
					slog.String("method", info.FullMethod),
				}
				if cfg.logStack {
					args = append(args, slog.String("stack", string(debug.Stack())))
				}
				cfg.logger.ErrorContext(ctx, "panic recovered", args...)
				err = status.Errorf(codes.Internal, "internal server error")
			}
		}()
		return handler(ctx, req)
	}
}

// StreamInterceptor returns a [grpc.StreamServerInterceptor] that recovers
// from panics in streaming RPC handlers.
func StreamInterceptor(opts ...Option) grpc.StreamServerInterceptor {
	cfg := &options{
		logStack: true,
		logger:   slog.Default(),
	}
	for _, opt := range opts {
		opt(cfg)
	}

	return func(
		srv any,
		ss grpc.ServerStream,
		info *grpc.StreamServerInfo,
		handler grpc.StreamHandler,
	) (err error) {
		defer func() {
			if rvr := recover(); rvr != nil {
				args := []any{
					slog.String("error", fmt.Sprint(rvr)),
					slog.String("method", info.FullMethod),
				}
				if cfg.logStack {
					args = append(args, slog.String("stack", string(debug.Stack())))
				}
				cfg.logger.ErrorContext(ss.Context(), "panic recovered", args...)
				err = status.Errorf(codes.Internal, "internal server error")
			}
		}()
		return handler(srv, ss)
	}
}
