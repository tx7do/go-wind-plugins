package local

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/tx7do/go-wind-plugins/cache"
)

// ---------------------------------------------------------------------------
// New / options
// ---------------------------------------------------------------------------

func TestNew_DefaultConfig(t *testing.T) {
	c := New()
	defer c.Close()
	if c == nil {
		t.Fatal("New() returned nil")
	}
}

func TestNew_WithSize(t *testing.T) {
	c := New(WithSize(1024 * 1024)) // 1MB
	defer c.Close()
	if c == nil {
		t.Fatal("New(WithSize) returned nil")
	}
}

func TestNew_WithDefaultTTL(t *testing.T) {
	c := New(WithDefaultTTL(10 * time.Second))
	defer c.Close()
	if c.cfg.defaultTTL != 10*time.Second {
		t.Errorf("defaultTTL = %v, want 10s", c.cfg.defaultTTL)
	}
}

// ---------------------------------------------------------------------------
// Set / Get
// ---------------------------------------------------------------------------

func TestSetAndGet(t *testing.T) {
	c := New(WithSize(1024 * 1024))
	defer c.Close()
	ctx := context.Background()

	err := c.Set(ctx, "key1", []byte("value1"), 0)
	if err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	val, err := c.Get(ctx, "key1")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if string(val) != "value1" {
		t.Errorf("Get() = %q, want %q", val, "value1")
	}
}

func TestGet_NotFound(t *testing.T) {
	c := New(WithSize(1024 * 1024))
	defer c.Close()
	ctx := context.Background()

	_, err := c.Get(ctx, "nonexistent")
	if !errors.Is(err, cache.ErrNotFound) {
		t.Errorf("Get(missing) error = %v, want ErrNotFound", err)
	}
}

// ---------------------------------------------------------------------------
// SetNX
// ---------------------------------------------------------------------------

func TestSetNX_NewKey(t *testing.T) {
	c := New(WithSize(1024 * 1024))
	defer c.Close()
	ctx := context.Background()

	ok, err := c.SetNX(ctx, "nx-key", []byte("val"), 0)
	if err != nil {
		t.Fatalf("SetNX() error = %v", err)
	}
	if !ok {
		t.Error("SetNX() on new key should return true")
	}
}

func TestSetNX_ExistingKey(t *testing.T) {
	c := New(WithSize(1024 * 1024))
	defer c.Close()
	ctx := context.Background()

	_ = c.Set(ctx, "exists", []byte("original"), 0)
	ok, err := c.SetNX(ctx, "exists", []byte("new"), 0)
	if err != nil {
		t.Fatalf("SetNX() error = %v", err)
	}
	if ok {
		t.Error("SetNX() on existing key should return false")
	}

	// Verify original value is intact
	val, _ := c.Get(ctx, "exists")
	if string(val) != "original" {
		t.Errorf("value after failed SetNX = %q, want %q", val, "original")
	}
}

// ---------------------------------------------------------------------------
// Delete
// ---------------------------------------------------------------------------

func TestDelete(t *testing.T) {
	c := New(WithSize(1024 * 1024))
	defer c.Close()
	ctx := context.Background()

	_ = c.Set(ctx, "del-key", []byte("val"), 0)
	_ = c.Delete(ctx, "del-key")

	_, err := c.Get(ctx, "del-key")
	if !errors.Is(err, cache.ErrNotFound) {
		t.Errorf("Get() after Delete = %v, want ErrNotFound", err)
	}
}

func TestDelete_NonExistent(t *testing.T) {
	c := New(WithSize(1024 * 1024))
	defer c.Close()
	ctx := context.Background()

	// Should not error
	if err := c.Delete(ctx, "does-not-exist"); err != nil {
		t.Errorf("Delete(non-existent) error = %v", err)
	}
}

// ---------------------------------------------------------------------------
// Has
// ---------------------------------------------------------------------------

func TestHas_Exists(t *testing.T) {
	c := New(WithSize(1024 * 1024))
	defer c.Close()
	ctx := context.Background()

	_ = c.Set(ctx, "has-key", []byte("val"), 0)
	exists, err := c.Has(ctx, "has-key")
	if err != nil {
		t.Fatalf("Has() error = %v", err)
	}
	if !exists {
		t.Error("Has() should return true for existing key")
	}
}

func TestHas_NotExists(t *testing.T) {
	c := New(WithSize(1024 * 1024))
	defer c.Close()
	ctx := context.Background()

	exists, err := c.Has(ctx, "missing")
	if err != nil {
		t.Fatalf("Has() error = %v", err)
	}
	if exists {
		t.Error("Has() should return false for missing key")
	}
}

// ---------------------------------------------------------------------------
// GetMulti / SetMulti
// ---------------------------------------------------------------------------

func TestSetMultiAndGetMulti(t *testing.T) {
	c := New(WithSize(1024 * 1024))
	defer c.Close()
	ctx := context.Background()

	items := []cache.Item{
		{Key: "m1", Value: []byte("v1")},
		{Key: "m2", Value: []byte("v2")},
		{Key: "m3", Value: []byte("v3")},
	}

	if err := c.SetMulti(ctx, items); err != nil {
		t.Fatalf("SetMulti() error = %v", err)
	}

	values, err := c.GetMulti(ctx, []string{"m1", "m2", "m3"})
	if err != nil {
		t.Fatalf("GetMulti() error = %v", err)
	}
	if len(values) != 3 {
		t.Fatalf("GetMulti() returned %d values, want 3", len(values))
	}
	for i, want := range []string{"v1", "v2", "v3"} {
		if string(values[i]) != want {
			t.Errorf("GetMulti()[%d] = %q, want %q", i, values[i], want)
		}
	}
}

func TestGetMulti_PartialMiss(t *testing.T) {
	c := New(WithSize(1024 * 1024))
	defer c.Close()
	ctx := context.Background()

	_ = c.Set(ctx, "exists", []byte("v"), 0)

	values, err := c.GetMulti(ctx, []string{"exists", "missing"})
	// Should return ErrNotFound because one key is missing
	if !errors.Is(err, cache.ErrNotFound) {
		t.Errorf("GetMulti() with partial miss error = %v, want ErrNotFound", err)
	}
	if len(values) != 2 {
		t.Fatalf("GetMulti() returned %d values, want 2", len(values))
	}
	if string(values[0]) != "v" {
		t.Errorf("values[0] = %q, want %q", values[0], "v")
	}
	if values[1] != nil {
		t.Errorf("values[1] = %v, want nil", values[1])
	}
}

// ---------------------------------------------------------------------------
// TTL expiration
// ---------------------------------------------------------------------------

func TestSet_WithTTL_Expires(t *testing.T) {
	c := New(WithSize(1024 * 1024))
	defer c.Close()
	ctx := context.Background()

	// FreeCache TTL has second precision; use 2 seconds.
	_ = c.Set(ctx, "ttl-key", []byte("temp"), 2*time.Second)

	// Should exist immediately
	val, err := c.Get(ctx, "ttl-key")
	if err != nil {
		t.Fatalf("Get() before expiry error = %v", err)
	}
	if string(val) != "temp" {
		t.Errorf("Get() = %q, want %q", val, "temp")
	}

	// Wait for expiry
	time.Sleep(3 * time.Second)

	_, err = c.Get(ctx, "ttl-key")
	if !errors.Is(err, cache.ErrNotFound) {
		t.Errorf("Get() after expiry error = %v, want ErrNotFound", err)
	}
}

// ---------------------------------------------------------------------------
// Metrics
// ---------------------------------------------------------------------------

func TestMetrics_HitAndMiss(t *testing.T) {
	c := New(WithSize(1024 * 1024))
	defer c.Close()
	ctx := context.Background()

	_ = c.Set(ctx, "hit-key", []byte("v"), 0)

	// Hit
	_, _ = c.Get(ctx, "hit-key")
	// Miss
	_, _ = c.Get(ctx, "miss-key")

	if c.HitCount() < 1 {
		t.Errorf("HitCount() = %d, want >= 1", c.HitCount())
	}
	if c.MissCount() < 1 {
		t.Errorf("MissCount() = %d, want >= 1", c.MissCount())
	}
}

// ---------------------------------------------------------------------------
// Close
// ---------------------------------------------------------------------------

func TestClose_ClearsCache(t *testing.T) {
	c := New(WithSize(1024 * 1024))
	ctx := context.Background()

	_ = c.Set(ctx, "close-key", []byte("v"), 0)
	_ = c.Close()

	// EntryCount should be 0 after Close (which calls Clear)
	if c.EntryCount() != 0 {
		t.Errorf("EntryCount() after Close = %d, want 0", c.EntryCount())
	}
}

// ---------------------------------------------------------------------------
// Concurrent access
// ---------------------------------------------------------------------------

func TestConcurrentAccess(t *testing.T) {
	c := New(WithSize(4 * 1024 * 1024))
	defer c.Close()
	ctx := context.Background()

	done := make(chan struct{})

	// Writers
	for i := 0; i < 10; i++ {
		go func(n int) {
			for j := 0; j < 100; j++ {
				key := "k-" + string(rune('A'+n))
				_ = c.Set(ctx, key, []byte("v"), 0)
			}
			done <- struct{}{}
		}(i)
	}

	// Readers
	for i := 0; i < 10; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				_, _ = c.Get(ctx, "k-A")
			}
			done <- struct{}{}
		}()
	}

	for i := 0; i < 20; i++ {
		<-done
	}
}
