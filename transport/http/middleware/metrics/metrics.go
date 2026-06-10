// Package metrics provides an HTTP middleware that records request metrics
// (request count, request duration, and in-flight requests) through the
// engine-agnostic [metrics.Metrics] interface.
//
// Usage:
//
//	m := metrics.Middleware(myMetrics)
//	srv.Use(m)
//	// or with options:
//	srv.Use(metrics.Middleware(myMetrics,
//	    metrics.WithRequestCounterName("http_requests_total"),
//	    metrics.WithLatencyHistogramName("http_request_duration_seconds"),
//	))
package metrics

import (
	"net/http"
	"strconv"
	"time"

	"github.com/tx7do/go-wind-plugins/metrics"
	httpPlugin "github.com/tx7do/go-wind-plugins/transport/http"
)

// Default metric names.
const (
	defaultRequestCounter   = "http_requests_total"
	defaultLatencyHistogram = "http_request_duration_seconds"
	defaultInFlightGauge    = "http_requests_in_flight"
)

// Option configures the metrics middleware.
type Option func(*options)

type options struct {
	requestCounter   string
	latencyHistogram string
	inFlightGauge    string
	labelFunc        func(r *http.Request, status int) map[string]string
	skipFunc         func(r *http.Request) bool
}

// WithRequestCounterName sets the counter metric name for total requests.
// Default: "http_requests_total".
func WithRequestCounterName(name string) Option {
	return func(o *options) { o.requestCounter = name }
}

// WithLatencyHistogramName sets the histogram metric name for request latency.
// Default: "http_request_duration_seconds".
func WithLatencyHistogramName(name string) Option {
	return func(o *options) { o.latencyHistogram = name }
}

// WithInFlightGaugeName sets the gauge metric name for in-flight requests.
// Default: "http_requests_in_flight".
func WithInFlightGaugeName(name string) Option {
	return func(o *options) { o.inFlightGauge = name }
}

// WithLabelFunc sets a custom function to derive metric labels from the
// request and response status. The default labels are:
//
//	{"method": r.Method, "path": r.URL.Path, "status": "200"}
func WithLabelFunc(fn func(r *http.Request, status int) map[string]string) Option {
	return func(o *options) { o.labelFunc = fn }
}

// WithSkipFunc sets a function that returns true for requests that should
// bypass metrics collection (e.g. health checks).
func WithSkipFunc(fn func(r *http.Request) bool) Option {
	return func(o *options) { o.skipFunc = fn }
}

// Middleware returns a [httpPlugin.Middleware] that records request metrics
// through the provided [metrics.Metrics] implementation.
func Middleware(m metrics.Metrics, opts ...Option) httpPlugin.Middleware {
	cfg := &options{
		requestCounter:   defaultRequestCounter,
		latencyHistogram: defaultLatencyHistogram,
		inFlightGauge:    defaultInFlightGauge,
		labelFunc:        defaultLabels,
	}
	for _, opt := range opts {
		opt(cfg)
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if cfg.skipFunc != nil && cfg.skipFunc(r) {
				next.ServeHTTP(w, r)
				return
			}

			m.Gauge(r.Context(), cfg.inFlightGauge, 1, cfg.labelFunc(r, 0))
			defer m.Gauge(r.Context(), cfg.inFlightGauge, 0, cfg.labelFunc(r, 0))

			rw := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
			start := time.Now()
			next.ServeHTTP(rw, r)
			latency := time.Since(start).Seconds()

			labels := cfg.labelFunc(r, rw.status)
			m.Counter(r.Context(), cfg.requestCounter, 1, labels)
			m.Histogram(r.Context(), cfg.latencyHistogram, latency, labels)
		})
	}
}

// defaultLabels returns the standard set of labels for an HTTP request.
func defaultLabels(r *http.Request, status int) map[string]string {
	return map[string]string{
		"method": r.Method,
		"path":   r.URL.Path,
		"status": strconv.Itoa(status),
	}
}

// statusRecorder wraps [http.ResponseWriter] to capture the status code.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (sr *statusRecorder) WriteHeader(code int) {
	sr.status = code
	sr.ResponseWriter.WriteHeader(code)
}
