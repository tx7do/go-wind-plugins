package pprof

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandler_IndexPage(t *testing.T) {
	h := NewHandler()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/debug/pprof/", nil)
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "goroutine") {
		t.Fatalf("expected index page to list profiles, got: %s", body)
	}
}

func TestHandler_HeapProfile(t *testing.T) {
	h := NewHandler()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/debug/pprof/heap", nil)
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestHandler_GoroutineProfile(t *testing.T) {
	h := NewHandler()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/debug/pprof/goroutine", nil)
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestHandler_Cmdline(t *testing.T) {
	h := NewHandler()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/debug/pprof/cmdline", nil)
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestHandler_Symbol(t *testing.T) {
	h := NewHandler()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/debug/pprof/symbol", nil)
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestHandler_Trace(t *testing.T) {
	h := NewHandler()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/debug/pprof/trace?seconds=0", nil)
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestHandler_AllocsProfile(t *testing.T) {
	h := NewHandler()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/debug/pprof/allocs", nil)
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestHandler_BlockProfile(t *testing.T) {
	h := NewHandler()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/debug/pprof/block", nil)
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestHandler_MutexProfile(t *testing.T) {
	h := NewHandler()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/debug/pprof/mutex", nil)
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestHandler_ThreadcreateProfile(t *testing.T) {
	h := NewHandler()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/debug/pprof/threadcreate", nil)
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestHandler_CPProfile(t *testing.T) {
	// pprof.Profile defaults to a 30-second CPU profile when seconds=0,
	// which is too slow for unit tests. Verify the route is registered by
	// checking that it does not return 404 (use a very short 1-second profile).
	h := NewHandler()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/debug/pprof/profile?seconds=1", nil)
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestHandler_CustomPrefix(t *testing.T) {
	h := NewHandler(WithPrefix("/internal/pprof"))

	// Should work under custom prefix.
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/internal/pprof/heap", nil)
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 with custom prefix, got %d", rec.Code)
	}

	// Default prefix should NOT work.
	rec2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodGet, "/debug/pprof/heap", nil)
	h.ServeHTTP(rec2, req2)

	if rec2.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for default prefix when custom prefix set, got %d", rec2.Code)
	}
}

func TestHandler_NotFound(t *testing.T) {
	h := NewHandler()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/debug/pprof/nonexistent", nil)
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestHandler_DefaultPrefix(t *testing.T) {
	// Verify default prefix constant matches expected value.
	if DefaultPrefix != "/debug/pprof" {
		t.Fatalf("expected '/debug/pprof', got %q", DefaultPrefix)
	}
}

func TestHandler_ServesUnderServer(t *testing.T) {
	h := NewHandler()
	mux := http.NewServeMux()
	mux.Handle("/debug/pprof/", h)

	server := httptest.NewServer(mux)
	defer server.Close()

	resp, err := http.Get(server.URL + "/debug/pprof/")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if !strings.Contains(string(body), "goroutine") {
		t.Fatal("expected index to list goroutine profile")
	}
}

func TestHandler_Routes(t *testing.T) {
	h := NewHandler()
	routes := h.Routes("/debug/pprof")

	expected := []string{
		"/debug/pprof/",
		"/debug/pprof/cmdline",
		"/debug/pprof/profile",
		"/debug/pprof/symbol",
		"/debug/pprof/trace",
		"/debug/pprof/allocs",
		"/debug/pprof/block",
		"/debug/pprof/goroutine",
		"/debug/pprof/heap",
		"/debug/pprof/mutex",
		"/debug/pprof/threadcreate",
	}

	if len(routes) != len(expected) {
		t.Fatalf("expected %d routes, got %d", len(expected), len(routes))
	}

	routeSet := make(map[string]bool, len(routes))
	for _, r := range routes {
		routeSet[r] = true
	}
	for _, e := range expected {
		if !routeSet[e] {
			t.Fatalf("expected route %q not found", e)
		}
	}
}
