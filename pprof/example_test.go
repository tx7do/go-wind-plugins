package pprof_test

import (
	"net/http"

	"github.com/tx7do/go-wind-plugins/pprof"
)

// ExampleNewHandler constructs the profiling handler with the default prefix
// and mounts it on the router's debug route. The handler serves the standard
// Go runtime profiling endpoints under the configured prefix; in production
// the route is placed behind authentication middleware or restricted to an
// internal network, since the endpoints expose internal runtime data.
func ExampleNewHandler() {
	h := pprof.NewHandler(pprof.WithPrefix(pprof.DefaultPrefix))

	mux := http.NewServeMux()
	mux.Handle("/debug/pprof/", h)
	_ = mux
}
