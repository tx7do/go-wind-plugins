// Package recovery provides an HTTP middleware that recovers from panics in
// downstream handlers, logs the panic with a stack trace, and returns a 500
// response. It should typically be the outermost middleware so that panics in
// any subsequent middleware or handler are caught.
//
// Usage:
//
//	srv.Use(recovery.Middleware())
//	// or with options:
//	srv.Use(recovery.Middleware(
//	    recovery.WithLogger(myLogger),
//	    recovery.WithStackTrace(false),
//	))
package recovery

import (
	"fmt"
	"net/http"
	"runtime/debug"

	"github.com/tx7do/go-wind/log"

	httpPlugin "github.com/tx7do/go-wind-plugins/transport/http"
)

// Option configures the recovery middleware.
type Option func(*options)

type options struct {
	logStack     bool
	errorHandler func(w http.ResponseWriter, r *http.Request, rvr any)
	logger       log.Logger
}

// WithStackTrace enables or disables stack trace logging.
// Defaults to true.
func WithStackTrace(enabled bool) Option {
	return func(o *options) { o.logStack = enabled }
}

// WithErrorHandler sets a custom handler invoked after a panic is recovered.
// The handler is responsible for writing the HTTP response.
func WithErrorHandler(h func(w http.ResponseWriter, r *http.Request, rvr any)) Option {
	return func(o *options) { o.errorHandler = h }
}

// WithLogger sets a custom [log.Logger] for panic logging.
// Defaults to [log.GetLogger].
func WithLogger(l log.Logger) Option {
	return func(o *options) { o.logger = l }
}

// Middleware returns a [httpPlugin.Middleware] that recovers from panics in
// downstream handlers, logs them, and returns a 500 Internal Server Error.
func Middleware(opts ...Option) httpPlugin.Middleware {
	cfg := &options{
		logStack:     true,
		errorHandler: defaultErrorHandler,
		logger:       log.GetLogger(),
	}
	for _, opt := range opts {
		opt(cfg)
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if rvr := recover(); rvr != nil {
					args := []any{
						"error", fmt.Sprint(rvr),
						"method", r.Method,
						"path", r.URL.Path,
					}
					if cfg.logStack {
						args = append(args, "stack", string(debug.Stack()))
					}
					cfg.logger.Error(r.Context(), "panic recovered", args...)
					cfg.errorHandler(w, r, rvr)
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}

// defaultErrorHandler writes a generic 500 response without leaking
// the panic value to the client.
func defaultErrorHandler(w http.ResponseWriter, _ *http.Request, _ any) {
	if headersWritten(w) {
		// Headers were already sent; we can only abort the connection.
		return
	}
	http.Error(w, "Internal Server Error", http.StatusInternalServerError)
}

// headersWritten is a best-effort check. Because the standard
// [http.ResponseWriter] does not expose whether headers have been flushed,
// we cannot reliably detect partial writes. In practice, the recovery
// middleware should be placed before any middleware that writes headers.
func headersWritten(_ http.ResponseWriter) bool {
	return false
}
