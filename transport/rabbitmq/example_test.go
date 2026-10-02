package rabbitmq_test

import (
	"context"

	"github.com/tx7do/go-wind-plugins/broker"
	_ "github.com/tx7do/go-wind-plugins/encoding/json" // register the JSON codec used for message payloads
	rabbitmqtransport "github.com/tx7do/go-wind-plugins/transport/rabbitmq"
)

type userEvent struct {
	UserID string `json:"user_id"`
	Action string `json:"action"`
}

// ExampleNewServer constructs a RabbitMQ consumer server and registers a
// typed subscriber. The generic helper deserializes each consumed message
// into the payload struct before the handler sees it.
func ExampleNewServer() {
	srv := rabbitmqtransport.NewServer(
		rabbitmqtransport.WithAddress([]string{"localhost:5672"}),
		rabbitmqtransport.WithExchange("placeholder-exchange", true),
		rabbitmqtransport.WithCodec("json"),
	)

	_ = rabbitmqtransport.RegisterSubscriber[userEvent](
		srv,
		context.Background(),
		"user-events",
		func(_ context.Context, _ string, _ broker.Headers, _ *userEvent) error {
			return nil
		},
	)
}
