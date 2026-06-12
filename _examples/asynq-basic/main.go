// Package main demonstrates an Asynq async task queue server using go-wind.
//
// This example shows:
//   - Creating an Asynq server backed by Redis
//   - Registering typed task handlers with JSON deserialization
//   - Enqueueing immediate and delayed tasks
//   - Registering a periodic (cron) task via the built-in scheduler
//   - Graceful shutdown with signal handling
//
// Prerequisites:
//   - A running Redis server at localhost:6379
//
// Run:
//
//	go run ./asynq-basic
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/hibiken/asynq"
	_ "github.com/tx7do/go-wind-plugins/encoding/json" // side-effect: register JSON codec
	asynqServer "github.com/tx7do/go-wind-plugins/transport/asynq"
)

// emailPayload is the JSON payload for the "email:send" task type.
type emailPayload struct {
	To      string `json:"to"`
	Subject string `json:"subject"`
	Body    string `json:"body"`
}

// cleanupPayload is the JSON payload for the "cleanup:temp" task type.
type cleanupPayload struct {
	Directory string `json:"directory"`
}

func main() {
	srv := asynqServer.NewServer(
		asynqServer.WithRedisAddress("127.0.0.1:6379"),
		asynqServer.WithConcurrency(10),
		asynqServer.WithCodec("json"),
	)

	// 1. Register a typed handler for "email:send" tasks.
	if err := asynqServer.RegisterSubscriber[emailPayload](
		srv, "email:send",
		func(taskType string, msg *emailPayload) error {
			log.Printf("[task] type=%s to=%s subject=%s", taskType, msg.To, msg.Subject)
			// send email here...
			return nil
		},
	); err != nil {
		fmt.Fprintf(os.Stderr, "register email handler failed: %v\n", err)
		os.Exit(1)
	}

	// 2. Register a handler with context for "cleanup:temp" tasks.
	if err := asynqServer.RegisterSubscriberWithCtx[cleanupPayload](
		srv, "cleanup:temp",
		func(ctx context.Context, taskType string, msg *cleanupPayload) error {
			log.Printf("[task] type=%s dir=%s", taskType, msg.Directory)
			// cleanup temp files here...
			return nil
		},
	); err != nil {
		fmt.Fprintf(os.Stderr, "register cleanup handler failed: %v\n", err)
		os.Exit(1)
	}

	// 3. Register a periodic task (cron) — runs every minute.
	if _, err := srv.NewPeriodicTask(
		"* * * * *", // every minute
		"cleanup:temp",
		&cleanupPayload{Directory: "/tmp/app"},
	); err != nil {
		fmt.Fprintf(os.Stderr, "register periodic task failed: %v\n", err)
		os.Exit(1)
	}

	// 4. Enqueue an immediate task.
	if err := srv.NewTask("email:send", &emailPayload{
		To:      "user@example.com",
		Subject: "Welcome",
		Body:    "Hello from asynq!",
	}); err != nil {
		fmt.Fprintf(os.Stderr, "enqueue email task failed: %v\n", err)
		os.Exit(1)
	}

	// 5. Enqueue a delayed task (processed after 30 seconds).
	if err := srv.NewTask("email:send", &emailPayload{
		To:      "delayed@example.com",
		Subject: "Delayed Welcome",
		Body:    "This was delayed by 30s",
	}, asynq.ProcessIn(30*time.Second)); err != nil {
		fmt.Fprintf(os.Stderr, "enqueue delayed task failed: %v\n", err)
		os.Exit(1)
	}

	// Graceful shutdown.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	fmt.Printf("Asynq server connecting to %s\n", srv.Endpoint())
	fmt.Println()
	fmt.Println("Make sure Redis is running at 127.0.0.1:6379.")
	fmt.Printf("Registered task types: %v\n", srv.GetRegisteredTaskTypes())
	fmt.Println()

	if err := srv.Start(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "server error: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("server stopped")
}
