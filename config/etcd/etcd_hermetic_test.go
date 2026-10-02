package etcd

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
)

// unreachableClient returns a client pointed at a closed port on localhost.
// clientv3.New without WithBlock does not dial, so construction is hermetic;
// any operation fails fast with a connection error.
func unreachableClient(t *testing.T) *clientv3.Client {
	t.Helper()
	cli, err := clientv3.New(clientv3.Config{
		Endpoints: []string{"127.0.0.1:1"},
	})
	if err != nil {
		t.Fatalf("clientv3.New() error = %v", err)
	}
	t.Cleanup(func() { _ = cli.Close() })
	return cli
}

// ---------------------------------------------------------------------------
// New / options
// ---------------------------------------------------------------------------

func TestNew_MissingPath(t *testing.T) {
	cli := unreachableClient(t)
	if _, err := New(cli); err == nil {
		t.Fatal("New() without a path must fail")
	}
}

func TestNew_Defaults(t *testing.T) {
	cli := unreachableClient(t)
	src, err := New(cli, WithPath("/app/config"))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if src.options.path != "/app/config" {
		t.Errorf("path = %q, want /app/config", src.options.path)
	}
	if src.options.prefix {
		t.Error("prefix should default to false")
	}
	if src.options.ctx == nil {
		t.Error("ctx should default to context.Background()")
	}
	if src.client != cli {
		t.Error("client not stored")
	}
}

func TestOptions_Setters(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	o := &options{}
	WithContext(ctx)(o)
	if o.ctx != ctx {
		t.Error("WithContext did not store the context")
	}
	WithPath("/p")(o)
	if o.path != "/p" {
		t.Errorf("path = %q, want /p", o.path)
	}
	WithPrefix(true)(o)
	if !o.prefix {
		t.Error("WithPrefix(true) did not enable prefix")
	}
}

// ---------------------------------------------------------------------------
// resolveKey / getConfigKey
// ---------------------------------------------------------------------------

func TestResolveKey(t *testing.T) {
	src := &source{options: &options{path: "/default/path"}}

	if got := src.resolveKey(""); got != "/default/path" {
		t.Errorf("resolveKey(\"\") = %q, want the configured path", got)
	}
	if got := src.resolveKey("/explicit"); got != "/explicit" {
		t.Errorf("resolveKey(\"/explicit\") = %q, want /explicit", got)
	}
}

func TestGetConfigKey(t *testing.T) {
	tests := []struct {
		key          string
		useBackslash bool
		want         string
	}{
		{"a.b.c", true, "a/b/c"},
		{"a.b.c", false, "a.b.c"},
		{"plain", true, "plain"},
		{"", false, ""},
	}
	for _, tt := range tests {
		if got := getConfigKey(tt.key, tt.useBackslash); got != tt.want {
			t.Errorf("getConfigKey(%q, %v) = %q, want %q", tt.key, tt.useBackslash, got, tt.want)
		}
	}
}

// ---------------------------------------------------------------------------
// connection-error classification
// ---------------------------------------------------------------------------

func TestIsConnError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"deadline", context.DeadlineExceeded, true},
		{"canceled", context.Canceled, true},
		{"refused", errors.New("dial tcp: connection refused"), true},
		{"reset", errors.New("read: connection reset by peer"), true},
		{"no endpoints", errors.New("no available endpoints"), true},
		{"transport closing", errors.New("transport is closing"), true},
		{"io timeout", errors.New("i/o timeout"), true},
		{"tls", errors.New("tls: bad certificate"), true},
		{"eof", errors.New("EOF"), true},
		{"unrelated", errors.New("some business error"), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isConnError(tt.err); got != tt.want {
				t.Errorf("isConnError(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

func TestWrapConnError(t *testing.T) {
	if got := wrapConnError("get key", "/p", nil); got != nil {
		t.Errorf("wrapConnError(nil) = %v, want nil", got)
	}

	inner := errors.New("connection refused")
	err := wrapConnError("get key", "/p", inner)
	if err == nil || !strings.Contains(err.Error(), "get key") ||
		!strings.Contains(err.Error(), "/p") ||
		!strings.Contains(err.Error(), "cannot reach etcd server") {
		t.Errorf("wrapConnError with path = %v, want wrapped op/path/server message", err)
	}

	err = wrapConnError("get key", "", inner)
	if err == nil || !strings.Contains(err.Error(), "cannot reach etcd server") ||
		strings.Contains(err.Error(), "for path") {
		t.Errorf("wrapConnError without path = %v, want pathless wrapped message", err)
	}

	unrelated := errors.New("plain failure")
	if got := wrapConnError("op", "/p", unrelated); got != unrelated {
		t.Errorf("wrapConnError(non-conn error) = %v, want passthrough", got)
	}
}

// ---------------------------------------------------------------------------
// Load / WatchValue against an unreachable endpoint
// ---------------------------------------------------------------------------

func TestLoad_UnreachableServer(t *testing.T) {
	cli := unreachableClient(t)
	src, err := New(cli, WithPath("/app/config"))
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	if _, err := src.Load(ctx, ""); err == nil {
		t.Fatal("Load() expected a connection error against an unreachable endpoint")
	}
}

func TestLoad_PrefixMode_UnreachableServer(t *testing.T) {
	cli := unreachableClient(t)
	src, err := New(cli, WithPath("/app/config"), WithPrefix(true))
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	if _, err := src.Load(ctx, ""); err == nil {
		t.Fatal("Load() expected a connection error in prefix mode")
	}
}

func TestWatchValue_UnreachableServer(t *testing.T) {
	cli := unreachableClient(t)
	src, err := New(cli, WithPath("/app/config"))
	if err != nil {
		t.Fatal(err)
	}

	// The 2s reachability probe inside WatchValue must fail fast.
	start := time.Now()
	_, err = src.WatchValue(context.Background(), "")
	if err == nil {
		t.Fatal("WatchValue() expected a connection error against an unreachable endpoint")
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Errorf("WatchValue() took %v to fail, want a fast probe failure", elapsed)
	}
}
