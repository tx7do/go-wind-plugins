package sqs_test

import (
	"context"

	"github.com/tx7do/go-wind-plugins/broker"
	_ "github.com/tx7do/go-wind-plugins/encoding/json" // register the JSON codec used for message payloads
	sqstransport "github.com/tx7do/go-wind-plugins/transport/sqs"
)

type userEvent struct {
	UserID string `json:"user_id"`
	Action string `json:"action"`
}

// ExampleNewServer constructs an SQS consumer server and registers a typed
// subscriber. The generic helper deserializes each consumed message into the
// payload struct before the handler sees it.
func ExampleNewServer() {
	srv := sqstransport.NewServer(
		sqstransport.WithRegion("us-east-1"),
		sqstransport.WithEndpoint("http://localhost:9324"),
		sqstransport.WithCodec("json"),
	)

	_ = sqstransport.RegisterSubscriber[userEvent](
		srv,
		"user-events",
		func(_ context.Context, _ string, _ broker.Headers, _ *userEvent) error {
			return nil
		},
	)
}
