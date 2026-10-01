package kcp

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"testing"
)

var testServer *Server

const (
	MessageTypeChat = iota + 1
)

type ChatMessage struct {
	Type    int    `json:"type"`
	Sender  string `json:"sender"`
	Message string `json:"message"`
}

func handleConnect(sessionId SessionID, register bool) {
	if register {
		fmt.Printf("%s registered\n", sessionId)
	} else {
		fmt.Printf("%s unregistered\n", sessionId)
	}
}

func handleChatMessage(sessionId SessionID, message *ChatMessage) error {
	fmt.Printf("[%s] Payload: %v\n", sessionId, message)

	testServer.Broadcast(MessageTypeChat, *message)

	return nil
}

func TestServer(t *testing.T) {
	skipWithoutIntegration(t)

	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	srv := NewServer(
		WithAddress(":9999"),
		WithSocketConnectHandler(handleConnect),
		WithCodec("json"),
		WithBlockCrypt(DefaultBlockCryptPassword, DefaultBlockCryptSalt),
	)

	RegisterServerMessageHandler(srv, MessageTypeChat, handleChatMessage)

	testServer = srv

	go func() {
		if err := srv.Start(ctx); err != nil {
			t.Errorf("server start failed: %v", err)
		}
	}()

	defer func() {
		cancel()
	}()

	<-interrupt
}
