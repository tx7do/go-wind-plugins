// Package logging provides a gRPC server interceptor that logs each RPC's
// method, status code, and latency.
//
// This is the gRPC counterpart of transport/http/middleware/logging.
//
// Usage:
//
//	srv := grpc.NewServer(grpc.ChainUnaryInterceptor(
//	    grpcLogging.UnaryInterceptor(),
//	))
package logging

import (
	"context"
	"log/slog"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Option configures the logging interceptor.
type Option func(*options)

type options struct {
	logger      *slog.Logger
	skipMethods map[string]bool
}

// WithLogger sets a custom [slog.Logger] for RPC logging.
// Defaults to [slog.Default].
func WithLogger(l *slog.Logger) Option {
	return func(o *options) { o.logger = l }
}

// WithSkipMethods adds full gRPC method names that should bypass logging.
// Useful for high-frequency health-check RPCs.
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

// UnaryInterceptor returns a [grpc.UnaryServerInterceptor] that logs each
// unary RPC after it completes, including method, status code, and latency.
func UnaryInterceptor(opts ...Option) grpc.UnaryServerInterceptor {
	cfg := &options{
		logger: slog.Default(),
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
		if cfg.skipMethods[info.FullMethod] {
			return handler(ctx, req)
		}

		start := time.Now()
		resp, err := handler(ctx, req)
		latency := time.Since(start)

		level := slog.LevelInfo
		if err != nil {
			st, _ := status.FromError(err)
			if st.Code() >= codes.Internal {
				level = slog.LevelError
			} else if st.Code() >= codes.NotFound {
				level = slog.LevelWarn
			}
		}

		args := []any{
			slog.String("method", info.FullMethod),
			slog.Int64("latency_ms", latency.Milliseconds()),
		}
		if err != nil {
			st, _ := status.FromError(err)
			args = append(args,
				slog.String("code", st.Code().String()),
				slog.String("error", err.Error()),
			)
		} else {
			args = append(args, slog.String("code", codes.OK.String()))
		}

		cfg.logger.Log(ctx, level, "grpc unary rpc", args...)
		return resp, err
	}
}

// StreamInterceptor returns a [grpc.StreamServerInterceptor] that logs each
// streaming RPC after it completes.
func StreamInterceptor(opts ...Option) grpc.StreamServerInterceptor {
	cfg := &options{
		logger: slog.Default(),
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

		start := time.Now()
		err := handler(srv, ss)
		latency := time.Since(start)

		level := slog.LevelInfo
		if err != nil {
			st, _ := status.FromError(err)
			if st.Code() >= codes.Internal {
				level = slog.LevelError
			} else if st.Code() >= codes.NotFound {
				level = slog.LevelWarn
			}
		}

		args := []any{
			slog.String("method", info.FullMethod),
			slog.Bool("client_stream", info.IsClientStream),
			slog.Bool("server_stream", info.IsServerStream),
			slog.Int64("latency_ms", latency.Milliseconds()),
		}
		if err != nil {
			st, _ := status.FromError(err)
			args = append(args,
				slog.String("code", st.Code().String()),
				slog.String("error", err.Error()),
			)
		} else {
			args = append(args, slog.String("code", codes.OK.String()))
		}

		cfg.logger.Log(ss.Context(), level, "grpc stream rpc", args...)
		return err
	}
}
