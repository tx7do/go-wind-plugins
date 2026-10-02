package nsq_test

import (
	"context"

	"github.com/tx7do/go-wind-plugins/broker"
	_ "github.com/tx7do/go-wind-plugins/encoding/json" // register the JSON codec used for message payloads
	nsqtransport "github.com/tx7do/go-wind-plugins/transport/nsq"
)

type userEvent struct {
	UserID string `json:"user_id"`
	Action string `json:"action"`
}

// ExampleNewServer constructs an NSQ consumer server and registers a typed
// subscriber. The generic helper deserializes each consumed message into the
// payload struct before the handler sees it.
func ExampleNewServer() {
	srv := nsqtransport.NewServer(
		nsqtransport.WithAddress([]string{"localhost:4150"}),
		nsqtransport.WithCodec("json"),
	)

	_ = nsqtransport.RegisterSubscriber[userEvent](
		srv,
		"user-events",
		func(_ context.Context, _ string, _ broker.Headers, _ *userEvent) error {
			return nil
		},
	)
}
