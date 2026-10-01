package otlp

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

// ---------------------------------------------------------------------------
// options assembly (pure)
// ---------------------------------------------------------------------------

func TestDefaultOptions(t *testing.T) {
	o := defaultOptions()
	if o.endpoint != "localhost:4317" {
		t.Errorf("endpoint = %q", o.endpoint)
	}
	if o.serviceName != "go-wind-service" {
		t.Errorf("serviceName = %q", o.serviceName)
	}
	if o.serviceVersion != "v0.0.1" {
		t.Errorf("serviceVersion = %q", o.serviceVersion)
	}
	if o.insecure || o.useHTTP {
		t.Error("insecure/useHTTP should default to false")
	}
	if o.sampleRatio != 1.0 {
		t.Errorf("sampleRatio = %v, want 1.0", o.sampleRatio)
	}
	if o.batchTimeout != 5*time.Second {
		t.Errorf("batchTimeout = %v, want 5s", o.batchTimeout)
	}
	if o.exportTimeout != 30*time.Second {
		t.Errorf("exportTimeout = %v, want 30s", o.exportTimeout)
	}
	if o.headers == nil {
		t.Error("headers should be initialized")
	}
}

func TestOptionConstructors(t *testing.T) {
	headers := map[string]string{"x-tenant": "acme"}

	o := defaultOptions()
	for _, opt := range []Option{
		WithEndpoint("collector:4318"),
		WithServiceName("svc"),
		WithServiceVersion("v9.9.9"),
		WithInsecure(true),
		WithHTTP(true),
		WithSampleRatio(0.5),
		WithBatchTimeout(time.Second),
		WithExportTimeout(2 * time.Second),
		WithHeaders(headers),
	} {
		opt(o)
	}

	if o.endpoint != "collector:4318" {
		t.Errorf("endpoint = %q", o.endpoint)
	}
	if o.serviceName != "svc" {
		t.Errorf("serviceName = %q", o.serviceName)
	}
	if o.serviceVersion != "v9.9.9" {
		t.Errorf("serviceVersion = %q", o.serviceVersion)
	}
	if !o.insecure {
		t.Error("insecure should be true")
	}
	if !o.useHTTP {
		t.Error("useHTTP should be true")
	}
	if o.sampleRatio != 0.5 {
		t.Errorf("sampleRatio = %v, want 0.5", o.sampleRatio)
	}
	if o.batchTimeout != time.Second {
		t.Errorf("batchTimeout = %v", o.batchTimeout)
	}
	if o.exportTimeout != 2*time.Second {
		t.Errorf("exportTimeout = %v", o.exportTimeout)
	}
	if o.headers["x-tenant"] != "acme" {
		t.Errorf("headers = %v", o.headers)
	}
}

// ---------------------------------------------------------------------------
// New: provider assembly for both transport protocols.
// The OTLP exporters are lazy — nothing is dialed and no export is triggered
// (no spans are created, no Shutdown that would flush is called).
// ---------------------------------------------------------------------------

func TestNew_GRPCExporter(t *testing.T) {
	tp, err := New(
		WithEndpoint("127.0.0.1:1"),
		WithInsecure(true),
		WithServiceName("grpc-svc"),
		WithBatchTimeout(time.Hour), // never flush during the test
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if tp == nil {
		t.Fatal("TracerProvider should not be nil")
	}
	// New installs itself as the global provider.
	if otel.GetTracerProvider() == nil {
		t.Error("global tracer provider should be set")
	}
	// The global propagator becomes the W3C trace-context composite.
	carrier := propagation.MapCarrier{}
	otel.GetTextMapPropagator().Inject(
		trace.ContextWithSpanContext(context.Background(), makeSpanContext()),
		carrier,
	)
	if carrier.Get("traceparent") == "" {
		t.Error("global propagator should inject W3C traceparent")
	}
}

func TestNew_HTTPExporter(t *testing.T) {
	tp, err := New(
		WithEndpoint("127.0.0.1:1"),
		WithHTTP(true),
		WithInsecure(true),
		WithHeaders(map[string]string{"x-tenant": "acme"}),
		WithBatchTimeout(time.Hour),
		WithExportTimeout(time.Second),
		WithSampleRatio(0),
		WithServiceVersion("v2"),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if tp == nil {
		t.Fatal("TracerProvider should not be nil")
	}
}

// ---------------------------------------------------------------------------
// Tracer behavior with a real (in-memory) SDK provider
// ---------------------------------------------------------------------------

func makeSpanContext() trace.SpanContext {
	tid, _ := trace.TraceIDFromHex("4bf92f3577b34da6a3ce929d0e0e4736")
	sid, _ := trace.SpanIDFromHex("00f067aa0ba902b7")
	return trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    tid,
		SpanID:     sid,
		TraceFlags: trace.FlagsSampled,
	})
}

// newRecordingTracer builds a Tracer over an in-memory SDK provider whose
// ended spans land in the returned recorder.
func newRecordingTracer(t *testing.T, kind trace.SpanKind, opts ...TracerOption) (*Tracer, *tracetest.SpanRecorder) {
	t.Helper()
	rec := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(rec))
	all := append([]TracerOption{WithTracerProvider(tp)}, opts...)
	return NewTracer(kind, "op", all...), rec
}

func TestTracer_ServerStartExtractsTraceContext(t *testing.T) {
	tr, _ := newRecordingTracer(t, trace.SpanKindServer)

	carrier := propagation.MapCarrier{}
	parent := makeSpanContext()
	propagation.TraceContext{}.Inject(trace.ContextWithSpanContext(context.Background(), parent), carrier)

	_, span := tr.Start(context.Background(), carrier)
	defer span.End()

	got := span.SpanContext()
	if got.TraceID() != parent.TraceID() {
		t.Errorf("span TraceID = %s, want extracted %s", got.TraceID(), parent.TraceID())
	}
	if got.SpanID() == parent.SpanID() {
		t.Error("span should be a child, not the parent itself")
	}
	if got.IsRemote() {
		t.Error("the new span itself must not be marked remote")
	}
}

func TestTracer_ClientStartInjectsTraceContext(t *testing.T) {
	tr, _ := newRecordingTracer(t, trace.SpanKindClient)

	carrier := propagation.MapCarrier{}
	ctx, span := tr.Start(context.Background(), carrier)
	defer span.End()

	extracted := trace.SpanContextFromContext(
		propagation.TraceContext{}.Extract(context.Background(), carrier),
	)
	if !extracted.IsValid() {
		t.Fatal("client Start should inject a valid traceparent into the carrier")
	}
	if extracted.TraceID() != span.SpanContext().TraceID() {
		t.Errorf("injected TraceID = %s, want %s", extracted.TraceID(), span.SpanContext().TraceID())
	}
	_ = ctx
}

func TestTracer_EndSetsErrorStatus(t *testing.T) {
	tr, rec := newRecordingTracer(t, trace.SpanKindServer)

	_, span := tr.Start(context.Background(), propagation.MapCarrier{})
	tr.End(context.Background(), span, errors.New("boom"), attribute.String("k", "v"))

	ended := rec.Ended()
	if len(ended) != 1 {
		t.Fatalf("ended spans = %d, want 1", len(ended))
	}
	st := ended[0].Status()
	if st.Code != codes.Error {
		t.Errorf("status code = %v, want Error", st.Code)
	}
	if st.Description != "boom" {
		t.Errorf("status description = %q, want boom", st.Description)
	}
}

func TestTracer_EndWithoutError(t *testing.T) {
	tr, rec := newRecordingTracer(t, trace.SpanKindServer)

	_, span := tr.Start(context.Background(), propagation.MapCarrier{})
	tr.End(context.Background(), span, nil)

	ended := rec.Ended()
	if len(ended) != 1 {
		t.Fatalf("ended spans = %d, want 1", len(ended))
	}
	if code := ended[0].Status().Code; code != codes.Unset {
		t.Errorf("status code = %v, want Unset", code)
	}
}

// End on a non-recording span must return early without panicking.
func TestTracer_EndNonRecordingSpan(t *testing.T) {
	tr := NewTracer(trace.SpanKindServer, "op", WithTracerProvider(trace.NewNoopTracerProvider()))

	_, span := tr.Start(context.Background(), propagation.MapCarrier{})
	tr.End(context.Background(), span, errors.New("ignored"))
}

func TestTracer_InjectUsesConfiguredPropagator(t *testing.T) {
	tr, _ := newRecordingTracer(t, trace.SpanKindClient)

	carrier := propagation.MapCarrier{}
	tr.Inject(trace.ContextWithSpanContext(context.Background(), makeSpanContext()), carrier)
	if carrier.Get("traceparent") == "" {
		t.Error("Inject should write traceparent via the configured propagator")
	}
}

func TestTracer_UnsupportedKindPanics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("NewTracer with an unsupported span kind should panic")
		}
	}()
	NewTracer(trace.SpanKindInternal, "op")
}

func TestTracer_GlobalProviderOption(t *testing.T) {
	// Install a known global provider, then build a tracer bound to it.
	otel.SetTracerProvider(trace.NewNoopTracerProvider())

	tr := NewTracer(trace.SpanKindServer, "op", WithGlobalTracerProvider())
	if tr.opt.tracerProvider == nil {
		t.Fatal("WithGlobalTracerProvider should bind otel.GetTracerProvider()")
	}
	if tr.tracer == nil {
		t.Error("tracer should be created from the bound provider")
	}
	if tr.opt.spanName != "op" {
		t.Errorf("spanName = %q", tr.opt.spanName)
	}
}
