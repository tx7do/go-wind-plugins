// Package cors provides an HTTP middleware that handles Cross-Origin Resource
// Sharing (CORS) by validating the Origin header and setting the appropriate
// Access-Control-* response headers.
//
// It fully handles preflight (OPTIONS) requests by short-circuiting the
// middleware chain with a 204 No Content response.
//
// Usage:
//
//	srv.Use(cors.Middleware(
//	    cors.WithAllowedOrigins("https://app.example.com"),
//	    cors.WithAllowedMethods("GET", "POST", "PUT", "DELETE"),
//	    cors.WithAllowedHeaders("Authorization", "Content-Type"),
//	    cors.WithAllowCredentials(true),
//	))
package cors

import (
	"net/http"
	"strconv"
	"strings"

	httpPlugin "github.com/tx7do/go-wind-plugins/transport/http"
)

// Option configures the CORS middleware.
type Option func(*options)

type options struct {
	allowedOrigins   map[string]struct{}
	allowedMethods   map[string]struct{}
	allowedHeaders   map[string]struct{}
	exposedHeaders   []string
	allowCredentials bool
	maxAge           int // seconds
}

// WithAllowedOrigins sets the allowed origin domains.
// An origin is the scheme + host + port (e.g. "https://app.example.com").
// If empty (the default), all origins are allowed ("*").
func WithAllowedOrigins(origins ...string) Option {
	return func(o *options) {
		if o.allowedOrigins == nil {
			o.allowedOrigins = make(map[string]struct{})
		}
		for _, origin := range origins {
			o.allowedOrigins[origin] = struct{}{}
		}
	}
}

// WithAllowedMethods sets the allowed HTTP methods.
// Defaults to GET, POST, PUT, PATCH, DELETE, HEAD, OPTIONS.
func WithAllowedMethods(methods ...string) Option {
	return func(o *options) {
		if o.allowedMethods == nil {
			o.allowedMethods = make(map[string]struct{})
		}
		for _, m := range methods {
			o.allowedMethods[strings.ToUpper(m)] = struct{}{}
		}
	}
}

// WithAllowedHeaders sets the allowed request headers for preflight responses.
func WithAllowedHeaders(headers ...string) Option {
	return func(o *options) {
		if o.allowedHeaders == nil {
			o.allowedHeaders = make(map[string]struct{})
		}
		for _, h := range headers {
			o.allowedHeaders[http.CanonicalHeaderKey(h)] = struct{}{}
		}
	}
}

// WithExposedHeaders sets the response headers that are visible to the browser.
func WithExposedHeaders(headers ...string) Option {
	return func(o *options) { o.exposedHeaders = headers }
}

// WithAllowCredentials enables or disables credentialed requests
// (cookies, Authorization headers). Defaults to false.
func WithAllowCredentials(enabled bool) Option {
	return func(o *options) { o.allowCredentials = enabled }
}

// WithMaxAge sets how long (in seconds) a preflight response can be cached
// by the browser. Defaults to 0 (no cache).
func WithMaxAge(seconds int) Option {
	return func(o *options) { o.maxAge = seconds }
}

// Middleware returns a [httpPlugin.Middleware] that handles CORS.
func Middleware(opts ...Option) httpPlugin.Middleware {
	cfg := &options{
		allowedMethods: map[string]struct{}{
			http.MethodGet:     {},
			http.MethodPost:    {},
			http.MethodPut:     {},
			http.MethodPatch:   {},
			http.MethodDelete:  {},
			http.MethodHead:    {},
			http.MethodOptions: {},
		},
	}
	for _, opt := range opts {
		opt(cfg)
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")

			// If there's no Origin header, this isn't a CORS request.
			if origin == "" {
				next.ServeHTTP(w, r)
				return
			}

			if !cfg.isOriginAllowed(origin) {
				next.ServeHTTP(w, r)
				return
			}

			cfg.setCORSHeaders(w, origin)

			// Short-circuit preflight requests.
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// isOriginAllowed checks whether the given origin is in the allowed set.
// If no origins are configured, all origins are allowed.
func (o *options) isOriginAllowed(origin string) bool {
	if len(o.allowedOrigins) == 0 {
		return true
	}
	_, ok := o.allowedOrigins[origin]
	return ok
}

// setCORSHeaders sets the Access-Control-* headers on the response.
func (o *options) setCORSHeaders(w http.ResponseWriter, origin string) {
	h := w.Header()

	h.Set("Access-Control-Allow-Origin", origin)

	if o.allowCredentials {
		h.Set("Access-Control-Allow-Credentials", "true")
	}

	if len(o.allowedMethods) > 0 {
		h.Set("Access-Control-Allow-Methods", joinKeys(o.allowedMethods))
	}

	if len(o.allowedHeaders) > 0 {
		h.Set("Access-Control-Allow-Headers", joinKeys(o.allowedHeaders))
	}

	if len(o.exposedHeaders) > 0 {
		h.Set("Access-Control-Expose-Headers", strings.Join(o.exposedHeaders, ", "))
	}

	if o.maxAge > 0 {
		h.Set("Access-Control-Max-Age", strconv.Itoa(o.maxAge))
	}
}

// joinKeys joins the keys of a map[string]struct{} into a comma-separated
// string.
func joinKeys(m map[string]struct{}) string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return strings.Join(keys, ", ")
}
