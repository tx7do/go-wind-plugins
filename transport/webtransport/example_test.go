package webtransport_test

import (
	_ "github.com/tx7do/go-wind-plugins/encoding/json" // register the JSON codec used for message payloads
	webtransporttransport "github.com/tx7do/go-wind-plugins/transport/webtransport"
)

// messageChat is an application-defined message type discriminator for the
// message handler registered below.
const messageChat webtransporttransport.MessageType = 1

type chatMessage struct {
	Sender  string `json:"sender"`
	Message string `json:"message"`
}

// ExampleNewServer constructs a WebTransport server and registers a message
// handler for an application-defined message type. The binder allocates the
// payload struct that each inbound message is deserialized into before the
// handler sees it.
func ExampleNewServer() {
	srv := webtransporttransport.NewServer(
		webtransporttransport.WithAddress("localhost:8800"),
		webtransporttransport.WithPath("/webtransport"),
		webtransporttransport.WithCodec("json"),
	)

	srv.RegisterMessageHandler(
		messageChat,
		func(_ webtransporttransport.SessionID, _ webtransporttransport.MessagePayload) error {
			return nil
		},
		func() any { return &chatMessage{} },
	)
}
