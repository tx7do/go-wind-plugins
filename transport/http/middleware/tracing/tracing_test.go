package tracing

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

// setupNoopOTel resets the global OTel state to noop providers so the
// middleware does not panic when no real TracerProvider is configured.
func setupNoopOTel() {
	otel.SetTracerProvider(trace.NewNoopTracerProvider())
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator())
}

// ---------------------------------------------------------------------------
// Middleware – handler delegation
// ---------------------------------------------------------------------------

func TestMiddleware_CallsNextHandler(t *testing.T) {
	setupNoopOTel()

	called := false
	handler := Middleware()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if !called {
		t.Error("expected next handler to be called")
	}
	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}
}

// ---------------------------------------------------------------------------
// Middleware – status code capture
// ---------------------------------------------------------------------------

func TestMiddleware_CapturesStatusCodes(t *testing.T) {
	setupNoopOTel()

	tests := []struct {
		name       string
		statusCode int
	}{
		{"OK", http.StatusOK},
		{"MovedPermanently", http.StatusMovedPermanently},
		{"BadRequest", http.StatusBadRequest},
		{"NotFound", http.StatusNotFound},
		{"InternalServerError", http.StatusInternalServerError},
		{"BadGateway", http.StatusBadGateway},
		{"ServiceUnavailable", http.StatusServiceUnavailable},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := Middleware()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.statusCode)
			}))

			req := httptest.NewRequest("GET", "/test", nil)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			if rec.Code != tt.statusCode {
				t.Errorf("expected status %d, got %d", tt.statusCode, rec.Code)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Middleware – options
// ---------------------------------------------------------------------------

func TestMiddleware_WithCustomTracer(t *testing.T) {
	setupNoopOTel()

	tp := trace.NewNoopTracerProvider()
	customTracer := tp.Tracer("test-tracer")

	handler := Middleware(WithTracer(customTracer))(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}
}

func TestMiddleware_WithCustomPropagators(t *testing.T) {
	setupNoopOTel()

	prop := propagation.NewCompositeTextMapPropagator(propagation.TraceContext{})
	handler := Middleware(WithPropagators(prop))(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}
}

// ---------------------------------------------------------------------------
// Middleware – HTTP methods & paths
// ---------------------------------------------------------------------------

func TestMiddleware_DifferentMethods(t *testing.T) {
	setupNoopOTel()

	methods := []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodPatch}

	for _, method := range methods {
		t.Run(method, func(t *testing.T) {
			handler := Middleware()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != method {
					t.Errorf("expected method %s, got %s", method, r.Method)
				}
				w.WriteHeader(http.StatusOK)
			}))

			req := httptest.NewRequest(method, "/api/resource", nil)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Errorf("expected status 200, got %d", rec.Code)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// statusRecorder
// ---------------------------------------------------------------------------

func TestStatusRecorder_CapturesStatus(t *testing.T) {
	rec := httptest.NewRecorder()
	sr := &statusRecorder{ResponseWriter: rec, status: http.StatusOK}

	sr.WriteHeader(http.StatusCreated)

	if sr.status != http.StatusCreated {
		t.Errorf("expected captured status 201, got %d", sr.status)
	}
	if rec.Code != http.StatusCreated {
		t.Errorf("expected underlying recorder status 201, got %d", rec.Code)
	}
}

func TestStatusRecorder_DefaultStatus(t *testing.T) {
	rec := httptest.NewRecorder()
	sr := &statusRecorder{ResponseWriter: rec, status: http.StatusOK}

	// Calling Write without WriteHeader should keep the default status.
	_, _ = sr.Write([]byte("hello"))

	if sr.status != http.StatusOK {
		t.Errorf("expected default status 200, got %d", sr.status)
	}
}

// ---------------------------------------------------------------------------
// requestScheme
// ---------------------------------------------------------------------------

func TestRequestScheme_HTTP(t *testing.T) {
	req := httptest.NewRequest("GET", "http://example.com/test", nil)
	if scheme := requestScheme(req); scheme != "http" {
		t.Errorf("expected http, got %s", scheme)
	}
}

func TestRequestScheme_HTTPS(t *testing.T) {
	req := httptest.NewRequest("GET", "https://example.com/test", nil)
	req.TLS = &tls.ConnectionState{} // non-nil triggers the https branch

	if scheme := requestScheme(req); scheme != "https" {
		t.Errorf("expected https, got %s", scheme)
	}
}
