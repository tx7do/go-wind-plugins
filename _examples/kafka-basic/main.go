// Package main demonstrates a Kafka consumer server using go-wind.
//
// This example shows:
//   - Creating a Kafka-based transport server with broker address
//   - Registering a typed subscriber with JSON deserialization
//   - Publishing messages to a topic
//   - Graceful shutdown with signal handling
//
// Prerequisites:
//   - A running Kafka broker at localhost:9092
//
// Run:
//
//	go run ./kafka-basic
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/tx7do/go-wind-plugins/broker"
	_ "github.com/tx7do/go-wind-plugins/encoding/json" // side-effect: register JSON codec
	kafkaServer "github.com/tx7do/go-wind-plugins/transport/kafka"
)

// userEvent is the JSON payload consumed from Kafka.
type userEvent struct {
	UserID string `json:"user_id"`
	Action string `json:"action"`
	Time   string `json:"time"`
}

func main() {
	srv := kafkaServer.NewServer(
		kafkaServer.WithAddress([]string{"localhost:9092"}),
		kafkaServer.WithCodec("json"),
	)

	// Register a typed subscriber for the "user-events" topic.
	// The generic helper auto-deserializes JSON into *userEvent.
	err := kafkaServer.RegisterSubscriber[userEvent](
		srv,
		context.Background(),
		"user-events", // topic
		"user-group",  // consumer group
		false,         // auto-ack
		func(_ context.Context, topic string, headers broker.Headers, msg *userEvent) error {
			log.Printf("[consume] topic=%s user_id=%s action=%s time=%s", topic, msg.UserID, msg.Action, msg.Time)
			return nil
		},
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "register subscriber failed: %v\n", err)
		os.Exit(1)
	}

	// Graceful shutdown.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	fmt.Printf("Kafka consumer server connecting to %s\n", srv.Endpoint())
	fmt.Println()
	fmt.Println("Make sure Kafka is running at localhost:9092.")
	fmt.Println("Produce a test message:")
	fmt.Println(`  echo '{"user_id":"u-001","action":"login","time":"2025-01-01T00:00:00Z"}' | kafka-console-producer --broker-list localhost:9092 --topic user-events`)
	fmt.Println()

	if err := srv.Start(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "server error: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("server stopped")
}
