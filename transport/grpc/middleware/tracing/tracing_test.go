package tracing

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	grpcCodes "google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	grpcStatus "google.golang.org/grpc/status"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	oteltrace "go.opentelemetry.io/otel/trace"
)

func newTestTracerProvider(exporter *tracetest.InMemoryExporter) *trace.TracerProvider {
	return trace.NewTracerProvider(trace.WithSyncer(exporter))
}

// fakeServerStream provides a minimal grpc.ServerStream with a context.
type fakeServerStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (s *fakeServerStream) Context() context.Context { return s.ctx }

func TestUnaryInterceptor_CreatesSpan(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	tp := newTestTracerProvider(exporter)
	defer tp.Shutdown(context.Background())

	tracer := tp.Tracer("test")
	interceptor := UnaryInterceptor(WithTracer(tracer))

	info := &grpc.UnaryServerInfo{FullMethod: "/pkg.UserService/GetUser"}
	handler := func(_ context.Context, _ any) (any, error) {
		return "ok", nil
	}

	resp, err := interceptor(context.Background(), nil, info, handler)
	require.NoError(t, err)
	assert.Equal(t, "ok", resp)

	spans := exporter.GetSpans()
	require.Len(t, spans, 1)
	assert.Equal(t, "/pkg.UserService/GetUser", spans[0].Name)
}

func TestUnaryInterceptor_RecordsError(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	tp := newTestTracerProvider(exporter)
	defer tp.Shutdown(context.Background())

	tracer := tp.Tracer("test")
	interceptor := UnaryInterceptor(WithTracer(tracer))

	info := &grpc.UnaryServerInfo{FullMethod: "/pkg.Svc/Fail"}
	handler := func(_ context.Context, _ any) (any, error) {
		return nil, grpcStatus.Error(grpcCodes.Internal, "db error")
	}

	_, err := interceptor(context.Background(), nil, info, handler)
	require.Error(t, err)

	spans := exporter.GetSpans()
	require.Len(t, spans, 1)
	assert.Equal(t, codes.Error, spans[0].Status.Code)
}

func TestUnaryInterceptor_ExtractsTraceContext(t *testing.T) {
	// Set up the global propagator so Extract works.
	otel.SetTextMapPropagator(propagation.TraceContext{})

	exporter := tracetest.NewInMemoryExporter()
	tp := newTestTracerProvider(exporter)
	defer tp.Shutdown(context.Background())

	tracer := tp.Tracer("test")

	// Inject a trace context into outgoing metadata.
	ctx, span := tracer.Start(context.Background(), "client-call",
		oteltrace.WithSpanKind(oteltrace.SpanKindClient))
	carrier := propagation.MapCarrier{}
	otel.GetTextMapPropagator().Inject(ctx, carrier)
	span.End()

	// Convert to gRPC metadata.
	md := metadata.MD{}
	for k, v := range carrier {
		md.Set(k, v)
	}
	ctxWithMD := metadata.NewIncomingContext(context.Background(), md)

	interceptor := UnaryInterceptor(WithTracer(tracer))
	info := &grpc.UnaryServerInfo{FullMethod: "/pkg.Svc/Get"}
	handler := func(_ context.Context, _ any) (any, error) {
		return "ok", nil
	}

	_, err := interceptor(ctxWithMD, nil, info, handler)
	require.NoError(t, err)

	spans := exporter.GetSpans()
	// Should have at least 2 spans: client + server.
	require.GreaterOrEqual(t, len(spans), 2)
}

func TestStreamInterceptor_CreatesSpan(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	tp := newTestTracerProvider(exporter)
	defer tp.Shutdown(context.Background())

	tracer := tp.Tracer("test")
	interceptor := StreamInterceptor(WithTracer(tracer))

	info := &grpc.StreamServerInfo{
		FullMethod:     "/pkg.Svc/StreamData",
		IsServerStream: true,
	}
	ss := &fakeServerStream{ctx: context.Background()}
	handler := func(_ any, _ grpc.ServerStream) error { return nil }

	err := interceptor(nil, ss, info, handler)
	require.NoError(t, err)

	spans := exporter.GetSpans()
	require.Len(t, spans, 1)
	assert.Equal(t, "/pkg.Svc/StreamData", spans[0].Name)
}

func TestServiceFromMethod(t *testing.T) {
	assert.Equal(t, "pkg.UserService", serviceFromMethod("/pkg.UserService/GetUser"))
	assert.Equal(t, "Svc", serviceFromMethod("/Svc/Method"))
	assert.Equal(t, "", serviceFromMethod("/unknown"))
	assert.Equal(t, "noslash", serviceFromMethod("noslash"))
}
