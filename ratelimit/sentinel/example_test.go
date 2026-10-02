package sentinel_test

import (
	"context"
	"fmt"
	"time"

	sentinelapi "github.com/alibaba/sentinel-golang/api"
	"github.com/alibaba/sentinel-golang/core/base"
	"github.com/tx7do/go-wind-plugins/ratelimit"
	"github.com/tx7do/go-wind-plugins/ratelimit/sentinel"
)

// ExampleNew constructs a Sentinel-backed flow limiter for a named
// resource and shows the non-blocking Allow, the AllowEntry/ReleaseEntry
// pair for concurrency rules, and the blocking Wait, which polls for
// admission until it succeeds or the context expires. Flow rules for the
// resource are configured separately through Sentinel's rule API;
// rejections surface as ratelimit.ErrLimited.
func ExampleNew() {
	limiter := sentinel.New("my-api",
		sentinel.WithTrafficType(base.Outbound),                    // traffic type of the wrapped entry
		sentinel.WithEntryOptions(sentinelapi.WithAcquireCount(2)), // raw Sentinel entry options
		sentinel.WithWaitInterval(25*time.Millisecond),             // polling interval used by Wait
	)
	defer limiter.Close()

	ok, _ := limiter.Allow()
	fmt.Println(ok, ratelimit.ErrLimited)

	entry, err := limiter.AllowEntry()
	if err != nil {
		return
	}
	limiter.ReleaseEntry(entry)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = limiter.Wait(ctx)
}
