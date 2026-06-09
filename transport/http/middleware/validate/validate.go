// Package validate provides an HTTP middleware that enforces common request
// validation rules before the handler is invoked:
//   - Maximum request body size (Content-Length pre-check + MaxBytesReader).
//   - Allowed content types (whitelist).
//   - Required headers.
//   - A custom validation function for user-defined rules.
//
// Usage:
//
//	srv.Use(validate.Middleware(
//	    validate.WithMaxBodySize(10 << 20),           // 10 MiB
//	    validate.WithAllowedContentTypes("application/json"),
//	    validate.WithRequiredHeaders("X-Request-ID"),
//	    validate.WithValidator(func(r *http.Request) error {
//	        if r.URL.Query().Get("tenant") == "" {
//	            return errors.New("tenant query parameter is required")
//	        }
//	        return nil
//	    }),
//	))
package validate

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	httpPlugin "github.com/tx7do/go-wind-plugins/transport/http"
)

// ErrBodyTooLarge is returned (wrapped) by the handler when the request body
// exceeds the configured maximum size while being read. It can be detected
// with [errors.Is].
var ErrBodyTooLarge = errors.New("request body too large")

// Option configures the validate middleware.
type Option func(*options)

type options struct {
	maxBodySize     int64
	allowedTypes    map[string]struct{}
	requiredHeaders map[string]struct{}
	validator       func(r *http.Request) error
}

// WithMaxBodySize sets the maximum allowed request body size in bytes.
// A value of 0 (the default) means unlimited.
//
// Requests that declare a Content-Length above the limit are rejected
// immediately with 413 Payload Too Large. For requests without a declared
// Content-Length, the body is wrapped with [http.MaxBytesReader] so that
// reads beyond the limit return an error.
func WithMaxBodySize(n int64) Option {
	return func(o *options) { o.maxBodySize = n }
}

// WithAllowedContentTypes restricts incoming requests to the given content
// types. The comparison ignores parameters (e.g. "; charset=utf-8").
// If no types are configured, all content types are allowed.
//
// Requests with a disallowed content type receive 415 Unsupported Media Type.
func WithAllowedContentTypes(types ...string) Option {
	return func(o *options) {
		if o.allowedTypes == nil {
			o.allowedTypes = make(map[string]struct{})
		}
		for _, t := range types {
			o.allowedTypes[strings.TrimSpace(strings.ToLower(t))] = struct{}{}
		}
	}
}

// WithRequiredHeaders sets header names that must be present (non-empty) in
// every request. Header names are matched case-insensitively.
//
// Missing required headers result in 400 Bad Request.
func WithRequiredHeaders(headers ...string) Option {
	return func(o *options) {
		if o.requiredHeaders == nil {
			o.requiredHeaders = make(map[string]struct{})
		}
		for _, h := range headers {
			o.requiredHeaders[http.CanonicalHeaderKey(h)] = struct{}{}
		}
	}
}

// WithValidator sets a custom validation function executed before the handler.
// If the function returns a non-nil error, the request is rejected with
// 400 Bad Request and the error message as the body.
func WithValidator(fn func(r *http.Request) error) Option {
	return func(o *options) { o.validator = fn }
}

// Middleware returns a [httpPlugin.Middleware] that validates incoming
// requests according to the configured rules.
func Middleware(opts ...Option) httpPlugin.Middleware {
	cfg := &options{}
	for _, opt := range opts {
		opt(cfg)
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// --- Required headers ---
			for h := range cfg.requiredHeaders {
				if r.Header.Get(h) == "" {
					http.Error(w, fmt.Sprintf("missing required header: %s", h), http.StatusBadRequest)
					return
				}
			}

			// --- Content type ---
			if len(cfg.allowedTypes) > 0 {
				ct := r.Header.Get("Content-Type")
				if ct != "" {
					// Strip parameters (e.g. "application/json; charset=utf-8").
					if i := strings.IndexByte(ct, ';'); i >= 0 {
						ct = strings.TrimSpace(ct[:i])
					}
					if _, ok := cfg.allowedTypes[strings.ToLower(ct)]; !ok {
						http.Error(w, fmt.Sprintf("unsupported content type: %s", ct), http.StatusUnsupportedMediaType)
						return
					}
				}
			}

			// --- Body size ---
			if cfg.maxBodySize > 0 {
				if r.ContentLength > cfg.maxBodySize {
					http.Error(w, fmt.Sprintf("request body exceeds %d bytes", cfg.maxBodySize), http.StatusRequestEntityTooLarge)
					return
				}
				if r.Body != nil {
					r.Body = http.MaxBytesReader(w, r.Body, cfg.maxBodySize)
				}
			}

			// --- Custom validator ---
			if cfg.validator != nil {
				if err := cfg.validator(r); err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
			}

			next.ServeHTTP(w, r)
		})
	}
}
