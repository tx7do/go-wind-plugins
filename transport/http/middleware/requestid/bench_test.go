package requestid

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// BenchmarkMiddleware_GenID 单独测 ID 生成开销（无 HTTP 往返）。
func BenchmarkMiddleware_GenID(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = defaultIDGenerator()
	}
}

// BenchmarkMiddleware_FullRequest 测完整中间件路径（含 context 传值 + header 设置）。
func BenchmarkMiddleware_FullRequest(b *testing.B) {
	mw := Middleware()
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		h.ServeHTTP(rec, req)
	}
}
