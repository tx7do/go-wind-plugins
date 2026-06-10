// Package metrics provides gRPC interceptors (both server and client side)
// that record RPC metrics (request count, request duration, and in-flight
// requests) through the engine-agnostic [metrics.Metrics] interface.
//
// Usage (server):
//
//	srv := grpc.NewServer(grpc.ChainUnaryInterceptor(
//	    grpcMetrics.UnaryServerInterceptor(myMetrics),
//	))
//
// Usage (client):
//
//	conn, _ := grpc.NewClient(addr,
//	    grpc.WithUnaryInterceptor(grpcMetrics.UnaryClientInterceptor(myMetrics)),
//	)
package metrics

import (
	"context"
	"strings"
	"time"

	"github.com/tx7do/go-wind-plugins/metrics"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Default metric names.
const (
	defaultRequestCounter   = "grpc_requests_total"
	defaultLatencyHistogram = "grpc_request_duration_seconds"
	defaultInFlightGauge    = "grpc_requests_in_flight"
)

// Option configures the metrics interceptors.
type Option func(*options)

type options struct {
	requestCounter   string
	latencyHistogram string
	inFlightGauge    string
	labelFunc        func(method string, err error) map[string]string
	skipFunc         func(method string) bool
}

// WithRequestCounterName sets the counter metric name.
func WithRequestCounterName(name string) Option {
	return func(o *options) { o.requestCounter = name }
}

// WithLatencyHistogramName sets the histogram metric name.
func WithLatencyHistogramName(name string) Option {
	return func(o *options) { o.latencyHistogram = name }
}

// WithInFlightGaugeName sets the gauge metric name.
func WithInFlightGaugeName(name string) Option {
	return func(o *options) { o.inFlightGauge = name }
}

// WithLabelFunc sets a custom function to derive metric labels.
func WithLabelFunc(fn func(method string, err error) map[string]string) Option {
	return func(o *options) { o.labelFunc = fn }
}

// WithSkipFunc sets a function that returns true for methods to skip.
func WithSkipFunc(fn func(method string) bool) Option {
	return func(o *options) { o.skipFunc = fn }
}

// ---------------------------------------------------------------------------
// Server interceptors
// ---------------------------------------------------------------------------

// UnaryServerInterceptor returns a [grpc.UnaryServerInterceptor] that records
// metrics for incoming unary RPCs.
func UnaryServerInterceptor(m metrics.Metrics, opts ...Option) grpc.UnaryServerInterceptor {
	cfg := newConfig(opts)

	return func(
		ctx context.Context,
		req any,
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (any, error) {
		if cfg.skipFunc != nil && cfg.skipFunc(info.FullMethod) {
			return handler(ctx, req)
		}

		labels := cfg.labelFunc(info.FullMethod, nil)
		m.Gauge(ctx, cfg.inFlightGauge, 1, labels)
		defer m.Gauge(ctx, cfg.inFlightGauge, 0, labels)

		start := time.Now()
		resp, err := handler(ctx, req)
		latency := time.Since(start).Seconds()

		labels = cfg.labelFunc(info.FullMethod, err)
		m.Counter(ctx, cfg.requestCounter, 1, labels)
		m.Histogram(ctx, cfg.latencyHistogram, latency, labels)

		return resp, err
	}
}

// StreamServerInterceptor returns a [grpc.StreamServerInterceptor] that records
// metrics for incoming streaming RPCs.
func StreamServerInterceptor(m metrics.Metrics, opts ...Option) grpc.StreamServerInterceptor {
	cfg := newConfig(opts)

	return func(
		srv any,
		ss grpc.ServerStream,
		info *grpc.StreamServerInfo,
		handler grpc.StreamHandler,
	) error {
		if cfg.skipFunc != nil && cfg.skipFunc(info.FullMethod) {
			return handler(srv, ss)
		}

		labels := cfg.labelFunc(info.FullMethod, nil)
		m.Gauge(ss.Context(), cfg.inFlightGauge, 1, labels)
		defer m.Gauge(ss.Context(), cfg.inFlightGauge, 0, labels)

		start := time.Now()
		err := handler(srv, ss)
		latency := time.Since(start).Seconds()

		labels = cfg.labelFunc(info.FullMethod, err)
		m.Counter(ss.Context(), cfg.requestCounter, 1, labels)
		m.Histogram(ss.Context(), cfg.latencyHistogram, latency, labels)

		return err
	}
}

// ---------------------------------------------------------------------------
// Client interceptors
// ---------------------------------------------------------------------------

// UnaryClientInterceptor returns a [grpc.UnaryClientInterceptor] that records
// metrics for outgoing unary RPCs.
func UnaryClientInterceptor(m metrics.Metrics, opts ...Option) grpc.UnaryClientInterceptor {
	cfg := newConfig(opts)

	return func(
		ctx context.Context,
		method string,
		req, reply any,
		cc *grpc.ClientConn,
		invoker grpc.UnaryInvoker,
		callOpts ...grpc.CallOption,
	) error {
		if cfg.skipFunc != nil && cfg.skipFunc(method) {
			return invoker(ctx, method, req, reply, cc, callOpts...)
		}

		labels := cfg.labelFunc(method, nil)
		m.Gauge(ctx, cfg.inFlightGauge, 1, labels)
		defer m.Gauge(ctx, cfg.inFlightGauge, 0, labels)

		start := time.Now()
		err := invoker(ctx, method, req, reply, cc, callOpts...)
		latency := time.Since(start).Seconds()

		labels = cfg.labelFunc(method, err)
		m.Counter(ctx, cfg.requestCounter, 1, labels)
		m.Histogram(ctx, cfg.latencyHistogram, latency, labels)

		return err
	}
}

// StreamClientInterceptor returns a [grpc.StreamClientInterceptor] that records
// metrics for outgoing streaming RPCs.
func StreamClientInterceptor(m metrics.Metrics, opts ...Option) grpc.StreamClientInterceptor {
	cfg := newConfig(opts)

	return func(
		ctx context.Context,
		desc *grpc.StreamDesc,
		cc *grpc.ClientConn,
		method string,
		streamer grpc.Streamer,
		callOpts ...grpc.CallOption,
	) (grpc.ClientStream, error) {
		if cfg.skipFunc != nil && cfg.skipFunc(method) {
			return streamer(ctx, desc, cc, method, callOpts...)
		}

		labels := cfg.labelFunc(method, nil)
		m.Gauge(ctx, cfg.inFlightGauge, 1, labels)

		start := time.Now()
		cs, err := streamer(ctx, desc, cc, method, callOpts...)

		if err != nil {
			latency := time.Since(start).Seconds()
			labels = cfg.labelFunc(method, err)
			m.Counter(ctx, cfg.requestCounter, 1, labels)
			m.Histogram(ctx, cfg.latencyHistogram, latency, labels)
			m.Gauge(ctx, cfg.inFlightGauge, 0, labels)
		}

		return cs, err
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func newConfig(opts []Option) *options {
	cfg := &options{
		requestCounter:   defaultRequestCounter,
		latencyHistogram: defaultLatencyHistogram,
		inFlightGauge:    defaultInFlightGauge,
		labelFunc:        defaultLabels,
	}
	for _, opt := range opts {
		opt(cfg)
	}
	return cfg
}

// defaultLabels returns standard gRPC metric labels.
func defaultLabels(method string, err error) map[string]string {
	code := codes.OK.String()
	if err != nil {
		st, _ := status.FromError(err)
		code = st.Code().String()
	}
	return map[string]string{
		"method":  method,
		"service": serviceFromMethod(method),
		"code":    code,
	}
}

// serviceFromMethod extracts the service name from a FullMethod string.
// FullMethod format: "/pkg.ServiceName/MethodName" → "pkg.ServiceName"
func serviceFromMethod(fullMethod string) string {
	parts := strings.Split(fullMethod, "/")
	if len(parts) >= 2 {
		return parts[len(parts)-2]
	}
	return fullMethod
}
