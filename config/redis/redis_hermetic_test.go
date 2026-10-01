package redis

import (
	"context"
	"net"
	"strings"
	"testing"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// fakeClient implements just enough of goredis.UniversalClient to drive Load
// without a server. All other interface methods would panic on the nil
// embedded interface, which only proves they are never called.
type fakeClient struct {
	goredis.UniversalClient

	val []byte
	err error
}

func (f *fakeClient) Get(_ context.Context, _ string) *goredis.StringCmd {
	cmd := goredis.NewStringCmd(context.Background())
	if f.err != nil {
		cmd.SetErr(f.err)
		return cmd
	}
	cmd.SetVal(string(f.val))
	return cmd
}

// lazyClient returns a client that is safe to construct but never connects
// until a command is issued (points at a closed local port).
func lazyClient(t *testing.T) goredis.UniversalClient {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := ln.Addr().String()
	_ = ln.Close() // nothing listens there anymore: any dial fails fast

	return goredis.NewClient(&goredis.Options{
		Addr:        addr,
		DialTimeout: 500 * time.Millisecond,
		MaxRetries:  -1, // no retries: fail on first dial error
	})
}

// ---------------------------------------------------------------------------
// Options
// ---------------------------------------------------------------------------

func TestOptions_Apply(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	o := &options{}
	WithContext(ctx)(o)
	WithPath("myapp:config")(o)

	if o.ctx != ctx {
		t.Error("WithContext should set ctx")
	}
	if o.path != "myapp:config" {
		t.Errorf("path = %q", o.path)
	}
}

// ---------------------------------------------------------------------------
// New: validation and success
// ---------------------------------------------------------------------------

func TestNew_MissingPath(t *testing.T) {
	// A non-nil (lazy) client without a path option must fail validation.
	if _, err := New(lazyClient(t)); err == nil {
		t.Fatal("expected error for empty path")
	}
}

func TestNew_SuccessStoresOptions(t *testing.T) {
	s, err := New(lazyClient(t), WithPath("myapp:config"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if s.options.path != "myapp:config" {
		t.Errorf("path = %q", s.options.path)
	}
	if s.options.ctx == nil {
		t.Error("ctx should default to context.Background()")
	}
	if s.client == nil {
		t.Error("client should be stored")
	}
}

func TestResolveKey_FallbackAndOverride(t *testing.T) {
	s := &source{options: &options{path: "myapp:config"}}
	if got := s.resolveKey(""); got != "myapp:config" {
		t.Errorf("resolveKey(\"\") = %q, want myapp:config", got)
	}
	if got := s.resolveKey("override"); got != "override" {
		t.Errorf("resolveKey(override) = %q, want override", got)
	}
}

// ---------------------------------------------------------------------------
// Load with a canned client (no server involved)
// ---------------------------------------------------------------------------

func TestLoad_ExplicitAndDefaultKey(t *testing.T) {
	fake := &fakeClient{val: []byte(`{"port":9090}`)}
	s := &source{client: fake, options: &options{path: "myapp:config"}}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	got, err := s.Load(ctx, "")
	if err != nil {
		t.Fatalf("Load default key: %v", err)
	}
	if string(got) != `{"port":9090}` {
		t.Errorf("Load = %q", got)
	}

	got, err = s.Load(ctx, "other:key")
	if err != nil {
		t.Fatalf("Load explicit key: %v", err)
	}
	if string(got) != `{"port":9090}` {
		t.Errorf("Load = %q", got)
	}
}

func TestLoad_KeyMissingReturnsNilNil(t *testing.T) {
	fake := &fakeClient{err: goredis.Nil}
	s := &source{client: fake, options: &options{path: "myapp:config"}}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	got, err := s.Load(ctx, "")
	if err != nil {
		t.Fatalf("Load with missing key should not error, got %v", err)
	}
	if got != nil {
		t.Errorf("Load = %q, want nil", got)
	}
}

func TestLoad_OtherErrorWrapped(t *testing.T) {
	fake := &fakeClient{err: context.DeadlineExceeded}
	s := &source{client: fake, options: &options{path: "myapp:config"}}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	_, err := s.Load(ctx, "")
	if err == nil || !strings.Contains(err.Error(), "redis get myapp:config") {
		t.Errorf("Load error = %v, want wrapped 'redis get myapp:config'", err)
	}
}

// ---------------------------------------------------------------------------
// WatchValue: subscribe failure path (no server needed)
// ---------------------------------------------------------------------------

func TestWatchValue_SubscribeError(t *testing.T) {
	// The client points at a closed local port, so the subscription
	// confirmation never arrives and WatchValue must fail promptly.
	s, err := New(lazyClient(t), WithPath("myapp:config"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, err = s.WatchValue(ctx, "")
	if err == nil {
		t.Fatal("expected subscribe error without a reachable server")
	}
	if !strings.Contains(err.Error(), "redis subscribe") {
		t.Errorf("error = %v, want wrapped 'redis subscribe'", err)
	}
}

func TestWatchValue_EmptyKeyRejected(t *testing.T) {
	// Both the explicit key and the configured path are empty: WatchValue
	// must reject the request instead of proceeding to Subscribe with
	// channel "__windcfg__:" (which would panic on a nil client), matching
	// the "no object key specified" guard in config/oss.
	s := &source{options: &options{}}

	_, err := s.WatchValue(context.Background(), "")
	if err == nil || !strings.Contains(err.Error(), "no redis key specified") {
		t.Errorf("WatchValue error = %v, want 'no redis key specified'", err)
	}

	// The guard must not trigger when the resolved key is non-empty: with an
	// explicit key and a client pointing at a closed port the failure comes
	// from the subscription attempt instead.
	s2 := &source{client: lazyClient(t), options: &options{}}
	_, err = s2.WatchValue(context.Background(), "explicit:key")
	if err == nil || !strings.Contains(err.Error(), "redis subscribe") {
		t.Errorf("WatchValue(explicit key) error = %v, want wrapped 'redis subscribe'", err)
	}
}
