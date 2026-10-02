package tracing

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	grpcCodes "google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	grpcStatus "google.golang.org/grpc/status"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

// newExporterProvider builds an SDK tracer provider with the given in-memory
// exporter and a TraceContext propagator for hermetic inject/extract tests.
func newExporterProvider(exporter *tracetest.InMemoryExporter) *sdktrace.TracerProvider {
	return sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
}

// fakeClientStream is a minimal grpc.ClientStream for wrapping tests.
type fakeClientStream struct {
	grpc.ClientStream
	headerErr   error
	closeErr    error
	closeCalled bool
}

func (s *fakeClientStream) Header() (metadata.MD, error) { return nil, s.headerErr }
func (s *fakeClientStream) CloseSend() error {
	s.closeCalled = true
	return s.closeErr
}

// ---------------------------------------------------------------------------
// mdCarrier
// ---------------------------------------------------------------------------

func TestMDCarrier(t *testing.T) {
	md := metadata.MD{}
	carrier := &mdCarrier{md: md}

	// Get on empty metadata → "".
	assert.Equal(t, "", carrier.Get("missing"))

	carrier.Set("traceparent", "00-trace-span-01")
	carrier.Set("x-custom", "v1")

	assert.Equal(t, "00-trace-span-01", carrier.Get("traceparent"))
	assert.Equal(t, "v1", carrier.Get("x-custom"))

	keys := carrier.Keys()
	assert.Len(t, keys, 2)
	for _, k := range keys {
		assert.True(t, k == "traceparent" || k == "x-custom", "unexpected key %q", k)
	}

	// The carrier writes through to the underlying metadata.
	assert.Equal(t, []string{"00-trace-span-01"}, md.Get("traceparent"))
}

// ---------------------------------------------------------------------------
// UnaryClientInterceptor
// ---------------------------------------------------------------------------

func TestUnaryClientInterceptor_CreatesSpanAndInjects(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	tp := newExporterProvider(exporter)
	defer func() { _ = tp.Shutdown(context.Background()) }()

	prop := propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{})
	interceptor := UnaryClientInterceptor(WithTracer(tp.Tracer("test")), WithPropagators(prop))

	var capturedCtx context.Context
	invoker := func(ctx context.Context, _ string, _, _ any, _ *grpc.ClientConn, _ ...grpc.CallOption) error {
		capturedCtx = ctx
		return nil
	}

	err := interceptor(context.Background(), "/pkg.UserService/GetUser", nil, nil, nil, invoker)
	require.NoError(t, err)

	// Trace context must be injected into outgoing metadata.
	md, ok := metadata.FromOutgoingContext(capturedCtx)
	require.True(t, ok, "outgoing metadata expected")
	require.NotEmpty(t, md.Get("traceparent"))
	assert.True(t, strings.HasPrefix(md.Get("traceparent")[0], "00-"), "expected traceparent format")

	spans := exporter.GetSpans()
	require.Len(t, spans, 1)
	stub := spans[0]
	assert.Equal(t, "/pkg.UserService/GetUser", stub.Name)
	assert.Equal(t, trace.SpanKindClient, stub.SpanKind)
	// Success is recorded as an attribute; the span status itself stays Unset.
	assert.Equal(t, codes.Unset, stub.Status.Code)

	// gRPC semantic attributes.
	expectAttrs := map[attribute.Key]string{
		"rpc.system":      "grpc",
		"rpc.method":      "/pkg.UserService/GetUser",
		"rpc.service":     "pkg.UserService",
		"rpc.grpc.status": "OK",
	}
	got := map[attribute.Key]string{}
	for _, kv := range stub.Attributes {
		got[kv.Key] = kv.Value.Emit()
	}
	for k, v := range expectAttrs {
		assert.Equal(t, v, got[k], "attribute %s", k)
	}
}

func TestUnaryClientInterceptor_RecordsInternalError(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	tp := newExporterProvider(exporter)
	defer func() { _ = tp.Shutdown(context.Background()) }()

	interceptor := UnaryClientInterceptor(
		WithTracer(tp.Tracer("test")),
		WithPropagators(propagation.TraceContext{}),
	)

	invoker := func(_ context.Context, _ string, _, _ any, _ *grpc.ClientConn, _ ...grpc.CallOption) error {
		return grpcStatus.Error(grpcCodes.Internal, "upstream unavailable")
	}

	err := interceptor(context.Background(), "/pkg.Svc/Get", nil, nil, nil, invoker)
	require.Error(t, err)

	spans := exporter.GetSpans()
	require.Len(t, spans, 1)
	assert.Equal(t, codes.Error, spans[0].Status.Code)
	assert.Equal(t, "upstream unavailable", spans[0].Status.Description)
}

func TestUnaryClientInterceptor_NonInternalErrorIsNotErrorStatus(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	tp := newExporterProvider(exporter)
	defer func() { _ = tp.Shutdown(context.Background()) }()

	interceptor := UnaryClientInterceptor(
		WithTracer(tp.Tracer("test")),
		WithPropagators(propagation.TraceContext{}),
	)

	invoker := func(_ context.Context, _ string, _, _ any, _ *grpc.ClientConn, _ ...grpc.CallOption) error {
		return grpcStatus.Error(grpcCodes.NotFound, "nope")
	}

	err := interceptor(context.Background(), "/pkg.Svc/Get", nil, nil, nil, invoker)
	require.Error(t, err)

	spans := exporter.GetSpans()
	require.Len(t, spans, 1)
	// NotFound < Internal → span status stays Unset.
	assert.Equal(t, codes.Unset, spans[0].Status.Code)
}

// ---------------------------------------------------------------------------
// StreamClientInterceptor
// ---------------------------------------------------------------------------

func TestStreamClientInterceptor_CreatesSpanAndInjects(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	tp := newExporterProvider(exporter)
	defer func() { _ = tp.Shutdown(context.Background()) }()

	prop := propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{})
	interceptor := StreamClientInterceptor(WithTracer(tp.Tracer("test")), WithPropagators(prop))

	var capturedCtx context.Context
	cs := &fakeClientStream{}
	streamer := func(ctx context.Context, _ *grpc.StreamDesc, _ *grpc.ClientConn, _ string, _ ...grpc.CallOption) (grpc.ClientStream, error) {
		capturedCtx = ctx
		return cs, nil
	}

	desc := &grpc.StreamDesc{StreamName: "Stream", ClientStreams: true, ServerStreams: true}
	stream, err := interceptor(context.Background(), desc, nil, "/pkg.Svc/StreamData", streamer)
	require.NoError(t, err)
	require.NotNil(t, stream)

	// Trace context injected into outgoing metadata.
	md, ok := metadata.FromOutgoingContext(capturedCtx)
	require.True(t, ok)
	require.NotEmpty(t, md.Get("traceparent"))

	// The returned stream must be the traced wrapper.
	_, isTraced := stream.(*tracedClientStream)
	assert.True(t, isTraced, "expected tracedClientStream wrapper")

	// The span is not ended yet → not exported.
	assert.Empty(t, exporter.GetSpans())

	// CloseSend ends the span with OK status.
	require.NoError(t, stream.CloseSend())
	assert.True(t, cs.closeCalled)

	spans := exporter.GetSpans()
	require.Len(t, spans, 1)
	assert.Equal(t, "/pkg.Svc/StreamData", spans[0].Name)
	assert.Equal(t, trace.SpanKindClient, spans[0].SpanKind)
	// Success is an attribute; span status stays Unset.
	assert.Equal(t, codes.Unset, spans[0].Status.Code)
}

func TestStreamClientInterceptor_StreamCreationError(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	tp := newExporterProvider(exporter)
	defer func() { _ = tp.Shutdown(context.Background()) }()

	interceptor := StreamClientInterceptor(
		WithTracer(tp.Tracer("test")),
		WithPropagators(propagation.TraceContext{}),
	)

	streamer := func(_ context.Context, _ *grpc.StreamDesc, _ *grpc.ClientConn, _ string, _ ...grpc.CallOption) (grpc.ClientStream, error) {
		return nil, grpcStatus.Error(grpcCodes.Unavailable, "cannot connect")
	}

	desc := &grpc.StreamDesc{StreamName: "Stream"}
	cs, err := interceptor(context.Background(), desc, nil, "/pkg.Svc/StreamData", streamer)
	require.Error(t, err)
	assert.Nil(t, cs)

	spans := exporter.GetSpans()
	require.Len(t, spans, 1)
	// Unavailable (14) >= Internal (13) → error status is recorded.
	assert.Equal(t, codes.Error, spans[0].Status.Code)
	assert.Equal(t, "cannot connect", spans[0].Status.Description)
}

func TestTracedClientStream_HeaderErrorEndsSpan(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	tp := newExporterProvider(exporter)
	defer func() { _ = tp.Shutdown(context.Background()) }()

	tracer := tp.Tracer("test")
	_, span := tracer.Start(context.Background(), "/pkg.Svc/Stream", trace.WithSpanKind(trace.SpanKindClient))

	headerErr := grpcStatus.Error(grpcCodes.Internal, "header failed")
	wrapped := &tracedClientStream{
		ClientStream: &fakeClientStream{headerErr: headerErr},
		span:         span,
	}

	_, err := wrapped.Header()
	require.Error(t, err)
	assert.True(t, errors.Is(err, headerErr))

	spans := exporter.GetSpans()
	require.Len(t, spans, 1)
	assert.Equal(t, codes.Error, spans[0].Status.Code)
	assert.Equal(t, "header failed", spans[0].Status.Description)
}

func TestTracedClientStream_HeaderSuccessDoesNotEndSpan(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	tp := newExporterProvider(exporter)
	defer func() { _ = tp.Shutdown(context.Background()) }()

	tracer := tp.Tracer("test")
	ctx, span := tracer.Start(context.Background(), "/pkg.Svc/Stream", trace.WithSpanKind(trace.SpanKindClient))
	_ = ctx

	wrapped := &tracedClientStream{
		ClientStream: &fakeClientStream{},
		span:         span,
	}

	// A successful Header must NOT end the span.
	md, err := wrapped.Header()
	require.NoError(t, err)
	assert.Nil(t, md)
	assert.Empty(t, exporter.GetSpans())

	span.End()
	assert.Len(t, exporter.GetSpans(), 1)
}

func TestTracedClientStream_CloseSendPropagatesError(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	tp := newExporterProvider(exporter)
	defer func() { _ = tp.Shutdown(context.Background()) }()

	tracer := tp.Tracer("test")
	_, span := tracer.Start(context.Background(), "/pkg.Svc/Stream", trace.WithSpanKind(trace.SpanKindClient))

	closeErr := errors.New("close failed")
	wrapped := &tracedClientStream{
		ClientStream: &fakeClientStream{closeErr: closeErr},
		span:         span,
	}

	err := wrapped.CloseSend()
	require.Error(t, err)
	assert.True(t, errors.Is(err, closeErr))

	// The span is ended with an "OK" attribute; status itself stays Unset.
	spans := exporter.GetSpans()
	require.Len(t, spans, 1)
	assert.Equal(t, codes.Unset, spans[0].Status.Code)
	got := map[attribute.Key]string{}
	for _, kv := range spans[0].Attributes {
		got[kv.Key] = kv.Value.Emit()
	}
	assert.Equal(t, "OK", got["rpc.grpc.status"])
}

// ---------------------------------------------------------------------------
// StreamInterceptor — parity and metadata paths
// ---------------------------------------------------------------------------

func TestStreamInterceptor_RecordsInternalError(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	tp := newExporterProvider(exporter)
	defer func() { _ = tp.Shutdown(context.Background()) }()

	interceptor := StreamInterceptor(
		WithTracer(tp.Tracer("test")),
		WithPropagators(propagation.TraceContext{}),
	)

	info := &grpc.StreamServerInfo{FullMethod: "/pkg.Svc/Fail", IsServerStream: true}
	ss := &fakeServerStream{ctx: context.Background()}
	handler := func(_ any, _ grpc.ServerStream) error {
		return grpcStatus.Error(grpcCodes.Internal, "stream db error")
	}

	err := interceptor(nil, ss, info, handler)
	require.Error(t, err)

	spans := exporter.GetSpans()
	require.Len(t, spans, 1)
	assert.Equal(t, codes.Error, spans[0].Status.Code)
	assert.Equal(t, "stream db error", spans[0].Status.Description)
}

func TestStreamInterceptor_NonInternalErrorIsNotErrorStatus(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	tp := newExporterProvider(exporter)
	defer func() { _ = tp.Shutdown(context.Background()) }()

	interceptor := StreamInterceptor(
		WithTracer(tp.Tracer("test")),
		WithPropagators(propagation.TraceContext{}),
	)

	info := &grpc.StreamServerInfo{FullMethod: "/pkg.Svc/Fail"}
	ss := &fakeServerStream{ctx: context.Background()}
	handler := func(_ any, _ grpc.ServerStream) error {
		return grpcStatus.Error(grpcCodes.NotFound, "gone")
	}

	err := interceptor(nil, ss, info, handler)
	require.Error(t, err)

	spans := exporter.GetSpans()
	require.Len(t, spans, 1)
	assert.Equal(t, codes.Unset, spans[0].Status.Code)
}

func TestUnaryInterceptor_NonInternalErrorIsNotErrorStatus(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	tp := newExporterProvider(exporter)
	defer func() { _ = tp.Shutdown(context.Background()) }()

	interceptor := UnaryInterceptor(
		WithTracer(tp.Tracer("test")),
		WithPropagators(propagation.TraceContext{}),
	)

	info := &grpc.UnaryServerInfo{FullMethod: "/pkg.Svc/Get"}
	handler := func(_ context.Context, _ any) (any, error) {
		return nil, grpcStatus.Error(grpcCodes.InvalidArgument, "bad input")
	}

	_, err := interceptor(context.Background(), nil, info, handler)
	require.Error(t, err)

	spans := exporter.GetSpans()
	require.Len(t, spans, 1)
	assert.Equal(t, codes.Unset, spans[0].Status.Code)
}

func TestUnaryInterceptor_HandlerContextCarriesSpan(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	tp := newExporterProvider(exporter)
	defer func() { _ = tp.Shutdown(context.Background()) }()

	tracer := tp.Tracer("test")
	interceptor := UnaryInterceptor(
		WithTracer(tracer),
		WithPropagators(propagation.TraceContext{}),
	)

	var handlerSpan trace.Span
	var recordingInHandler bool
	info := &grpc.UnaryServerInfo{FullMethod: "/pkg.Svc/Get"}
	handler := func(ctx context.Context, _ any) (any, error) {
		handlerSpan = trace.SpanFromContext(ctx)
		recordingInHandler = handlerSpan.IsRecording()
		return "ok", nil
	}

	_, err := interceptor(context.Background(), nil, info, handler)
	require.NoError(t, err)
	require.NotNil(t, handlerSpan)

	// While the handler runs, its context must carry a live, recording span.
	// (After the interceptor's deferred span.End(), IsRecording flips to false.)
	assert.True(t, recordingInHandler)
	assert.True(t, handlerSpan.SpanContext().IsValid())
	assert.False(t, handlerSpan.SpanContext().IsRemote())
}

func TestStreamInterceptor_ExtractsTraceContextFromMetadata(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	tp := newExporterProvider(exporter)
	defer func() { _ = tp.Shutdown(context.Background()) }()

	tracer := tp.Tracer("test")
	prop := propagation.TraceContext{}

	// Build a client span and inject it into metadata.
	ctx, clientSpan := tracer.Start(context.Background(), "client-call",
		trace.WithSpanKind(trace.SpanKindClient))
	carrier := propagation.MapCarrier{}
	prop.Inject(ctx, carrier)
	clientSpan.End()

	md := metadata.MD{}
	for k, v := range carrier {
		md.Set(k, v)
	}
	ssCtx := metadata.NewIncomingContext(context.Background(), md)

	interceptor := StreamInterceptor(WithTracer(tracer), WithPropagators(prop))
	info := &grpc.StreamServerInfo{
		FullMethod:     "/pkg.Svc/StreamData",
		IsClientStream: true,
		IsServerStream: true,
	}
	ss := &fakeServerStream{ctx: ssCtx}

	var serverTraceID string
	handler := func(_ any, stream grpc.ServerStream) error {
		serverSpan := trace.SpanFromContext(stream.Context())
		serverTraceID = serverSpan.SpanContext().TraceID().String()
		return nil
	}

	err := interceptor(nil, ss, info, handler)
	require.NoError(t, err)

	spans := exporter.GetSpans()
	require.GreaterOrEqual(t, len(spans), 2)
	// spans[0] is the client span (ended first), spans[1] the server span.
	// The server span must share the client span's trace ID (extracted parent).
	assert.Equal(t, spans[0].SpanContext.TraceID().String(), serverTraceID)
	assert.Equal(t, spans[0].SpanContext.TraceID().String(), spans[1].SpanContext.TraceID().String())
	assert.Equal(t, spans[0].SpanContext.SpanID().String(), spans[1].Parent.SpanID().String())
	assert.Equal(t, trace.SpanKindServer, spans[1].SpanKind)
}

func TestStreamInterceptor_NoMetadata(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	tp := newExporterProvider(exporter)
	defer func() { _ = tp.Shutdown(context.Background()) }()

	interceptor := StreamInterceptor(
		WithTracer(tp.Tracer("test")),
		WithPropagators(propagation.TraceContext{}),
	)

	info := &grpc.StreamServerInfo{FullMethod: "/pkg.Svc/Stream", IsClientStream: true, IsServerStream: true}
	ss := &fakeServerStream{ctx: context.Background()} // no incoming metadata
	handler := func(_ any, stream grpc.ServerStream) error {
		// Stream attributes should still record stream direction flags.
		return nil
	}

	err := interceptor(nil, ss, info, handler)
	require.NoError(t, err)

	spans := exporter.GetSpans()
	require.Len(t, spans, 1)
	got := map[attribute.Key]string{}
	for _, kv := range spans[0].Attributes {
		got[kv.Key] = kv.Value.Emit()
	}
	assert.Equal(t, "true", got["rpc.grpc.client_stream"])
	assert.Equal(t, "true", got["rpc.grpc.server_stream"])
}
