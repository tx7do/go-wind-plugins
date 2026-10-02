package cache_test

import (
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/tx7do/go-wind-plugins/crud/cache"
)

type demoRow struct {
	Name string
}

// ExampleNewCacheSupport wires a Redis-backed cache helper for one table
// model. In the data access layer every read goes through GetOrLoad, which
// serves warm entries from the cache and otherwise invokes the supplied
// loader once per key under the bundled single flight; per-call options such
// as WithTTL and WithNoCache adjust the caching behavior of a single
// request.
func ExampleNewCacheSupport() {
	client := redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
	})

	_ = cache.NewCacheSupport[demoRow](client, 5*time.Minute, nil)
}
