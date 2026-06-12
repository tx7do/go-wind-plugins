package thrift

import (
	"context"
	"fmt"
	"runtime/debug"
	"time"

	"github.com/apache/thrift/lib/go/thrift"

	"github.com/tx7do/go-wind/log"

	"github.com/tx7do/go-wind-plugins/metrics"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// ---------------------------------------------------------------------------
// ProcessorWrapper is a function that wraps a TProcessor with additional
// cross-cutting behavior (tracing, logging, recovery, metrics, etc.).
//
// Wrappers are applied in reverse order so that the first wrapper in the list
// is the outermost one (first to execute on the way in, last on the way out).
// ---------------------------------------------------------------------------

// ProcessorWrapper wraps a [thrift.TProcessor] with additional behavior.
type ProcessorWrapper func(thrift.TProcessor) thrift.TProcessor

// ---------------------------------------------------------------------------
// Tracing
// ---------------------------------------------------------------------------

// tracingProcessor wraps [thrift.TProcessor] to create an OTel span for each RPC call.
type tracingProcessor struct {
	thrift.TProcessor
	tracer trace.Tracer
}

func (p *tracingProcessor) Process(ctx context.Context, in, out thrift.TProtocol) (bool, thrift.TException) {
	ctx, span := p.tracer.Start(ctx, "thrift.rpc",
		trace.WithSpanKind(trace.SpanKindServer),
		trace.WithAttributes(
			attribute.String("rpc.system", "thrift"),
		),
	)
	defer span.End()

	ok, err := p.TProcessor.Process(ctx, in, out)
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
	} else if !ok {
		span.SetStatus(codes.Error, "processing failed")
	}
	return ok, err
}

// TracingWrapper returns a [ProcessorWrapper] that creates an OTel span for
// each RPC invocation.
func TracingWrapper(tracer trace.Tracer) ProcessorWrapper {
	return func(next thrift.TProcessor) thrift.TProcessor {
		return &tracingProcessor{TProcessor: next, tracer: tracer}
	}
}

// ---------------------------------------------------------------------------
// Logging
// ---------------------------------------------------------------------------

// loggingProcessor wraps [thrift.TProcessor] to log each RPC call's duration and result.
type loggingProcessor struct {
	thrift.TProcessor
	logger log.Logger
}

func (p *loggingProcessor) Process(ctx context.Context, in, out thrift.TProtocol) (bool, thrift.TException) {
	start := time.Now()
	ok, err := p.TProcessor.Process(ctx, in, out)
	latency := time.Since(start)

	args := []any{
		"duration_ms", latency.Milliseconds(),
		"ok", ok,
	}
	if err != nil {
		args = append(args, "error", err.Error())
		p.logger.Error(ctx, "thrift rpc", args...)
	} else {
		p.logger.Info(ctx, "thrift rpc", args...)
	}
	return ok, err
}

// LoggingWrapper returns a [ProcessorWrapper] that logs each RPC call.
// If logger is nil, it uses the global logger from [log.GetLogger].
func LoggingWrapper(logger log.Logger) ProcessorWrapper {
	if logger == nil {
		logger = log.GetLogger()
	}
	return func(next thrift.TProcessor) thrift.TProcessor {
		return &loggingProcessor{TProcessor: next, logger: logger}
	}
}

// ---------------------------------------------------------------------------
// Recovery
// ---------------------------------------------------------------------------

// recoveryProcessor wraps [thrift.TProcessor] to recover from panics in RPC handlers,
// log the panic with a stack trace, and return a TApplicationException to the caller.
type recoveryProcessor struct {
	thrift.TProcessor
	logger log.Logger
}

func (p *recoveryProcessor) Process(ctx context.Context, in, out thrift.TProtocol) (_ bool, retErr thrift.TException) {
	defer func() {
		if r := recover(); r != nil {
			p.logger.Error(ctx, "thrift rpc panic recovered",
				"error", fmt.Sprint(r),
				"stack", string(debug.Stack()),
			)
			retErr = thrift.NewTApplicationException(
				thrift.INTERNAL_ERROR,
				fmt.Sprintf("panic: %v", r),
			)
		}
	}()
	return p.TProcessor.Process(ctx, in, out)
}

// RecoveryWrapper returns a [ProcessorWrapper] that recovers from panics in
// RPC handlers, preventing the entire server from crashing.
// If logger is nil, it uses the global logger from [log.GetLogger].
func RecoveryWrapper(logger log.Logger) ProcessorWrapper {
	if logger == nil {
		logger = log.GetLogger()
	}
	return func(next thrift.TProcessor) thrift.TProcessor {
		return &recoveryProcessor{TProcessor: next, logger: logger}
	}
}

// ---------------------------------------------------------------------------
// Metrics
// ---------------------------------------------------------------------------

// metricsProcessor wraps [thrift.TProcessor] to record RPC call metrics
// (request count and latency histogram).
type metricsProcessor struct {
	thrift.TProcessor
	m metrics.Metrics
}

func (p *metricsProcessor) Process(ctx context.Context, in, out thrift.TProtocol) (bool, thrift.TException) {
	start := time.Now()
	ok, err := p.TProcessor.Process(ctx, in, out)
	latency := time.Since(start)

	labels := map[string]string{
		"rpc.system": "thrift",
	}
	if err != nil {
		labels["status"] = "error"
	} else if !ok {
		labels["status"] = "failed"
	} else {
		labels["status"] = "ok"
	}

	p.m.Counter(ctx, "thrift_rpc_calls_total", 1, labels)
	p.m.Histogram(ctx, "thrift_rpc_duration_ms", float64(latency.Milliseconds()), labels)

	return ok, err
}

// MetricsWrapper returns a [ProcessorWrapper] that records RPC call count and
// latency histogram via the [metrics.Metrics] interface.
func MetricsWrapper(m metrics.Metrics) ProcessorWrapper {
	return func(next thrift.TProcessor) thrift.TProcessor {
		return &metricsProcessor{TProcessor: next, m: m}
	}
}
