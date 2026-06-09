package health

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// Status
// ---------------------------------------------------------------------------

func TestStatusString(t *testing.T) {
	tests := []struct {
		status Status
		want   string
	}{
		{StatusUp, "up"},
		{StatusDown, "down"},
		{StatusUnknown, "unknown"},
	}
	for _, tt := range tests {
		if got := tt.status.String(); got != tt.want {
			t.Errorf("Status(%d).String() = %q, want %q", tt.status, got, tt.want)
		}
	}
}

// ---------------------------------------------------------------------------
// PingFunc
// ---------------------------------------------------------------------------

func TestPingFunc_NilReturnsUnknown(t *testing.T) {
	var f PingFunc
	r := f.Check(context.Background())
	if r.Status != StatusUnknown {
		t.Errorf("nil PingFunc status = %v, want %v", r.Status, StatusUnknown)
	}
}

func TestPingFunc_SuccessReturnsUp(t *testing.T) {
	f := PingFunc(func(ctx context.Context) error { return nil })
	r := f.Check(context.Background())
	if r.Status != StatusUp {
		t.Errorf("success PingFunc status = %v, want %v", r.Status, StatusUp)
	}
}

func TestPingFunc_ErrorReturnsDown(t *testing.T) {
	f := PingFunc(func(ctx context.Context) error { return errors.New("connection refused") })
	r := f.Check(context.Background())
	if r.Status != StatusDown {
		t.Errorf("error PingFunc status = %v, want %v", r.Status, StatusDown)
	}
	if r.Message != "connection refused" {
		t.Errorf("error PingFunc message = %q, want %q", r.Message, "connection refused")
	}
}

// ---------------------------------------------------------------------------
// Health — Register / Deregister / Names
// ---------------------------------------------------------------------------

func TestHealth_RegisterAndDeregister(t *testing.T) {
	h := New()
	h.Register("redis", PingFunc(func(ctx context.Context) error { return nil }))
	h.Register("db", PingFunc(func(ctx context.Context) error { return nil }))

	names := h.Names()
	if len(names) != 2 {
		t.Fatalf("expected 2 checkers, got %d", len(names))
	}

	h.Deregister("redis")
	names = h.Names()
	if len(names) != 1 {
		t.Fatalf("after deregister expected 1 checker, got %d", len(names))
	}
}

func TestHealth_DeregisterNonExistent(t *testing.T) {
	h := New()
	h.Deregister("does-not-exist") // should be a no-op
	if len(h.Names()) != 0 {
		t.Error("deregister of non-existent checker should be a no-op")
	}
}

func TestHealth_RegisterReplacesExisting(t *testing.T) {
	h := New()
	h.Register("svc", PingFunc(func(ctx context.Context) error { return nil }))
	h.Register("svc", PingFunc(func(ctx context.Context) error { return errors.New("fail") }))
	if len(h.Names()) != 1 {
		t.Fatalf("expected 1 checker after re-register, got %d", len(h.Names()))
	}
}

// ---------------------------------------------------------------------------
// Health — Check aggregation
// ---------------------------------------------------------------------------

func TestHealth_CheckAllUp(t *testing.T) {
	h := New(WithTimeout(2 * time.Second))
	h.Register("a", PingFunc(func(ctx context.Context) error { return nil }))
	h.Register("b", PingFunc(func(ctx context.Context) error { return nil }))

	r := h.Check(context.Background())
	if r.Status != StatusUp {
		t.Errorf("expected StatusUp, got %v", r.Status)
	}
}

func TestHealth_CheckOneDown(t *testing.T) {
	h := New(WithTimeout(2 * time.Second))
	h.Register("a", PingFunc(func(ctx context.Context) error { return nil }))
	h.Register("b", PingFunc(func(ctx context.Context) error { return errors.New("down") }))

	r := h.Check(context.Background())
	if r.Status != StatusDown {
		t.Errorf("expected StatusDown, got %v", r.Status)
	}
}

func TestHealth_CheckNoCheckers(t *testing.T) {
	h := New()
	r := h.Check(context.Background())
	if r.Status != StatusUp {
		t.Errorf("expected StatusUp for no checkers, got %v", r.Status)
	}
	if r.Message == "" {
		t.Error("expected a non-empty message for no checkers")
	}
}

func TestHealth_CheckTimeout(t *testing.T) {
	h := New(WithTimeout(50 * time.Millisecond))
	h.Register("slow", PingFunc(func(ctx context.Context) error {
		select {
		case <-time.After(2 * time.Second):
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}))

	r := h.Check(context.Background())
	if r.Status != StatusDown {
		t.Errorf("expected StatusDown on timeout, got %v", r.Status)
	}
}

// ---------------------------------------------------------------------------
// AllCheckers / AnyCheckers
// ---------------------------------------------------------------------------

func TestAllCheckers_AllPass(t *testing.T) {
	a := AllCheckers(
		PingFunc(func(ctx context.Context) error { return nil }),
		PingFunc(func(ctx context.Context) error { return nil }),
	)
	r := a.Check(context.Background())
	if r.Status != StatusUp {
		t.Errorf("expected StatusUp, got %v", r.Status)
	}
}

func TestAllCheckers_OneFails(t *testing.T) {
	a := AllCheckers(
		PingFunc(func(ctx context.Context) error { return nil }),
		PingFunc(func(ctx context.Context) error { return errors.New("fail") }),
		PingFunc(func(ctx context.Context) error { return nil }),
	)
	r := a.Check(context.Background())
	if r.Status != StatusDown {
		t.Errorf("expected StatusDown, got %v", r.Status)
	}
}

func TestAnyCheckers_AllFail(t *testing.T) {
	a := AnyCheckers(
		PingFunc(func(ctx context.Context) error { return errors.New("fail1") }),
		PingFunc(func(ctx context.Context) error { return errors.New("fail2") }),
	)
	r := a.Check(context.Background())
	if r.Status != StatusDown {
		t.Errorf("expected StatusDown, got %v", r.Status)
	}
}

func TestAnyCheckers_OnePasses(t *testing.T) {
	a := AnyCheckers(
		PingFunc(func(ctx context.Context) error { return errors.New("fail") }),
		PingFunc(func(ctx context.Context) error { return nil }),
	)
	r := a.Check(context.Background())
	if r.Status != StatusUp {
		t.Errorf("expected StatusUp, got %v", r.Status)
	}
}

// ---------------------------------------------------------------------------
// Handler (readiness)
// ---------------------------------------------------------------------------

func TestHandler_AllUp_Returns200(t *testing.T) {
	h := New()
	h.Register("svc", PingFunc(func(ctx context.Context) error { return nil }))

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()

	NewHandler(h).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}

	var resp handlerResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}
	if resp.Status != "up" {
		t.Errorf("expected status 'up', got %q", resp.Status)
	}
}

func TestHandler_Down_Returns503(t *testing.T) {
	h := New()
	h.Register("svc", PingFunc(func(ctx context.Context) error { return errors.New("unreachable") }))

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()

	NewHandler(h).ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", rec.Code)
	}

	var resp handlerResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}
	if resp.Status != "down" {
		t.Errorf("expected status 'down', got %q", resp.Status)
	}
}

// ---------------------------------------------------------------------------
// LivenessHandler
// ---------------------------------------------------------------------------

func TestLivenessHandler_AlwaysReturns200(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()

	NewLivenessHandler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}

	var resp map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}
	if resp["status"] != "up" {
		t.Errorf("expected status 'up', got %q", resp["status"])
	}
}

// ---------------------------------------------------------------------------
// TCPChecker
// ---------------------------------------------------------------------------

func TestTCPChecker_Unreachable(t *testing.T) {
	// Use a port that's almost certainly not listening.
	c := TCP("127.0.0.1:1", 100*time.Millisecond)
	r := c.Check(context.Background())
	if r.Status != StatusDown {
		t.Errorf("expected StatusDown for unreachable TCP, got %v", r.Status)
	}
}

func TestTCPChecker_Reachable(t *testing.T) {
	// Start a local TCP listener.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("cannot create TCP listener: %v", err)
	}
	defer ln.Close()

	c := TCP(ln.Addr().String(), time.Second)
	r := c.Check(context.Background())
	if r.Status != StatusUp {
		t.Errorf("expected StatusUp for reachable TCP, got %v (msg: %s)", r.Status, r.Message)
	}
}

// ---------------------------------------------------------------------------
// HTTPChecker
// ---------------------------------------------------------------------------

func TestHTTPChecker_Healthy(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := HTTP(srv.URL+"/healthz", 2*time.Second)
	r := c.Check(context.Background())
	if r.Status != StatusUp {
		t.Errorf("expected StatusUp, got %v (msg: %s)", r.Status, r.Message)
	}
}

func TestHTTPChecker_Unhealthy(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := HTTP(srv.URL+"/healthz", 2*time.Second)
	r := c.Check(context.Background())
	if r.Status != StatusDown {
		t.Errorf("expected StatusDown, got %v (msg: %s)", r.Status, r.Message)
	}
}

func TestHTTPChecker_Unreachable(t *testing.T) {
	c := HTTP("http://127.0.0.1:1/healthz", 100*time.Millisecond)
	r := c.Check(context.Background())
	if r.Status != StatusDown {
		t.Errorf("expected StatusDown, got %v (msg: %s)", r.Status, r.Message)
	}
}

// ---------------------------------------------------------------------------
// Concurrency safety
// ---------------------------------------------------------------------------

func TestHealth_ConcurrentRegisterAndCheck(t *testing.T) {
	h := New(WithTimeout(time.Second))

	var counter int64
	done := make(chan struct{})

	// Writer goroutine: registers checkers
	go func() {
		defer close(done)
		for i := 0; i < 20; i++ {
			name := fmt.Sprintf("svc-%d", i)
			h.Register(name, PingFunc(func(ctx context.Context) error {
				atomic.AddInt64(&counter, 1)
				return nil
			}))
		}
	}()

	// Reader goroutine: checks concurrently
	for i := 0; i < 10; i++ {
		h.Check(context.Background())
	}

	<-done
	// Final check should include all 20 checkers
	h.Check(context.Background())
}
