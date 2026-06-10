// Package pprof exposes Go runtime profiling endpoints (CPU profile, heap,
// goroutine, block, mutex, etc.) over HTTP via the standard library's
// net/http/pprof package.
//
// This is the profiling counterpart of the health check handler. It is not a
// middleware — it is an [http.Handler] intended to be registered on a
// dedicated route, typically under "/debug/pprof".
//
// Usage:
//
//	// Register on the HTTP server:
//	h := pprof.NewHandler()
//	srv.GET("/debug/pprof/*", h.ServeHTTP)
//
//	// Or with a custom prefix:
//	h := pprof.NewHandler(pprof.WithPrefix("/debug/pprof"))
//	srv.GET("/debug/pprof/*", h.ServeHTTP)
//
// Security note: profiling endpoints leak internal runtime data. In production,
// protect them with authentication middleware or restrict access to internal
// networks only.
package pprof

import (
	"fmt"
	"net/http"
	"net/http/pprof"
	"strings"
)

// DefaultPrefix is the default URL prefix for pprof endpoints.
const DefaultPrefix = "/debug/pprof"

// Option configures the pprof handler.
type Option func(*options)

type options struct {
	prefix string
}

// WithPrefix sets the URL prefix under which pprof endpoints are exposed.
// Default: "/debug/pprof".
//
// The prefix should match the route registered on the HTTP server, e.g. if
// the server registers "/debug/pprof/*", the prefix should be "/debug/pprof".
func WithPrefix(prefix string) Option {
	return func(o *options) {
		if prefix != "" {
			o.prefix = strings.TrimRight(prefix, "/")
		}
	}
}

// Handler exposes Go runtime profiling endpoints over HTTP.
//
// It internally uses an [http.ServeMux] to register the standard pprof
// handlers: index, cmdline, profile (CPU), symbol, trace, and the individual
// profile pages (heap, goroutine, block, mutex, allocs, threadcreate).
type Handler struct {
	mux *http.ServeMux
}

// NewHandler creates a [Handler] that serves pprof endpoints under the
// configured prefix.
func NewHandler(opts ...Option) *Handler {
	cfg := &options{prefix: DefaultPrefix}
	for _, opt := range opts {
		opt(cfg)
	}

	h := &Handler{mux: http.NewServeMux()}
	h.register(cfg.prefix)
	return h
}

// ServeHTTP implements [http.Handler].
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.mux.ServeHTTP(w, r)
}

// register wires up all standard pprof handlers under the given prefix.
func (h *Handler) register(prefix string) {
	h.mux.HandleFunc(prefix+"/", pprof.Index)
	h.mux.HandleFunc(prefix+"/cmdline", pprof.Cmdline)
	h.mux.HandleFunc(prefix+"/profile", pprof.Profile)
	h.mux.HandleFunc(prefix+"/symbol", pprof.Symbol)
	h.mux.HandleFunc(prefix+"/trace", pprof.Trace)

	// Named profile handlers (heap, goroutine, etc.) use Handler() to return
	// an http.Handler for the given profile name.
	for _, name := range []string{
		"allocs",
		"block",
		"goroutine",
		"heap",
		"mutex",
		"threadcreate",
	} {
		h.mux.Handle(prefix+"/"+name, pprof.Handler(name))
	}
}

// Routes returns the list of full endpoint paths exposed by this handler.
// Useful for documentation or automated route registration.
func (h *Handler) Routes(prefix string) []string {
	return []string{
		fmt.Sprintf("%s/", prefix),
		fmt.Sprintf("%s/cmdline", prefix),
		fmt.Sprintf("%s/profile", prefix),
		fmt.Sprintf("%s/symbol", prefix),
		fmt.Sprintf("%s/trace", prefix),
		fmt.Sprintf("%s/allocs", prefix),
		fmt.Sprintf("%s/block", prefix),
		fmt.Sprintf("%s/goroutine", prefix),
		fmt.Sprintf("%s/heap", prefix),
		fmt.Sprintf("%s/mutex", prefix),
		fmt.Sprintf("%s/threadcreate", prefix),
	}
}
