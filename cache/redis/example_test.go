package redis_test

import (
	"context"
	"time"

	goredis "github.com/redis/go-redis/v9"
	"github.com/tx7do/go-wind-plugins/cache"
	"github.com/tx7do/go-wind-plugins/cache/redis"
)

// ExampleNew constructs a Redis-backed cache and shows the CRUD, SetNX
// (distributed-lock primitive), and batch surface, with multi-key batch
// operations translated into their native single-round-trip commands. The
// application creates and owns the underlying client, and the key prefix
// namespaces this application's keys on a shared instance.
func ExampleNew() {
	ctx := context.Background()

	client := goredis.NewClient(&goredis.Options{
		Addr: "localhost:6379",
	})
	defer client.Close()

	c := redis.New(client,
		redis.WithKeyPrefix("myapp:"), // namespace isolation for keys on a shared instance
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
}
