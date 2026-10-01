package consul

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/hashicorp/consul/api"
)

// newTestClient builds a consul API client pointed at addr without dialing.
// The consul client is lazy: no connection is made until a request is issued.
func newTestClient(t *testing.T, addr string) *api.Client {
	t.Helper()
	cfg := api.DefaultConfig()
	cfg.Address = addr
	client, err := api.NewClient(cfg)
	if err != nil {
		t.Fatalf("failed to create consul client: %v", err)
	}
	return client
}

const testPath = "app/config"

// ---------------------------------------------------------------------------
// Constructor and options
// ---------------------------------------------------------------------------

func TestNew_NilClient(t *testing.T) {
	if _, err := New(nil, WithPath(testPath)); err == nil {
		t.Fatal("New with a nil client should fail")
	}
}

func TestNew_MissingPath(t *testing.T) {
	client := newTestClient(t, "127.0.0.1:1") // lazy client, never dialed
	if _, err := New(client); err == nil {
		t.Fatal("New without a path should fail")
	}
}

func TestNew_Success(t *testing.T) {
	client := newTestClient(t, "127.0.0.1:1") // lazy client, never dialed
	s, err := New(client, WithPath(testPath))
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	if s == nil {
		t.Fatal("New returned nil source")
	}
	if s.client != client {
		t.Error("New should store the provided client as-is")
	}
	if s.options.path != testPath {
		t.Errorf("options.path = %q, want %q", s.options.path, testPath)
	}
}

func TestOptions(t *testing.T) {
	o := &options{ctx: context.Background()}

	WithPath("some/path")(o)
	if o.path != "some/path" {
		t.Errorf("WithPath set path = %q, want %q", o.path, "some/path")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	WithContext(ctx)(o)
	if o.ctx != ctx {
		t.Error("WithContext should store the provided context")
	}
}

// ---------------------------------------------------------------------------
// resolveKey
// ---------------------------------------------------------------------------

func TestResolveKey(t *testing.T) {
	tests := []struct {
		name       string
		configured string
		key        string
		want       string
	}{
		{"explicit key wins", "default/path", "explicit/key", "explicit/key"},
		{"empty key falls back to configured path", "default/path", "", "default/path"},
		{"both empty", "", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &source{options: &options{path: tt.configured}}
			if got := s.resolveKey(tt.key); got != tt.want {
				t.Errorf("resolveKey(%q) = %q, want %q", tt.key, got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// getConfigKey helper
// ---------------------------------------------------------------------------

func TestGetConfigKey(t *testing.T) {
	tests := []struct {
		name         string
		key          string
		useBackslash bool
		want         string
	}{
		{"dots kept when not converting", "a.b.c", false, "a.b.c"},
		{"dots replaced when converting", "a.b.c", true, "a/b/c"},
		{"no dots", "abc", true, "abc"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := getConfigKey(tt.key, tt.useBackslash); got != tt.want {
				t.Errorf("getConfigKey(%q, %v) = %q, want %q", tt.key, tt.useBackslash, got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Load against a stubbed Consul HTTP API
// ---------------------------------------------------------------------------

// kvHandler serves a single KV entry on GET /v1/kv/<path>.
func kvHandler(path, value string, statusCode int) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/kv/"+path {
			// Unknown path: emulate consul's "key not found" response.
			w.Header().Set("X-Consul-Index", "7")
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("X-Consul-Index", "7")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(statusCode)
		if statusCode == http.StatusOK {
			pair := map[string]interface{}{
				"Key":         path,
				"Value":       base64.StdEncoding.EncodeToString([]byte(value)),
				"CreateIndex": 1,
				"ModifyIndex": 7,
			}
			_ = json.NewEncoder(w).Encode([]interface{}{pair})
		}
	})
}

func TestLoad_Success(t *testing.T) {
	srv := httptest.NewServer(kvHandler(testPath, "hello-consul", http.StatusOK))
	defer srv.Close()

	s, err := New(newTestClient(t, srv.URL), WithPath(testPath))
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	data, err := s.Load(context.Background(), "")
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if string(data) != "hello-consul" {
		t.Errorf("Load = %q, want %q", string(data), "hello-consul")
	}
}

func TestLoad_ExplicitKeyOverridesPath(t *testing.T) {
	srv := httptest.NewServer(kvHandler("explicit/key", "explicit-value", http.StatusOK))
	defer srv.Close()

	s, err := New(newTestClient(t, srv.URL), WithPath(testPath))
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	data, err := s.Load(context.Background(), "explicit/key")
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if string(data) != "explicit-value" {
		t.Errorf("Load = %q, want %q", string(data), "explicit-value")
	}
}

func TestLoad_KeyNotFound(t *testing.T) {
	// A 404 from consul means "key does not exist": Load must return
	// (nil, nil) in that case.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Consul-Index", "7")
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	s, err := New(newTestClient(t, srv.URL), WithPath(testPath))
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	data, err := s.Load(context.Background(), "")
	if err != nil {
		t.Fatalf("Load should not fail for a missing key: %v", err)
	}
	if data != nil {
		t.Errorf("Load = %q, want nil for missing key", string(data))
	}
}

func TestLoad_ConnectionError(t *testing.T) {
	// Port 1 on loopback is closed: the request must fail promptly with an
	// error instead of hanging.
	s, err := New(newTestClient(t, "127.0.0.1:1"), WithPath(testPath))
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	done := make(chan struct{})
	var loadErr error
	go func() {
		defer close(done)
		_, loadErr = s.Load(context.Background(), "")
	}()

	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("Load against an unreachable endpoint did not return within 10s")
	}

	if loadErr == nil {
		t.Error("Load should return an error when the endpoint is unreachable")
	}
}

// ---------------------------------------------------------------------------
// WatchValue against a stubbed Consul HTTP API
// ---------------------------------------------------------------------------

func TestWatchValue_DeliversInitialValueAndChange(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		index := r.URL.Query().Get("index")
		var value, nextIndex string
		switch index {
		case "", "0":
			// First (non-blocking) query: serve the initial value.
			value, nextIndex = "initial-value", "1"
		case "1":
			// Blocking query after the first index: serve the changed value.
			value, nextIndex = "changed-value", "2"
		default:
			// Subsequent blocking queries: hold the connection briefly so
			// the watch does not spin; the client cancels it on ctx.Done.
			select {
			case <-r.Context().Done():
				return
			case <-time.After(2 * time.Second):
			}
			value, nextIndex = "changed-value", "2"
		}

		w.Header().Set("X-Consul-Index", nextIndex)
		w.Header().Set("Content-Type", "application/json")
		pair := map[string]interface{}{
			"Key":         testPath,
			"Value":       base64.StdEncoding.EncodeToString([]byte(value)),
			"CreateIndex": 1,
			"ModifyIndex": 2,
		}
		_ = json.NewEncoder(w).Encode([]interface{}{pair})
	}))
	defer srv.Close()

	s, err := New(newTestClient(t, srv.URL), WithPath(testPath))
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch, err := s.WatchValue(ctx, "")
	if err != nil {
		t.Fatalf("WatchValue returned error: %v", err)
	}
	if ch == nil {
		t.Fatal("WatchValue returned nil channel")
	}

	readValue := func(want string) {
		t.Helper()
		select {
		case got, ok := <-ch:
			if !ok {
				t.Fatalf("watch channel closed early, want %q", want)
			}
			if string(got) != want {
				t.Errorf("watch value = %q, want %q", string(got), want)
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("watch did not deliver %q within 5s", want)
		}
	}

	readValue("initial-value")
	readValue("changed-value")

	// Cancelling the context must close the channel.
	cancel()
	select {
	case _, ok := <-ch:
		if ok {
			t.Error("watch channel should be closed after ctx cancellation")
		}
	case <-time.After(15 * time.Second):
		t.Error("watch channel was not closed within 15s after ctx cancellation")
	}
}

func TestWatchValue_UnreachableEndpointClosesOnCancel(t *testing.T) {
	s, err := New(newTestClient(t, "127.0.0.1:1"), WithPath(testPath))
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	ch, err := s.WatchValue(ctx, "")
	if err != nil {
		t.Fatalf("WatchValue returned error: %v", err)
	}

	// No value should arrive because the endpoint is unreachable. Cancelling
	// the context stops the watch plan and closes the channel.
	cancel()
	select {
	case v, ok := <-ch:
		if ok {
			t.Errorf("watch channel should be closed, got value %q", string(v))
		}
	case <-time.After(15 * time.Second):
		t.Error("watch channel was not closed within 15s after ctx cancellation")
	}
}
