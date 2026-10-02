package rocketmq_test

import (
	"context"

	"github.com/tx7do/go-wind-plugins/broker"
	rocketmqOption "github.com/tx7do/go-wind-plugins/broker/rocketmq/option"
	_ "github.com/tx7do/go-wind-plugins/encoding/json" // register the JSON codec used for message payloads
	rocketmqtransport "github.com/tx7do/go-wind-plugins/transport/rocketmq"
)

type userEvent struct {
	UserID string `json:"user_id"`
	Action string `json:"action"`
}

// ExampleNewServer constructs a RocketMQ consumer server and registers a typed
// subscriber. The generic helper deserializes each consumed message into the
// payload struct before the handler sees it.
func ExampleNewServer() {
	srv := rocketmqtransport.NewServer(
		rocketmqOption.DriverTypeV2,
		rocketmqtransport.WithNameServer([]string{"localhost:9876"}),
		rocketmqtransport.WithCodec("json"),
	)

	_ = rocketmqtransport.RegisterSubscriber[userEvent](
		srv,
		context.Background(),
		"user-events",
		"user-group",
		func(_ context.Context, _ string, _ broker.Headers, _ *userEvent) error {
			return nil
		},
	)
}
