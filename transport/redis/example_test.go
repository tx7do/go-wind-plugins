package redis_test

import (
	"context"

	"github.com/tx7do/go-wind-plugins/broker"
	_ "github.com/tx7do/go-wind-plugins/encoding/json" // register the JSON codec used for message payloads
	redistransport "github.com/tx7do/go-wind-plugins/transport/redis"
)

type userEvent struct {
	UserID string `json:"user_id"`
	Action string `json:"action"`
}

// ExampleNewServer constructs a Redis consumer server and registers a typed
// subscriber. The generic helper deserializes each consumed message into the
// payload struct before the handler sees it.
func ExampleNewServer() {
	srv := redistransport.NewServer(
		redistransport.WithAddress("localhost:6379"),
		redistransport.WithCodec("json"),
	)

	_ = redistransport.RegisterSubscriber[userEvent](
		srv,
		"user-events",
		func(_ context.Context, _ string, _ broker.Headers, _ *userEvent) error {
			return nil
		},
	)
}
