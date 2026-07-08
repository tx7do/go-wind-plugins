package logging

import (
	"context"
	"testing"

	"github.com/tx7do/go-wind/log"
	"google.golang.org/grpc"
)

// enabledLogger 恒为 true 的空实现 logger，使 benchmark 测 interceptor
// + args 装箱开销，不被真实日志后端拖慢。
type enabledLogger struct{}

func (enabledLogger) Debug(context.Context, string, ...any) {}
func (enabledLogger) Info(context.Context, string, ...any)  {}
func (enabledLogger) Warn(context.Context, string, ...any)  {}
func (enabledLogger) Error(context.Context, string, ...any) {}
func (enabledLogger) Enabled(log.Level) bool                 { return true }
func (l enabledLogger) With(...any) log.Logger               { return l }

// noopLogger 是 enabledLogger 的别名，语义更清晰。
type noopLogger = enabledLogger

// BenchmarkUnaryInterceptor_Enabled 测日志启用时的完整路径（含 args 装箱）。
func BenchmarkUnaryInterceptor_Enabled(b *testing.B) {
	interceptor := UnaryInterceptor(WithLogger(enabledLogger{}))
	info := &grpc.UnaryServerInfo{FullMethod: "/pkg.Svc/GetUser"}
	handler := func(context.Context, any) (any, error) { return "ok", nil }

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = interceptor(context.Background(), nil, info, handler)
	}
}

// BenchmarkUnaryInterceptor_Disabled 测日志级别被过滤时的路径
// （应跳过 args 装箱，显著减少分配）。
func BenchmarkUnaryInterceptor_Disabled(b *testing.B) {
	interceptor := UnaryInterceptor(WithLogger(disabledLogger{}))
	info := &grpc.UnaryServerInfo{FullMethod: "/pkg.Svc/GetUser"}
	handler := func(context.Context, any) (any, error) { return "ok", nil }

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = interceptor(context.Background(), nil, info, handler)
	}
}
