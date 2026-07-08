package metrics

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// BenchmarkMiddleware_FullRequest 测完整中间件路径：in-flight gauge + counter + histogram。
// 复用 metrics_test.go 中的 fakeMetrics（线程安全），benchmark 只测中间件自身开销。
func BenchmarkMiddleware_FullRequest(b *testing.B) {
	fm := newFakeMetrics()
	mw := Middleware(fm)
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/users", nil)
	rec := httptest.NewRecorder()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		h.ServeHTTP(rec, req)
	}
}
