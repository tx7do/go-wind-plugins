package webrtc_test

import (
	_ "github.com/tx7do/go-wind-plugins/encoding/json" // register the JSON codec used for message payloads
	webrtctransport "github.com/tx7do/go-wind-plugins/transport/webrtc"
)

// messageChat is an application-defined message type discriminator for the
// channel handler registered below.
const messageChat webrtctransport.NetMessageType = 1

type chatMessage struct {
	Sender  string `json:"sender"`
	Message string `json:"message"`
}

// ExampleNewServer constructs a WebRTC signaling server and registers a typed
// channel handler for an application-defined message type. The generic helper
// deserializes each inbound data-channel message into the payload struct
// before the handler sees it.
func ExampleNewServer() {
	srv := webrtctransport.NewServer(
		webrtctransport.WithAddress("localhost:8800"),
		webrtctransport.WithPath("/signal"),
		webrtctransport.WithCodec("json"),
	)

	webrtctransport.RegisterServerMessageHandler[chatMessage](
		srv,
		messageChat,
		func(_ webrtctransport.SessionID, _ *chatMessage) error {
			return nil
		},
	)
}
