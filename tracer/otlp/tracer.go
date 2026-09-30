package otlp

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

const defaultTracerName = "go-wind"

// Tracer 封装了 OpenTelemetry tracer，提供 span 生命周期管理和 propagation。
type Tracer struct {
	tracer trace.Tracer
	opt    *tracerOptions
}

// tracerOptions 是 Tracer 的内部配置。
type tracerOptions struct {
	propagator     propagation.TextMapPropagator
	tracerProvider trace.TracerProvider
	kind           trace.SpanKind
	tracerName     string
	spanName       string
}

// TracerOption 是 Tracer 的配置选项。
type TracerOption func(*tracerOptions)

// WithTracerName 设置 tracer 名称。
func WithTracerName(name string) TracerOption {
	return func(o *tracerOptions) {
		o.tracerName = name
	}
}

// WithPropagator 设置 propagation 传播器。
func WithPropagator(p propagation.TextMapPropagator) TracerOption {
	return func(o *tracerOptions) {
		o.propagator = p
	}
}

// WithTracerProvider 设置自定义 TracerProvider。
func WithTracerProvider(provider trace.TracerProvider) TracerOption {
	return func(o *tracerOptions) {
		o.tracerProvider = provider
	}
}

// WithGlobalTracerProvider 绑定全局 TracerProvider。
func WithGlobalTracerProvider() TracerOption {
	return func(o *tracerOptions) {
		o.tracerProvider = otel.GetTracerProvider()
	}
}

// NewTracer 创建一个 Tracer 实例。
//
// kind 是 span 类型（Server/Client/Producer/Consumer），
// spanName 是 span 的默认名称。
func NewTracer(kind trace.SpanKind, spanName string, opts ...TracerOption) *Tracer {
	o := tracerOptions{
		propagator: propagation.NewCompositeTextMapPropagator(
			propagation.Baggage{},
			propagation.TraceContext{},
		),
		kind:       kind,
		tracerName: defaultTracerName,
	}
	for _, opt := range opts {
		opt(&o)
	}

	// 优先使用注入的 provider 创建 tracer；
	// 全局 provider 的设置只作为兼容行为保留（旧版本依赖它让 otel.Tracer 生效）。
	var tracer trace.Tracer
	if o.tracerProvider != nil {
		otel.SetTracerProvider(o.tracerProvider)
		tracer = o.tracerProvider.Tracer(o.tracerName)
	} else {
		tracer = otel.Tracer(o.tracerName)
	}
	o.spanName = spanName

	switch kind {
	case trace.SpanKindProducer, trace.SpanKindConsumer,
		trace.SpanKindServer, trace.SpanKindClient:
		return &Tracer{tracer: tracer, opt: &o}
	default:
		panic(fmt.Sprintf("unsupported span kind: %v", kind))
	}
}

// Start 创建并启动一个新 span。
//
// 对于 Server/Consumer 类型的 span，会从 carrier 中提取 trace context；
// 对于 Client/Producer 类型的 span，会将 trace context 注入到 carrier 中。
func (t *Tracer) Start(ctx context.Context, carrier propagation.TextMapCarrier, attrs ...attribute.KeyValue) (context.Context, trace.Span) {
	if t.opt.kind == trace.SpanKindServer || t.opt.kind == trace.SpanKindConsumer {
		ctx = t.opt.propagator.Extract(ctx, carrier)
	}

	opts := []trace.SpanStartOption{
		trace.WithAttributes(attrs...),
		trace.WithSpanKind(t.opt.kind),
	}

	ctx, span := t.tracer.Start(ctx, t.opt.spanName, opts...)

	if t.opt.kind == trace.SpanKindClient || t.opt.kind == trace.SpanKindProducer {
		t.opt.propagator.Inject(ctx, carrier)
	}

	return ctx, span
}

// End 结束 span。
//
// 如果 err 不为 nil，设置 span 状态为 Error。
func (t *Tracer) End(_ context.Context, span trace.Span, err error, attrs ...attribute.KeyValue) {
	if span == nil {
		return
	}
	if !span.IsRecording() {
		return
	}

	span.SetAttributes(attrs...)

	if err != nil {
		span.SetStatus(codes.Error, err.Error())
	}

	span.End()
}

// Inject 将 trace context 注入到 carrier 中。
func (t *Tracer) Inject(ctx context.Context, carrier propagation.TextMapCarrier) {
	t.opt.propagator.Inject(ctx, carrier)
}
