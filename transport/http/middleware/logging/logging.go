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
	"log/slog"
	"net/http"
	"time"

	httpPlugin "github.com/tx7do/go-wind-plugins/transport/http"
)

// Option configures the logging middleware.
type Option func(*options)

type options struct {
	logger    *slog.Logger
	skipPaths map[string]struct{}
}

// WithLogger sets a custom [slog.Logger] for request logging.
// Defaults to [slog.Default].
func WithLogger(l *slog.Logger) Option {
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
		logger: slog.Default(),
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
			rw := &responseWriter{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(rw, r)
			latency := time.Since(start)

			level := slog.LevelInfo
			if rw.status >= 500 {
				level = slog.LevelError
			} else if rw.status >= 400 {
				level = slog.LevelWarn
			}

			cfg.logger.Log(r.Context(), level, "http request",
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.String("query", r.URL.RawQuery),
				slog.Int("status", rw.status),
				slog.Int("size", rw.size),
				slog.Int64("latency_ms", latency.Milliseconds()),
				slog.String("remote", r.RemoteAddr),
			)
		})
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
