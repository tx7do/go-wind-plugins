package socketio

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	socketio "github.com/googollee/go-socket.io"
	"github.com/gorilla/handlers"

	_ "github.com/tx7do/go-wind-plugins/encoding/json" // register the JSON codec used by WithCodec
)

// ---------------------------------------------------------------------------
// Construction and options
// ---------------------------------------------------------------------------

func TestNewServerDefaults(t *testing.T) {
	srv := NewServer()

	if srv.network != "tcp" {
		t.Errorf("default network = %q, want %q", srv.network, "tcp")
	}
	if srv.address != ":0" {
		t.Errorf("default address = %q, want %q", srv.address, ":0")
	}
	if srv.path != "/socket.io/" {
		t.Errorf("default path = %q, want %q", srv.path, "/socket.io/")
	}
	if srv.router == nil {
		t.Error("default router is nil, want an initialized mux.Router")
	}
	if srv.Server == nil {
		t.Error("embedded socket.io server is nil after NewServer")
	}
	if srv.err != nil {
		t.Errorf("err = %v, want nil", srv.err)
	}
	if srv.codec != nil {
		t.Error("codec is set without WithCodec, want nil (no package default)")
	}

	// The default checkOrigin must allow every origin.
	req := httptest.NewRequest(http.MethodGet, "http://example.com/socket.io/", nil)
	req.Header.Set("Origin", "https://evil.example")
	if !srv.checkOrigin(req) {
		t.Error("default checkOrigin denied an origin, want allow-all")
	}
}

func TestServerOptions(t *testing.T) {
	tlsConf := &tls.Config{}

	connectCalled := false
	srv := NewServer(
		WithNetwork("tcp4"),
		WithAddress("127.0.0.1:9200"),
		WithTLSConfig(tlsConf),
		WithCodec("json"),
		WithPath("/custom.io/"),
		WithConnectHandler("/", func(socketio.Conn) error {
			connectCalled = true
			return nil
		}),
		WithDisconnectHandler("/", func(socketio.Conn, string) {}),
		WithErrorHandler("/", func(socketio.Conn, error) {}),
		WithEventHandler("/", "notice", func(socketio.Conn, string) {}),
		WithCheckOrigin(func(r *http.Request) bool { return false }),
	)

	if srv.network != "tcp4" {
		t.Errorf("network = %q, want %q", srv.network, "tcp4")
	}
	if srv.address != "127.0.0.1:9200" {
		t.Errorf("address = %q, want %q", srv.address, "127.0.0.1:9200")
	}
	if srv.tlsConf != tlsConf {
		t.Error("tlsConf was not applied")
	}
	if srv.codec == nil {
		t.Error("codec is nil after WithCodec")
	}
	if srv.path != "/custom.io/" {
		t.Errorf("path = %q, want %q", srv.path, "/custom.io/")
	}

	if len(srv.Registrations) != 4 {
		t.Fatalf("Registrations = %d entries, want 4", len(srv.Registrations))
	}
	wantKinds := []string{"connect", "disconnect", "error", "event"}
	for i, kind := range wantKinds {
		if srv.Registrations[i].kind != kind {
			t.Errorf("Registrations[%d].kind = %q, want %q", i, srv.Registrations[i].kind, kind)
		}
	}
	if srv.Registrations[3].event != "notice" {
		t.Errorf("event registration name = %q, want %q", srv.Registrations[3].event, "notice")
	}

	// WithCheckOrigin must be captured by the transports created during init:
	// a denied origin rejects the request via the recorded closure.
	req := httptest.NewRequest(http.MethodGet, "http://example.com/custom.io/", nil)
	if srv.checkOrigin(req) {
		t.Error("WithCheckOrigin closure not applied")
	}
	_ = connectCalled // the handler only fires on a live connection
}

func TestWithNilTLSConfig(t *testing.T) {
	srv := NewServer(WithTLSConfig(nil))
	if srv.tlsConf != nil {
		t.Error("tlsConf should stay nil after WithTLSConfig(nil)")
	}
}

// ---------------------------------------------------------------------------
// Handler registration and replay
// ---------------------------------------------------------------------------

func TestRegisterHandlersRecordsAndAttaches(t *testing.T) {
	srv := NewServer()

	srv.RegisterConnectHandler("/chat", func(socketio.Conn) error { return nil })
	srv.RegisterDisconnectHandler("/chat", func(socketio.Conn, string) {})
	srv.RegisterErrorHandler("/chat", func(socketio.Conn, error) {})
	srv.RegisterEventHandler("/chat", "msg", func(socketio.Conn, string) {})
	srv.RegisterEventHandler("/chat", "ack", func(socketio.Conn, string) string { return "" })

	if len(srv.Registrations) != 5 {
		t.Fatalf("Registrations = %d entries, want 5", len(srv.Registrations))
	}

	// Rebuilding the underlying server must replay every registration
	// without error (this is the path Start takes after Stop).
	srv.closed = true
	rebuilt := srv.createServer()
	if rebuilt == nil {
		t.Fatal("createServer returned nil")
	}
	if len(srv.Registrations) != 5 {
		t.Errorf("Registrations after replay = %d, want 5 (replay must keep the record)", len(srv.Registrations))
	}
}

// ---------------------------------------------------------------------------
// Endpoint
// ---------------------------------------------------------------------------

func TestEndpointWithoutListener(t *testing.T) {
	tests := []struct {
		name    string
		address string
		want    string
	}{
		{"host and port", "127.0.0.1:9200", "socket.io://127.0.0.1:9200"},
		{"empty host becomes localhost", ":9200", "socket.io://localhost:9200"},
		{"0.0.0.0 becomes localhost", "0.0.0.0:9200", "socket.io://localhost:9200"},
		{"no port falls back to raw address", "localhost", "socket.io://localhost"},
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

// ---------------------------------------------------------------------------
// Use middleware
// ---------------------------------------------------------------------------

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
	handler := handlers.CORS()(srv.router)
	for i := len(srv.middlewares) - 1; i >= 0; i-- {
		handler = srv.middlewares[i](handler)
	}

	req := httptest.NewRequest(http.MethodGet, "http://example.com/nothing", nil)
	handler.ServeHTTP(httptest.NewRecorder(), req)

	if len(order) != 2 || order[0] != "outer" || order[1] != "inner" {
		t.Errorf("middleware order = %v, want [outer inner]", order)
	}
}

// ---------------------------------------------------------------------------
// Engine.IO handshake through the router (hermetic, no listener)
// ---------------------------------------------------------------------------

// TestPollingHandshakeThroughRouter drives an engine.io v4 polling handshake
// straight through the router's delegate handler; the embedded socket.io
// server answers in-memory, so no listener is needed.
func TestPollingHandshakeThroughRouter(t *testing.T) {
	srv := NewServer(WithCodec("json"))

	req := httptest.NewRequest(http.MethodGet, "http://example.com/socket.io/?EIO=4&transport=polling", nil)
	rec := httptest.NewRecorder()
	srv.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("handshake status = %d, want %d; body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	body := rec.Body.String()
	// The engine.io v3 framing prefixes the open packet with a binary length
	// header, so only assert on the packet payload itself.
	if !strings.Contains(body, `0{"sid"`) {
		t.Errorf("handshake body %q missing engine.io open packet with session id", body)
	}
}

// TestDelegateHandlerFollowsRebuild verifies the router keeps serving after
// the underlying socket.io server was closed and rebuilt (the restart path).
func TestDelegateHandlerFollowsRebuild(t *testing.T) {
	srv := NewServer()

	serveHandshake := func() *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "http://example.com/socket.io/?EIO=4&transport=polling", nil)
		rec := httptest.NewRecorder()
		srv.router.ServeHTTP(rec, req)
		return rec
	}

	if rec := serveHandshake(); rec.Code != http.StatusOK {
		t.Fatalf("first handshake status = %d, want %d", rec.Code, http.StatusOK)
	}

	// Simulate the Stop/Start rebuild cycle.
	srv.closed = true
	srv.Server = srv.createServer()

	rec := serveHandshake()
	if rec.Code != http.StatusOK {
		t.Fatalf("handshake after rebuild status = %d, want %d; body: %s",
			rec.Code, http.StatusOK, rec.Body.String())
	}
}
