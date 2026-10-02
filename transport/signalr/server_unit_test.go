package signalr

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	_ "github.com/tx7do/go-wind-plugins/encoding/json" // register the JSON codec used by WithCodec
)

// ---------------------------------------------------------------------------
// Options
// ---------------------------------------------------------------------------

func TestNewServerDefaults(t *testing.T) {
	srv := NewServer()

	if srv.network != "tcp" {
		t.Errorf("default network = %q, want %q", srv.network, "tcp")
	}
	if srv.address != ":0" {
		t.Errorf("default address = %q, want %q", srv.address, ":0")
	}
	if srv.keepAliveInterval != 2*time.Second {
		t.Errorf("default keepAliveInterval = %v, want %v", srv.keepAliveInterval, 2*time.Second)
	}
	if srv.chanReceiveTimeout != 200*time.Millisecond {
		t.Errorf("default chanReceiveTimeout = %v, want %v", srv.chanReceiveTimeout, 200*time.Millisecond)
	}
	if srv.streamBufferCapacity != 5 {
		t.Errorf("default streamBufferCapacity = %d, want 5", srv.streamBufferCapacity)
	}
	if srv.debug {
		t.Error("default debug = true, want false")
	}
	if srv.router == nil {
		t.Error("default router is nil, want an initialized ServeMux")
	}
	if srv.hub != nil {
		t.Error("default hub is not nil, want nil")
	}
	// Without WithHub the underlying library refuses to build a hub factory,
	// so init() logs the error and leaves the embedded Server nil. That is
	// documented behavior; TestNewServerWithHub asserts the populated path.
}

func TestNewServerWithHub(t *testing.T) {
	srv := NewServer(WithHub(&chat{}))
	if srv.Server == nil {
		t.Error("embedded signalr.Server is nil after NewServer with a hub")
	}
	// The codec field is only populated through WithCodec; there is no
	// package-level default (TestServerOptions covers the populated path).
}

func TestServerOptions(t *testing.T) {
	hub := &chat{}
	tlsConf := &tls.Config{}

	srv := NewServer(
		WithNetwork("tcp4"),
		WithAddress("127.0.0.1:9100"),
		WithTLSConfig(tlsConf),
		WithCodec("json"),
		WithKeepAliveInterval(7*time.Second),
		WithChanReceiveTimeout(3*time.Second),
		WithStreamBufferCapacity(11),
		WithDebug(true),
		WithHub(hub),
		WithAllowedOrigins([]string{"https://example.com"}, true),
	)

	if srv.network != "tcp4" {
		t.Errorf("network = %q, want %q", srv.network, "tcp4")
	}
	if srv.address != "127.0.0.1:9100" {
		t.Errorf("address = %q, want %q", srv.address, "127.0.0.1:9100")
	}
	if srv.tlsConf != tlsConf {
		t.Error("tlsConf was not applied")
	}
	if srv.codec == nil {
		t.Error("codec is nil after WithCodec")
	}
	if srv.keepAliveInterval != 7*time.Second {
		t.Errorf("keepAliveInterval = %v, want %v", srv.keepAliveInterval, 7*time.Second)
	}
	if srv.chanReceiveTimeout != 3*time.Second {
		t.Errorf("chanReceiveTimeout = %v, want %v", srv.chanReceiveTimeout, 3*time.Second)
	}
	if srv.streamBufferCapacity != 11 {
		t.Errorf("streamBufferCapacity = %d, want 11", srv.streamBufferCapacity)
	}
	if !srv.debug {
		t.Error("debug = false, want true")
	}
	if srv.hub != hub {
		t.Error("hub was not applied")
	}
	if len(srv.allowedOrigins) != 1 || srv.allowedOrigins[0] != "https://example.com" {
		t.Errorf("allowedOrigins = %v, want [https://example.com]", srv.allowedOrigins)
	}
	if !srv.allowCredentials {
		t.Error("allowCredentials = false, want true")
	}
}

// ---------------------------------------------------------------------------
// CORS middleware
// ---------------------------------------------------------------------------

// doCORSRequest drives one request through the CORS middleware and returns
// the recorder so callers can assert on headers and status.
func doCORSRequest(t *testing.T, srv *Server, method, origin string) *httptest.ResponseRecorder {
	t.Helper()

	nextCalled := false
	handler := srv.CORS(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(method, "http://example.com/chat", nil)
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if method != http.MethodOptions && !nextCalled {
		t.Error("next handler was not called for a non-OPTIONS request")
	}
	return rec
}

func TestCORS(t *testing.T) {
	tests := []struct {
		name             string
		origins          []string
		allowCredentials bool
		method           string
		origin           string
		wantAllowed      bool
		wantCredentials  bool
	}{
		{
			name:        "no whitelist: origin not allowed",
			method:      http.MethodGet,
			origin:      "https://example.com",
			wantAllowed: false,
		},
		{
			name:        "no whitelist: no origin header",
			method:      http.MethodGet,
			wantAllowed: false,
		},
		{
			name:        "exact match allows origin",
			origins:     []string{"https://example.com"},
			method:      http.MethodGet,
			origin:      "https://example.com",
			wantAllowed: true,
		},
		{
			name:        "match is case-insensitive",
			origins:     []string{"https://EXAMPLE.com"},
			method:      http.MethodGet,
			origin:      "https://example.COM",
			wantAllowed: true,
		},
		{
			name:        "non-matching whitelist entry",
			origins:     []string{"https://other.com"},
			method:      http.MethodGet,
			origin:      "https://example.com",
			wantAllowed: false,
		},
		{
			name:        "wildcard allows without credentials",
			origins:     []string{"*"},
			method:      http.MethodGet,
			origin:      "https://anything.com",
			wantAllowed: true,
		},
		{
			name:             "wildcard with credentials denies (security)",
			origins:          []string{"*"},
			allowCredentials: true,
			method:           http.MethodGet,
			origin:           "https://anything.com",
			wantAllowed:      false,
		},
		{
			name:             "exact match with credentials allowed",
			origins:          []string{"https://example.com"},
			allowCredentials: true,
			method:           http.MethodGet,
			origin:           "https://example.com",
			wantAllowed:      true,
			wantCredentials:  true,
		},
		{
			name:        "preflight allowed",
			origins:     []string{"https://example.com"},
			method:      http.MethodOptions,
			origin:      "https://example.com",
			wantAllowed: true,
		},
		{
			name:        "preflight denied",
			origins:     []string{"https://example.com"},
			method:      http.MethodOptions,
			origin:      "https://evil.com",
			wantAllowed: false,
		},
		{
			name:        "preflight with wildcard",
			origins:     []string{"*"},
			method:      http.MethodOptions,
			origin:      "https://anything.com",
			wantAllowed: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv := NewServer(WithAllowedOrigins(tc.origins, tc.allowCredentials))

			// The CORS layer must not call next for OPTIONS preflight
			// requests; doCORSRequest only checks next for non-OPTIONS.
			if tc.method == http.MethodOptions {
				handler := srv.CORS(nil)
				req := httptest.NewRequest(http.MethodOptions, "http://example.com/chat", nil)
				if tc.origin != "" {
					req.Header.Set("Origin", tc.origin)
				}
				rec := httptest.NewRecorder()
				handler.ServeHTTP(rec, req)

				if rec.Code != http.StatusOK {
					t.Errorf("OPTIONS status = %d, want %d", rec.Code, http.StatusOK)
				}
				got := rec.Header().Get(corsAllowOriginHeader)
				if tc.wantAllowed && got != tc.origin {
					t.Errorf("Allow-Origin = %q, want %q", got, tc.origin)
				}
				if !tc.wantAllowed && got != "" {
					t.Errorf("Allow-Origin = %q, want empty for denied preflight", got)
				}
				if tc.wantAllowed {
					if v := rec.Header().Get(corsAllowMethodsHeader); v == "" {
						t.Error("Allow-Methods header missing on allowed preflight")
					}
					if v := rec.Header().Get(corsMaxAgeHeader); v == "" {
						t.Error("Max-Age header missing on allowed preflight")
					}
					if v := rec.Header().Get(corsVaryHeader); v != "Origin" {
						t.Errorf("Vary = %q, want Origin", v)
					}
					gotCred := rec.Header().Get(corsAllowCredentialsHeader)
					if tc.wantCredentials && gotCred != "true" {
						t.Errorf("Allow-Credentials = %q, want true", gotCred)
					}
					if !tc.wantCredentials && gotCred != "" {
						t.Errorf("Allow-Credentials = %q, want empty", gotCred)
					}
				}
				return
			}

			rec := doCORSRequest(t, srv, tc.method, tc.origin)

			got := rec.Header().Get(corsAllowOriginHeader)
			if tc.wantAllowed && got != tc.origin {
				t.Errorf("Allow-Origin = %q, want %q", got, tc.origin)
			}
			if !tc.wantAllowed && got != "" {
				t.Errorf("Allow-Origin = %q, want empty for denied origin", got)
			}
			gotCred := rec.Header().Get(corsAllowCredentialsHeader)
			if tc.wantCredentials && gotCred != "true" {
				t.Errorf("Allow-Credentials = %q, want true", gotCred)
			}
			if !tc.wantCredentials && gotCred != "" {
				t.Errorf("Allow-Credentials = %q, want empty", gotCred)
			}
			if tc.wantAllowed {
				if v := rec.Header().Get(corsExposeHeadersHeader); v != corsAllowOriginHeader {
					t.Errorf("Expose-Headers = %q, want %q", v, corsAllowOriginHeader)
				}
			}
		})
	}
}

func TestAllowedOrigin(t *testing.T) {
	tests := []struct {
		name             string
		origins          []string
		allowCredentials bool
		origin           string
		want             bool
	}{
		{"empty whitelist", nil, false, "https://example.com", false},
		{"empty whitelist with wildcard absent", []string{}, false, "*", false},
		{"match", []string{"https://a.com", "https://b.com"}, false, "https://b.com", true},
		{"case-insensitive match", []string{"https://A.com"}, false, "https://a.com", true},
		{"no match", []string{"https://a.com"}, false, "https://c.com", false},
		{"wildcard no credentials", []string{"*"}, false, "https://x.com", true},
		{"wildcard with credentials", []string{"*"}, true, "https://x.com", false},
		{"empty origin no whitelist", []string{"https://a.com"}, false, "", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv := &Server{allowedOrigins: tc.origins, allowCredentials: tc.allowCredentials}
			if got := srv.allowedOrigin(tc.origin); got != tc.want {
				t.Errorf("allowedOrigin(%q) = %v, want %v", tc.origin, got, tc.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Endpoint / Use / MapHTTP
// ---------------------------------------------------------------------------

func TestEndpointWithoutListener(t *testing.T) {
	tests := []struct {
		name    string
		address string
		want    string
	}{
		{"host and port", "127.0.0.1:9100", "signalr://127.0.0.1:9100"},
		{"empty host becomes localhost", ":9100", "signalr://localhost:9100"},
		{"0.0.0.0 becomes localhost", "0.0.0.0:9100", "signalr://localhost:9100"},
		{"no port falls back to raw address", "localhost", "signalr://localhost"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv := NewServer(WithAddress(tc.address))
			if got := srv.Endpoint(); got != tc.want {
				t.Errorf("Endpoint() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestUseRegistersMiddlewares(t *testing.T) {
	srv := NewServer()
	if len(srv.middlewares) != 0 {
		t.Fatalf("middlewares = %d, want 0 before Use", len(srv.middlewares))
	}

	srv.Use(func(next http.Handler) http.Handler { return next })
	srv.Use(
		func(next http.Handler) http.Handler { return next },
		func(next http.Handler) http.Handler { return next },
	)

	if len(srv.middlewares) != 3 {
		t.Errorf("middlewares = %d, want 3 after Use", len(srv.middlewares))
	}
}

// TestUseMiddlewareChain verifies that Use-registered middleware wraps the
// router in registration order (first registered is outermost).
func TestUseMiddlewareChain(t *testing.T) {
	srv := NewServer()
	var order []string

	srv.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			order = append(order, "outer")
			next.ServeHTTP(w, r)
		})
	})
	srv.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			order = append(order, "inner")
			next.ServeHTTP(w, r)
		})
	})

	// Rebuild the same chain Start() would build and drive a request through it.
	handler := srv.CORS(srv.router)
	for i := len(srv.middlewares) - 1; i >= 0; i-- {
		handler = srv.middlewares[i](handler)
	}

	req := httptest.NewRequest(http.MethodGet, "http://example.com/anything", nil)
	handler.ServeHTTP(httptest.NewRecorder(), req)

	if len(order) != 2 || order[0] != "outer" || order[1] != "inner" {
		t.Errorf("middleware order = %v, want [outer inner]", order)
	}
}

// TestMapHTTPNegotiate maps the hub onto an HTTP path and exercises the
// SignalR negotiate handshake through the router directly, without a listener.
func TestMapHTTPNegotiate(t *testing.T) {
	srv := NewServer(WithHub(&chat{}))
	srv.MapHTTP("/chat")

	req := httptest.NewRequest(http.MethodPost, "http://example.com/chat/negotiate?negotiateVersion=1", nil)
	rec := httptest.NewRecorder()
	srv.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("negotiate status = %d, want %d; body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("negotiate response is not JSON: %v; body: %s", err, rec.Body.String())
	}
	if payload["connectionId"] == nil && payload["connectionToken"] == nil {
		t.Errorf("negotiate response missing connectionId/connectionToken: %v", payload)
	}
}

// ---------------------------------------------------------------------------
// Lifecycle (hermetic, bound to 127.0.0.1:0)
// ---------------------------------------------------------------------------

// TestStartStopLifecycle starts the server on a random local port, verifies
// the negotiate endpoint over a real HTTP round trip, then stops it. All
// waits are bounded by waitForServerReady and client timeouts.
func TestStartStopLifecycle(t *testing.T) {
	srv := NewServer(
		WithAddress("127.0.0.1:0"),
		WithHub(&chat{}),
	)
	srv.MapHTTP("/chat")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() { done <- srv.Start(ctx) }()

	// Wait until the listener is bound: Endpoint() reports the resolved
	// address once Start has assigned it.
	deadline := time.Now().Add(5 * time.Second)
	endpoint := ""
	for time.Now().Before(deadline) {
		endpoint = srv.Endpoint()
		if endpoint != "signalr://127.0.0.1:0" && strings.Contains(endpoint, ":") {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if endpoint == "" || strings.HasSuffix(endpoint, ":0") {
		t.Fatalf("server did not report a bound endpoint within 5s, last: %q", endpoint)
	}
	httpEndpoint := "http://" + strings.TrimPrefix(endpoint, KindSignalR+"://")

	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Post(httpEndpoint+"/chat/negotiate?negotiateVersion=1", "", nil)
	if err != nil {
		t.Fatalf("negotiate request failed: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("negotiate status = %d, want %d; body: %s", resp.StatusCode, http.StatusOK, body)
	}
	if !strings.Contains(string(body), "connectionId") && !strings.Contains(string(body), "connectionToken") {
		t.Errorf("negotiate body missing connection identifier: %s", body)
	}

	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Start returned error after cancel: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Start did not return within 5s after context cancellation")
	}
}

// TestStopWithoutListener verifies Stop on a server that was never started
// closes nothing and reports no error.
func TestStopWithoutListener(t *testing.T) {
	srv := NewServer()
	if err := srv.Stop(context.Background()); err != nil {
		t.Errorf("Stop on unstarted server returned error: %v", err)
	}
}
