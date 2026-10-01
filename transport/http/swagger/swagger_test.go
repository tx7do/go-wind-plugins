package swagger_test

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
	"github.com/tx7do/go-wind-plugins/transport/http/swagger"
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
	// Swagger only uses HandlePrefix; Handle registrations are appended as
	// exact-path prefixes for completeness.
	m.prefixes = append(m.prefixes, prefixEntry{prefix: path, handler: handler})
}

func (m *mockDriver) HandlePrefix(prefix string, h http.Handler) {
	m.prefixes = append(m.prefixes, prefixEntry{prefix: prefix, handler: h})
}

func (m *mockDriver) Start(ctx context.Context, ln net.Listener) error {
	return nil
}

func (m *mockDriver) Stop(ctx context.Context) error { return nil }

// serve dispatches a request to the longest registered matching prefix,
// mirroring how a real router mounts prefix handlers.
func (m *mockDriver) serve(req *http.Request) *httptest.ResponseRecorder {
	var best *prefixEntry
	for i := range m.prefixes {
		p := m.prefixes[i].prefix
		if strings.HasPrefix(req.URL.Path, strings.TrimSuffix(p, "/")) || strings.HasPrefix(req.URL.Path, p) {
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
	cfg := swagger.NewConfig()
	if cfg.BasePath != "/docs/" {
		t.Errorf("BasePath = %q, want /docs/", cfg.BasePath)
	}
}

func TestHandlerOptions(t *testing.T) {
	keys := map[string]string{"api_key": "secret"}
	settings := map[string]string{"displayOperationId": "true"}

	cfg := swagger.NewConfig()
	opts := []swagger.HandlerOption{
		swagger.WithTitle("My API"),
		swagger.WithBasePath("/api-docs"),
		swagger.WithShowTopBar(true),
		swagger.WithHideCurl(true),
		swagger.WithJsonEditor(true),
		swagger.WithPreAuthorizeApiKey(keys),
		swagger.WithSettingsUI(settings),
		swagger.WithLocalFile("/tmp/openapi.json"),
		swagger.WithMemoryData([]byte(`{"openapi":"3.0.0"}`), "json"),
		swagger.WithRemoteFileURL("https://example.com/openapi.json"),
	}
	for _, o := range opts {
		o(cfg)
	}

	if cfg.Title != "My API" {
		t.Errorf("Title = %q", cfg.Title)
	}
	if cfg.BasePath != "/api-docs" {
		t.Errorf("BasePath = %q", cfg.BasePath)
	}
	if !cfg.ShowTopBar || !cfg.HideCurl || !cfg.JsonEditor {
		t.Error("boolean options should be set")
	}
	if cfg.PreAuthorizeApiKey["api_key"] != "secret" {
		t.Errorf("PreAuthorizeApiKey = %v", cfg.PreAuthorizeApiKey)
	}
	if cfg.SettingsUI["displayOperationId"] != "true" {
		t.Errorf("SettingsUI = %v", cfg.SettingsUI)
	}
	if cfg.LocalOpenApiFile != "/tmp/openapi.json" {
		t.Errorf("LocalOpenApiFile = %q", cfg.LocalOpenApiFile)
	}
	if string(cfg.OpenApiData) != `{"openapi":"3.0.0"}` || cfg.OpenApiDataType != "json" {
		t.Errorf("OpenApiData/Type = %q/%q", cfg.OpenApiData, cfg.OpenApiDataType)
	}
	if cfg.SwaggerJsonUrl != "https://example.com/openapi.json" {
		t.Errorf("SwaggerJsonUrl = %q", cfg.SwaggerJsonUrl)
	}
}

// ---------------------------------------------------------------------------
// Handler rendering (httptest, no network)
// ---------------------------------------------------------------------------

func TestNew_ServesHTML(t *testing.T) {
	h := swagger.New("Petstore API", "https://petstore.example/openapi.json", "/docs/")

	req := httptest.NewRequest(http.MethodGet, "/docs/", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "text/html" {
		t.Errorf("Content-Type = %q, want text/html", ct)
	}
	body := rec.Body.String()
	for _, marker := range []string{
		"Petstore API",                          // title rendered into the page
		"https://petstore.example/openapi.json", // spec URL wired into the config
		"/docs/",                                // base path used for asset URLs
		"SwaggerUIBundle",                       // swagger-ui bootstrap script
	} {
		if !strings.Contains(body, marker) {
			t.Errorf("HTML should contain %q", marker)
		}
	}
}

// The base path is matched with trailing slash trimmed, so "/docs" (no slash)
// must render the index page too.
func TestNew_MatchesBasePathWithoutTrailingSlash(t *testing.T) {
	h := swagger.New("T", "spec.json", "/docs/")

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/docs", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "SwaggerUIBundle") {
		t.Error("expected the index page for /docs")
	}
}

// BasePath without a trailing slash is normalized to "/x/" at construction.
func TestNewWithOption_NormalizesBasePath(t *testing.T) {
	h := swagger.NewWithOption(
		swagger.WithTitle("Normalized"),
		swagger.WithBasePath("/api-docs"),
	)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api-docs/", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "/api-docs/") {
		t.Error("rendered page should reference the normalized base path")
	}
}

// With the default options the embedded swagger-ui assets are served under
// the base path (gzipped on disk, transparently decompressed by statigz).
func TestNewWithOption_ServesEmbeddedAssets(t *testing.T) {
	h := swagger.NewWithOption(swagger.WithTitle("Assets"))

	req := httptest.NewRequest(http.MethodGet, "/docs/swagger-ui-bundle.js", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("asset status = %d, want 200", rec.Code)
	}
	if rec.Body.Len() == 0 {
		t.Error("asset body should not be empty")
	}

	// Unknown asset paths fall through to the static file server (404),
	// proving sub-paths are not answered with the index page.
	rec2 := httptest.NewRecorder()
	h.ServeHTTP(rec2, httptest.NewRequest(http.MethodGet, "/docs/no-such-file.js", nil))
	if rec2.Code != http.StatusNotFound {
		t.Errorf("unknown asset status = %d, want 404", rec2.Code)
	}
	if strings.Contains(rec2.Body.String(), "SwaggerUIBundle") {
		t.Error("unknown asset path must not be answered with the index page")
	}
}

// A handler built without a static server answers every path with the page.
func TestNewHandlerWithConfig_NilStaticServer(t *testing.T) {
	h := swagger.NewHandlerWithConfig(&swagger.Config{
		Title:          "Bare",
		SwaggerJsonUrl: "spec.json",
		BasePath:       "/docs/",
	}, "{{ .BasePath }}", "{{ .BasePath }}", nil)

	for _, path := range []string{"/docs/", "/docs/anything-at-all"} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != http.StatusOK {
			t.Errorf("GET %s status = %d, want 200", path, rec.Code)
		}
		if !strings.Contains(rec.Body.String(), "SwaggerUIBundle") {
			t.Errorf("GET %s should render the index page", path)
		}
	}
}

// ---------------------------------------------------------------------------
// Register: mounting onto a Server, per data source
// ---------------------------------------------------------------------------

func TestRegister_RemoteURL(t *testing.T) {
	srv, drv := newTestServer(t)
	h := swagger.Register(srv,
		swagger.WithTitle("Remote"),
		swagger.WithRemoteFileURL("https://petstore.example/openapi.json"),
		swagger.WithBasePath("/docs/"),
	)
	if h == nil {
		t.Fatal("Register should return the created handler")
	}

	// Exactly one prefix registration: the UI itself (no spec sub-route).
	if len(drv.prefixes) != 1 || drv.prefixes[0].prefix != "/docs/" {
		t.Fatalf("registered prefixes = %+v, want only /docs/", drv.prefixes)
	}
	rec := drv.serve(httptest.NewRequest(http.MethodGet, "/docs/", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "SwaggerUIBundle") {
		t.Errorf("GET /docs/ status=%d body=%q", rec.Code, rec.Body.String())
	}
}

func TestRegister_MemoryData(t *testing.T) {
	srv, drv := newTestServer(t)
	spec := `{"openapi":"3.0.0","info":{"title":"Mem"}}`
	swagger.Register(srv,
		swagger.WithTitle("Mem"),
		swagger.WithMemoryData([]byte(spec), "json"),
		swagger.WithBasePath("/docs/"),
	)

	if len(drv.prefixes) != 2 {
		t.Fatalf("registered prefixes = %+v, want 2 (UI + spec)", drv.prefixes)
	}

	// The spec is served at <base>/openapi.<ext> with the in-memory content.
	rec := drv.serve(httptest.NewRequest(http.MethodGet, "/docs/openapi.json", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("spec status = %d, want 200", rec.Code)
	}
	if rec.Body.String() != spec {
		t.Errorf("spec body = %q, want %q", rec.Body.String(), spec)
	}

	// The UI page points the frontend at the hosted spec URL.
	page := drv.serve(httptest.NewRequest(http.MethodGet, "/docs/", nil))
	if !strings.Contains(page.Body.String(), "/docs/openapi.json") {
		t.Error("index page should reference the hosted spec URL")
	}
}

func TestRegister_LocalFile(t *testing.T) {
	dir := t.TempDir()
	specPath := filepath.Join(dir, "openapi.yaml")
	spec := "openapi: 3.0.0\ninfo:\n  title: FromFile\n"
	if err := os.WriteFile(specPath, []byte(spec), 0o600); err != nil {
		t.Fatalf("write spec: %v", err)
	}

	srv, drv := newTestServer(t)
	swagger.Register(srv,
		swagger.WithTitle("FromFile"),
		swagger.WithLocalFile(specPath),
		swagger.WithBasePath("/docs/"),
	)

	if len(drv.prefixes) != 2 {
		t.Fatalf("registered prefixes = %+v, want 2 (UI + spec)", drv.prefixes)
	}
	// The pattern derives the extension from the file name.
	rec := drv.serve(httptest.NewRequest(http.MethodGet, "/docs/openapi.yaml", nil))
	if rec.Code != http.StatusOK || rec.Body.String() != spec {
		t.Errorf("spec: status=%d body=%q", rec.Code, rec.Body.String())
	}
}

func TestRegister_LocalFile_MissingFileStillMountsUI(t *testing.T) {
	srv, drv := newTestServer(t)
	swagger.Register(srv,
		swagger.WithTitle("Broken"),
		swagger.WithLocalFile(filepath.Join(t.TempDir(), "nope.json")),
		swagger.WithBasePath("/docs/"),
	)

	// Load failure only skips the spec sub-route; the UI itself is mounted.
	if len(drv.prefixes) != 1 || drv.prefixes[0].prefix != "/docs/" {
		t.Fatalf("registered prefixes = %+v, want only /docs/", drv.prefixes)
	}
	rec := drv.serve(httptest.NewRequest(http.MethodGet, "/docs/", nil))
	if rec.Code != http.StatusOK {
		t.Errorf("UI status = %d, want 200", rec.Code)
	}
}

func TestRegister_DefaultBasePath(t *testing.T) {
	srv, drv := newTestServer(t)
	swagger.Register(srv, swagger.WithRemoteFileURL("https://example.com/openapi.json"))

	if len(drv.prefixes) != 1 || drv.prefixes[0].prefix != "/docs/" {
		t.Fatalf("default base path should be /docs/, got %+v", drv.prefixes)
	}
}
