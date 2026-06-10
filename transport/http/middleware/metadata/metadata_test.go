package metadata

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMiddleware_ExtractsHeaders(t *testing.T) {
	mw := Middleware(WithKeys("X-Tenant-ID", "X-User-ID"))

	var tenantID, userID string
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tenantID = FromContext(r.Context(), "X-Tenant-ID")
		userID = FromContext(r.Context(), "X-User-ID")
	}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Tenant-ID", "acme")
	req.Header.Set("X-User-ID", "user-456")
	h.ServeHTTP(rec, req)

	if tenantID != "acme" {
		t.Fatalf("expected tenant 'acme', got %q", tenantID)
	}
	if userID != "user-456" {
		t.Fatalf("expected user 'user-456', got %q", userID)
	}
}

func TestMiddleware_MissingHeader(t *testing.T) {
	mw := Middleware(WithKeys("X-Tenant-ID"))

	var tenantID string
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tenantID = FromContext(r.Context(), "X-Tenant-ID")
	}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	h.ServeHTTP(rec, req)

	if tenantID != "" {
		t.Fatalf("expected empty for missing header, got %q", tenantID)
	}
}

func TestMiddleware_PartialHeaders(t *testing.T) {
	mw := Middleware(WithKeys("X-Tenant-ID", "X-User-ID"))

	var tenantID, userID string
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tenantID = FromContext(r.Context(), "X-Tenant-ID")
		userID = FromContext(r.Context(), "X-User-ID")
	}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Tenant-ID", "acme")
	h.ServeHTTP(rec, req)

	if tenantID != "acme" {
		t.Fatalf("expected 'acme', got %q", tenantID)
	}
	if userID != "" {
		t.Fatalf("expected empty for missing header, got %q", userID)
	}
}

func TestMiddleware_NoKeys(t *testing.T) {
	mw := Middleware()

	called := false
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	h.ServeHTTP(rec, req)

	if !called {
		t.Fatal("handler should be called even with no keys configured")
	}
}

func TestMiddleware_HandlerNotAffected(t *testing.T) {
	mw := Middleware(WithKeys("X-Tenant-ID"))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Tenant-ID", "acme")

	statusCode := http.StatusOK
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		statusCode = http.StatusCreated
	}))
	h.ServeHTTP(rec, req)

	if statusCode != http.StatusCreated {
		t.Fatalf("expected handler to set status, got %d", statusCode)
	}
}

func TestFromContext_NilCtx(t *testing.T) {
	if v := FromContext(nil, "X-Key"); v != "" {
		t.Fatalf("expected empty, got %q", v)
	}
}

func TestFromContext_NotPresent(t *testing.T) {
	if v := FromContext(nil, "X-Key"); v != "" {
		t.Fatalf("expected empty, got %q", v)
	}
}
