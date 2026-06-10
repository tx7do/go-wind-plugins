package requestid

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMiddleware_ExtractsExistingID(t *testing.T) {
	mw := Middleware()

	called := false
	var extractedID string
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		extractedID = FromContext(r.Context())
	}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set(DefaultHeaderName, "test-id-123")
	h.ServeHTTP(rec, req)

	if !called {
		t.Fatal("handler was not called")
	}
	if extractedID != "test-id-123" {
		t.Fatalf("expected extracted ID 'test-id-123', got %q", extractedID)
	}
	if got := rec.Header().Get(DefaultHeaderName); got != "test-id-123" {
		t.Fatalf("expected response header 'test-id-123', got %q", got)
	}
}

func TestMiddleware_GeneratesNewID(t *testing.T) {
	mw := Middleware()

	var generatedID string
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		generatedID = FromContext(r.Context())
	}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	h.ServeHTTP(rec, req)

	if generatedID == "" {
		t.Fatal("expected a generated ID, got empty string")
	}
	if len(generatedID) != 32 {
		t.Fatalf("expected 32-char hex ID, got %d chars: %q", len(generatedID), generatedID)
	}
	if got := rec.Header().Get(DefaultHeaderName); got != generatedID {
		t.Fatalf("response header should match context ID, got %q vs %q", got, generatedID)
	}
}

func TestMiddleware_CustomHeaderName(t *testing.T) {
	mw := Middleware(WithHeaderName("X-Correlation-ID"))

	var id string
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id = FromContext(r.Context())
	}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Correlation-ID", "corr-456")
	h.ServeHTTP(rec, req)

	if id != "corr-456" {
		t.Fatalf("expected 'corr-456', got %q", id)
	}
	if got := rec.Header().Get("X-Correlation-ID"); got != "corr-456" {
		t.Fatalf("expected custom header, got %q", got)
	}
}

func TestMiddleware_CustomGenerator(t *testing.T) {
	mw := Middleware(WithIDGenerator(func() string { return "fixed-id" }))

	var id string
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id = FromContext(r.Context())
	}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	h.ServeHTTP(rec, req)

	if id != "fixed-id" {
		t.Fatalf("expected 'fixed-id', got %q", id)
	}
}

func TestFromContext_Empty(t *testing.T) {
	id := FromContext(nil)
	if id != "" {
		t.Fatalf("expected empty string, got %q", id)
	}
}

func TestMiddleware_TwoRequestsDifferentIDs(t *testing.T) {
	mw := Middleware()

	ids := make(map[string]bool)
	for i := 0; i < 2; i++ {
		h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ids[FromContext(r.Context())] = true
		}))

		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		h.ServeHTTP(rec, req)
	}

	if len(ids) != 2 {
		t.Fatalf("expected 2 unique IDs, got %d", len(ids))
	}
}
