package nats_test

import (
	"context"

	"github.com/tx7do/go-wind-plugins/broker"
	_ "github.com/tx7do/go-wind-plugins/encoding/json" // register the JSON codec used for message payloads
	natstransport "github.com/tx7do/go-wind-plugins/transport/nats"
)

type userEvent struct {
	UserID string `json:"user_id"`
	Action string `json:"action"`
}

// ExampleNewServer constructs a NATS consumer server and registers a typed
// subscriber. The generic helper deserializes each consumed message into the
// payload struct before the handler sees it.
func ExampleNewServer() {
	srv := natstransport.NewServer(
		natstransport.WithAddress([]string{"localhost:4222"}),
		natstransport.WithCodec("json"),
	)

	_ = natstransport.RegisterSubscriber[userEvent](
		srv,
		"user-events",
		func(_ context.Context, _ string, _ broker.Headers, _ *userEvent) error {
			return nil
		},
	)
}
