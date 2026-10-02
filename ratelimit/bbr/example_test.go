package bbr_test

import (
	"context"
	"fmt"
	"time"

	"github.com/tx7do/go-wind-plugins/ratelimit"
	"github.com/tx7do/go-wind-plugins/ratelimit/bbr"
)

// ExampleNew constructs a BBR limiter, which adapts its admission rate to
// an estimated throughput ceiling, and shows the non-blocking Allow, the
// blocking Wait, the Done call that completes an admitted request by
// reporting its latency, and the MaxInflight observability accessor.
// Rejections surface as ratelimit.ErrLimited.
func ExampleNew() {
	limiter := bbr.New(
		bbr.WithCPUThreshold(0.8),     // load threshold above which requests are throttled
		bbr.WithWindow(5*time.Second), // sliding-window length
		bbr.WithBucketCount(40),       // buckets within window
		bbr.WithMinQPS(1.0),           // floor on the allowed QPS under heavy load
	)
	defer limiter.Close()

	ok, _ := limiter.Allow()
	if ok {
		// The request was admitted; Done reports its completion and
		// end-to-end latency back to the limiter.
		limiter.Done(5 * time.Millisecond)
	}
	fmt.Println(ok, ratelimit.ErrLimited)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = limiter.Wait(ctx)

	_ = limiter.MaxInflight()
}
