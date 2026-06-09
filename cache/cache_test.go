package cache

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestErrNotFound(t *testing.T) {
	if !errors.Is(ErrNotFound, ErrNotFound) {
		t.Error("ErrNotFound should match itself via errors.Is")
	}
	if ErrNotFound.Error() == "" {
		t.Error("ErrNotFound should have a non-empty message")
	}
}

func TestItem(t *testing.T) {
	item := Item{
		Key:   "test-key",
		Value: []byte("test-value"),
		TTL:   5 * time.Second,
	}
	if item.Key != "test-key" {
		t.Errorf("Key = %q, want %q", item.Key, "test-key")
	}
	if string(item.Value) != "test-value" {
		t.Errorf("Value = %q, want %q", item.Value, "test-value")
	}
	if item.TTL != 5*time.Second {
		t.Errorf("TTL = %v, want %v", item.TTL, 5*time.Second)
	}
}

// dummyCache is a minimal Cache for interface verification.
type dummyCache struct{}

func (dummyCache) Get(context.Context, string) ([]byte, error)              { return nil, nil }
func (dummyCache) Set(context.Context, string, []byte, time.Duration) error { return nil }
func (dummyCache) SetNX(context.Context, string, []byte, time.Duration) (bool, error) {
	return true, nil
}
func (dummyCache) Delete(context.Context, string) error      { return nil }
func (dummyCache) Has(context.Context, string) (bool, error) { return true, nil }
func (dummyCache) GetMulti(_ context.Context, keys []string) ([][]byte, error) {
	result := make([][]byte, len(keys))
	return result, nil
}
func (dummyCache) SetMulti(context.Context, []Item) error { return nil }
func (dummyCache) Close() error                           { return nil }

var _ Cache = dummyCache{}

func TestCacheInterface(t *testing.T) {
	var c Cache = dummyCache{}
	ctx := context.Background()

	val, err := c.Get(ctx, "key")
	if err != nil {
		t.Errorf("Get() error = %v", err)
	}
	_ = val

	if err := c.Set(ctx, "key", []byte("val"), time.Second); err != nil {
		t.Errorf("Set() error = %v", err)
	}

	ok, err := c.SetNX(ctx, "key", []byte("val"), time.Second)
	if err != nil || !ok {
		t.Errorf("SetNX() = (%v, %v), want (true, nil)", ok, err)
	}

	if err := c.Delete(ctx, "key"); err != nil {
		t.Errorf("Delete() error = %v", err)
	}

	exists, err := c.Has(ctx, "key")
	if err != nil || !exists {
		t.Errorf("Has() = (%v, %v), want (true, nil)", exists, err)
	}

	values, err := c.GetMulti(ctx, []string{"a", "b"})
	if err != nil {
		t.Errorf("GetMulti() error = %v", err)
	}
	if len(values) != 2 {
		t.Errorf("GetMulti() returned %d values, want 2", len(values))
	}

	if err := c.SetMulti(ctx, []Item{{Key: "a", Value: []byte("1")}}); err != nil {
		t.Errorf("SetMulti() error = %v", err)
	}

	if err := c.Close(); err != nil {
		t.Errorf("Close() error = %v", err)
	}
}
