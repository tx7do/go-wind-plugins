// Package main demonstrates a Server-Sent Events (SSE) server using go-wind.
//
// SSE runs over standard HTTP, so all transport/http middleware (recovery,
// logging, request-id, etc.) can be reused directly via Server.Use().
//
// Run:
//
//	go run ./sse-basic
//
// Test:
//
//	# Subscribe to the "notifications" stream (blocks, prints events)
//	curl -N "http://localhost:8080/events?stream=notifications"
//
//	# In another terminal, trigger a manual publish
//	curl http://localhost:8080/publish?msg=hello
package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	_ "github.com/tx7do/go-wind-plugins/encoding/json" // side-effect: register JSON codec
	"github.com/tx7do/go-wind-plugins/transport/http/middleware/logging"
	"github.com/tx7do/go-wind-plugins/transport/http/middleware/recovery"
	"github.com/tx7do/go-wind-plugins/transport/http/middleware/requestid"
	sseServer "github.com/tx7do/go-wind-plugins/transport/sse"
)

// notification is a JSON-serializable payload for SSE events.
type notification struct {
	Title   string `json:"title"`
	Message string `json:"message"`
	Time    string `json:"time"`
}

func main() {
	srv := sseServer.NewServer(":8080",
		sseServer.WithPath("/events"),       // SSE subscription path
		sseServer.WithStreamIdKey("stream"), // query param: ?stream=<id>
		sseServer.WithAutoStream(true),      // auto-create streams on subscribe
		sseServer.WithAutoReplay(true),      // replay missed events on reconnect
	)

	// Reuse HTTP middleware — SSE is HTTP, so all http middleware work out of the box.
	srv.Use(
		recovery.Middleware(),
		requestid.Middleware(),
		logging.Middleware(),
	)

	// Pre-create a named stream.
	srv.CreateStream("notifications")

	// Register a plain HTTP endpoint that manually publishes an event.
	srv.HandleFunc("/publish", func(w http.ResponseWriter, r *http.Request) {
		msg := r.URL.Query().Get("msg")
		if msg == "" {
			msg = "(empty)"
		}
		if err := srv.PublishDataWithEventName(
			r.Context(), "notifications", "user-message",
			&notification{Title: "Publish", Message: msg, Time: time.Now().Format(time.RFC3339)},
		); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		fmt.Fprintf(w, "published to 'notifications': %s\n", msg)
	})

	// Periodic event publisher: sends a heartbeat every 5 seconds.
	go periodicPublisher(srv)

	// Graceful shutdown on SIGINT / SIGTERM.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	fmt.Printf("SSE server listening on %s\n", srv.Endpoint())
	fmt.Println()
	fmt.Println("Subscribe:")
	fmt.Printf("  curl -N \"%s/events?stream=notifications\"\n", srv.Endpoint())
	fmt.Println()
	fmt.Println("Publish manually:")
	fmt.Printf("  curl \"%s/publish?msg=hello\"\n", srv.Endpoint())
	fmt.Println()

	if err := srv.Start(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "server error: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("server stopped")
}

// periodicPublisher sends a heartbeat notification every 5 seconds.
func periodicPublisher(srv *sseServer.Server) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	count := 0
	for range ticker.C {
		count++
		err := srv.PublishDataWithEventName(
			context.Background(), "notifications", "heartbeat",
			&notification{
				Title:   fmt.Sprintf("Heartbeat #%d", count),
				Message: "Periodic event from server",
				Time:    time.Now().Format(time.RFC3339),
			},
		)
		if err != nil {
			fmt.Fprintf(os.Stderr, "publish error: %v\n", err)
		}
	}
}
