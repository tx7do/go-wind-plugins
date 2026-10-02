package gcpubsub_test

import (
	"context"

	"github.com/tx7do/go-wind-plugins/broker"
	_ "github.com/tx7do/go-wind-plugins/encoding/json" // register the JSON codec used for message payloads
	gcpubsubtransport "github.com/tx7do/go-wind-plugins/transport/gcpubsub"
)

type userEvent struct {
	UserID string `json:"user_id"`
	Action string `json:"action"`
}

// ExampleNewServer constructs a Google Cloud Pub/Sub consumer server and
// registers a typed subscriber. The generic helper deserializes each consumed
// message into the payload struct before the handler sees it.
func ExampleNewServer() {
	srv := gcpubsubtransport.NewServer(
		gcpubsubtransport.WithProjectID("placeholder-project-id"),
		gcpubsubtransport.WithCodec("json"),
	)

	_ = gcpubsubtransport.RegisterSubscriber[userEvent](
		srv,
		"user-events",
		func(_ context.Context, _ string, _ broker.Headers, _ *userEvent) error {
			return nil
		},
	)
}
