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
	"sync"
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

			// 优化：in-flight gauge 的进入/退出复用同一份 label（status=0），
			// 将 labelFunc 调用次数从 3 次降到 2 次。
			// 注意：不池化 label map —— metrics 后端实现（如 prometheus）
			// 可能异步持有 labels map，池化会导致 data race。
			gaugeLabels := cfg.labelFunc(r, 0)
			m.Gauge(r.Context(), cfg.inFlightGauge, 1, gaugeLabels)
			defer m.Gauge(r.Context(), cfg.inFlightGauge, 0, gaugeLabels)

			// 池化 statusRecorder（仅捕获 status，同步使用，handler 返回即归还，安全）。
			rw := acquireStatusRecorder(w)
			defer releaseStatusRecorder(rw)

			start := time.Now()
			next.ServeHTTP(rw, r)
			latency := time.Since(start).Seconds()

			// 结束时的 counter/histogram 用带真实 status 的 label（调用 1 次）。
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

// --- sync.Pool 优化（statusRecorder 复用） ---
//
// statusRecorder 是同步包装结构：在 next.ServeHTTP 返回后立即归还，
// 不存在异步持有的风险，故可安全池化。
// （label map 不池化，因 metrics 后端可能异步读取 labels。）

var statusRecorderPool = sync.Pool{
	New: func() any { return &statusRecorder{} },
}

func acquireStatusRecorder(w http.ResponseWriter) *statusRecorder {
	sr := statusRecorderPool.Get().(*statusRecorder)
	sr.ResponseWriter = w
	sr.status = http.StatusOK
	return sr
}

func releaseStatusRecorder(sr *statusRecorder) {
	sr.ResponseWriter = nil
	statusRecorderPool.Put(sr)
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
