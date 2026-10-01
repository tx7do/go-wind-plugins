package zookeeper

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/go-zookeeper/zk"
)

// newTestConn returns a zk.Conn without a reachable server. zk.Connect
// returns a non-nil connection immediately; the actual dialing happens in a
// background goroutine. The caller must Close the connection to stop the
// background reconnect loop.
func newTestConn(t *testing.T) *zk.Conn {
	t.Helper()
	// Port 1 on loopback is closed: no server is needed.
	conn, _, err := zk.Connect([]string{"127.0.0.1:1"}, time.Second)
	if err != nil {
		t.Fatalf("zk.Connect returned error: %v", err)
	}
	if conn == nil {
		t.Fatal("zk.Connect returned nil connection")
	}
	return conn
}

const testPath = "/app/config"

// ---------------------------------------------------------------------------
// Constructor and options
// ---------------------------------------------------------------------------

func TestNew_NilClient(t *testing.T) {
	if _, err := New(nil, WithPath(testPath)); err == nil {
		t.Fatal("New with a nil client should fail")
	}
}

func TestNew_MissingPath(t *testing.T) {
	conn := newTestConn(t)
	defer conn.Close()

	if _, err := New(conn); err == nil {
		t.Fatal("New without a path should fail")
	}
}

func TestNew_Success(t *testing.T) {
	conn := newTestConn(t)
	defer conn.Close()

	s, err := New(conn, WithPath(testPath))
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	if s == nil {
		t.Fatal("New returned nil source")
	}
	if s.client != conn {
		t.Error("New should store the provided client")
	}
	if s.options.path != testPath {
		t.Errorf("options.path = %q, want %q", s.options.path, testPath)
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

	WithPath("/some/path")(o)
	if o.path != "/some/path" {
		t.Errorf("WithPath set path = %q, want %q", o.path, "/some/path")
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
		{"explicit key wins", "/default", "/explicit", "/explicit"},
		{"empty key falls back", "/default", "", "/default"},
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
// Load
// ---------------------------------------------------------------------------

func TestLoad_ConnectionClosed(t *testing.T) {
	conn := newTestConn(t)
	conn.Close() // close up front so queued requests fail immediately

	s, err := New(conn, WithPath(testPath))
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
	case <-time.After(5 * time.Second):
		t.Fatal("Load on a closed connection did not return within 5s")
	}

	if loadErr == nil {
		t.Error("Load should return an error when the connection is closed")
	}
}

func TestLoad_InvalidPath(t *testing.T) {
	conn := newTestConn(t)
	defer conn.Close()

	s, err := New(conn, WithPath(testPath))
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	// A key that does not start with "/" is rejected by the zk client before
	// any network I/O happens.
	data, err := s.Load(context.Background(), "relative-path")
	if err == nil {
		t.Fatal("Load should fail for a relative path")
	}
	if data != nil {
		t.Errorf("Load = %q, want nil on error", string(data))
	}
	if !strings.Contains(err.Error(), "zk:") {
		t.Errorf("Load error = %q, want it to originate from the zk client", err.Error())
	}
}

// ---------------------------------------------------------------------------
// WatchValue
// ---------------------------------------------------------------------------

func TestWatchValue_ClosedConnection(t *testing.T) {
	conn := newTestConn(t)
	conn.Close()

	s, err := New(conn, WithPath(testPath))
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

	// GetW fails immediately on the closed connection, so the watch loop
	// exits and the channel is closed without delivering a value.
	select {
	case v, ok := <-ch:
		if ok {
			t.Errorf("watch channel should be closed, got value %q", string(v))
		}
	case <-time.After(5 * time.Second):
		t.Error("watch channel was not closed within 5s")
	}
}

func TestWatchValue_InvalidPath(t *testing.T) {
	conn := newTestConn(t)
	defer conn.Close()

	s, err := New(conn, WithPath(testPath))
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch, err := s.WatchValue(ctx, "relative-path")
	if err != nil {
		t.Fatalf("WatchValue returned error: %v", err)
	}

	// GetW fails immediately for the invalid path, so the watch loop exits
	// and the channel is closed without delivering a value.
	select {
	case v, ok := <-ch:
		if ok {
			t.Errorf("watch channel should be closed, got value %q", string(v))
		}
	case <-time.After(5 * time.Second):
		t.Error("watch channel was not closed within 5s")
	}
}
