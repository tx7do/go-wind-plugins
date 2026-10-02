package retry_test

import (
	"time"

	coreRetry "github.com/tx7do/go-wind-plugins/retry"
	windhttp "github.com/tx7do/go-wind-plugins/transport/http"
	"github.com/tx7do/go-wind-plugins/transport/http/middleware/retry"
)

// ExampleMiddleware attaches the retry middleware to a server's global chain.
// When an idempotent request (GET, HEAD, OPTIONS, PUT, DELETE) yields one of
// the configured retryable statuses (502, 503 and 504 by default; customised
// with WithRetryStatus), the middleware re-issues the request through the
// handler loop of the supplied retrier and discards the buffered response of
// each failed attempt. Non-idempotent methods such as POST are never retried
// unless WithRetryAllMethods is set.
//
// The attempt budget and backoff schedule come from the core retry package;
// here three attempts with an exponential backoff capped at five seconds. A
// runnable end-to-end setup additionally needs a driver (see
// transport/http/driver/std).
func ExampleMiddleware() {
	r := coreRetry.New(
		coreRetry.WithMaxAttempts(3),
		coreRetry.WithBackoff(coreRetry.ExponentialBackoff{
			Initial: 200 * time.Millisecond,
			Factor:  2,
			Max:     5 * time.Second,
		}),
	)

	srv := windhttp.NewServer(":8080")
	srv.Use(retry.Middleware(r))
}
