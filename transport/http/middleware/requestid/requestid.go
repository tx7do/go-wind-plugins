// Package requestid provides an HTTP middleware that extracts or generates a
// unique request ID for each request, propagates it through the request
// context, and echoes it back in the response header.
//
// The request ID is extracted from the incoming "X-Request-ID" header (or a
// custom header). If absent, a new random ID is generated.
//
// Usage:
//
//	srv.Use(requestid.Middleware())
//	// or with options:
//	srv.Use(requestid.Middleware(
//	    requestid.WithHeaderName("X-Correlation-ID"),
//	    requestid.WithIDGenerator(myGenerator),
//	))
//
// To retrieve the request ID inside a handler:
//
//	id := requestid.FromContext(r.Context())
package requestid

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"

	httpPlugin "github.com/tx7do/go-wind-plugins/transport/http"
)

// DefaultHeaderName is the default HTTP header for the request ID.
const DefaultHeaderName = "X-Request-ID"

// ctxKey is an unexported type for context keys in this package.
type ctxKey struct{}

// Option configures the requestid middleware.
type Option func(*options)

type options struct {
	headerName  string
	idGenerator func() string
}

// WithHeaderName sets the HTTP header used to extract and set the request ID.
// Default: "X-Request-ID".
func WithHeaderName(name string) Option {
	return func(o *options) { o.headerName = name }
}

// WithIDGenerator sets a custom function for generating new request IDs.
// Default: a 16-byte hex-encoded random ID.
func WithIDGenerator(fn func() string) Option {
	return func(o *options) { o.idGenerator = fn }
}

// Middleware returns a [httpPlugin.Middleware] that extracts or generates a
// request ID for each request.
func Middleware(opts ...Option) httpPlugin.Middleware {
	cfg := &options{
		headerName:  DefaultHeaderName,
		idGenerator: defaultIDGenerator,
	}
	for _, opt := range opts {
		opt(cfg)
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			id := r.Header.Get(cfg.headerName)
			if id == "" {
				id = cfg.idGenerator()
			}

			// Echo the ID back in the response.
			w.Header().Set(cfg.headerName, id)

			// Propagate through context.
			ctx := WithRequestID(r.Context(), id)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// WithRequestID stores a request ID in the context.
func WithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, ctxKey{}, id)
}

// FromContext extracts the request ID from the context.
// Returns an empty string if no ID is present.
func FromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	id, _ := ctx.Value(ctxKey{}).(string)
	return id
}

// defaultIDGenerator generates a 16-byte hex-encoded random ID (32 chars).
func defaultIDGenerator() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
