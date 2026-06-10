// Package retry provides an HTTP middleware that retries idempotent requests
// when the downstream handler returns a transient failure.
//
// The middleware wraps the handler in a [retry.Retrier] loop. A request is
// considered retryable when:
//   - The HTTP method is idempotent (GET, HEAD, OPTIONS, PUT, DELETE) or the
//     [WithRetryAllMethods] option is set.
//   - The response status code matches the configured retry-on set (default:
//     502, 503, 504). Customised with [WithRetryStatus].
//
// Non-idempotent methods (POST, PATCH) are NOT retried by default.
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
//	srv.Use(retry.Middleware(r))
package retry

import (
	"context"
	"net/http"

	"github.com/tx7do/go-wind-plugins/retry"

	httpPlugin "github.com/tx7do/go-wind-plugins/transport/http"
)

var idempotentMethods = map[string]bool{
	http.MethodGet:     true,
	http.MethodHead:    true,
	http.MethodOptions: true,
	http.MethodPut:     true,
	http.MethodDelete:  true,
}

// Option configures the retry middleware.
type Option func(*options)

type options struct {
	retryAllMethods bool
	retryStatus     map[int]bool
	skipFunc        func(r *http.Request) bool
}

// WithRetryAllMethods makes the middleware retry ALL HTTP methods, not just
// idempotent ones. Use with caution — retrying non-idempotent methods (POST,
// PATCH) can cause duplicate side effects.
func WithRetryAllMethods() Option {
	return func(o *options) { o.retryAllMethods = true }
}

// WithRetryStatus sets the HTTP status codes that should trigger a retry.
// Default: 502 Bad Gateway, 503 Service Unavailable, 504 Gateway Timeout.
func WithRetryStatus(codes ...int) Option {
	return func(o *options) {
		o.retryStatus = make(map[int]bool, len(codes))
		for _, c := range codes {
			o.retryStatus[c] = true
		}
	}
}

// WithSkipFunc sets a function that, if it returns true, causes the request
// to bypass retrying entirely.
func WithSkipFunc(f func(r *http.Request) bool) Option {
	return func(o *options) { o.skipFunc = f }
}

// Middleware returns an [httpPlugin.Middleware] that retries idempotent
// requests using the provided [retry.Retrier].
func Middleware(r *retry.Retrier, opts ...Option) httpPlugin.Middleware {
	cfg := &options{
		retryStatus: map[int]bool{
			http.StatusBadGateway:         true,
			http.StatusServiceUnavailable: true,
			http.StatusGatewayTimeout:     true,
		},
	}
	for _, opt := range opts {
		opt(cfg)
	}

	shouldRetry := func(statusCode int) bool {
		return cfg.retryStatus[statusCode]
	}

	isRetryable := func(r *http.Request) bool {
		if cfg.retryAllMethods {
			return true
		}
		return idempotentMethods[r.Method]
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			if cfg.skipFunc != nil && cfg.skipFunc(req) {
				next.ServeHTTP(w, req)
				return
			}

			if !isRetryable(req) {
				next.ServeHTTP(w, req)
				return
			}

			var lastRW *retryRecorder
			flushed := false

			_ = r.Do(req.Context(), func(ctx context.Context) error {
				// Create a fresh request with the retrier's context.
				attemptReq := req.WithContext(ctx)

				// Buffer the response so we can discard on retry.
				rw := newRetryRecorder()
				next.ServeHTTP(rw, attemptReq)

				lastRW = rw

				if shouldRetry(rw.status) {
					return errRetryable{code: rw.status}
				}

				// Success or non-retryable — flush to the real writer.
				rw.flushTo(w)
				flushed = true
				return nil
			})

			// If we never flushed (all attempts returned retryable status),
			// flush the last buffered response.
			if !flushed && lastRW != nil {
				lastRW.flushTo(w)
			}
		})
	}
}

// --- internal helpers ---

// errRetryable signals the retrier that the handler returned a retryable status.
type errRetryable struct {
	code int
}

func (e errRetryable) Error() string {
	return http.StatusText(e.code)
}

// retryRecorder buffers a single attempt's response so it can be discarded
// on retry. It implements http.ResponseWriter.
type retryRecorder struct {
	header http.Header
	status int
	buf    []byte
}

func newRetryRecorder() *retryRecorder {
	return &retryRecorder{
		header: make(http.Header),
		status: http.StatusOK,
	}
}

func (r *retryRecorder) Header() http.Header {
	return r.header
}

func (r *retryRecorder) WriteHeader(code int) {
	r.status = code
}

func (r *retryRecorder) Write(b []byte) (int, error) {
	r.buf = append(r.buf, b...)
	return len(b), nil
}

// flushTo writes the buffered response to the real ResponseWriter.
func (r *retryRecorder) flushTo(w http.ResponseWriter) {
	dst := w.Header()
	for k, vv := range r.header {
		for _, v := range vv {
			dst.Add(k, v)
		}
	}
	w.WriteHeader(r.status)
	_, _ = w.Write(r.buf)
}
