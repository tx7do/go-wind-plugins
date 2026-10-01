package vault

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	vaultapi "github.com/hashicorp/vault/api"
)

// newTestClient builds a Vault API client pointed at addr without dialing.
// Retries are disabled so unreachable endpoints fail promptly.
func newTestClient(t *testing.T, addr string) *vaultapi.Client {
	t.Helper()
	client, err := vaultapi.NewClient(&vaultapi.Config{
		Address:    addr,
		MaxRetries: 0,
		Timeout:    5 * time.Second,
	})
	if err != nil {
		t.Fatalf("failed to create vault client: %v", err)
	}
	return client
}

const testPath = "secret/data/myapp/config"

// ---------------------------------------------------------------------------
// Constructor and options
// ---------------------------------------------------------------------------

func TestNew_NilClient(t *testing.T) {
	if _, err := New(nil, WithPath(testPath)); err == nil {
		t.Fatal("New with a nil client should fail")
	}
}

func TestNew_MissingPath(t *testing.T) {
	if _, err := New(newTestClient(t, "http://127.0.0.1:1")); err == nil {
		t.Fatal("New without a path should fail")
	}
}

func TestNew_SuccessWithDefaults(t *testing.T) {
	s, err := New(newTestClient(t, "http://127.0.0.1:1"), WithPath(testPath))
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	if s == nil {
		t.Fatal("New returned nil source")
	}
	if s.options.dataKey != "content" {
		t.Errorf("default dataKey = %q, want %q", s.options.dataKey, "content")
	}
	if s.options.pollInterval != defaultPollInterval {
		t.Errorf("default pollInterval = %v, want %v", s.options.pollInterval, defaultPollInterval)
	}
	if defaultPollInterval != 30*time.Second {
		t.Errorf("defaultPollInterval = %v, want 30s", defaultPollInterval)
	}
}

func TestOptions(t *testing.T) {
	o := &options{}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	WithContext(ctx)(o)
	if o.ctx != ctx {
		t.Error("WithContext should store the provided context")
	}

	WithPath("secret/data/foo")(o)
	if o.path != "secret/data/foo" {
		t.Errorf("WithPath set path = %q, want %q", o.path, "secret/data/foo")
	}

	WithDataKey("payload")(o)
	if o.dataKey != "payload" {
		t.Errorf("WithDataKey set dataKey = %q, want %q", o.dataKey, "payload")
	}

	WithPollInterval(time.Minute)(o)
	if o.pollInterval != time.Minute {
		t.Errorf("WithPollInterval set pollInterval = %v, want %v", o.pollInterval, time.Minute)
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
		{"explicit key wins", "secret/data/default", "secret/data/explicit", "secret/data/explicit"},
		{"empty key falls back", "secret/data/default", "", "secret/data/default"},
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
// extractValue / bytesEqual pure helpers
// ---------------------------------------------------------------------------

func TestExtractValue(t *testing.T) {
	tests := []struct {
		name    string
		dataKey string
		data    map[string]interface{}
		want    string
	}{
		{
			name:    "flat string value",
			dataKey: "content",
			data:    map[string]interface{}{"content": "hello"},
			want:    "hello",
		},
		{
			name:    "kv v2 wrapper is unwrapped",
			dataKey: "content",
			data: map[string]interface{}{
				"data":     map[string]interface{}{"content": "kv2-value"},
				"metadata": map[string]interface{}{"version": 1},
			},
			want: "kv2-value",
		},
		{
			name:    "custom data key",
			dataKey: "payload",
			data:    map[string]interface{}{"payload": "custom", "content": "ignored"},
			want:    "custom",
		},
		{
			name:    "byte value",
			dataKey: "content",
			data:    map[string]interface{}{"content": []byte("raw-bytes")},
			want:    "raw-bytes",
		},
		{
			name:    "number is json encoded",
			dataKey: "content",
			data:    map[string]interface{}{"content": 42},
			want:    "42",
		},
		{
			name:    "map is json encoded",
			dataKey: "content",
			data:    map[string]interface{}{"content": map[string]interface{}{"a": "b"}},
			want:    `{"a":"b"}`,
		},
		{
			name:    "missing data key falls back to whole map",
			dataKey: "absent",
			data:    map[string]interface{}{"other": "value"},
			want:    `{"other":"value"}`,
		},
		{
			name:    "kv v2 unwrap with missing inner key falls back to unwrapped map",
			dataKey: "absent",
			data: map[string]interface{}{
				"data": map[string]interface{}{"inner": "val"},
			},
			want: `{"inner":"val"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractValue(tt.data, tt.dataKey)
			if string(got) != tt.want {
				t.Errorf("extractValue = %q, want %q", string(got), tt.want)
			}
		})
	}
}

func TestBytesEqual(t *testing.T) {
	tests := []struct {
		name string
		a    []byte
		b    []byte
		want bool
	}{
		{"both nil", nil, nil, true},
		{"nil vs empty", nil, []byte{}, true},
		{"equal content", []byte("abc"), []byte("abc"), true},
		{"different lengths", []byte("abc"), []byte("abcd"), false},
		{"different content", []byte("abc"), []byte("abd"), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := bytesEqual(tt.a, tt.b); got != tt.want {
				t.Errorf("bytesEqual(%q, %q) = %v, want %v", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Load against a stubbed Vault HTTP API
// ---------------------------------------------------------------------------

// vaultReadHandler emulates a KV v2 secret read on GET /v1/<path>.
func vaultReadHandler(content string, statusCode int) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/v1/") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(statusCode)
		if statusCode == http.StatusOK {
			body := map[string]interface{}{
				"request_id": "test-request-id",
				"data": map[string]interface{}{
					"data": map[string]interface{}{
						"content": content,
					},
					"metadata": map[string]interface{}{
						"version": 1,
					},
				},
			}
			_ = json.NewEncoder(w).Encode(body)
		}
	})
}

func TestLoad_Success(t *testing.T) {
	srv := httptest.NewServer(vaultReadHandler("vault-config-payload", http.StatusOK))
	defer srv.Close()

	s, err := New(newTestClient(t, srv.URL), WithPath(testPath))
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	data, err := s.Load(context.Background(), "")
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if string(data) != "vault-config-payload" {
		t.Errorf("Load = %q, want %q", string(data), "vault-config-payload")
	}
}

func TestLoad_CustomDataKey(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		body := map[string]interface{}{
			"data": map[string]interface{}{
				"data": map[string]interface{}{
					"config": "custom-key-payload",
				},
			},
		}
		_ = json.NewEncoder(w).Encode(body)
	}))
	defer srv.Close()

	s, err := New(newTestClient(t, srv.URL), WithPath(testPath), WithDataKey("config"))
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	data, err := s.Load(context.Background(), "")
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if string(data) != "custom-key-payload" {
		t.Errorf("Load = %q, want %q", string(data), "custom-key-payload")
	}
}

func TestLoad_ExplicitKeyOverridesPath(t *testing.T) {
	srv := httptest.NewServer(vaultReadHandler("explicit-path-value", http.StatusOK))
	defer srv.Close()

	s, err := New(newTestClient(t, srv.URL), WithPath(testPath))
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	data, err := s.Load(context.Background(), "secret/data/other")
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if string(data) != "explicit-path-value" {
		t.Errorf("Load = %q, want %q", string(data), "explicit-path-value")
	}
}

func TestLoad_SecretNotFound(t *testing.T) {
	// A 404 from Vault means "secret does not exist": Load returns (nil, nil).
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	s, err := New(newTestClient(t, srv.URL), WithPath(testPath))
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	data, err := s.Load(context.Background(), "")
	if err != nil {
		t.Fatalf("Load should not fail for a missing secret: %v", err)
	}
	if data != nil {
		t.Errorf("Load = %q, want nil for missing secret", string(data))
	}
}

func TestLoad_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	s, err := New(newTestClient(t, srv.URL), WithPath(testPath))
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	data, err := s.Load(context.Background(), "")
	if err == nil {
		t.Fatal("Load should fail when Vault returns a server error")
	}
	if data != nil {
		t.Errorf("Load = %q, want nil on error", string(data))
	}
	if !strings.Contains(err.Error(), "vault: read") {
		t.Errorf("Load error = %q, want it to mention %q", err.Error(), "vault: read")
	}
}

func TestLoad_ConnectionError(t *testing.T) {
	// Port 1 on loopback is closed: the request must fail promptly instead
	// of hanging.
	s, err := New(newTestClient(t, "http://127.0.0.1:1"), WithPath(testPath))
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
// WatchValue (poll-based)
// ---------------------------------------------------------------------------

func TestWatchValue_PushesInitialValueAndChange(t *testing.T) {
	// The first read returns "initial-value", all later reads return
	// "changed-value". The poller pushes the initial value immediately and
	// the changed value on the next tick.
	var reads atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		content := "initial-value"
		if reads.Add(1) > 1 {
			content = "changed-value"
		}
		w.Header().Set("Content-Type", "application/json")
		body := map[string]interface{}{
			"data": map[string]interface{}{
				"data": map[string]interface{}{
					"content": content,
				},
			},
		}
		_ = json.NewEncoder(w).Encode(body)
	}))
	defer srv.Close()

	s, err := New(
		newTestClient(t, srv.URL),
		WithPath(testPath),
		WithPollInterval(20*time.Millisecond),
	)
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
	case <-time.After(5 * time.Second):
		t.Error("watch channel was not closed within 5s after ctx cancellation")
	}
}

func TestWatchValue_UnreachableEndpointClosesOnCancel(t *testing.T) {
	s, err := New(
		newTestClient(t, "http://127.0.0.1:1"),
		WithPath(testPath),
		WithPollInterval(20*time.Millisecond),
	)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	ch, err := s.WatchValue(ctx, "")
	if err != nil {
		t.Fatalf("WatchValue returned error: %v", err)
	}

	// The initial read fails (endpoint unreachable), so no value is pushed.
	// Cancelling the context must close the channel.
	cancel()
	select {
	case v, ok := <-ch:
		if ok {
			t.Errorf("watch channel should be closed, got value %q", string(v))
		}
	case <-time.After(5 * time.Second):
		t.Error("watch channel was not closed within 5s after ctx cancellation")
	}
}
