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
	"time"

	"github.com/tx7do/go-wind/log"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Option configures the logging interceptor.
type Option func(*options)

type options struct {
	logger      log.Logger
	skipMethods map[string]bool
}

// WithLogger sets a custom [log.Logger] for RPC logging.
// Defaults to [log.GetLogger].
func WithLogger(l log.Logger) Option {
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
		logger: log.GetLogger(),
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

		level := log.LevelInfo
		if err != nil {
			st, _ := status.FromError(err)
			if st.Code() >= codes.Internal {
				level = log.LevelError
			} else if st.Code() >= codes.NotFound {
				level = log.LevelWarn
			}
		}

		// 若该日志级别被过滤，跳过 args 装箱（log.Logger 接口提供 Enabled，
		// 专为此设计），避免无谓的 slice + interface{} 装箱开销。
		if !cfg.logger.Enabled(level) {
			return resp, err
		}

		args := []any{
			"method", info.FullMethod,
			"latency_ms", latency.Milliseconds(),
		}
		if err != nil {
			st, _ := status.FromError(err)
			args = append(args,
				"code", st.Code().String(),
				"error", err.Error(),
			)
		} else {
			args = append(args, "code", codes.OK.String())
		}

		logAt(cfg.logger, level, ctx, "grpc unary rpc", args...)
		return resp, err
	}
}

// StreamInterceptor returns a [grpc.StreamServerInterceptor] that logs each
// streaming RPC after it completes.
func StreamInterceptor(opts ...Option) grpc.StreamServerInterceptor {
	cfg := &options{
		logger: log.GetLogger(),
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

		level := log.LevelInfo
		if err != nil {
			st, _ := status.FromError(err)
			if st.Code() >= codes.Internal {
				level = log.LevelError
			} else if st.Code() >= codes.NotFound {
				level = log.LevelWarn
			}
		}

		// 若该日志级别被过滤，跳过 args 装箱，避免无谓开销。
		if !cfg.logger.Enabled(level) {
			return err
		}

		args := []any{
			"method", info.FullMethod,
			"client_stream", info.IsClientStream,
			"server_stream", info.IsServerStream,
			"latency_ms", latency.Milliseconds(),
		}
		if err != nil {
			st, _ := status.FromError(err)
			args = append(args,
				"code", st.Code().String(),
				"error", err.Error(),
			)
		} else {
			args = append(args, "code", codes.OK.String())
		}

		logAt(cfg.logger, level, ss.Context(), "grpc stream rpc", args...)
		return err
	}
}

// logAt dispatches to the appropriate log level method.
func logAt(l log.Logger, level log.Level, ctx context.Context, msg string, args ...any) {
	switch level {
	case log.LevelDebug:
		l.Debug(ctx, msg, args...)
	case log.LevelWarn:
		l.Warn(ctx, msg, args...)
	case log.LevelError:
		l.Error(ctx, msg, args...)
	default:
		l.Info(ctx, msg, args...)
	}
}
