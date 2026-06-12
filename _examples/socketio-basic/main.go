// Package main demonstrates a Socket.IO server using go-wind.
//
// This example shows:
//   - Reusing HTTP middleware (recovery, request-id, logging) via Server.Use()
//   - Connection/disconnection callbacks
//   - Registering event handlers in different namespaces
//   - Emitting replies back to clients
//
// Run:
//
//	go run ./socketio-basic
//
// Test with a browser console:
//
//	const socket = io("http://localhost:8000/", { transports: ["websocket"] });
//	socket.on("connect", () => console.log("connected:", socket.id));
//	socket.on("reply", (msg) => console.log("reply:", msg));
//	socket.emit("notice", "hello world");
//
//	// Chat namespace
//	const chat = io("http://localhost:8000/chat", { transports: ["websocket"] });
//	chat.on("connect", () => console.log("chat connected"));
//	chat.emit("msg", "hi from chat");
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	socketio "github.com/googollee/go-socket.io"
	_ "github.com/tx7do/go-wind-plugins/encoding/json" // side-effect: register JSON codec
	"github.com/tx7do/go-wind-plugins/transport/http/middleware/logging"
	"github.com/tx7do/go-wind-plugins/transport/http/middleware/recovery"
	"github.com/tx7do/go-wind-plugins/transport/http/middleware/requestid"
	sioServer "github.com/tx7do/go-wind-plugins/transport/socketio"
)

func main() {
	srv := sioServer.NewServer(
		sioServer.WithAddress(":8000"),
		sioServer.WithCodec("json"),
		sioServer.WithPath("/socket.io/"),
	)

	// Reuse HTTP middleware — Socket.IO uses HTTP polling and WebSocket upgrade,
	// so all transport/http middleware work out of the box via Server.Use().
	srv.Use(
		recovery.Middleware(),
		requestid.Middleware(),
		logging.Middleware(),
	)

	// Root namespace "/"
	srv.RegisterConnectHandler("/", func(s socketio.Conn) error {
		s.SetContext("")
		log.Printf("[connect] %s joined /", s.ID())
		return nil
	})

	srv.RegisterDisconnectHandler("/", func(s socketio.Conn, reason string) {
		log.Printf("[disconnect] %s left /: %s", s.ID(), reason)
	})

	// "notice" event in "/" namespace: reply with an acknowledgement
	srv.RegisterEventHandler("/", "notice", func(s socketio.Conn, msg string) {
		log.Printf("[notice] %s: %s", s.ID(), msg)
		s.Emit("reply", "server received: "+msg)
	})

	// "bye" event in "/" namespace: close connection
	srv.RegisterEventHandler("/", "bye", func(s socketio.Conn) string {
		last := ""
		if ctx := s.Context(); ctx != nil {
			last = ctx.(string)
		}
		s.Emit("bye", last)
		_ = s.Close()
		return last
	})

	// Chat namespace "/chat"
	srv.RegisterConnectHandler("/chat", func(s socketio.Conn) error {
		log.Printf("[connect] %s joined /chat", s.ID())
		return nil
	})

	// "msg" event in "/chat" namespace: returns ack string
	srv.RegisterEventHandler("/chat", "msg", func(s socketio.Conn, msg string) string {
		s.SetContext(msg)
		log.Printf("[chat] %s: %s", s.ID(), msg)
		return "server ack: " + msg
	})

	// Error handler
	srv.RegisterErrorHandler("/", func(s socketio.Conn, e error) {
		log.Printf("[error] %s: %v", s.ID(), e)
	})

	// Graceful shutdown.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	fmt.Printf("Socket.IO server listening on %s\n", srv.Endpoint())
	fmt.Println()
	fmt.Println("Test with a browser (include socket.io client):")
	fmt.Println()
	fmt.Println(`  const socket = io("http://localhost:8000/", {transports:["websocket"]});`)
	fmt.Println(`  socket.on("connect", () => console.log("connected:", socket.id));`)
	fmt.Println(`  socket.on("reply", (msg) => console.log("reply:", msg));`)
	fmt.Println(`  socket.emit("notice", "hello world");`)
	fmt.Println()

	if err := srv.Start(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "server error: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("server stopped")
}
