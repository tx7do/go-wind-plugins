package mqtt_test

import (
	"context"

	"github.com/tx7do/go-wind-plugins/broker"
	_ "github.com/tx7do/go-wind-plugins/encoding/json" // register the JSON codec used for message payloads
	mqtttransport "github.com/tx7do/go-wind-plugins/transport/mqtt"
)

type userEvent struct {
	UserID string `json:"user_id"`
	Action string `json:"action"`
}

// ExampleNewServer constructs an MQTT consumer server and registers a typed
// subscriber. The generic helper deserializes each consumed message into the
// payload struct before the handler sees it.
func ExampleNewServer() {
	srv := mqtttransport.NewServer(
		mqtttransport.WithAddress([]string{"localhost:1883"}),
		mqtttransport.WithCodec("json"),
	)

	_ = mqtttransport.RegisterSubscriber[userEvent](
		srv,
		context.Background(),
		"user-events",
		func(_ context.Context, _ string, _ broker.Headers, _ *userEvent) error {
			return nil
		},
	)
}
