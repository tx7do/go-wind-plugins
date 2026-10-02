package ratelimit_test

import (
	"context"

	windhttp "github.com/tx7do/go-wind-plugins/transport/http"
	"github.com/tx7do/go-wind-plugins/transport/http/middleware/ratelimit"
)

type demoLimiter struct{}

func (demoLimiter) Allow() (bool, error) { return true, nil }

func (demoLimiter) Wait(context.Context) error { return nil }

func (demoLimiter) Close() error { return nil }

// ExampleMiddleware attaches the rate-limiting middleware to a server's global
// chain. Every request is checked against the supplied limiter: in the default
// reject mode a request that exceeds the limit is answered with 429 Too Many
// Requests, while WithWait instead blocks until a token is available or the
// request context is cancelled.
//
// The limiter argument implements the Limiter contract of
// github.com/tx7do/go-wind-plugins/ratelimit; concrete implementations (token
// bucket, BBR, Sentinel) are distributed as separate modules, and demoLimiter
// above is a trivial stand-in so the example compiles without importing one.
// A runnable end-to-end setup additionally needs a driver (see
// transport/http/driver/std).
func ExampleMiddleware() {
	srv := windhttp.NewServer(":8080")
	srv.Use(ratelimit.Middleware(demoLimiter{}))
}
