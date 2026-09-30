package tokenbucket_test

import (
	"context"
	"fmt"
	"time"

	"github.com/tx7do/go-wind-plugins/ratelimit"
	"github.com/tx7do/go-wind-plugins/ratelimit/tokenbucket"
)

// ExampleNew constructs a token-bucket limiter and shows both the
// non-blocking Allow and the blocking Wait, which parks until a token is
// available or the context expires. Rejections surface as
// ratelimit.ErrLimited.
func ExampleNew() {
	limiter, err := tokenbucket.New(10, 5) // rate=10 tokens/s, burst=5
	if err != nil {
		return
	}
	defer limiter.Close()

	ok, _ := limiter.Allow()
	fmt.Println(ok, ratelimit.ErrLimited)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = limiter.Wait(ctx)
}
