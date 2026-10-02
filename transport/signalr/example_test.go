package signalr_test

import (
	signalrLib "github.com/philippseith/signalr"
	signalrtransport "github.com/tx7do/go-wind-plugins/transport/signalr"
)

// chatHub satisfies the hub contract by embedding the library hub; any method
// the type declares becomes callable by connected clients.
type chatHub struct {
	signalrLib.Hub
}

// ExampleNewServer constructs a SignalR server with a hub attached and maps
// the hub onto an HTTP path. Register the server with the application's
// transport lifecycle so that clients connecting to the mapped path are
// served by the hub.
func ExampleNewServer() {
	srv := signalrtransport.NewServer(
		signalrtransport.WithAddress("localhost:8800"),
		signalrtransport.WithHub(&chatHub{}),
	)

	srv.MapHTTP("/chat")
}
