package pulsar_test

import (
	"context"

	"github.com/tx7do/go-wind-plugins/broker"
	"github.com/tx7do/go-wind-plugins/broker/pulsar"
	_ "github.com/tx7do/go-wind-plugins/encoding/json" // register the JSON codec used for message payloads
)

type userEvent struct {
	UserID string `json:"user_id"`
	Action string `json:"action"`
}

// ExampleNewBroker constructs a Pulsar broker and registers a typed
// subscriber for a topic. The subscription name option identifies the
// subscription the consumer attaches to, and the generic helper
// deserializes each consumed message into the payload struct before the
// handler sees it.
func ExampleNewBroker() {
	b := pulsar.NewBroker(
		broker.WithAddress("pulsar://127.0.0.1:6650"),
		broker.WithCodec("json"),
	)

	_, err := broker.Subscribe[userEvent](
		b,
		"user-events",
		func(_ context.Context, _ string, _ broker.Headers, _ *userEvent) error {
			return nil
		},
		pulsar.WithSubscriptionName("user-events-sub"),
	)
	if err != nil {
		return
	}
}
