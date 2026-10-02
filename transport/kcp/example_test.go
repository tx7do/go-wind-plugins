package kcp_test

import (
	kcptransport "github.com/tx7do/go-wind-plugins/transport/kcp"
)

const messageTypeChat kcptransport.NetMessageType = iota + 1

type chatMessage struct {
	Sender  string `json:"sender"`
	Message string `json:"message"`
}

// ExampleNewServer constructs a KCP socket server and registers a typed
// message handler. The generic helper deserializes each incoming packet into
// the payload struct before the handler sees it.
func ExampleNewServer() {
	srv := kcptransport.NewServer(
		kcptransport.WithAddress("localhost:9999"),
		kcptransport.WithCodec("json"),
	)

	kcptransport.RegisterServerMessageHandler[chatMessage](
		srv,
		messageTypeChat,
		func(_ kcptransport.SessionID, _ *chatMessage) error {
			return nil
		},
	)
}
