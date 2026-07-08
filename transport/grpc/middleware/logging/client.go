package logging

import (
	"context"
	"time"

	"github.com/tx7do/go-wind/log"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// UnaryClientInterceptor returns a [grpc.UnaryClientInterceptor] that logs each
// outgoing unary RPC after it completes, including method, status code, and
// latency.
func UnaryClientInterceptor(opts ...Option) grpc.UnaryClientInterceptor {
	cfg := &options{
		logger: log.GetLogger(),
	}
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

		start := time.Now()
		err := invoker(ctx, method, req, reply, cc, callOpts...)
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
			"method", method,
			"target", cc.Target(),
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

		logAt(cfg.logger, level, ctx, "grpc unary client rpc", args...)
		return err
	}
}

// StreamClientInterceptor returns a [grpc.StreamClientInterceptor] that logs
// each outgoing streaming RPC after the stream is created.
//
// Note: only the stream-creation result is logged. Individual SendMsg/RecvMsg
// errors are not captured.
func StreamClientInterceptor(opts ...Option) grpc.StreamClientInterceptor {
	cfg := &options{
		logger: log.GetLogger(),
	}
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

		start := time.Now()
		stream, err := streamer(ctx, desc, cc, method, callOpts...)
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
			return stream, err
		}

		args := []any{
			"method", method,
			"target", cc.Target(),
			"client_stream", desc.ClientStreams,
			"server_stream", desc.ServerStreams,
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

		logAt(cfg.logger, level, ctx, "grpc stream client rpc", args...)
		return stream, err
	}
}
