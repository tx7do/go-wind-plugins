package hptimer_test

import (
	"context"
	"time"

	hptimertransport "github.com/tx7do/go-wind-plugins/transport/hptimer"
)

// ExampleNewServer constructs a timer server and registers a one-shot task
// carrying a callback with the scheduler. The callback fires when the task's
// trigger time arrives while the server is running inside the application.
func ExampleNewServer() {
	srv := hptimertransport.NewServer(
		hptimertransport.WithGracefullyShutdown(true),
	)

	task := hptimertransport.NewTimerTask(
		"cleanup-task",
		time.Now().Add(5*time.Minute),
		hptimertransport.WithCallback(func(_ context.Context) error {
			return nil
		}),
	)

	_ = srv.AddTask(task)
}
