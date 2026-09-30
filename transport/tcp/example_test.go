package tcp_test

import (
	_ "github.com/tx7do/go-wind-plugins/encoding/json" // register the JSON codec used for message payloads
	tcptransport "github.com/tx7do/go-wind-plugins/transport/tcp"
)

type chatMessage struct {
	Text     string `json:"text"`
	Username string `json:"username,omitempty"`
}

// ExampleNewServer constructs a TCP server and registers typed message
// handlers. Messages are framed as [4-byte little-endian type][payload]; the
// generic helper deserializes the payload into the struct before the handler
// sees it. Messages can be sent to a single session with SendMessage or to
// every connected session with Broadcast.
func ExampleNewServer() {
	const msgTypeEcho tcptransport.NetMessageType = 1

	var sid tcptransport.SessionID

	srv := tcptransport.NewServer(
		tcptransport.WithAddress(":9000"),
		tcptransport.WithCodec("json"),
		tcptransport.WithSocketConnectHandler(func(_ tcptransport.SessionID, _ bool) {}),
	)

	tcptransport.RegisterServerMessageHandler[chatMessage](srv, msgTypeEcho,
		func(_ tcptransport.SessionID, _ *chatMessage) error {
			return nil
		},
	)

	_ = srv.SendMessage(sid, msgTypeEcho, &chatMessage{Text: "hello"})
	srv.Broadcast(msgTypeEcho, &chatMessage{Text: "hello"})
}
