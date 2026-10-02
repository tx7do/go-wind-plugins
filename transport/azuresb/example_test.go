package azuresb_test

import (
	"context"

	"github.com/tx7do/go-wind-plugins/broker"
	_ "github.com/tx7do/go-wind-plugins/encoding/json" // register the JSON codec used for message payloads
	azuresbtransport "github.com/tx7do/go-wind-plugins/transport/azuresb"
)

type userEvent struct {
	UserID string `json:"user_id"`
	Action string `json:"action"`
}

// ExampleNewServer constructs an Azure Service Bus consumer server and
// registers a typed subscriber. The generic helper deserializes each consumed
// message into the payload struct before the handler sees it.
func ExampleNewServer() {
	srv := azuresbtransport.NewServer(
		azuresbtransport.WithConnectionString("Endpoint=sb://example.servicebus.windows.net/;SharedAccessKeyName=placeholder;SharedAccessKey=placeholder"),
		azuresbtransport.WithCodec("json"),
	)

	_ = azuresbtransport.RegisterSubscriber[userEvent](
		srv,
		"user-events",
		func(_ context.Context, _ string, _ broker.Headers, _ *userEvent) error {
			return nil
		},
	)
}
