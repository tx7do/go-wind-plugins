package thrift

import (
	"crypto/tls"

	"github.com/apache/thrift/lib/go/thrift"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"

	"github.com/tx7do/go-wind/log"

	"github.com/tx7do/go-wind-plugins/metrics"
)

// Option 是 Thrift 服务器的配置选项。
type Option func(*Server)

// WithTLSConfig 设置 TLS 配置，启用加密传输。
func WithTLSConfig(c *tls.Config) Option {
	return func(s *Server) { s.tlsConfig = c }
}

// WithProcessor 设置 Thrift 请求处理器（必需）。
// processor 由 thrift IDL 编译器生成的代码创建，例如：
//
//	processor := api.NewMyServiceProcessor(handler)
func WithProcessor(processor thrift.TProcessor) Option {
	return func(s *Server) { s.processor = processor }
}

// WithProtocol 设置 Thrift 协议类型。
// 支持的值："binary"（默认）, "compact", "json"。
func WithProtocol(protocol string) Option {
	return func(s *Server) { s.protocol = protocol }
}

// WithTransportConfig 配置传输层参数。
//   - buffered: 启用缓冲传输
//   - framed: 启用帧传输（非阻塞服务必需）
//   - bufferSize: 缓冲区大小（字节），默认 8192
func WithTransportConfig(buffered, framed bool, bufferSize int) Option {
	return func(s *Server) {
		s.buffered = buffered
		s.framed = framed
		s.bufferSize = bufferSize
	}
}

// ---------------------------------------------------------------------------
// Processor Wrappers (legacy convenience Options)
// ---------------------------------------------------------------------------

// WithTracer 设置 OpenTelemetry tracer，启用 RPC 级别的链路追踪。
// Deprecated: 使用 WithProcessorWrappers(TracingWrapper(t)) 代替。
func WithTracer(t trace.Tracer) Option {
	return func(s *Server) {
		s.wrappers = append(s.wrappers, TracingWrapper(t))
	}
}

// WithTracerProvider 从全局 TracerProvider 创建并设置 tracer。
// Deprecated: 使用 WithProcessorWrappers(TracingWrapper(...)) 代替。
func WithTracerProvider() Option {
	return func(s *Server) {
		tracer := otel.GetTracerProvider().Tracer("go-wind/plugins/thrift")
		s.wrappers = append(s.wrappers, TracingWrapper(tracer))
	}
}

// ---------------------------------------------------------------------------
// Processor Wrappers (new)
// ---------------------------------------------------------------------------

// WithProcessorWrappers adds one or more [ProcessorWrapper] functions to the
// server. Wrappers are applied in order: the first wrapper is outermost (runs
// first on the way in, last on the way out).
//
// Example:
//
//	srv := thrift.NewServer(":7700",
//	    thrift.WithProcessor(processor),
//	    thrift.WithProcessorWrappers(
//	        RecoveryWrapper(nil),
//	        LoggingWrapper(nil),
//	    ),
//	)
func WithProcessorWrappers(wrappers ...ProcessorWrapper) Option {
	return func(s *Server) {
		s.wrappers = append(s.wrappers, wrappers...)
	}
}

// WithLogging enables logging for each RPC call.
// If logger is nil, the global logger is used.
func WithLogging(logger log.Logger) Option {
	return func(s *Server) {
		s.wrappers = append(s.wrappers, LoggingWrapper(logger))
	}
}

// WithRecovery enables panic recovery for RPC handlers.
// If logger is nil, the global logger is used.
func WithRecovery(logger log.Logger) Option {
	return func(s *Server) {
		s.wrappers = append(s.wrappers, RecoveryWrapper(logger))
	}
}

// WithMetrics enables RPC call metrics (counter + latency histogram).
func WithMetrics(m metrics.Metrics) Option {
	return func(s *Server) {
		s.wrappers = append(s.wrappers, MetricsWrapper(m))
	}
}
