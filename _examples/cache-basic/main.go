// Package main demonstrates how to use the in-memory cache plugin
// (cache/local) for basic CRUD operations, SetNX, and batch operations.
//
// The local cache is backed by FreeCache — a zero-GC, pre-allocated ring
// buffer with LRU eviction. No external services (Redis, Memcached, etc.)
// are required.
//
// Run:
//
//	go run ./examples/cache-basic
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/tx7do/go-wind-plugins/cache"
	"github.com/tx7do/go-wind-plugins/cache/local"
)

// user is a sample data type stored in the cache.
type user struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
	Age  int    `json:"age"`
}

func main() {
	ctx := context.Background()

	// ---------------------------------------------------------------
	// 1. Create a local cache (100 MB, default TTL = 5 minutes)
	// ---------------------------------------------------------------
	c := local.New(
		local.WithSize(100*1024*1024), // 100 MB
		local.WithDefaultTTL(5*time.Minute),
	)
	defer c.Close()

	fmt.Println("=== Basic CRUD ===")

	// ---------------------------------------------------------------
	// 2. Set / Get
	// ---------------------------------------------------------------
	u1 := user{ID: 1, Name: "Alice", Age: 30}
	data, _ := json.Marshal(u1)

	if err := c.Set(ctx, "user:1", data, 10*time.Minute); err != nil {
		fmt.Fprintf(os.Stderr, "Set failed: %v\n", err)
		return
	}
	fmt.Println("Set user:1 -> Alice")

	got, err := c.Get(ctx, "user:1")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Get failed: %v\n", err)
		return
	}
	var retrieved user
	_ = json.Unmarshal(got, &retrieved)
	fmt.Printf("Get user:1 -> %+v\n", retrieved)

	// ---------------------------------------------------------------
	// 3. Has / Delete
	// ---------------------------------------------------------------
	exists, _ := c.Has(ctx, "user:1")
	fmt.Printf("Has user:1 -> %v\n", exists)

	_ = c.Delete(ctx, "user:1")
	exists2, _ := c.Has(ctx, "user:1")
	fmt.Printf("Has user:1 (after delete) -> %v\n", exists2)

	fmt.Println("\n=== SetNX (Distributed Lock Primitive) ===")

	// ---------------------------------------------------------------
	// 4. SetNX — set only if key does not exist
	// ---------------------------------------------------------------
	ok, _ := c.SetNX(ctx, "lock:order:123", []byte("locked"), 30*time.Second)
	fmt.Printf("SetNX lock:order:123 (first attempt) -> %v\n", ok)

	ok2, _ := c.SetNX(ctx, "lock:order:123", []byte("locked-again"), 30*time.Second)
	fmt.Printf("SetNX lock:order:123 (second attempt) -> %v\n", ok2)

	fmt.Println("\n=== Batch Operations ===")

	// ---------------------------------------------------------------
	// 5. SetMulti / GetMulti
	// ---------------------------------------------------------------
	users := []cache.Item{
		{Key: "user:2", Value: mustMarshal(user{ID: 2, Name: "Bob", Age: 25}), TTL: 5 * time.Minute},
		{Key: "user:3", Value: mustMarshal(user{ID: 3, Name: "Charlie", Age: 35}), TTL: 5 * time.Minute},
		{Key: "user:4", Value: mustMarshal(user{ID: 4, Name: "Diana", Age: 28}), TTL: 5 * time.Minute},
	}
	_ = c.SetMulti(ctx, users)
	fmt.Println("SetMulti user:2, user:3, user:4")

	values, _ := c.GetMulti(ctx, []string{"user:2", "user:3", "user:4", "user:99"})
	for i, v := range values {
		if v == nil {
			fmt.Printf("  user:%d -> (nil)\n", i+2)
			continue
		}
		var u user
		_ = json.Unmarshal(v, &u)
		fmt.Printf("  %s -> %+v\n", fmt.Sprintf("user:%d", i+2), u)
	}

	fmt.Println("\n=== Cache Statistics ===")

	// ---------------------------------------------------------------
	// 6. Cache stats
	// ---------------------------------------------------------------
	fmt.Printf("Entries:  %d\n", c.EntryCount())
	fmt.Printf("Hits:     %d\n", c.HitCount())
	fmt.Printf("Misses:   %d\n", c.MissCount())
	fmt.Printf("Evictions:%d\n", c.EvacuateCount())

	fmt.Println("\nDone!")
}

func mustMarshal(v any) []byte {
	data, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return data
}
