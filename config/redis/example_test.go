package redis_test

import (
	"context"

	goredis "github.com/redis/go-redis/v9"
	"github.com/tx7do/go-wind-plugins/config/redis"
)

// ExampleNew constructs a Redis-backed configuration source. Load performs a
// one-shot read of the configured key; WatchValue listens on the
// change-notification channel for that key and delivers updated values on the
// returned channel so configuration can be reloaded live.
func ExampleNew() {
	client := goredis.NewClient(&goredis.Options{
		Addr: "localhost:6379",
	})
	defer client.Close()

	src, err := redis.New(client,
		redis.WithPath("myapp:config"),
	)
	if err != nil {
		return
	}

	raw, err := src.Load(context.Background(), "")
	if err != nil {
		return
	}
	_ = raw

	ch, err := src.WatchValue(context.Background(), "")
	if err != nil {
		return
	}
	_ = ch
}
