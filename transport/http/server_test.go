package http_test

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"
	"time"

	windhttp "github.com/tx7do/go-wind-plugins/transport/http"
)

// ---------------------------------------------------------------------------
// Mock driver: records registrations and can serve recorded handlers without
// importing any concrete driver (the root module must not depend on one).
// ---------------------------------------------------------------------------

type handleCall struct {
	method  string
	path    string
	handler http.HandlerFunc
}

type prefixCall struct {
	prefix  string
	handler http.Handler
}

type mockDriver struct {
	mu       sync.Mutex
	handles  []handleCall
	prefixes []prefixCall
	listener net.Listener
	started  chan struct{}
	stopped  int
}

func newMockDriver() *mockDriver {
	return &mockDriver{started: make(chan struct{})}
}

func (m *mockDriver) Handle(method, path string, handler http.HandlerFunc) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.handles = append(m.handles, handleCall{method, path, handler})
}

func (m *mockDriver) HandlePrefix(prefix string, h http.Handler) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.prefixes = append(m.prefixes, prefixCall{prefix, h})
}

// Start records the listener, signals readiness and blocks until ctx is done,
// mirroring the contract of a real blocking driver.
func (m *mockDriver) Start(ctx context.Context, ln net.Listener) error {
	m.mu.Lock()
	m.listener = ln
	m.mu.Unlock()
	close(m.started)
	<-ctx.Done()
	return nil
}

func (m *mockDriver) Stop(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stopped++
	return nil
}

func (m *mockDriver) snapshot() (hs []handleCall, ps []prefixCall) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]handleCall(nil), m.handles...), append([]prefixCall(nil), m.prefixes...)
}

// ---------------------------------------------------------------------------
// Construction and options
// ---------------------------------------------------------------------------

func TestNewServer_Defaults(t *testing.T) {
	srv := windhttp.NewServer(":8080")
	if srv.Addr() != ":8080" {
		t.Errorf("Addr() = %q, want :8080", srv.Addr())
	}
	// No listener yet: derived from the configured address, host empty -> localhost.
	if got, want := srv.Endpoint(), "http://localhost:8080"; got != want {
		t.Errorf("Endpoint() = %q, want %q", got, want)
	}
}

func TestEndpoint_AddressVariants(t *testing.T) {
	cases := []struct {
		addr string
		tls  bool
		want string
	}{
		{addr: ":8080", want: "http://localhost:8080"},
		{addr: "0.0.0.0:9000", want: "http://localhost:9000"},
		{addr: "192.168.1.1:80", want: "http://192.168.1.1:80"},
		{addr: "[::1]:8080", want: "http://[::1]:8080"},
		{addr: "localhost", want: "http://localhost"}, // no port: raw addr
		{addr: ":8080", tls: true, want: "https://localhost:8080"},
		{addr: "plain", tls: true, want: "https://plain"}, // no port at all
	}
	for _, tc := range cases {
		var opts []windhttp.Option
		if tc.tls {
			opts = append(opts, windhttp.WithTLSConfig(&tls.Config{}))
		}
		if got := windhttp.NewServer(tc.addr, opts...).Endpoint(); got != tc.want {
			t.Errorf("NewServer(%q).Endpoint() with tls=%v = %q, want %q", tc.addr, tc.tls, got, tc.want)
		}
	}
}

func TestEndpoint_UsesActualListenerAddr(t *testing.T) {
	drv := newMockDriver()
	srv := windhttp.NewServer("127.0.0.1:0", windhttp.WithDriver(drv))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errChan := make(chan error, 1)
	go func() { errChan <- srv.Start(ctx) }()

	select {
	case <-drv.started:
	case <-time.After(3 * time.Second):
		t.Fatal("driver Start not reached within 3s")
	}

	ep := srv.Endpoint()
	// Random port assigned: endpoint must carry the real listener address.
	if ep == "http://localhost:0" || ep == "http://127.0.0.1:0" {
		t.Errorf("Endpoint() = %q, want resolved random port", ep)
	}
	if !net.ParseIP(hostOf(ep)).IsLoopback() {
		t.Errorf("Endpoint() = %q, want loopback host", ep)
	}

	cancel()
	select {
	case err := <-errChan:
		if err != nil {
			t.Errorf("Start = %v, want nil", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Start did not return after cancel")
	}
}

func hostOf(endpoint string) string {
	// strip scheme
	h := endpoint[len("http://"):]
	// strip :port (works for both host:port and [v6]:port forms)
	for k := len(h) - 1; k >= 0; k-- {
		if h[k] == ':' {
			h = h[:k]
			break
		}
	}
	return h
}

// ---------------------------------------------------------------------------
// Missing driver: panics / no-op
// ---------------------------------------------------------------------------

func TestHandle_WithoutDriverPanics(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("Handle without driver should panic")
		}
		if r != windhttp.ErrNoDriver {
			t.Errorf("panic value = %v, want ErrNoDriver", r)
		}
	}()
	windhttp.NewServer(":8080").Handle(http.MethodGet, "/x", func(w http.ResponseWriter, r *http.Request) {})
}

func TestHandlePrefix_WithoutDriverPanics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("HandlePrefix without driver should panic")
		}
	}()
	windhttp.NewServer(":8080").HandlePrefix("/docs/", http.NotFoundHandler())
}

func TestStart_WithoutDriverPanics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("Start without driver should panic")
		}
	}()
	_ = windhttp.NewServer(":8080").Start(context.Background())
}

func TestStop_WithoutDriverReturnsNil(t *testing.T) {
	if err := windhttp.NewServer(":8080").Stop(context.Background()); err != nil {
		t.Errorf("Stop without driver = %v, want nil", err)
	}
}

// ---------------------------------------------------------------------------
// Route registration: verb helpers dispatch to Driver.Handle
// ---------------------------------------------------------------------------

func TestVerbHelpers_RegisterRoutes(t *testing.T) {
	verbs := []struct {
		name   string
		method string
		call   func(*windhttp.Server, string, http.HandlerFunc)
	}{
		{"GET", http.MethodGet, func(s *windhttp.Server, p string, h http.HandlerFunc) { s.GET(p, h) }},
		{"POST", http.MethodPost, func(s *windhttp.Server, p string, h http.HandlerFunc) { s.POST(p, h) }},
		{"PUT", http.MethodPut, func(s *windhttp.Server, p string, h http.HandlerFunc) { s.PUT(p, h) }},
		{"DELETE", http.MethodDelete, func(s *windhttp.Server, p string, h http.HandlerFunc) { s.DELETE(p, h) }},
		{"PATCH", http.MethodPatch, func(s *windhttp.Server, p string, h http.HandlerFunc) { s.PATCH(p, h) }},
		{"HEAD", http.MethodHead, func(s *windhttp.Server, p string, h http.HandlerFunc) { s.HEAD(p, h) }},
		{"OPTIONS", http.MethodOptions, func(s *windhttp.Server, p string, h http.HandlerFunc) { s.OPTIONS(p, h) }},
	}
	for _, v := range verbs {
		t.Run(v.name, func(t *testing.T) {
			drv := newMockDriver()
			srv := windhttp.NewServer(":8080", windhttp.WithDriver(drv))
			v.call(srv, "/res", func(w http.ResponseWriter, r *http.Request) {})

			hs, _ := drv.snapshot()
			if len(hs) != 1 {
				t.Fatalf("registered %d handlers, want 1", len(hs))
			}
			if hs[0].method != v.method || hs[0].path != "/res" {
				t.Errorf("driver.Handle got (%q, %q), want (%q, /res)", hs[0].method, hs[0].path, v.method)
			}
		})
	}
}

func TestHandle_MiddlewareWrapsHandler(t *testing.T) {
	var order []string
	mkMW := func(name string) windhttp.Middleware {
		return func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				order = append(order, name+"-before")
				next.ServeHTTP(w, r)
				order = append(order, name+"-after")
			})
		}
	}

	drv := newMockDriver()
	srv := windhttp.NewServer(":8080",
		windhttp.WithDriver(drv),
		windhttp.WithMiddleware(mkMW("outer")),
	)
	srv.Use(mkMW("inner"))
	srv.Handle(http.MethodGet, "/x", func(w http.ResponseWriter, r *http.Request) {
		order = append(order, "handler")
	})

	hs, _ := drv.snapshot()
	if len(hs) != 1 {
		t.Fatalf("registered %d handlers, want 1", len(hs))
	}

	rec := httptest.NewRecorder()
	hs[0].handler(rec, httptest.NewRequest(http.MethodGet, "/x", nil))

	want := []string{"outer-before", "inner-before", "handler", "inner-after", "outer-after"}
	if len(order) != len(want) {
		t.Fatalf("order = %v, want %v", order, want)
	}
	for i := range want {
		if order[i] != want[i] {
			t.Fatalf("order = %v, want %v", order, want)
		}
	}
}

func TestHandlePrefix_BypassesMiddleware(t *testing.T) {
	called := false
	mw := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			called = true
			next.ServeHTTP(w, r)
		})
	}

	drv := newMockDriver()
	srv := windhttp.NewServer(":8080", windhttp.WithDriver(drv))
	srv.Use(mw)

	handlerCalled := false
	srv.HandlePrefix("/docs/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerCalled = true
	}))

	_, ps := drv.snapshot()
	if len(ps) != 1 || ps[0].prefix != "/docs/" {
		t.Fatalf("prefixes = %+v, want one /docs/ registration", ps)
	}
	ps[0].handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/docs/", nil))
	if !handlerCalled {
		t.Error("prefix handler should run")
	}
	if called {
		t.Error("middleware must not wrap HandlePrefix handlers")
	}
}

// ---------------------------------------------------------------------------
// Start / Stop / TLS
// ---------------------------------------------------------------------------

func TestStart_ListenError(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	srv := windhttp.NewServer(ln.Addr().String(), windhttp.WithDriver(newMockDriver()))
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := srv.Start(ctx); err == nil {
		t.Error("Start on an occupied address should fail")
	}
}

func TestStart_WrapsListenerWithTLS(t *testing.T) {
	dir := t.TempDir()
	certFile, keyFile := writeSelfSignedCert(t, dir)

	drv := newMockDriver()
	srv := windhttp.NewServer("127.0.0.1:0",
		windhttp.WithDriver(drv),
		windhttp.WithTLS(certFile, keyFile),
	)
	if got, want := srv.Endpoint(), "https://"; len(got) < len(want) || got[:len(want)] != want {
		t.Errorf("Endpoint() = %q, want https scheme", got)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = srv.Start(ctx) }()

	select {
	case <-drv.started:
	case <-time.After(3 * time.Second):
		t.Fatal("driver Start not reached within 3s")
	}

	drv.mu.Lock()
	ln := drv.listener
	drv.mu.Unlock()
	if ln == nil {
		t.Fatal("listener not passed to driver")
	}
	// TLS wrapping replaces the raw TCP listener.
	if _, ok := ln.(*net.TCPListener); ok {
		t.Error("listener should be TLS-wrapped, got raw *net.TCPListener")
	}
}

func TestWithTLS_BadFilePanics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("WithTLS with a missing cert file should panic")
		}
	}()
	_ = windhttp.NewServer(":8080",
		windhttp.WithDriver(newMockDriver()),
		windhttp.WithTLS(t.TempDir()+"/missing.crt", t.TempDir()+"/missing.key"),
	)
}

func TestStop_DelegatesToDriver(t *testing.T) {
	drv := newMockDriver()
	srv := windhttp.NewServer(":8080", windhttp.WithDriver(drv))
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := srv.Stop(ctx); err != nil {
		t.Errorf("Stop = %v, want nil", err)
	}
	drv.mu.Lock()
	n := drv.stopped
	drv.mu.Unlock()
	if n != 1 {
		t.Errorf("driver Stop called %d times, want 1", n)
	}
}

// ---------------------------------------------------------------------------
// Chain
// ---------------------------------------------------------------------------

func TestChain_ComposesInOrder(t *testing.T) {
	var order []string
	mkMW := func(name string) windhttp.Middleware {
		return func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				order = append(order, name)
				next.ServeHTTP(w, r)
			})
		}
	}

	chained := windhttp.Chain(mkMW("a"), mkMW("b"), mkMW("c"))
	terminal := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		order = append(order, "end")
	})

	chained(terminal).ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))

	want := []string{"a", "b", "c", "end"}
	if len(order) != len(want) {
		t.Fatalf("order = %v, want %v", order, want)
	}
	for i := range want {
		if order[i] != want[i] {
			t.Fatalf("order = %v, want %v", order, want)
		}
	}
}

func TestChain_EmptyReturnsPassThrough(t *testing.T) {
	chained := windhttp.Chain()
	called := false
	terminal := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called = true })
	chained(terminal).ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
	if !called {
		t.Error("empty Chain should pass through to the terminal handler")
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// writeSelfSignedCert generates a throwaway TLS keypair with the standard
// library and writes PEM files into dir.
func writeSelfSignedCert(t *testing.T, dir string) (certPath, keyPath string) {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	tmpl := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "localhost"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:     []string{"localhost"},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
	}
	der, err := x509.CreateCertificate(rand.Reader, &tmpl, &tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create certificate: %v", err)
	}

	certPath = dir + "/cert.pem"
	keyPath = dir + "/key.pem"
	certOut := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatalf("marshal key: %v", err)
	}
	keyOut := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})

	if err := errors.Join(
		os.WriteFile(certPath, certOut, 0o600),
		os.WriteFile(keyPath, keyOut, 0o600),
	); err != nil {
		t.Fatalf("write cert files: %v", err)
	}
	return certPath, keyPath
}
