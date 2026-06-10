// Package timeout provides an HTTP middleware that enforces a per-request
// timeout on downstream handler execution. When the timeout elapses, the
// context is cancelled and a 503 Service Unavailable response is returned.
//
// Usage:
//
//	srv.Use(timeout.Middleware(30 * time.Second))
//	// or with a custom timeout response:
//	srv.Use(timeout.Middleware(30 * time.Second,
//	    timeout.WithStatus(http.StatusGatewayTimeout),
//	))
package timeout

import (
	"context"
	"net/http"
	"time"

	httpPlugin "github.com/tx7do/go-wind-plugins/transport/http"
)

// Option configures the timeout middleware.
type Option func(*options)

type options struct {
	status      int
	message     string
	skipFunc    func(r *http.Request) bool
	timeoutFunc func(r *http.Request) time.Duration
}

// WithStatus sets the HTTP status code returned when the timeout fires.
// Default: 503 Service Unavailable.
func WithStatus(code int) Option {
	return func(o *options) { o.status = code }
}

// WithMessage sets the response body text returned when the timeout fires.
// Default: "Service Unavailable".
func WithMessage(msg string) Option {
	return func(o *options) { o.message = msg }
}

// WithSkipFunc sets a function that returns true for requests that should
// bypass the timeout (e.g. long-polling or streaming endpoints).
func WithSkipFunc(fn func(r *http.Request) bool) Option {
	return func(o *options) { o.skipFunc = fn }
}

// WithTimeoutFunc sets a function that returns a per-request timeout,
// overriding the static default timeout. This allows different timeouts
// for different routes.
func WithTimeoutFunc(fn func(r *http.Request) time.Duration) Option {
	return func(o *options) { o.timeoutFunc = fn }
}

// Middleware returns a [httpPlugin.Middleware] that enforces a timeout on
// downstream handler execution. If the handler does not complete within the
// timeout, the context is cancelled and a timeout error response is sent.
func Middleware(defaultTimeout time.Duration, opts ...Option) httpPlugin.Middleware {
	cfg := &options{
		status:  http.StatusServiceUnavailable,
		message: "Service Unavailable",
	}
	for _, opt := range opts {
		opt(cfg)
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if cfg.skipFunc != nil && cfg.skipFunc(r) {
				next.ServeHTTP(w, r)
				return
			}

			timeout := defaultTimeout
			if cfg.timeoutFunc != nil {
				if t := cfg.timeoutFunc(r); t > 0 {
					timeout = t
				}
			}

			if timeout <= 0 {
				next.ServeHTTP(w, r)
				return
			}

			ctx, cancel := context.WithTimeout(r.Context(), timeout)
			defer cancel()

			done := make(chan struct{})
			timedOut := false

			go func() {
				defer close(done)
				next.ServeHTTP(w, r.WithContext(ctx))
			}()

			select {
			case <-done:
				// Handler completed normally.
			case <-ctx.Done():
				timedOut = true
				http.Error(w, cfg.message, cfg.status)
			}

			_ = timedOut
		})
	}
}
