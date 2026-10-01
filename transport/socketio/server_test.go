package socketio

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"testing"

	socketio "github.com/googollee/go-socket.io"
)

// skipWithoutIntegration skips tests that need a live external server.
func skipWithoutIntegration(t *testing.T) {
	t.Helper()
	if os.Getenv("KRATOS_IT") == "" {
		t.Skip("skipping integration test: requires a live server; set KRATOS_IT to enable")
	}
}

func TestServer(t *testing.T) {
	skipWithoutIntegration(t)

	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	srv := NewServer(
		WithAddress(":8000"),
		WithCodec("json"),
		WithPath("/socket.io/"),
	)

	srv.RegisterConnectHandler("/", func(s socketio.Conn) error {
		s.SetContext("")
		fmt.Println("connected:", s.ID())
		return nil
	})

	srv.RegisterEventHandler("/", "notice", func(s socketio.Conn, msg string) {
		fmt.Println("notice:", msg)
		s.Emit("reply", "have "+msg)
	})

	srv.RegisterEventHandler("/chat", "msg", func(s socketio.Conn, msg string) string {
		s.SetContext(msg)
		return "recv " + msg
	})

	srv.RegisterEventHandler("/", "bye", func(s socketio.Conn) string {
		last := s.Context().(string)
		s.Emit("bye", last)
		_ = s.Close()
		return last
	})

	srv.RegisterErrorHandler("/", func(s socketio.Conn, e error) {
		fmt.Println("meet error:", e)
	})

	srv.RegisterDisconnectHandler("/", func(s socketio.Conn, reason string) {
		fmt.Println("closed", reason)
	})

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
