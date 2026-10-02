package kafka_test

import (
	"context"

	"github.com/tx7do/go-wind-plugins/broker"
	"github.com/tx7do/go-wind-plugins/broker/kafka"
	_ "github.com/tx7do/go-wind-plugins/encoding/json" // register the JSON codec used for message payloads
)

type userEvent struct {
	UserID string `json:"user_id"`
	Action string `json:"action"`
}

// ExampleNewBroker constructs a Kafka broker and registers a typed subscriber
// for a topic. The queue name option selects the consumer group the reader
// joins, and the generic helper deserializes each consumed message into the
// payload struct before the handler sees it.
func ExampleNewBroker() {
	b := kafka.NewBroker(
		broker.WithAddress("localhost:9092"),
		broker.WithCodec("json"),
	)

	_, err := broker.Subscribe[userEvent](
		b,
		"user-events",
		func(_ context.Context, _ string, _ broker.Headers, _ *userEvent) error {
			return nil
		},
		broker.WithSubscribeQueueName("user-group"),
	)
	if err != nil {
		return
	}
}
