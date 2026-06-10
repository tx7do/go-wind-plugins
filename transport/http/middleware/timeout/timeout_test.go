package timeout

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestMiddleware_CompletesBeforeTimeout(t *testing.T) {
	mw := Middleware(1 * time.Second)

	called := false
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	h.ServeHTTP(rec, req)

	if !called {
		t.Fatal("handler was not called")
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestMiddleware_TimesOut(t *testing.T) {
	mw := Middleware(50 * time.Millisecond)

	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
			return
		case <-time.After(2 * time.Second):
			t.Error("context should have been cancelled")
		}
	}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", rec.Code)
	}
}

func TestMiddleware_CustomStatusAndMessage(t *testing.T) {
	mw := Middleware(50*time.Millisecond,
		WithStatus(http.StatusGatewayTimeout),
		WithMessage("Request Timeout"),
	)

	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusGatewayTimeout {
		t.Fatalf("expected 504, got %d", rec.Code)
	}
	if rec.Body.String() != "Request Timeout\n" {
		t.Fatalf("expected 'Request Timeout', got %q", rec.Body.String())
	}
}

func TestMiddleware_SkipFunc(t *testing.T) {
	mw := Middleware(50*time.Millisecond,
		WithSkipFunc(func(r *http.Request) bool {
			return r.URL.Path == "/stream"
		}),
	)

	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/stream", nil)
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for skipped path, got %d", rec.Code)
	}
}

func TestMiddleware_PerRequestTimeoutFunc(t *testing.T) {
	mw := Middleware(1*time.Second,
		WithTimeoutFunc(func(r *http.Request) time.Duration {
			if r.URL.Path == "/fast" {
				return 50 * time.Millisecond
			}
			return 0 // use default
		}),
	)

	// /fast should time out quickly
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/fast", nil)
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 for /fast, got %d", rec.Code)
	}
}

func TestMiddleware_ContextCancelled(t *testing.T) {
	mw := Middleware(100 * time.Millisecond)

	handlerCtx := make(chan context.Context, 1)
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerCtx <- r.Context()
		select {
		case <-r.Context().Done():
			return
		case <-time.After(5 * time.Second):
		}
	}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	h.ServeHTTP(rec, req)

	select {
	case ctx := <-handlerCtx:
		if ctx.Err() == nil {
			t.Fatal("expected context to be cancelled")
		}
	default:
		t.Fatal("handler context not captured")
	}
}
