package graphql

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/99designs/gqlgen/graphql"
	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/handler/transport"
	"github.com/tx7do/go-wind/log"
	"github.com/vektah/gqlparser/v2/ast"
)

// ---------------------------------------------------------------------------
// stub ExecutableSchema
// ---------------------------------------------------------------------------

// stubSchema is a minimal hermetic graphql.ExecutableSchema: it answers every
// operation with a canned payload without running any resolver machinery.
type stubSchema struct {
	mu       sync.Mutex
	response *graphql.Response
}

func (s *stubSchema) Schema() *ast.Schema {
	query := &ast.Definition{
		Kind: ast.Object,
		Name: "Query",
		Fields: []*ast.FieldDefinition{
			{Name: "hello", Type: ast.NamedType("String", nil)},
		},
	}
	return &ast.Schema{
		Types: map[string]*ast.Definition{
			"Query":  query,
			"String": {Kind: ast.Scalar, Name: "String"},
		},
		Query: query,
	}
}

func (s *stubSchema) Complexity(_ context.Context, _, _ string, _ int, _ map[string]any) (int, bool) {
	return 0, false
}

func (s *stubSchema) Exec(ctx context.Context) graphql.ResponseHandler {
	return func(ctx context.Context) *graphql.Response {
		s.mu.Lock()
		resp := s.response
		s.mu.Unlock()
		if resp == nil {
			return graphql.ErrorResponse(ctx, "no response configured")
		}
		return resp
	}
}

// ---------------------------------------------------------------------------
// Endpoint edge cases
// ---------------------------------------------------------------------------

func TestServer_Endpoint_NoPort(t *testing.T) {
	srv := NewServer("kettle")
	if got := srv.Endpoint(); got != "http://kettle" {
		t.Errorf("Endpoint() = %q, want http://kettle", got)
	}
}

func TestServer_Endpoint_EmptyHost(t *testing.T) {
	srv := NewServer(":9099")
	// Endpoint normalizes an empty host to localhost.
	if got := srv.Endpoint(); got != "http://localhost:9099" {
		t.Errorf("Endpoint() = %q, want http://localhost:9099", got)
	}
}

// ---------------------------------------------------------------------------
// Start error paths and restart
// ---------------------------------------------------------------------------

func TestServer_Start_BadAddr(t *testing.T) {
	srv := NewServer("invalid-addr-no-port")
	if err := srv.Start(context.Background()); err == nil {
		t.Fatal("Start() expected an error for an unparseable address")
	}
}

func TestServer_StopThenStart(t *testing.T) {
	srv := NewServer("127.0.0.1:0")
	srv.HandleFunc("/ping", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	ctx, cancel := context.WithCancel(context.Background())

	errCh := make(chan error, 1)
	go func() { errCh <- srv.Start(ctx) }()
	time.Sleep(100 * time.Millisecond)

	if srv.listener == nil {
		t.Fatal("listener not set after Start")
	}
	firstAddr := srv.listener.Addr().String()

	resp, err := http.Get("http://" + firstAddr + "/ping")
	if err != nil {
		t.Fatalf("first run: GET failed: %v", err)
	}
	resp.Body.Close()

	// Stop must release the listener and allow a subsequent Start.
	if err := srv.Stop(context.Background()); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
	if srv.listener != nil {
		t.Error("Stop() should clear the listener reference")
	}

	cancel()
	select {
	case err := <-errCh:
		if err != nil {
			t.Errorf("Start returned error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("first Start did not return after Stop+cancel")
	}

	// Start again on the same server: the stale-listener release path.
	ctx2, cancel2 := context.WithCancel(context.Background())
	defer cancel2()

	errCh2 := make(chan error, 1)
	go func() { errCh2 <- srv.Start(ctx2) }()
	time.Sleep(100 * time.Millisecond)

	if srv.listener == nil {
		t.Fatal("listener not set after restart")
	}
	resp2, err := http.Get("http://" + srv.listener.Addr().String() + "/ping")
	if err != nil {
		t.Fatalf("second run: GET failed: %v", err)
	}
	resp2.Body.Close()

	cancel2()
	select {
	case err := <-errCh2:
		if err != nil {
			t.Errorf("restarted Start returned error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("restarted Start did not return after cancel")
	}
}

// ---------------------------------------------------------------------------
// GraphQL handler wiring with a stub schema
// ---------------------------------------------------------------------------

func startWithSchema(t *testing.T, es graphql.ExecutableSchema) (baseURL string, done <-chan error, stop func()) {
	t.Helper()
	srv := NewServer("127.0.0.1:0")
	if es != nil {
		srv.Handle("/query", es)
	}
	runCtx, runCancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() { errCh <- srv.Start(runCtx) }()
	time.Sleep(100 * time.Millisecond)
	if srv.listener == nil {
		runCancel()
		t.Fatal("server failed to start")
	}
	return "http://" + srv.listener.Addr().String(), errCh, func() {
		runCancel()
		select {
		case <-errCh:
		case <-time.After(5 * time.Second):
		}
	}
}

func postJSON(t *testing.T, url, body string) *http.Response {
	t.Helper()
	resp, err := http.Post(url, "application/json", bytes.NewBufferString(body))
	if err != nil {
		t.Fatalf("POST failed: %v", err)
	}
	return resp
}

// TestServer_Handle_TransportsRegistered is the flipped regression test for
// the former usability gap: Server.Handle wired handler.New without any HTTP
// transports, so every request was rejected with 400 "transport not
// supported". The standard transports are now mounted, and a GraphQL POST (as
// well as a GET) must return a GraphQL response.
func TestServer_Handle_TransportsRegistered(t *testing.T) {
	es := &stubSchema{response: &graphql.Response{Data: []byte(`{"hello":"world"}`)}}
	base, _, stop := startWithSchema(t, es)
	defer stop()

	assertGraphQLData := func(t *testing.T, kind string, resp *http.Response) {
		t.Helper()
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("%s status = %d, want 200", kind, resp.StatusCode)
		}
		var out struct {
			Data json.RawMessage `json:"data"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
			t.Fatalf("%s response is not valid JSON: %v", kind, err)
		}
		if string(out.Data) != `{"hello":"world"}` {
			t.Errorf("%s data = %s, want {\"hello\":\"world\"}", kind, out.Data)
		}
	}

	resp := postJSON(t, base+"/query", `{"query":"{ hello }"}`)
	assertGraphQLData(t, "POST", resp)

	getResp, err := http.Get(base + "/query?query=%7B%20hello%20%7D")
	if err != nil {
		t.Fatalf("GET failed: %v", err)
	}
	assertGraphQLData(t, "GET", getResp)
}

// gqlgenPOSTHandler builds a gqlgen HTTP handler with the POST transport, the
// minimal production wiring for a Server.Handle-style endpoint.
func gqlgenPOSTHandler(es graphql.ExecutableSchema) http.Handler {
	h := handler.New(es)
	h.AddTransport(transport.Options{})
	h.AddTransport(transport.POST{})
	return h
}

func TestServer_HandleFunc_WithPOSTTransport(t *testing.T) {
	es := &stubSchema{response: &graphql.Response{Data: []byte(`{"hello":"world"}`)}}

	srv := NewServer("127.0.0.1:0")
	srv.HandleFunc("/query", gqlgenPOSTHandler(es).ServeHTTP)

	runCtx, runCancel := context.WithCancel(context.Background())
	defer runCancel()
	errCh := make(chan error, 1)
	go func() { errCh <- srv.Start(runCtx) }()
	time.Sleep(100 * time.Millisecond)
	if srv.listener == nil {
		t.Fatal("server failed to start")
	}
	base := "http://" + srv.listener.Addr().String()
	defer func() {
		runCancel()
		select {
		case <-errCh:
		case <-time.After(5 * time.Second):
		}
	}()

	resp := postJSON(t, base+"/query", `{"query":"{ hello }"}`)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var out struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("response is not valid JSON: %v", err)
	}
	if string(out.Data) != `{"hello":"world"}` {
		t.Errorf("data = %s, want {\"hello\":\"world\"}", out.Data)
	}
}

func TestServer_HandleFunc_SchemaError(t *testing.T) {
	es := &stubSchema{} // Exec returns an error response

	srv := NewServer("127.0.0.1:0")
	srv.HandleFunc("/query", gqlgenPOSTHandler(es).ServeHTTP)

	runCtx, runCancel := context.WithCancel(context.Background())
	defer runCancel()
	go func() { _ = srv.Start(runCtx) }()
	time.Sleep(100 * time.Millisecond)
	if srv.listener == nil {
		t.Fatal("server failed to start")
	}
	base := "http://" + srv.listener.Addr().String()

	resp := postJSON(t, base+"/query", `{"query":"{ hello }"}`)
	defer resp.Body.Close()

	var out struct {
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("response is not valid JSON: %v", err)
	}
	if len(out.Errors) == 0 || out.Errors[0].Message != "no response configured" {
		t.Errorf("errors = %+v, want the stub's error message", out.Errors)
	}
}

func TestServer_HandleFunc_MalformedBody(t *testing.T) {
	srv := NewServer("127.0.0.1:0")
	srv.HandleFunc("/query", gqlgenPOSTHandler(&stubSchema{}).ServeHTTP)

	runCtx, runCancel := context.WithCancel(context.Background())
	defer runCancel()
	go func() { _ = srv.Start(runCtx) }()
	time.Sleep(100 * time.Millisecond)
	if srv.listener == nil {
		t.Fatal("server failed to start")
	}
	base := "http://" + srv.listener.Addr().String()

	resp := postJSON(t, base+"/query", "this is not json")
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		t.Errorf("status = %d, want an error status for a malformed body", resp.StatusCode)
	}
}

func TestServer_HandleFunc_InvalidGraphQLDocument(t *testing.T) {
	srv := NewServer("127.0.0.1:0")
	srv.HandleFunc("/query", gqlgenPOSTHandler(&stubSchema{}).ServeHTTP)

	runCtx, runCancel := context.WithCancel(context.Background())
	defer runCancel()
	go func() { _ = srv.Start(runCtx) }()
	time.Sleep(100 * time.Millisecond)
	if srv.listener == nil {
		t.Fatal("server failed to start")
	}
	base := "http://" + srv.listener.Addr().String()

	resp := postJSON(t, base+"/query", `{"query":"{ unclosed"}`)
	defer resp.Body.Close()

	var out struct {
		Errors []json.RawMessage `json:"errors"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("response is not valid JSON: %v", err)
	}
	if len(out.Errors) == 0 {
		t.Error("expected parse errors for an invalid GraphQL document")
	}
}

// ---------------------------------------------------------------------------
// WithTLS option (self-signed fixture generated in-process)
// ---------------------------------------------------------------------------

func writeSelfSignedCert(t *testing.T, dir string) (certPath, keyPath string) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("key generation failed: %v", err)
	}
	tmpl := x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "localhost"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IsCA:                  true,
		BasicConstraintsValid: true,
		DNSNames:              []string{"localhost"},
	}
	der, err := x509.CreateCertificate(rand.Reader, &tmpl, &tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("certificate creation failed: %v", err)
	}
	certPath = filepath.Join(dir, "cert.pem")
	keyPath = filepath.Join(dir, "key.pem")

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatalf("key marshalling failed: %v", err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	if err := os.WriteFile(certPath, certPEM, 0o600); err != nil {
		t.Fatalf("writing cert failed: %v", err)
	}
	if err := os.WriteFile(keyPath, keyPEM, 0o600); err != nil {
		t.Fatalf("writing key failed: %v", err)
	}
	return certPath, keyPath
}

func TestWithTLS_ValidPair(t *testing.T) {
	certPath, keyPath := writeSelfSignedCert(t, t.TempDir())

	var srv *Server
	assertNoPanic(t, func() {
		srv = NewServer(":0", WithTLS(certPath, keyPath))
	})
	if srv.tlsConfig == nil {
		t.Fatal("WithTLS did not configure TLS")
	}
	if len(srv.tlsConfig.Certificates) != 1 {
		t.Errorf("certificates = %d, want 1", len(srv.tlsConfig.Certificates))
	}
	if got := srv.Endpoint(); !strings.HasPrefix(got, "https://") {
		t.Errorf("Endpoint() = %q, want https scheme with TLS configured", got)
	}
}

func TestWithTLS_MissingFiles_Panics(t *testing.T) {
	assertPanic(t, func() {
		NewServer(":0", WithTLS("/nonexistent/cert.pem", "/nonexistent/key.pem"))
	})
}

func TestWithMiddleware_Option(t *testing.T) {
	srv := NewServer(":0", WithMiddleware(func(next http.Handler) http.Handler {
		return next
	}))
	if len(srv.middlewares) != 1 {
		t.Errorf("middlewares = %d, want 1", len(srv.middlewares))
	}
	srv.Use(func(next http.Handler) http.Handler { return next })
	if len(srv.middlewares) != 2 {
		t.Errorf("middlewares after Use = %d, want 2", len(srv.middlewares))
	}
}

// ---------------------------------------------------------------------------
// logger
// ---------------------------------------------------------------------------

type recordingLogger struct {
	mu      sync.Mutex
	matches []string
}

func (l *recordingLogger) Debug(_ context.Context, msg string, _ ...any) { l.record(msg) }
func (l *recordingLogger) Info(_ context.Context, msg string, _ ...any)  { l.record(msg) }
func (l *recordingLogger) Warn(_ context.Context, msg string, _ ...any)  { l.record(msg) }
func (l *recordingLogger) Error(_ context.Context, msg string, _ ...any) { l.record(msg) }
func (l *recordingLogger) Enabled(_ log.Level) bool                      { return true }
func (l *recordingLogger) With(_ ...any) log.Logger                      { return l }

func (l *recordingLogger) record(msg string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.matches = append(l.matches, msg)
}

func (l *recordingLogger) messages() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]string(nil), l.matches...)
}

func TestLogger_Injection(t *testing.T) {
	rec := &recordingLogger{}
	SetLogger(rec)
	t.Cleanup(func() { SetLogger(nil) })

	LogDebug("d")
	LogDebugf("d%d", 0)
	LogInfof("i%d", 1)
	LogInfo("i")
	LogWarn("w")
	LogWarnf("w%d", 2)
	LogErrorf("e%v", 2)
	LogError("e")
	LogFatal("f")
	LogFatalf("f%d", 3)

	msgs := rec.messages()
	if len(msgs) != 10 {
		t.Fatalf("logged messages = %v, want 10", msgs)
	}
	for _, m := range msgs {
		if !strings.Contains(m, "[graphql]") {
			t.Errorf("message %q missing the [graphql] prefix", m)
		}
	}
	if !strings.Contains(msgs[2], "i1") || !strings.Contains(msgs[8], "f") {
		t.Errorf("formatted messages wrong: %v", msgs)
	}

	// SetLogger(nil) restores the global logger without panicking.
	SetLogger(nil)
	LogInfo("after reset")
}

// ---------------------------------------------------------------------------
// panic helpers
// ---------------------------------------------------------------------------

func assertNoPanic(t *testing.T, fn func()) {
	t.Helper()
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("unexpected panic: %v", r)
		}
	}()
	fn()
}

func assertPanic(t *testing.T, fn func()) {
	t.Helper()
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected a panic")
		}
	}()
	fn()
}
