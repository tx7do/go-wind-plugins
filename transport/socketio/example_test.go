package socketio_test

import (
	socketiolib "github.com/googollee/go-socket.io"
	_ "github.com/tx7do/go-wind-plugins/encoding/json" // register the JSON codec used for event payloads
	siotransport "github.com/tx7do/go-wind-plugins/transport/socketio"
)

// ExampleNewServer constructs a Socket.IO server and registers connection,
// disconnect, event, and error handlers. Handlers are registered per
// namespace; "/" addresses the root namespace.
func ExampleNewServer() {
	srv := siotransport.NewServer(
		siotransport.WithAddress(":8000"),
		siotransport.WithCodec("json"),
		siotransport.WithPath("/socket.io/"),
	)

	srv.RegisterConnectHandler("/", func(_ socketiolib.Conn) error {
		return nil
	})

	srv.RegisterDisconnectHandler("/", func(_ socketiolib.Conn, _ string) {})

	srv.RegisterEventHandler("/", "notice", func(_ socketiolib.Conn, _ string) {})

	srv.RegisterErrorHandler("/", func(_ socketiolib.Conn, _ error) {})
}
