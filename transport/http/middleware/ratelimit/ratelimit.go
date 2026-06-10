// Package ratelimit provides an HTTP middleware that enforces a rate-limiting
// policy using any implementation of [ratelimit.Limiter] (e.g. token-bucket,
// BBR, Sentinel).
//
// Two modes are supported:
//   - Reject (default): when the limit is exceeded the request is rejected
//     immediately with 429 Too Many Requests.
//   - Wait: the middleware blocks until a token is available or the request
//     context is cancelled (useful for smoothing traffic spikes).
//
// Usage:
//
//	limiter, _ := tokenbucket.New(100, 200) // 100 QPS, burst 200
//	srv.Use(ratelimit.Middleware(limiter))
//	// or with wait mode:
//	srv.Use(ratelimit.Middleware(limiter, ratelimit.WithWait()))
package ratelimit

import (
	"net/http"
	"sync"

	"github.com/tx7do/go-wind-plugins/ratelimit"

	httpPlugin "github.com/tx7do/go-wind-plugins/transport/http"
)

// Option configures the rate-limit middleware.
type Option func(*options)

type options struct {
	waitMode     bool
	errorHandler func(w http.ResponseWriter, r *http.Request, err error)
	skipFunc     func(r *http.Request) bool
}

// WithWait enables wait mode: instead of rejecting immediately, the middleware
// blocks until the limiter allows the request or the context is cancelled.
func WithWait() Option {
	return func(o *options) { o.waitMode = true }
}

// WithErrorHandler sets a custom handler for rejected requests.
// The handler is responsible for writing the HTTP response.
func WithErrorHandler(h func(w http.ResponseWriter, r *http.Request, err error)) Option {
	return func(o *options) { o.errorHandler = h }
}

// WithSkipFunc sets a function that, if it returns true for a given request,
// causes the middleware to bypass rate-limiting and pass the request through.
func WithSkipFunc(f func(r *http.Request) bool) Option {
	return func(o *options) { o.skipFunc = f }
}

// Middleware returns an [httpPlugin.Middleware] that enforces rate-limiting
// using the provided [ratelimit.Limiter].
//
// The limiter must be safe for concurrent use (all implementations in this
// framework are).
func Middleware(limiter ratelimit.Limiter, opts ...Option) httpPlugin.Middleware {
	cfg := &options{
		errorHandler: defaultErrorHandler,
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

			if cfg.waitMode {
				if err := limiter.Wait(r.Context()); err != nil {
					cfg.errorHandler(w, r, err)
					return
				}
			} else {
				ok, err := limiter.Allow()
				if !ok || err != nil {
					cfg.errorHandler(w, r, err)
					return
				}
			}

			next.ServeHTTP(w, r)
		})
	}
}

// defaultErrorHandler writes a 429 Too Many Requests response.
func defaultErrorHandler(w http.ResponseWriter, _ *http.Request, _ error) {
	http.Error(w, "Too Many Requests", http.StatusTooManyRequests)
}

// --- Keyed limiter (per-client) ---

// KeyFunc extracts a rate-limiting key from the request (e.g. client IP,
// user ID). When used with [MiddlewareKeyed], each key gets its own limiter
// created by the factory.
type KeyFunc func(r *http.Request) string

// LimiterFactory creates a new [ratelimit.Limiter] for a given key.
type LimiterFactory func(key string) ratelimit.Limiter

// MiddlewareKeyed returns an [httpPlugin.Middleware] that applies
// per-key rate-limiting. Each unique key produced by keyFn gets its own
// limiter created by factory.
//
// This is useful for per-client rate limiting:
//
//	srv.Use(ratelimit.MiddlewareKeyed(
//	    func(r *http.Request) string { return clientIP(r) },
//	    func(_ string) ratelimit.Limiter {
//	        l, _ := tokenbucket.New(10, 20)
//	        return l
//	    },
//	))
func MiddlewareKeyed(keyFn KeyFunc, factory LimiterFactory, opts ...Option) httpPlugin.Middleware {
	cfg := &options{
		errorHandler: defaultErrorHandler,
	}
	for _, opt := range opts {
		opt(cfg)
	}

	var mu sync.Mutex
	limiters := make(map[string]ratelimit.Limiter)

	getLimiter := func(key string) ratelimit.Limiter {
		mu.Lock()
		defer mu.Unlock()
		if l, ok := limiters[key]; ok {
			return l
		}
		l := factory(key)
		limiters[key] = l
		return l
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if cfg.skipFunc != nil && cfg.skipFunc(r) {
				next.ServeHTTP(w, r)
				return
			}

			key := keyFn(r)
			limiter := getLimiter(key)

			if cfg.waitMode {
				if err := limiter.Wait(r.Context()); err != nil {
					cfg.errorHandler(w, r, err)
					return
				}
			} else {
				ok, err := limiter.Allow()
				if !ok || err != nil {
					cfg.errorHandler(w, r, err)
					return
				}
			}

			next.ServeHTTP(w, r)
		})
	}
}
