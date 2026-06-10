package http3

import (
	"context"
	"encoding/json"
	"net/http"
)

// FilterFunc is an HTTP filter function that wraps an http.Handler.
type FilterFunc func(h http.Handler) http.Handler

// FilterChain builds an http.Handler from the given filters.
// Filters are applied in reverse order so the first filter is the outermost.
func FilterChain(filters ...FilterFunc) FilterFunc {
	return func(next http.Handler) http.Handler {
		for i := len(filters) - 1; i >= 0; i-- {
			next = filters[i](next)
		}
		return next
	}
}

// DecodeRequestFunc decodes an HTTP request body into v.
type DecodeRequestFunc func(req *http.Request, v any) error

// EncodeResponseFunc encodes v into the HTTP response.
type EncodeResponseFunc func(w http.ResponseWriter, req *http.Request, v any) error

// EncodeErrorFunc encodes err into the HTTP response.
type EncodeErrorFunc func(w http.ResponseWriter, req *http.Request, err error) error

// Handler is a middleware handler function.
type Handler func(ctx context.Context, req any) (any, error)

// Middleware is a middleware function that wraps a Handler.
type Middleware func(Handler) Handler

// Chain builds a Middleware from the given middlewares.
// Middlewares are applied in reverse order so the first middleware is the outermost.
func Chain(m ...Middleware) Middleware {
	return func(next Handler) Handler {
		for i := len(m) - 1; i >= 0; i-- {
			next = m[i](next)
		}
		return next
	}
}

// DefaultRequestDecoder decodes the request body as JSON.
func DefaultRequestDecoder(req *http.Request, v any) error {
	if req.Body == nil || req.ContentLength == 0 {
		return nil
	}
	return json.NewDecoder(req.Body).Decode(v)
}

// DefaultResponseEncoder encodes v as JSON into the response.
func DefaultResponseEncoder(w http.ResponseWriter, _ *http.Request, v any) error {
	w.Header().Set("Content-Type", "application/json")
	return json.NewEncoder(w).Encode(v)
}

// DefaultErrorEncoder encodes err as a JSON error response.
func DefaultErrorEncoder(w http.ResponseWriter, _ *http.Request, err error) error {
	w.Header().Set("Content-Type", "application/json")
	code := http.StatusInternalServerError
	if se, ok := err.(interface{ HTTPStatus() int }); ok {
		code = se.HTTPStatus()
	}
	w.WriteHeader(code)
	return json.NewEncoder(w).Encode(map[string]any{
		"error": err.Error(),
	})
}
