package nats_test

import (
	"context"

	"github.com/tx7do/go-wind-plugins/broker"
	"github.com/tx7do/go-wind-plugins/broker/nats"
	_ "github.com/tx7do/go-wind-plugins/encoding/json" // register the JSON codec used for message payloads
)

type userEvent struct {
	UserID string `json:"user_id"`
	Action string `json:"action"`
}

// ExampleNewBroker constructs a NATS broker and registers a typed subscriber
// for a topic. The generic helper deserializes each consumed message into
// the payload struct before the handler sees it.
func ExampleNewBroker() {
	b := nats.NewBroker(
		broker.WithAddress("nats://127.0.0.1:4222"),
		broker.WithCodec("json"),
	)

	_, err := broker.Subscribe[userEvent](
		b,
		"user-events",
		func(_ context.Context, _ string, _ broker.Headers, _ *userEvent) error {
			return nil
		},
	)
	if err != nil {
		return
	}
}

// ExampleNewJetStreamBroker constructs a NATS JetStream broker and registers
// a typed subscriber for a topic. The generic helper deserializes each
// consumed message into the payload struct before the handler sees it.
func ExampleNewJetStreamBroker() {
	b := nats.NewJetStreamBroker(
		broker.WithAddress("nats://127.0.0.1:4222"),
		broker.WithCodec("json"),
	)

	_, err := broker.Subscribe[userEvent](
		b,
		"user-events",
		func(_ context.Context, _ string, _ broker.Headers, _ *userEvent) error {
			return nil
		},
	)
	if err != nil {
		return
	}
}
