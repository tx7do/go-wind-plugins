// Package tracing provides an HTTP middleware that creates an OpenTelemetry
// server span for each request, extracts trace context from incoming headers,
// and records HTTP semantic attributes.
//
// Usage:
//
//	// After initialising the global TracerProvider via the tracer/otlp plugin:
//	srv.Use(tracing.Middleware())
//	// or with options:
//	srv.Use(tracing.Middleware(
//	    tracing.WithTracer(myTracer),
//	    tracing.WithPropagators(myPropagators),
//	))
package tracing

import (
	"net/http"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"

	httpPlugin "github.com/tx7do/go-wind-plugins/transport/http"
)

const (
	// instrumentationName is the OTel tracer name reported to the backend.
	instrumentationName = "go-wind/plugins/http/middleware/tracing"
)

// Option configures the tracing middleware.
type Option func(*options)

type options struct {
	tracer      trace.Tracer
	propagators propagation.TextMapPropagator
}

// WithTracer sets a custom [trace.Tracer]. Defaults to the global tracer
// provider's tracer registered under [instrumentationName].
func WithTracer(t trace.Tracer) Option {
	return func(o *options) { o.tracer = t }
}

// WithPropagators sets custom [propagation.TextMapPropagator] for extracting
// and injecting trace context. Defaults to the global text-map propagator.
func WithPropagators(p propagation.TextMapPropagator) Option {
	return func(o *options) { o.propagators = p }
}

// Middleware returns a [httpPlugin.Middleware] that creates an OpenTelemetry
// server span for each HTTP request.
func Middleware(opts ...Option) httpPlugin.Middleware {
	cfg := &options{
		tracer:      otel.GetTracerProvider().Tracer(instrumentationName),
		propagators: otel.GetTextMapPropagator(),
	}
	for _, opt := range opts {
		opt(cfg)
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Extract parent trace context from incoming headers.
			ctx := cfg.propagators.Extract(r.Context(), propagation.HeaderCarrier(r.Header))

			spanName := r.Method + " " + r.URL.Path

			attrs := []attribute.KeyValue{
				attribute.String("http.method", r.Method),
				attribute.String("http.target", r.URL.RequestURI()),
				attribute.String("http.host", r.Host),
				attribute.String("http.scheme", requestScheme(r)),
				attribute.String("http.flavor", r.Proto),
				attribute.String("net.peer.name", r.RemoteAddr),
			}

			ctx, span := cfg.tracer.Start(ctx, spanName,
				trace.WithSpanKind(trace.SpanKindServer),
				trace.WithAttributes(attrs...),
			)
			defer span.End()

			rw := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(rw, r.WithContext(ctx))

			span.SetAttributes(attribute.Int("http.status_code", rw.status))

			if rw.status >= 500 {
				span.SetStatus(codes.Error, http.StatusText(rw.status))
			}
		})
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

// requestScheme returns "https" for TLS connections, "http" otherwise.
func requestScheme(r *http.Request) string {
	if r.TLS != nil {
		return "https"
	}
	return "http"
}
