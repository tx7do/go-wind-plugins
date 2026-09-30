package sse_test

import (
	"fmt"
	"net/http"

	_ "github.com/tx7do/go-wind-plugins/encoding/json" // register the JSON codec used for event payloads
	"github.com/tx7do/go-wind-plugins/transport/sse"
)

type notification struct {
	Title   string `json:"title"`
	Message string `json:"message"`
}

// ExampleNewServer shows the SSE server API: choosing the subscription path
// and the stream-id query parameter, pre-creating a named stream, and
// publishing events to it. Because SSE runs over plain HTTP, ordinary routes
// can be registered next to the event stream with HandleFunc, and Use accepts
// the same middleware instances as the standard HTTP server.
func ExampleNewServer() {
	srv := sse.NewServer(":8080",
		sse.WithPath("/events"),
		sse.WithStreamIdKey("stream"),
		sse.WithAutoStream(true),
		sse.WithAutoReplay(true),
	)

	srv.CreateStream("notifications")

	srv.HandleFunc("/publish", func(w http.ResponseWriter, r *http.Request) {
		_ = srv.PublishDataWithEventName(
			r.Context(), "notifications", "user-message",
			&notification{Title: "Publish", Message: r.URL.Query().Get("msg")},
		)
		fmt.Fprintln(w, "published")
	})

	fmt.Println(srv.Endpoint())
}
