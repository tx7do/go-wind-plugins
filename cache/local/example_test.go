package local_test

import (
	"context"
	"time"

	"github.com/tx7do/go-wind-plugins/cache"
	"github.com/tx7do/go-wind-plugins/cache/local"
)

// ExampleNew constructs an in-memory cache backed by FreeCache — a
// pre-allocated ring buffer with LRU eviction — and shows the CRUD, SetNX
// (distributed-lock primitive), batch, and statistics surface. The local
// cache needs no external services.
func ExampleNew() {
	ctx := context.Background()

	c := local.New(
		local.WithSize(100*1024*1024), // 100 MB
		local.WithDefaultTTL(5*time.Minute),
	)
	defer c.Close()

	_ = c.Set(ctx, "user:1", []byte("alice"), 10*time.Minute)

	_, _ = c.Get(ctx, "user:1")
	_, _ = c.Has(ctx, "user:1")
	_ = c.Delete(ctx, "user:1")

	_, _ = c.SetNX(ctx, "lock:order:123", []byte("locked"), 30*time.Second)

	items := []cache.Item{
		{Key: "user:2", Value: []byte("bob"), TTL: 5 * time.Minute},
		{Key: "user:3", Value: []byte("charlie"), TTL: 5 * time.Minute},
	}
	_ = c.SetMulti(ctx, items)

	_, _ = c.GetMulti(ctx, []string{"user:2", "user:3"})

	_ = c.EntryCount()
	_ = c.HitCount()
	_ = c.MissCount()
	_ = c.EvacuateCount()
}
