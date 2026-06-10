// Package circuitbreaker provides an HTTP middleware that wraps downstream
// handlers in a circuit-breaker policy using any implementation of
// [circuitbreaker.CircuitBreaker] (e.g. SRE, Hystrix, Vegas, Sentinel).
//
// Before calling the next handler the middleware calls [CircuitBreaker.Allow].
// If the circuit is open, the request is rejected immediately with 503
// Service Unavailable. Otherwise the handler executes and the response status
// code determines whether [MarkSuccess] or [MarkFailure] is called.
//
// By default any 5xx response is treated as a failure. Use [WithFailureStatus]
// to customise the set of failing status codes.
//
// Usage:
//
//	cb, _ := sres.New(sres.WithFailureRatio(0.5))
//	srv.Use(circuitbreaker.Middleware(cb))
package circuitbreaker

import (
	"net/http"
	"sync"

	"github.com/tx7do/go-wind-plugins/circuitbreaker"

	httpPlugin "github.com/tx7do/go-wind-plugins/transport/http"
)

// Option configures the circuit-breaker middleware.
type Option func(*options)

type options struct {
	failureStatus map[int]bool
	errorHandler  func(w http.ResponseWriter, r *http.Request, err error)
	skipFunc      func(r *http.Request) bool
}

// WithFailureStatus sets the HTTP status codes that are treated as failures.
// By default any status code >= 500 is a failure.
func WithFailureStatus(codes ...int) Option {
	return func(o *options) {
		o.failureStatus = make(map[int]bool, len(codes))
		for _, c := range codes {
			o.failureStatus[c] = true
		}
	}
}

// WithErrorHandler sets a custom handler for rejected requests (circuit open).
func WithErrorHandler(h func(w http.ResponseWriter, r *http.Request, err error)) Option {
	return func(o *options) { o.errorHandler = h }
}

// WithSkipFunc sets a function that, if it returns true for a given request,
// causes the middleware to bypass the circuit breaker.
func WithSkipFunc(f func(r *http.Request) bool) Option {
	return func(o *options) { o.skipFunc = f }
}

// Middleware returns an [httpPlugin.Middleware] that enforces a circuit-breaker
// policy using the provided [circuitbreaker.CircuitBreaker].
//
// The circuit breaker must be safe for concurrent use (all implementations in
// this framework are).
func Middleware(cb circuitbreaker.CircuitBreaker, opts ...Option) httpPlugin.Middleware {
	cfg := &options{
		failureStatus: nil, // nil means default: >= 500
		errorHandler:  defaultErrorHandler,
	}
	for _, opt := range opts {
		opt(cfg)
	}

	isFailure := func(statusCode int) bool {
		if cfg.failureStatus != nil {
			return cfg.failureStatus[statusCode]
		}
		return statusCode >= 500
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if cfg.skipFunc != nil && cfg.skipFunc(r) {
				next.ServeHTTP(w, r)
				return
			}

			if err := cb.Allow(); err != nil {
				cfg.errorHandler(w, r, err)
				return
			}

			rw := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(rw, r)

			if isFailure(rw.status) {
				cb.MarkFailure()
			} else {
				cb.MarkSuccess()
			}
		})
	}
}

// defaultErrorHandler writes a 503 Service Unavailable response.
func defaultErrorHandler(w http.ResponseWriter, _ *http.Request, _ error) {
	http.Error(w, "Service Unavailable", http.StatusServiceUnavailable)
}

// statusRecorder wraps [http.ResponseWriter] to capture the status code.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

// WriteHeader captures the status code before delegating.
func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

// --- Keyed circuit breaker ---

// KeyFunc extracts a circuit-breaker key from the request (e.g. service name,
// route pattern). When used with [MiddlewareKeyed], each key gets its own
// circuit breaker created by the factory.
type KeyFunc func(r *http.Request) string

// BreakerFactory creates a new [circuitbreaker.CircuitBreaker] for a given key.
type BreakerFactory func(key string) circuitbreaker.CircuitBreaker

// MiddlewareKeyed returns an [httpPlugin.Middleware] that applies per-key
// circuit-breaking. Each unique key gets its own breaker.
func MiddlewareKeyed(keyFn KeyFunc, factory BreakerFactory, opts ...Option) httpPlugin.Middleware {
	cfg := &options{
		errorHandler: defaultErrorHandler,
	}
	for _, opt := range opts {
		opt(cfg)
	}

	isFailure := func(statusCode int) bool {
		if cfg.failureStatus != nil {
			return cfg.failureStatus[statusCode]
		}
		return statusCode >= 500
	}

	var mu sync.Mutex
	breakers := make(map[string]circuitbreaker.CircuitBreaker)

	getBreaker := func(key string) circuitbreaker.CircuitBreaker {
		mu.Lock()
		defer mu.Unlock()
		if cb, ok := breakers[key]; ok {
			return cb
		}
		cb := factory(key)
		breakers[key] = cb
		return cb
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if cfg.skipFunc != nil && cfg.skipFunc(r) {
				next.ServeHTTP(w, r)
				return
			}

			cb := getBreaker(keyFn(r))

			if err := cb.Allow(); err != nil {
				cfg.errorHandler(w, r, err)
				return
			}

			rw := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(rw, r)

			if isFailure(rw.status) {
				cb.MarkFailure()
			} else {
				cb.MarkSuccess()
			}
		})
	}
}
