package std

import (
	"bytes"
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	httpPlugin "github.com/tx7do/go-wind-plugins/transport/http"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// startServer spins the driver up on 127.0.0.1 with a random port and waits
// until it accepts connections. It registers cleanup that stops the driver.
func startServer(t *testing.T, d *stdDriver) (baseURL string, cancel context.CancelFunc) {
	t.Helper()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	errChan := make(chan error, 1)
	go func() { errChan <- d.Start(ctx, ln) }()

	baseURL = "http://" + ln.Addr().String()

	// Readiness probe: any successful TCP/HTTP round-trip means the server
	// is accepting connections (a 404 for an unregistered path is fine).
	client := &http.Client{Timeout: 2 * time.Second}
	deadline := time.Now().Add(3 * time.Second)
	for {
		resp, err := client.Get(baseURL + "/__ready__")
		if err == nil {
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
			break
		}
		if time.Now().After(deadline) {
			cancel()
			t.Fatalf("driver not ready: %v", err)
		}
		time.Sleep(5 * time.Millisecond)
	}

	t.Cleanup(func() {
		cancel()
		select {
		case <-errChan:
		case <-time.After(3 * time.Second):
			t.Error("Start did not return within 3s after cancel")
		}
	})
	return baseURL, cancel
}

// ---------------------------------------------------------------------------
// NewDriver
// ---------------------------------------------------------------------------

func TestNewDriver_InitializesMux(t *testing.T) {
	var drv httpPlugin.Driver = NewDriver()
	d, ok := drv.(*stdDriver)
	if !ok {
		t.Fatalf("NewDriver() returned %T, want *stdDriver", drv)
	}
	if d.mux == nil {
		t.Error("mux should be initialized")
	}
	if d.server != nil {
		t.Error("server should be nil before Start")
	}
}

// ---------------------------------------------------------------------------
// Handle: method dispatch (checked inside the driver, no network needed)
// ---------------------------------------------------------------------------

func TestHandle_MethodDispatch(t *testing.T) {
	d := &stdDriver{mux: http.NewServeMux()}
	called := false
	d.Handle(http.MethodGet, "/ping", func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("pong"))
	})

	// Matching method reaches the handler.
	rec := httptest.NewRecorder()
	d.mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/ping", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET status = %d, want 200", rec.Code)
	}
	if !called {
		t.Error("handler should have been called for matching method")
	}
	if rec.Body.String() != "pong" {
		t.Errorf("body = %q, want pong", rec.Body.String())
	}

	// Non-matching method is rejected with 405 before the handler runs.
	called = false
	rec2 := httptest.NewRecorder()
	d.mux.ServeHTTP(rec2, httptest.NewRequest(http.MethodPost, "/ping", nil))
	if rec2.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST status = %d, want 405", rec2.Code)
	}
	if called {
		t.Error("handler must not run when the method does not match")
	}
	if !strings.Contains(rec2.Body.String(), http.StatusText(http.StatusMethodNotAllowed)) {
		t.Errorf("405 body should contain reason, got %q", rec2.Body.String())
	}
}

// ---------------------------------------------------------------------------
// HandlePrefix: subtree mounting via ServeMux patterns
// ---------------------------------------------------------------------------

func TestHandlePrefix_ServesSubtree(t *testing.T) {
	d := &stdDriver{mux: http.NewServeMux()}
	d.HandlePrefix("/assets/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("asset:" + r.URL.Path))
	}))

	// The prefix itself and any sub-path are both routed to the handler.
	for _, path := range []string{"/assets/", "/assets/app.js", "/assets/css/site.css"} {
		rec := httptest.NewRecorder()
		d.mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != http.StatusOK {
			t.Errorf("GET %s status = %d, want 200", path, rec.Code)
		}
		want := "asset:" + path
		if rec.Body.String() != want {
			t.Errorf("GET %s body = %q, want %q", path, rec.Body.String(), want)
		}
	}

	// Outside the prefix is not matched (ServeMux default 404).
	rec := httptest.NewRecorder()
	d.mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/other", nil))
	if rec.Code != http.StatusNotFound {
		t.Errorf("GET /other status = %d, want 404", rec.Code)
	}
}

func TestHandlePrefix_AppendsTrailingSlash(t *testing.T) {
	d := &stdDriver{mux: http.NewServeMux()}
	// Prefix without trailing slash: per net/http subtree semantics the
	// driver normalizes it so "/docs" and "/docs/x" both match.
	d.HandlePrefix("/docs", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	rec := httptest.NewRecorder()
	d.mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/docs/", nil))
	if rec.Code != http.StatusOK {
		t.Errorf("GET /docs/ status = %d, want 200", rec.Code)
	}
}

// ---------------------------------------------------------------------------
// Start / Stop: end-to-end on 127.0.0.1 with a random port
// ---------------------------------------------------------------------------

func TestStart_EndToEndRequests(t *testing.T) {
	d := &stdDriver{mux: http.NewServeMux()}
	d.Handle(http.MethodGet, "/users/42", func(w http.ResponseWriter, r *http.Request) {
		// Path, query, header and body binding over a real socket.
		name := r.URL.Query().Get("name")
		h := r.Header.Get("X-Test-Header")
		w.Header().Set("X-Resp-Header", "resp")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("user-42 name=" + name + " header=" + h))
	})
	d.Handle(http.MethodPost, "/echo", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
	})
	d.Handle(http.MethodGet, "/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	baseURL, _ := startServer(t, d)
	client := &http.Client{Timeout: 2 * time.Second}

	// GET with query and header binding.
	req, err := http.NewRequest(http.MethodGet, baseURL+"/users/42?name=alice", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("X-Test-Header", "hv")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("GET /users/42: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET status = %d, want 200", resp.StatusCode)
	}
	if got := string(body); got != "user-42 name=alice header=hv" {
		t.Errorf("GET body = %q", got)
	}
	if resp.Header.Get("X-Resp-Header") != "resp" {
		t.Errorf("response header X-Resp-Header = %q, want resp", resp.Header.Get("X-Resp-Header"))
	}

	// POST with body echo.
	resp2, err := client.Post(baseURL+"/echo", "text/plain", strings.NewReader("hello-std"))
	if err != nil {
		t.Fatalf("POST /echo: %v", err)
	}
	body2, _ := io.ReadAll(resp2.Body)
	resp2.Body.Close()
	if resp2.StatusCode != http.StatusOK || string(body2) != "hello-std" {
		t.Errorf("POST: status=%d body=%q", resp2.StatusCode, body2)
	}

	// Method mismatch is a 405 over the wire.
	resp3, err := client.Post(baseURL+"/users/42", "text/plain", bytes.NewReader(nil))
	if err != nil {
		t.Fatalf("POST /users/42: %v", err)
	}
	io.Copy(io.Discard, resp3.Body)
	resp3.Body.Close()
	if resp3.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("POST /users/42 status = %d, want 405", resp3.StatusCode)
	}

	// Unregistered path is a 404.
	resp4, err := client.Get(baseURL + "/missing")
	if err != nil {
		t.Fatalf("GET /missing: %v", err)
	}
	io.Copy(io.Discard, resp4.Body)
	resp4.Body.Close()
	if resp4.StatusCode != http.StatusNotFound {
		t.Errorf("GET /missing status = %d, want 404", resp4.StatusCode)
	}
}

func TestStart_ContextCancelShutsDown(t *testing.T) {
	d := &stdDriver{mux: http.NewServeMux()}
	d.Handle(http.MethodGet, "/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	baseURL, cancel := startServer(t, d)

	// Cancelling the Start context must trigger a graceful Shutdown.
	cancel()

	// Poll until the server stops accepting connections (bounded).
	client := &http.Client{Timeout: 2 * time.Second}
	deadline := time.Now().Add(3 * time.Second)
	for {
		_, err := client.Get(baseURL + "/health")
		if err != nil {
			break // connection refused/reset: shutdown completed
		}
		if time.Now().After(deadline) {
			t.Fatal("server still accepting connections 3s after context cancel")
		}
		time.Sleep(10 * time.Millisecond)
	}

	if d.server == nil {
		t.Error("server should be set after Start")
	}
}

func TestStart_ReturnsServeError(t *testing.T) {
	d := &stdDriver{mux: http.NewServeMux()}

	// A pre-closed listener makes http.Server.Serve fail immediately.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := ln.Addr()
	if err := ln.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := d.Start(ctx, &deadListener{addr: addr}); err == nil {
		t.Error("Start should return the Serve error for a dead listener")
	}
}

// deadListener is a listener whose Accept always fails, forcing Serve to
// return an error on the first call.
type deadListener struct{ addr net.Addr }

func (l *deadListener) Accept() (net.Conn, error) {
	return nil, net.ErrClosed
}
func (l *deadListener) Close() error   { return nil }
func (l *deadListener) Addr() net.Addr { return l.addr }

func TestStop_BeforeStartReturnsNil(t *testing.T) {
	d := &stdDriver{mux: http.NewServeMux()}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := d.Stop(ctx); err != nil {
		t.Errorf("Stop before Start = %v, want nil", err)
	}
}

func TestStop_AfterStartShutsDown(t *testing.T) {
	d := &stdDriver{mux: http.NewServeMux()}
	d.Handle(http.MethodGet, "/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	baseURL := "http://" + ln.Addr().String()

	// Start blocks until Stop/cancel; run it in the background.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errChan := make(chan error, 1)
	go func() { errChan <- d.Start(ctx, ln) }()

	// Wait for readiness.
	client := &http.Client{Timeout: 2 * time.Second}
	deadline := time.Now().Add(3 * time.Second)
	for {
		resp, err := client.Get(baseURL + "/health")
		if err == nil {
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("driver not ready: %v", err)
		}
		time.Sleep(5 * time.Millisecond)
	}

	// Stop must shut the server down and unblock Start (with nil error).
	stopCtx, stopCancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer stopCancel()
	if err := d.Stop(stopCtx); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	select {
	case err := <-errChan:
		if err != nil {
			t.Errorf("Start after Stop = %v, want nil", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Start did not return after Stop")
	}

	// The socket must be closed now.
	if _, err := client.Get(baseURL + "/health"); err == nil {
		t.Error("request after Stop should fail, but succeeded")
	}
}
