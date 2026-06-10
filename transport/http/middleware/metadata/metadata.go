// Package metadata provides an HTTP middleware that propagates arbitrary
// key-value metadata between services via HTTP headers.
//
// On the server side, metadata is extracted from incoming HTTP headers and
// stored in the request context for downstream handlers to consume.
//
// This is the HTTP counterpart of transport/grpc/middleware/metadata.
// Unlike the dedicated requestid middleware (which handles a single key),
// this middleware handles an arbitrary set of keys.
//
// Usage:
//
//	srv.Use(metadata.Middleware(metadata.WithKeys("X-Tenant-ID", "X-User-ID")))
//
// To read metadata in a handler:
//
//	tenantID := metadata.FromContext(r.Context(), "X-Tenant-ID")
package metadata

import (
	"context"
	"net/http"

	httpPlugin "github.com/tx7do/go-wind-plugins/transport/http"
)

// ctxKey is an unexported type for context keys in this package.
type ctxKey struct {
	key string
}

// Option configures the metadata middleware.
type Option func(*options)

type options struct {
	keys []string
}

// WithKeys sets the HTTP header names to extract and propagate through the
// context. The header names should use canonical form (e.g. "X-Tenant-ID").
func WithKeys(keys ...string) Option {
	return func(o *options) { o.keys = append(o.keys, keys...) }
}

// Middleware returns a [httpPlugin.Middleware] that extracts the configured
// header keys from incoming requests and stores them in the request context.
func Middleware(opts ...Option) httpPlugin.Middleware {
	cfg := &options{}
	for _, opt := range opts {
		opt(cfg)
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			for _, key := range cfg.keys {
				if val := r.Header.Get(key); val != "" {
					ctx = WithMetadata(ctx, key, val)
				}
			}
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// WithMetadata stores a metadata value in the context under the given key.
func WithMetadata(ctx context.Context, key, value string) context.Context {
	return context.WithValue(ctx, ctxKey{key: key}, value)
}

// FromContext retrieves a metadata value from the context.
// Returns an empty string if the key is not present.
func FromContext(ctx context.Context, key string) string {
	if ctx == nil {
		return ""
	}
	v, _ := ctx.Value(ctxKey{key: key}).(string)
	return v
}
