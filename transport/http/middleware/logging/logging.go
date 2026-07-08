// Package logging provides an HTTP middleware that logs each request's method,
// path, status code, response size, latency, and remote address.
//
// Usage:
//
//	srv.Use(logging.Middleware())
//	// or with options:
//	srv.Use(logging.Middleware(
//	    logging.WithLogger(myLogger),
//	    logging.WithSkipPaths("/healthz", "/readyz"),
//	))
package logging

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/tx7do/go-wind/log"

	httpPlugin "github.com/tx7do/go-wind-plugins/transport/http"
)

// Option configures the logging middleware.
type Option func(*options)

type options struct {
	logger    log.Logger
	skipPaths map[string]struct{}
}

// WithLogger sets a custom [log.Logger] for request logging.
// Defaults to [log.GetLogger].
func WithLogger(l log.Logger) Option {
	return func(o *options) { o.logger = l }
}

// WithSkipPaths sets URL paths to skip logging, useful for high-frequency
// health-check endpoints such as "/healthz" or "/readyz".
func WithSkipPaths(paths ...string) Option {
	return func(o *options) {
		if o.skipPaths == nil {
			o.skipPaths = make(map[string]struct{})
		}
		for _, p := range paths {
			o.skipPaths[p] = struct{}{}
		}
	}
}

// Middleware returns a [httpPlugin.Middleware] that logs each request after
// it completes, including method, path, status code, response size, latency,
// and remote address.
func Middleware(opts ...Option) httpPlugin.Middleware {
	cfg := &options{
		logger: log.GetLogger(),
	}
	for _, opt := range opts {
		opt(cfg)
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Skip logging for excluded paths.
			if _, skip := cfg.skipPaths[r.URL.Path]; skip {
				next.ServeHTTP(w, r)
				return
			}

			start := time.Now()
			// 池化 responseWriter：同步包装，handler 返回后即归还，安全。
			rw := acquireResponseWriter(w)
			defer releaseResponseWriter(rw)
			next.ServeHTTP(rw, r)
			latency := time.Since(start)

			level := log.LevelInfo
			if rw.status >= 500 {
				level = log.LevelError
			} else if rw.status >= 400 {
				level = log.LevelWarn
			}

			// 优化：若该日志级别被过滤，跳过 args 装箱（log.Logger 接口
			// 提供 Enabled 方法，专为此场景设计）。避免 7 个 kv 的 ...any 装箱。
			if !cfg.logger.Enabled(level) {
				return
			}

			logAt(cfg.logger, level, r.Context(), "http request",
				"method", r.Method,
				"path", r.URL.Path,
				"query", r.URL.RawQuery,
				"status", rw.status,
				"size", rw.size,
				"latency_ms", latency.Milliseconds(),
				"remote", r.RemoteAddr,
			)
		})
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

// responseWriter wraps [http.ResponseWriter] to capture the status code and
// the number of bytes written to the body.
type responseWriter struct {
	http.ResponseWriter
	status int
	size   int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.status = code
	rw.ResponseWriter.WriteHeader(code)
}

func (rw *responseWriter) Write(b []byte) (int, error) {
	size, err := rw.ResponseWriter.Write(b)
	rw.size += size
	return size, err
}

// --- sync.Pool 优化（responseWriter 复用） ---
//
// responseWriter 是同步包装：在 next.ServeHTTP 返回后立即归还，handler 不会
// 异步持有它（标准 HTTP handler 语义），故可安全池化。

var responseWriterPool = sync.Pool{
	New: func() any { return &responseWriter{} },
}

func acquireResponseWriter(w http.ResponseWriter) *responseWriter {
	rw := responseWriterPool.Get().(*responseWriter)
	rw.ResponseWriter = w
	rw.status = http.StatusOK
	rw.size = 0
	return rw
}

func releaseResponseWriter(rw *responseWriter) {
	rw.ResponseWriter = nil
	responseWriterPool.Put(rw)
}
