// Package tracing provides a gRPC server interceptor that creates an
// OpenTelemetry server span for each RPC, extracts trace context from
// incoming gRPC metadata, and records gRPC semantic attributes.
//
// This is the gRPC counterpart of transport/http/middleware/tracing.
//
// Usage:
//
//	// After initialising the global TracerProvider via the tracer/otlp plugin:
//	srv := grpc.NewServer(grpc.ChainUnaryInterceptor(
//	    grpcTracing.UnaryInterceptor(),
//	))
package tracing

import (
	"context"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	otelcodes "go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

const (
	// instrumentationName is the OTel tracer name reported to the backend.
	instrumentationName = "go-wind/plugins/grpc/middleware/tracing"
)

// Option configures the tracing interceptor.
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

// UnaryInterceptor returns a [grpc.UnaryServerInterceptor] that creates an
// OpenTelemetry server span for each unary RPC.
func UnaryInterceptor(opts ...Option) grpc.UnaryServerInterceptor {
	cfg := &options{
		tracer:      otel.GetTracerProvider().Tracer(instrumentationName),
		propagators: otel.GetTextMapPropagator(),
	}
	for _, opt := range opts {
		opt(cfg)
	}

	return func(
		ctx context.Context,
		req any,
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (any, error) {
		// Extract parent trace context from incoming gRPC metadata.
		ctx = extractTrace(ctx, cfg.propagators)

		attrs := []attribute.KeyValue{
			attribute.String("rpc.system", "grpc"),
			attribute.String("rpc.method", info.FullMethod),
			attribute.String("rpc.service", serviceFromMethod(info.FullMethod)),
		}

		ctx, span := cfg.tracer.Start(ctx, info.FullMethod,
			trace.WithSpanKind(trace.SpanKindServer),
			trace.WithAttributes(attrs...),
		)
		defer span.End()

		resp, err := handler(ctx, req)
		if err != nil {
			st, _ := status.FromError(err)
			span.SetAttributes(attribute.String("rpc.grpc.status", st.Code().String()))
			if st.Code() >= codes.Internal {
				span.SetStatus(otelcodes.Error, st.Message())
			}
		} else {
			span.SetAttributes(attribute.String("rpc.grpc.status", codes.OK.String()))
		}

		return resp, err
	}
}

// StreamInterceptor returns a [grpc.StreamServerInterceptor] that creates an
// OpenTelemetry server span for each streaming RPC.
func StreamInterceptor(opts ...Option) grpc.StreamServerInterceptor {
	cfg := &options{
		tracer:      otel.GetTracerProvider().Tracer(instrumentationName),
		propagators: otel.GetTextMapPropagator(),
	}
	for _, opt := range opts {
		opt(cfg)
	}

	return func(
		srv any,
		ss grpc.ServerStream,
		info *grpc.StreamServerInfo,
		handler grpc.StreamHandler,
	) error {
		ctx := extractTrace(ss.Context(), cfg.propagators)

		attrs := []attribute.KeyValue{
			attribute.String("rpc.system", "grpc"),
			attribute.String("rpc.method", info.FullMethod),
			attribute.String("rpc.service", serviceFromMethod(info.FullMethod)),
			attribute.Bool("rpc.grpc.client_stream", info.IsClientStream),
			attribute.Bool("rpc.grpc.server_stream", info.IsServerStream),
		}

		ctx, span := cfg.tracer.Start(ctx, info.FullMethod,
			trace.WithSpanKind(trace.SpanKindServer),
			trace.WithAttributes(attrs...),
		)
		defer span.End()

		wrapped := &wrappedServerStream{ServerStream: ss, ctx: ctx}
		err := handler(srv, wrapped)
		if err != nil {
			st, _ := status.FromError(err)
			span.SetAttributes(attribute.String("rpc.grpc.status", st.Code().String()))
			if st.Code() >= codes.Internal {
				span.SetStatus(otelcodes.Error, st.Message())
			}
		} else {
			span.SetAttributes(attribute.String("rpc.grpc.status", codes.OK.String()))
		}

		return err
	}
}

// extractTrace extracts the parent trace context from incoming gRPC metadata
// using the configured propagator.
func extractTrace(ctx context.Context, propagators propagation.TextMapPropagator) context.Context {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return ctx
	}
	return propagators.Extract(ctx, &mdCarrier{md: md})
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

// ---------------------------------------------------------------------------
// mdCarrier adapts gRPC metadata.MD to the otel propagation.TextMapCarrier
// interface, enabling trace-context extraction from incoming gRPC metadata.
// ---------------------------------------------------------------------------

type mdCarrier struct {
	md metadata.MD
}

func (c *mdCarrier) Get(key string) string {
	values := c.md.Get(key)
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

func (c *mdCarrier) Keys() []string {
	keys := make([]string, 0, len(c.md))
	for k := range c.md {
		keys = append(keys, k)
	}
	return keys
}

func (c *mdCarrier) Set(key, value string) {
	c.md.Set(key, value)
}

// wrappedServerStream wraps a [grpc.ServerStream] to override its context,
// allowing the injected span context to flow through to the service handler.
type wrappedServerStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (w *wrappedServerStream) Context() context.Context {
	return w.ctx
}
