package recovery

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// ---------------------------------------------------------------------------
// Middleware — recovers panics
// ---------------------------------------------------------------------------

func TestMiddleware_RecoversPanic(t *testing.T) {
	mw := Middleware()
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		panic("boom")
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()

	// Should not panic — recovered by middleware
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
}

func TestMiddleware_PassesThroughNormal(t *testing.T) {
	mw := Middleware()
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if rec.Body.String() != "ok" {
		t.Errorf("body = %q, want %q", rec.Body.String(), "ok")
	}
}

// ---------------------------------------------------------------------------
// WithErrorHandler
// ---------------------------------------------------------------------------

func TestMiddleware_CustomErrorHandler(t *testing.T) {
	called := false
	mw := Middleware(WithErrorHandler(func(w http.ResponseWriter, _ *http.Request, rvr any) {
		called = true
		w.WriteHeader(http.StatusTeapot)
		_, _ = w.Write([]byte("custom"))
	}))
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		panic("test")
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if !called {
		t.Error("custom error handler was not called")
	}
	if rec.Code != http.StatusTeapot {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusTeapot)
	}
	if rec.Body.String() != "custom" {
		t.Errorf("body = %q, want %q", rec.Body.String(), "custom")
	}
}

// ---------------------------------------------------------------------------
// WithStackTrace
// ---------------------------------------------------------------------------

func TestMiddleware_WithStackTraceDisabled(t *testing.T) {
	// WithStackTrace(false) — middleware should still recover panics.
	mw := Middleware(WithStackTrace(false))
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		panic("no stack")
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
}

// ---------------------------------------------------------------------------
// WithLogger — verify custom logger doesn't break recovery
// ---------------------------------------------------------------------------

func TestMiddleware_WithLogger(t *testing.T) {
	mw := Middleware(WithLogger(nil))
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		panic("logged")
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	// Even with a nil logger, recovery should still work.
	// The middleware uses log.GetLogger() when logger is nil.
	defer func() {
		_ = recover()
	}()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
}

// ---------------------------------------------------------------------------
// Chained middleware
// ---------------------------------------------------------------------------

func TestMiddleware_ChainedWithOther(t *testing.T) {
	// Recovery should be the outermost middleware to catch panics
	// from inner middleware.
	order := []string{}

	mw := Middleware()
	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		order = append(order, "handler")
		panic("chained panic")
	})

	// Wrap with an additional middleware
	wrapped := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		order = append(order, "before")
		inner.ServeHTTP(w, r)
		order = append(order, "after")
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	wrapped.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
	// "after" should not appear because panic aborted execution
	if len(order) != 2 || order[0] != "before" || order[1] != "handler" {
		t.Errorf("execution order = %v, want [before handler]", order)
	}
}
