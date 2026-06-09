package validate

import (
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

// ---------------------------------------------------------------------------
// Middleware — pass-through (no rules configured)
// ---------------------------------------------------------------------------

func TestMiddleware_NoRules_PassesThrough(t *testing.T) {
	mw := Middleware()
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}

// ---------------------------------------------------------------------------
// WithMaxBodySize — Content-Length pre-check
// ---------------------------------------------------------------------------

func TestMiddleware_MaxBodySize_ContentLengthExceeds(t *testing.T) {
	mw := Middleware(WithMaxBodySize(100))
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Error("handler should not be called for oversized body")
	}))

	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(make([]byte, 200)))
	req.ContentLength = 200
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusRequestEntityTooLarge)
	}
}

func TestMiddleware_MaxBodySize_ContentLengthOK(t *testing.T) {
	mw := Middleware(WithMaxBodySize(1024))
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader([]byte("hello")))
	req.ContentLength = 5
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}

// ---------------------------------------------------------------------------
// WithAllowedContentTypes
// ---------------------------------------------------------------------------

func TestMiddleware_AllowedContentTypes_RejectsUnknown(t *testing.T) {
	mw := Middleware(WithAllowedContentTypes("application/json"))
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Error("handler should not be called for unsupported content type")
	}))

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Header.Set("Content-Type", "text/xml")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnsupportedMediaType {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusUnsupportedMediaType)
	}
}

func TestMiddleware_AllowedContentTypes_AcceptsKnown(t *testing.T) {
	mw := Middleware(WithAllowedContentTypes("application/json"))
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestMiddleware_AllowedContentTypes_StripsParameters(t *testing.T) {
	mw := Middleware(WithAllowedContentTypes("application/json"))
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d (charset should be stripped)", rec.Code, http.StatusOK)
	}
}

func TestMiddleware_AllowedContentTypes_NoContentType(t *testing.T) {
	mw := Middleware(WithAllowedContentTypes("application/json"))
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// No Content-Type header → should pass through (not all requests have one)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d (no content-type should be allowed)", rec.Code, http.StatusOK)
	}
}

// ---------------------------------------------------------------------------
// WithRequiredHeaders
// ---------------------------------------------------------------------------

func TestMiddleware_RequiredHeaders_Missing(t *testing.T) {
	mw := Middleware(WithRequiredHeaders("X-Request-ID"))
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Error("handler should not be called when required header is missing")
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestMiddleware_RequiredHeaders_Present(t *testing.T) {
	mw := Middleware(WithRequiredHeaders("X-Request-ID"))
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Request-ID", "abc-123")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestMiddleware_RequiredHeaders_CaseInsensitive(t *testing.T) {
	// Go's http.Header.Get uses canonical header keys, so case should match.
	mw := Middleware(WithRequiredHeaders("X-Request-ID"))
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("x-request-id", "abc") // lowercase — Go normalizes
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d (header matching is case-insensitive)", rec.Code, http.StatusOK)
	}
}

// ---------------------------------------------------------------------------
// WithValidator
// ---------------------------------------------------------------------------

func TestMiddleware_Validator_Rejects(t *testing.T) {
	mw := Middleware(WithValidator(func(r *http.Request) error {
		if r.URL.Query().Get("tenant") == "" {
			return errors.New("tenant parameter is required")
		}
		return nil
	}))
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Error("handler should not be called when validator fails")
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
	if rec.Body.String() == "" {
		t.Error("error message should be in response body")
	}
}

func TestMiddleware_Validator_Accepts(t *testing.T) {
	mw := Middleware(WithValidator(func(r *http.Request) error {
		if r.URL.Query().Get("tenant") == "" {
			return errors.New("tenant parameter is required")
		}
		return nil
	}))
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/?tenant=acme", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}

// ---------------------------------------------------------------------------
// Combined rules
// ---------------------------------------------------------------------------

func TestMiddleware_CombinedRules(t *testing.T) {
	mw := Middleware(
		WithMaxBodySize(1024),
		WithAllowedContentTypes("application/json"),
		WithRequiredHeaders("X-Api-Key"),
		WithValidator(func(r *http.Request) error { return nil }),
	)
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	body := bytes.NewReader([]byte(`{"key":"value"}`))
	req := httptest.NewRequest(http.MethodPost, "/", body)
	req.ContentLength = int64(body.Len())
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Api-Key", "secret")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d with all rules satisfied", rec.Code, http.StatusOK)
	}
}

func TestMiddleware_CombinedRules_MissingHeader(t *testing.T) {
	mw := Middleware(
		WithMaxBodySize(1024),
		WithAllowedContentTypes("application/json"),
		WithRequiredHeaders("X-Api-Key"),
	)
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Error("handler should not be called")
	}))

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

// ---------------------------------------------------------------------------
// ErrBodyTooLarge
// ---------------------------------------------------------------------------

func TestErrBodyTooLarge(t *testing.T) {
	if ErrBodyTooLarge == nil {
		t.Error("ErrBodyTooLarge should not be nil")
	}
	if ErrBodyTooLarge.Error() == "" {
		t.Error("ErrBodyTooLarge should have a non-empty message")
	}
}
