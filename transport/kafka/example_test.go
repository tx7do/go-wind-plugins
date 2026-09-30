package kafka_test

import (
	"context"

	"github.com/tx7do/go-wind-plugins/broker"
	_ "github.com/tx7do/go-wind-plugins/encoding/json" // register the JSON codec used for message payloads
	kafkatransport "github.com/tx7do/go-wind-plugins/transport/kafka"
)

type userEvent struct {
	UserID string `json:"user_id"`
	Action string `json:"action"`
}

// ExampleNewServer constructs a Kafka consumer server and registers a typed
// subscriber. The generic helper deserializes each consumed message into the
// payload struct before the handler sees it.
func ExampleNewServer() {
	srv := kafkatransport.NewServer(
		kafkatransport.WithAddress([]string{"localhost:9092"}),
		kafkatransport.WithCodec("json"),
	)

	_ = kafkatransport.RegisterSubscriber[userEvent](
		srv,
		context.Background(),
		"user-events",
		"user-group",
		false,
		func(_ context.Context, _ string, _ broker.Headers, _ *userEvent) error {
			return nil
		},
	)
}
