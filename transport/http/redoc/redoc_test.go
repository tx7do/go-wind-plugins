package redoc_test

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	windhttp "github.com/tx7do/go-wind-plugins/transport/http"
	"github.com/tx7do/go-wind-plugins/transport/http/redoc"
)

// ---------------------------------------------------------------------------
// Mock driver: records prefix registrations and serves them like a real mux.
// ---------------------------------------------------------------------------

type mockDriver struct {
	prefixes []prefixEntry
}

type prefixEntry struct {
	prefix  string
	handler http.Handler
}

func (m *mockDriver) Handle(method, path string, handler http.HandlerFunc) {
	m.prefixes = append(m.prefixes, prefixEntry{prefix: path, handler: handler})
}

func (m *mockDriver) HandlePrefix(prefix string, h http.Handler) {
	m.prefixes = append(m.prefixes, prefixEntry{prefix: prefix, handler: h})
}

func (m *mockDriver) Start(ctx context.Context, ln net.Listener) error {
	return nil
}

func (m *mockDriver) Stop(ctx context.Context) error { return nil }

// serve dispatches a request to the longest registered matching prefix.
func (m *mockDriver) serve(req *http.Request) *httptest.ResponseRecorder {
	var best *prefixEntry
	for i := range m.prefixes {
		p := m.prefixes[i].prefix
		if strings.HasPrefix(req.URL.Path, strings.TrimSuffix(p, "/")) {
			if best == nil || len(p) >= len(best.prefix) {
				best = &m.prefixes[i]
			}
		}
	}
	rec := httptest.NewRecorder()
	if best == nil {
		rec.WriteHeader(http.StatusNotFound)
		return rec
	}
	best.handler.ServeHTTP(rec, req)
	return rec
}

func newTestServer(t *testing.T) (*windhttp.Server, *mockDriver) {
	t.Helper()
	drv := &mockDriver{}
	srv := windhttp.NewServer(":8080", windhttp.WithDriver(drv))
	return srv, drv
}

// ---------------------------------------------------------------------------
// Config and options
// ---------------------------------------------------------------------------

func TestNewConfig_Defaults(t *testing.T) {
	cfg := redoc.NewConfig()
	if cfg.BasePath != "/docs/" {
		t.Errorf("BasePath = %q, want /docs/", cfg.BasePath)
	}
}

func TestHandlerOptions(t *testing.T) {
	cfg := redoc.NewConfig()
	for _, o := range []redoc.HandlerOption{
		redoc.WithTitle("My API"),
		redoc.WithDescription("API docs"),
		redoc.WithBasePath("/api-docs"),
		redoc.WithLocalFile("/tmp/openapi.json"),
		redoc.WithRemoteFileURL("https://example.com/openapi.json"),
		redoc.WithSpecPath("/custom/spec.json"),
	} {
		o(cfg)
	}

	if cfg.Title != "My API" {
		t.Errorf("Title = %q", cfg.Title)
	}
	if cfg.Description != "API docs" {
		t.Errorf("Description = %q", cfg.Description)
	}
	if cfg.BasePath != "/api-docs" {
		t.Errorf("BasePath = %q", cfg.BasePath)
	}
	if cfg.SpecFile != "/tmp/openapi.json" {
		t.Errorf("SpecFile = %q", cfg.SpecFile)
	}
	if cfg.SpecURL != "https://example.com/openapi.json" {
		t.Errorf("SpecURL = %q", cfg.SpecURL)
	}
	if cfg.SpecPath != "/custom/spec.json" {
		t.Errorf("SpecPath = %q", cfg.SpecPath)
	}
}

// ---------------------------------------------------------------------------
// Register: remote URL mode
// ---------------------------------------------------------------------------

func TestRegister_RemoteURL(t *testing.T) {
	srv, drv := newTestServer(t)
	redoc.Register(srv,
		redoc.WithTitle("Remote API"),
		redoc.WithDescription("Docs via remote spec"),
		redoc.WithRemoteFileURL("https://petstore.example/openapi.json"),
		redoc.WithBasePath("/docs/"),
	)

	if len(drv.prefixes) != 1 || drv.prefixes[0].prefix != "/docs/" {
		t.Fatalf("registered prefixes = %+v, want only /docs/", drv.prefixes)
	}

	rec := drv.serve(httptest.NewRequest(http.MethodGet, "/docs/", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "text/html; charset=utf-8" {
		t.Errorf("Content-Type = %q, want text/html; charset=utf-8", ct)
	}
	body := rec.Body.String()
	for _, marker := range []string{
		"<title>Remote API - ReDoc</title>",
		`<meta name="description" content="Docs via remote spec">`,
		"Redoc.init(",
		"redoc-container",
	} {
		if !strings.Contains(body, marker) {
			t.Errorf("HTML should contain %q", marker)
		}
	}
	// The spec URL lands inside Redoc.init as a JS string, quoted exactly
	// once: the `json` helper emits template.JS (already escaped), so
	// html/template must not add a second level of quoting.
	if !strings.Contains(body, `"https://petstore.example/openapi.json"`) {
		t.Errorf("Redoc.init should embed the once-quoted spec URL, got: %s", body)
	}
	if strings.Contains(body, `\"https://petstore.example/openapi.json\"`) {
		t.Errorf("spec URL must not be double-encoded, got: %s", body)
	}
}

func TestRegister_RemoteURL_WithoutDescription(t *testing.T) {
	srv, drv := newTestServer(t)
	redoc.Register(srv,
		redoc.WithTitle("NoDesc"),
		redoc.WithRemoteFileURL("https://example.com/openapi.json"),
	)

	rec := drv.serve(httptest.NewRequest(http.MethodGet, "/docs/", nil))
	body := rec.Body.String()
	if strings.Contains(body, `meta name="description"`) {
		t.Error("description meta tag should be omitted when no description is set")
	}
	if !strings.Contains(body, "<title>NoDesc - ReDoc</title>") {
		t.Error("title should still render")
	}
}

// ---------------------------------------------------------------------------
// Register: local file mode (go-redoc native handler)
// ---------------------------------------------------------------------------

func writeSpec(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "openapi.json")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write spec: %v", err)
	}
	return path
}

func TestRegister_LocalFile_DefaultSpecPath(t *testing.T) {
	spec := `{"openapi":"3.0.0"}`
	srv, drv := newTestServer(t)
	redoc.Register(srv,
		redoc.WithTitle("Local API"),
		redoc.WithLocalFile(writeSpec(t, spec)),
		redoc.WithBasePath("/docs/"),
	)

	if len(drv.prefixes) != 1 || drv.prefixes[0].prefix != "/docs/" {
		t.Fatalf("registered prefixes = %+v, want only /docs/", drv.prefixes)
	}

	// Spec served as JSON at the derived path <base>/openapi.json.
	specRec := drv.serve(httptest.NewRequest(http.MethodGet, "/docs/openapi.json", nil))
	if specRec.Code != http.StatusOK {
		t.Fatalf("spec status = %d, want 200", specRec.Code)
	}
	if ct := specRec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("spec Content-Type = %q, want application/json", ct)
	}
	if specRec.Body.String() != spec {
		t.Errorf("spec body = %q, want %q", specRec.Body.String(), spec)
	}

	// Every other path under the prefix renders the HTML page.
	pageRec := drv.serve(httptest.NewRequest(http.MethodGet, "/docs/", nil))
	if pageRec.Code != http.StatusOK {
		t.Fatalf("page status = %d, want 200", pageRec.Code)
	}
	if ct := pageRec.Header().Get("Content-Type"); ct != "text/html" {
		t.Errorf("page Content-Type = %q, want text/html", ct)
	}
	body := pageRec.Body.String()
	for _, marker := range []string{
		"Local API",
		`"/docs/openapi.json"`, // spec path wired into the page
		"redoc",
	} {
		if !strings.Contains(body, marker) {
			t.Errorf("HTML should contain %q", marker)
		}
	}

	// HEAD works for both the spec and the page.
	headRec := drv.serve(httptest.NewRequest(http.MethodHead, "/docs/openapi.json", nil))
	if headRec.Code != http.StatusOK {
		t.Errorf("HEAD spec status = %d, want 200", headRec.Code)
	}

	// Non-GET/HEAD methods are answered with an empty 200 (go-redoc behavior).
	postRec := drv.serve(httptest.NewRequest(http.MethodPost, "/docs/", nil))
	if postRec.Code != http.StatusOK || postRec.Body.Len() != 0 {
		t.Errorf("POST page: status=%d body=%q, want empty 200", postRec.Code, postRec.Body.String())
	}
}

func TestRegister_LocalFile_CustomSpecPath(t *testing.T) {
	spec := `{"openapi":"3.0.0"}`
	srv, drv := newTestServer(t)
	redoc.Register(srv,
		redoc.WithTitle("Custom"),
		redoc.WithLocalFile(writeSpec(t, spec)),
		redoc.WithSpecPath("/docs/api-spec.json"),
	)

	got := drv.serve(httptest.NewRequest(http.MethodGet, "/docs/api-spec.json", nil))
	if got.Code != http.StatusOK || got.Body.String() != spec {
		t.Errorf("custom spec path: status=%d body=%q", got.Code, got.Body.String())
	}

	// The default path no longer serves the spec (falls through to HTML).
	def := drv.serve(httptest.NewRequest(http.MethodGet, "/docs/openapi.json", nil))
	if strings.Contains(def.Body.String(), spec) {
		t.Error("default spec path should not serve the document anymore")
	}
}

func TestRegister_DefaultBasePath(t *testing.T) {
	srv, drv := newTestServer(t)
	redoc.Register(srv, redoc.WithRemoteFileURL("https://example.com/openapi.json"))

	if len(drv.prefixes) != 1 || drv.prefixes[0].prefix != "/docs/" {
		t.Fatalf("default base path should be /docs/, got %+v", drv.prefixes)
	}
}
