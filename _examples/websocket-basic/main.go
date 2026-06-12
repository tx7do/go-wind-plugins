// Package main demonstrates a WebSocket server using go-wind.
//
// This example shows:
//   - Reusing HTTP middleware (recovery, request-id, logging) via Server.Use()
//   - Registering typed message handlers with JSON deserialization
//   - Echoing messages back to the sender
//   - Broadcasting messages to all connected clients
//   - Connection/disconnection callbacks
//
// Run:
//
//	go run ./websocket-basic
//
// Test with websocat (https://github.com/nickelc/websocat):
//
//	# Connect and send a JSON text packet:
//	websocat ws://localhost:8080/ws
//	# Then type: {"type":1,"payload":"{\"text\":\"hello\"}"}
//
// Or use a browser console:
//
//	let ws = new WebSocket("ws://localhost:8080/ws");
//	ws.onmessage = e => console.log("recv:", e.data);
//	ws.send(JSON.stringify({type:1, payload: JSON.stringify({text:"hello"})}));
package main

import (
	"context"
	"fmt"
	"log"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

	_ "github.com/tx7do/go-wind-plugins/encoding/json" // side-effect: register JSON codec
	"github.com/tx7do/go-wind-plugins/transport/http/middleware/logging"
	"github.com/tx7do/go-wind-plugins/transport/http/middleware/recovery"
	"github.com/tx7do/go-wind-plugins/transport/http/middleware/requestid"
	wsServer "github.com/tx7do/go-wind-plugins/transport/websocket"
)

// Message types used by this example.
const (
	MsgTypeEcho wsServer.NetMessageType = 1 // echo back to sender
	MsgTypeChat wsServer.NetMessageType = 2 // broadcast to all
)

// chatMessage is the JSON payload for chat messages.
type chatMessage struct {
	Text     string `json:"text"`
	Username string `json:"username,omitempty"`
}

func main() {
	var srv *wsServer.Server
	srv = wsServer.NewServer(":8080",
		wsServer.WithPath("/ws"),
		wsServer.WithPayloadType(wsServer.PayloadTypeText), // use JSON text packets
		wsServer.WithSocketConnectHandler(func(sid wsServer.SessionID, _ url.Values, connect bool) {
			if connect {
				log.Printf("[connect] session %s connected (%d total)", sid, srv.SessionCount())
			} else {
				log.Printf("[disconnect] session %s disconnected (%d total)", sid, srv.SessionCount())
			}
		}),
	)

	// Reuse HTTP middleware — WebSocket upgrade is an HTTP request, so all
	// transport/http middleware work out of the box via Server.Use().
	srv.Use(
		recovery.Middleware(),
		requestid.Middleware(),
		logging.Middleware(),
	)

	// Echo handler: deserializes chatMessage, logs it, sends it back.
	wsServer.RegisterServerMessageHandler[chatMessage](srv, MsgTypeEcho,
		func(sessionId wsServer.SessionID, msg *chatMessage) error {
			log.Printf("[echo] %s: %s", sessionId, msg.Text)
			return srv.SendMessage(sessionId, MsgTypeEcho, &chatMessage{
				Text:     "echo: " + msg.Text,
				Username: "server",
			})
		},
	)

	// Chat handler: broadcasts to all connected clients.
	wsServer.RegisterServerMessageHandler[chatMessage](srv, MsgTypeChat,
		func(sessionId wsServer.SessionID, msg *chatMessage) error {
			log.Printf("[chat] %s (%s): %s", sessionId, msg.Username, msg.Text)
			srv.Broadcast(MsgTypeChat, &chatMessage{
				Text:     msg.Text,
				Username: msg.Username,
			})
			return nil
		},
	)

	// Periodic broadcast every 10 seconds.
	go periodicBroadcast(srv)

	// Graceful shutdown.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	fmt.Printf("WebSocket server listening on %s\n", srv.Endpoint())
	fmt.Println()
	fmt.Println("Connect:")
	fmt.Printf("  websocat %s/ws\n", srv.Endpoint())
	fmt.Println()
	fmt.Println("Send echo:")
	fmt.Println(`  {"type":1,"payload":"{\"text\":\"hello\"}"}`)
	fmt.Println()
	fmt.Println("Send broadcast:")
	fmt.Println(`  {"type":2,"payload":"{\"text\":\"hi all\",\"username\":\"alice\"}"}`)
	fmt.Println()

	if err := srv.Start(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "server error: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("server stopped")
}

// periodicBroadcast sends a server notice to all clients every 10 seconds.
func periodicBroadcast(srv *wsServer.Server) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	count := 0
	for range ticker.C {
		count++
		active := srv.SessionCount()
		if active == 0 {
			continue
		}
		log.Printf("[broadcast] sending notice #%d to %d client(s)", count, active)
		srv.Broadcast(MsgTypeChat, &chatMessage{
			Text:     fmt.Sprintf("Server notice #%d", count),
			Username: "system",
		})
	}
}
