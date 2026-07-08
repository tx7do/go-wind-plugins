package logging

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/tx7do/go-wind/log"
)

// benchmarkLogger 是一个 Enabled 恒为 true 的空实现 logger，
// 使 benchmark 测中间件 + 装箱开销，不被真实日志后端拖慢。
type benchmarkLogger struct{}

func (benchmarkLogger) Debug(context.Context, string, ...any) {}
func (benchmarkLogger) Info(context.Context, string, ...any)  {}
func (benchmarkLogger) Warn(context.Context, string, ...any)  {}
func (benchmarkLogger) Error(context.Context, string, ...any) {}
func (benchmarkLogger) Enabled(log.Level) bool                 { return true }
func (l benchmarkLogger) With(...any) log.Logger               { return l }

// disabledLogger 的 Enabled 恒为 false，测 level 过滤跳过装箱的路径。
type disabledLogger struct{ benchmarkLogger }

func (disabledLogger) Enabled(log.Level) bool { return false }

// BenchmarkMiddleware_FullRequest 测完整中间件路径（日志启用）。
func BenchmarkMiddleware_FullRequest(b *testing.B) {
	mw := Middleware(WithLogger(benchmarkLogger{}))
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/items", nil)
	rec := httptest.NewRecorder()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		h.ServeHTTP(rec, req)
	}
}

// BenchmarkMiddleware_LogDisabled 测日志级别被过滤时的路径
// （应跳过 args 装箱，显著减少分配）。
func BenchmarkMiddleware_LogDisabled(b *testing.B) {
	mw := Middleware(WithLogger(disabledLogger{}))
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/items", nil)
	rec := httptest.NewRecorder()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		h.ServeHTTP(rec, req)
	}
}
