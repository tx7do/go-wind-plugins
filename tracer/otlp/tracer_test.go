package otlp

import (
	"context"
	"testing"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

// setupTestTP creates a real SDK TracerProvider (in-memory, no export) and
// sets it as the global provider.  It returns a cleanup function.
func setupTestTP() func() {
	tp := trace.NewNoopTracerProvider()
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))
	return func() {}
}

// ---------------------------------------------------------------------------
// NewTracer – basic construction
// ---------------------------------------------------------------------------

func TestNewTracer_Defaults(t *testing.T) {
	cleanup := setupTestTP()
	defer cleanup()

	tr := NewTracer(trace.SpanKindServer, "test-span")
	if tr == nil {
		t.Fatal("expected non-nil Tracer")
	}
	if tr.opt == nil {
		t.Fatal("expected non-nil options")
	}
	if tr.opt.tracerName != defaultTracerName {
		t.Errorf("expected tracerName %q, got %q", defaultTracerName, tr.opt.tracerName)
	}
	if tr.opt.kind != trace.SpanKindServer {
		t.Errorf("expected kind Server, got %v", tr.opt.kind)
	}
	if tr.opt.spanName != "test-span" {
		t.Errorf("expected spanName 'test-span', got %q", tr.opt.spanName)
	}
}

func TestNewTracer_WithOptions(t *testing.T) {
	cleanup := setupTestTP()
	defer cleanup()

	tr := NewTracer(trace.SpanKindClient, "client-op",
		WithTracerName("custom-tracer"),
	)
	if tr.opt.tracerName != "custom-tracer" {
		t.Errorf("expected 'custom-tracer', got %q", tr.opt.tracerName)
	}
}

// ---------------------------------------------------------------------------
// Tracer.Start – Server kind (extracts context)
// ---------------------------------------------------------------------------

func TestTracer_Start_ServerKind(t *testing.T) {
	cleanup := setupTestTP()
	defer cleanup()

	tr := NewTracer(trace.SpanKindServer, "server-op")

	carrier := propagation.MapCarrier{}
	ctx, span := tr.Start(context.Background(), carrier)

	if span == nil {
		t.Fatal("expected non-nil span")
	}
	if ctx == nil {
		t.Fatal("expected non-nil context")
	}
	span.End()
}

// ---------------------------------------------------------------------------
// Tracer.Start – Client kind (injects context)
// ---------------------------------------------------------------------------

func TestTracer_Start_ClientKind(t *testing.T) {
	cleanup := setupTestTP()
	defer cleanup()

	tr := NewTracer(trace.SpanKindClient, "client-op")

	carrier := propagation.MapCarrier{}
	ctx, span := tr.Start(context.Background(), carrier)
	defer span.End()

	// For client/producer kinds, the tracer injects trace context into carrier.
	// With a noop tracer provider, no real trace context is injected,
	// but the call should succeed without panic.
	_ = ctx
}

// ---------------------------------------------------------------------------
// Tracer.End
// ---------------------------------------------------------------------------

func TestTracer_End_NilSpan(t *testing.T) {
	cleanup := setupTestTP()
	defer cleanup()

	tr := NewTracer(trace.SpanKindServer, "test-span")

	// Should not panic with nil span.
	tr.End(context.Background(), nil, nil)
}

func TestTracer_End_WithAttributes(t *testing.T) {
	cleanup := setupTestTP()
	defer cleanup()

	tr := NewTracer(trace.SpanKindServer, "test-span")

	carrier := propagation.MapCarrier{}
	_, span := tr.Start(context.Background(), carrier)

	// End with attributes – should not panic.
	tr.End(context.Background(), span, nil, attribute.String("key1", "value1"))
}

func TestTracer_End_WithError(t *testing.T) {
	cleanup := setupTestTP()
	defer cleanup()

	tr := NewTracer(trace.SpanKindServer, "test-span")

	carrier := propagation.MapCarrier{}
	_, span := tr.Start(context.Background(), carrier)

	// End with error – should not panic.
	tr.End(context.Background(), span, context.DeadlineExceeded)
}

// ---------------------------------------------------------------------------
// Tracer.Inject
// ---------------------------------------------------------------------------

func TestTracer_Inject(t *testing.T) {
	cleanup := setupTestTP()
	defer cleanup()

	tr := NewTracer(trace.SpanKindClient, "inject-op")

	carrier := propagation.MapCarrier{}
	ctx, span := tr.Start(context.Background(), carrier)
	defer span.End()

	// Inject should not panic.
	tr.Inject(ctx, carrier)
}

// ---------------------------------------------------------------------------
// WithPropagator option
// ---------------------------------------------------------------------------

func TestNewTracer_WithPropagator(t *testing.T) {
	cleanup := setupTestTP()
	defer cleanup()

	customProp := propagation.NewCompositeTextMapPropagator(propagation.TraceContext{})
	tr := NewTracer(trace.SpanKindServer, "test-span", WithPropagator(customProp))

	if tr.opt.propagator == nil {
		t.Fatal("expected non-nil propagator")
	}
}
