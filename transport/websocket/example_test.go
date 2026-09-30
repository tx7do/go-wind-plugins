package websocket_test

import (
	_ "github.com/tx7do/go-wind-plugins/encoding/json" // register the JSON codec used for message payloads
	"github.com/tx7do/go-wind-plugins/transport/websocket"
	"net/url"
)

type chatMessage struct {
	Text     string `json:"text"`
	Username string `json:"username,omitempty"`
}

// ExampleNewServer constructs a WebSocket server configured for JSON text
// packets and registers a typed message handler for a message type.
func ExampleNewServer() {
	const msgTypeEcho websocket.NetMessageType = 1

	srv := websocket.NewServer(":8080",
		websocket.WithPath("/ws"),
		websocket.WithPayloadType(websocket.PayloadTypeText),
		websocket.WithSocketConnectHandler(func(_ websocket.SessionID, _ url.Values, _ bool) {}),
	)

	websocket.RegisterServerMessageHandler[chatMessage](srv, msgTypeEcho,
		func(_ websocket.SessionID, _ *chatMessage) error {
			return nil
		},
	)
}
