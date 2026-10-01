package oss

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// newTestClient builds an S3 client pointed at a local test endpoint.
// Credentials are anonymous (the fake server ignores auth) and no real OSS/S3
// endpoint is ever contacted.
func newTestClient(endpoint string) *awss3.Client {
	return awss3.NewFromConfig(aws.Config{
		Region:       "us-east-1",
		BaseEndpoint: aws.String(endpoint),
		Credentials:  aws.AnonymousCredentials{},
	})
}

// fakeS3 serves GetObject/HeadObject for a single object whose content and
// ETag can be mutated concurrently (used to drive the WatchValue poll loop).
type fakeS3 struct {
	mu      sync.Mutex
	etag    string
	content []byte

	gets  int
	heads int
}

func (f *fakeS3) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	etag, content := f.etag, f.content
	f.gets++
	f.mu.Unlock()

	switch r.Method {
	case http.MethodGet:
		w.Header().Set("ETag", etag)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(content)
	case http.MethodHead:
		w.Header().Set("ETag", etag)
		w.WriteHeader(http.StatusOK)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (f *fakeS3) set(etag string, content []byte) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.etag, f.content = etag, content
}

func (f *fakeS3) stats() (gets, heads int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.gets, f.heads
}

// readChan reads one value from a watch channel, bounded by timeout.
func readChan(t *testing.T, ch <-chan []byte, what string) []byte {
	t.Helper()
	select {
	case v, ok := <-ch:
		if !ok {
			t.Fatalf("%s: channel closed unexpectedly", what)
		}
		return v
	case <-time.After(3 * time.Second):
		t.Fatalf("%s: timed out waiting for value", what)
		return nil
	}
}

// ---------------------------------------------------------------------------
// Options
// ---------------------------------------------------------------------------

func TestOptions_Apply(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	o := &options{pollInterval: defaultPollInterval}
	WithContext(ctx)(o)
	WithBucket("my-bucket")(o)
	WithKey("configs/app.yaml")(o)
	WithPollInterval(5 * time.Second)(o)

	if o.ctx != ctx {
		t.Error("WithContext should set ctx")
	}
	if o.bucket != "my-bucket" {
		t.Errorf("bucket = %q", o.bucket)
	}
	if o.key != "configs/app.yaml" {
		t.Errorf("key = %q", o.key)
	}
	if o.pollInterval != 5*time.Second {
		t.Errorf("pollInterval = %v, want 5s", o.pollInterval)
	}
}

func TestWithPollInterval_NonPositiveIgnored(t *testing.T) {
	o := &options{pollInterval: defaultPollInterval}
	WithPollInterval(0)(o)
	WithPollInterval(-1 * time.Second)(o)
	if o.pollInterval != defaultPollInterval {
		t.Errorf("pollInterval = %v, want unchanged %v", o.pollInterval, defaultPollInterval)
	}
}

func TestDefaultPollInterval(t *testing.T) {
	if defaultPollInterval != 30*time.Second {
		t.Errorf("defaultPollInterval = %v, want 30s", defaultPollInterval)
	}
}

// ---------------------------------------------------------------------------
// New: validation and defaults
// ---------------------------------------------------------------------------

func TestNew_NilClientRejected(t *testing.T) {
	if _, err := New(nil, WithBucket("b")); err == nil {
		t.Fatal("expected error for nil client")
	}
}

func TestNew_MissingBucket(t *testing.T) {
	// A valid (lazy) client with no bucket option must fail validation.
	if _, err := New(newTestClient("http://127.0.0.1:1")); err == nil {
		t.Fatal("expected error for missing bucket")
	}
}

func TestNew_SuccessAppliesDefaults(t *testing.T) {
	s, err := New(newTestClient("http://127.0.0.1:1"), WithBucket("b"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if s.options.pollInterval != defaultPollInterval {
		t.Errorf("pollInterval = %v, want default %v", s.options.pollInterval, defaultPollInterval)
	}
	if s.options.ctx == nil {
		t.Error("ctx should default to context.Background()")
	}
	if s.client == nil {
		t.Error("client should be stored")
	}
}

func TestResolveKey_FallbackAndOverride(t *testing.T) {
	s := &source{options: &options{key: "default.yaml"}}
	if got := s.resolveKey(""); got != "default.yaml" {
		t.Errorf("resolveKey(\"\") = %q, want default.yaml", got)
	}
	if got := s.resolveKey("override.yaml"); got != "override.yaml" {
		t.Errorf("resolveKey(override) = %q, want override.yaml", got)
	}
}

// ---------------------------------------------------------------------------
// Load against a local fake S3 endpoint
// ---------------------------------------------------------------------------

func TestLoad_ExplicitAndDefaultKey(t *testing.T) {
	fake := &fakeS3{etag: `"v1"`, content: []byte("hello-config")}
	srv := httptest.NewServer(fake)
	defer srv.Close()

	s, err := New(newTestClient(srv.URL), WithBucket("cfg-bucket"), WithKey("configs/app.yaml"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	// Explicit key.
	got, err := s.Load(ctx, "explicit.yaml")
	if err != nil {
		t.Fatalf("Load explicit: %v", err)
	}
	if string(got) != "hello-config" {
		t.Errorf("Load = %q, want hello-config", got)
	}

	// Empty key falls back to the configured default.
	got, err = s.Load(ctx, "")
	if err != nil {
		t.Fatalf("Load default key: %v", err)
	}
	if string(got) != "hello-config" {
		t.Errorf("Load = %q, want hello-config", got)
	}
}

func TestLoad_EmptyResolvedKey(t *testing.T) {
	// No client call happens; the error must come from key resolution.
	s := &source{client: nil, options: &options{}}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_, err := s.Load(ctx, "")
	if err == nil || !strings.Contains(err.Error(), "no object key specified") {
		t.Errorf("Load with no key = %v, want 'no object key specified'", err)
	}
}

func TestLoad_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`<Error><Code>NoSuchKey</Code></Error>`))
	}))
	defer srv.Close()

	s, err := New(newTestClient(srv.URL), WithBucket("b"), WithKey("k"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_, err = s.Load(ctx, "k")
	if err == nil {
		t.Fatal("expected error for missing object")
	}
	if !strings.Contains(err.Error(), "s3 get object") {
		t.Errorf("error should be wrapped as 's3 get object', got %v", err)
	}
}

// ---------------------------------------------------------------------------
// WatchValue: initial push plus ETag-change driven updates
// ---------------------------------------------------------------------------

func TestWatchValue_PollsOnETagChange(t *testing.T) {
	fake := &fakeS3{etag: `"v1"`, content: []byte("content-v1")}
	srv := httptest.NewServer(fake)
	defer srv.Close()

	s, err := New(newTestClient(srv.URL),
		WithBucket("cfg-bucket"),
		WithKey("configs/app.yaml"),
		WithPollInterval(15*time.Millisecond),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	ch, err := s.WatchValue(ctx, "")
	if err != nil {
		t.Fatalf("WatchValue: %v", err)
	}

	// Initial load is pushed immediately.
	if got := readChan(t, ch, "initial"); string(got) != "content-v1" {
		t.Errorf("initial value = %q, want content-v1", got)
	}

	// Change the ETag: the next poll tick must pick up the new content.
	fake.set(`"v2"`, []byte("content-v2"))
	if got := readChan(t, ch, "after-change"); string(got) != "content-v2" {
		t.Errorf("updated value = %q, want content-v2", got)
	}

	// No ETag change -> no further values; verify with a short window.
	select {
	case v, ok := <-ch:
		t.Errorf("unexpected extra value %q (ok=%v) without ETag change", v, ok)
	case <-time.After(80 * time.Millisecond):
	}
}

func TestWatchValue_EmptyResolvedKey(t *testing.T) {
	s := &source{client: nil, options: &options{}}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_, err := s.WatchValue(ctx, "")
	if err == nil || !strings.Contains(err.Error(), "no object key specified") {
		t.Errorf("WatchValue with no key = %v, want 'no object key specified'", err)
	}
}

func TestWatchValue_ChannelClosedOnCancel(t *testing.T) {
	fake := &fakeS3{etag: `"v1"`, content: []byte("c")}
	srv := httptest.NewServer(fake)
	defer srv.Close()

	s, err := New(newTestClient(srv.URL),
		WithBucket("b"),
		WithKey("k"),
		WithPollInterval(10*time.Millisecond),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	ch, err := s.WatchValue(ctx, "k")
	if err != nil {
		t.Fatalf("WatchValue: %v", err)
	}
	_ = readChan(t, ch, "initial")

	cancel()
	select {
	case _, ok := <-ch:
		if ok {
			t.Error("channel should be closed after ctx cancel, got a value")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("channel not closed within 3s after cancel")
	}
}
