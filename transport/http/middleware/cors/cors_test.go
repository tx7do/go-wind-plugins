package cors

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMiddleware_Preflight(t *testing.T) {
	mw := Middleware(
		WithAllowedOrigins("https://app.example.com"),
		WithAllowedMethods("GET", "POST"),
		WithAllowedHeaders("Authorization", "Content-Type"),
		WithAllowCredentials(true),
		WithMaxAge(3600),
	)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodOptions, "/api/data", nil)
	req.Header.Set("Origin", "https://app.example.com")
	req.Header.Set("Access-Control-Request-Method", "POST")
	req.Header.Set("Access-Control-Request-Headers", "Authorization")

	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler should not be called for preflight")
	}))
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", rec.Code)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "https://app.example.com" {
		t.Fatalf("expected ACAO header, got %q", got)
	}
	if got := rec.Header().Get("Access-Control-Allow-Credentials"); got != "true" {
		t.Fatalf("expected credentials header, got %q", got)
	}
	if got := rec.Header().Get("Access-Control-Max-Age"); got != "3600" {
		t.Fatalf("expected max-age 3600, got %q", got)
	}
}

func TestMiddleware_DisallowedOrigin(t *testing.T) {
	mw := Middleware(WithAllowedOrigins("https://app.example.com"))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/data", nil)
	req.Header.Set("Origin", "https://evil.example.com")

	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	h.ServeHTTP(rec, req)

	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("expected no ACAO for disallowed origin, got %q", got)
	}
}

func TestMiddleware_NoOriginHeader(t *testing.T) {
	mw := Middleware()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/data", nil)

	called := false
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))
	h.ServeHTTP(rec, req)

	if !called {
		t.Fatal("handler should be called for non-CORS request")
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("expected no CORS headers for non-CORS request, got %q", got)
	}
}

func TestMiddleware_AllOriginsAllowed(t *testing.T) {
	mw := Middleware()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/data", nil)
	req.Header.Set("Origin", "https://anything.example.com")

	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	h.ServeHTTP(rec, req)

	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "https://anything.example.com" {
		t.Fatalf("expected ACAO to echo origin, got %q", got)
	}
}

func TestMiddleware_ExposedHeaders(t *testing.T) {
	mw := Middleware(
		WithAllowedOrigins("https://app.example.com"),
		WithExposedHeaders("X-Request-ID", "X-Trace-ID"),
	)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/data", nil)
	req.Header.Set("Origin", "https://app.example.com")

	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	h.ServeHTTP(rec, req)

	if got := rec.Header().Get("Access-Control-Expose-Headers"); got == "" {
		t.Fatal("expected exposed headers to be set")
	}
}

func TestMiddleware_PostRequestPassesThrough(t *testing.T) {
	mw := Middleware(WithAllowedOrigins("https://app.example.com"))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/data", nil)
	req.Header.Set("Origin", "https://app.example.com")

	called := false
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusCreated)
	}))
	h.ServeHTTP(rec, req)

	if !called {
		t.Fatal("handler should be called for non-preflight CORS request")
	}
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", rec.Code)
	}
}
