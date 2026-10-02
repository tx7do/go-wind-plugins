package pulsar_test

import (
	"context"

	"github.com/tx7do/go-wind-plugins/broker"
	_ "github.com/tx7do/go-wind-plugins/encoding/json" // register the JSON codec used for message payloads
	pulsartransport "github.com/tx7do/go-wind-plugins/transport/pulsar"
)

type userEvent struct {
	UserID string `json:"user_id"`
	Action string `json:"action"`
}

// ExampleNewServer constructs a Pulsar consumer server and registers a typed
// subscriber. The generic helper deserializes each consumed message into the
// payload struct before the handler sees it.
func ExampleNewServer() {
	srv := pulsartransport.NewServer(
		pulsartransport.WithAddress([]string{"localhost:6650"}),
		pulsartransport.WithCodec("json"),
	)

	_ = pulsartransport.RegisterSubscriber[userEvent](
		srv,
		context.Background(),
		"user-events",
		func(_ context.Context, _ string, _ broker.Headers, _ *userEvent) error {
			return nil
		},
	)
}
