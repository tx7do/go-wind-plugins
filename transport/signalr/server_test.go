package signalr

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/philippseith/signalr"
)

// skipWithoutIntegration skips tests that need a live external server.
func skipWithoutIntegration(t *testing.T) {
	t.Helper()
	if os.Getenv("KRATOS_IT") == "" {
		t.Skip("skipping integration test: requires a live server; set KRATOS_IT to enable")
	}
}

type chat struct {
	signalr.Hub
}

func (c *chat) OnConnected(connectionID string) {
	fmt.Printf("%s connected\n", connectionID)
	c.Groups().AddToGroup("group", connectionID)
}

func (c *chat) OnDisconnected(connectionID string) {
	fmt.Printf("%s disconnected\n", connectionID)
	c.Groups().RemoveFromGroup("group", connectionID)
}

func (c *chat) Broadcast(message string) {
	// Broadcast to all clients
	c.Clients().Group("group").Send("receive", message)
}

func (c *chat) Echo(message string) {
	c.Clients().Caller().Send("receive", message)
}

func (c *chat) Panic() {
	panic("Don't panic!")
}

func (c *chat) RequestAsync(message string) <-chan map[string]string {
	r := make(chan map[string]string)
	go func() {
		defer close(r)
		time.Sleep(4 * time.Second)
		m := make(map[string]string)
		m["ToUpper"] = strings.ToUpper(message)
		m["ToLower"] = strings.ToLower(message)
		m["len"] = fmt.Sprint(len(message))
		r <- m
	}()
	return r
}

func (c *chat) RequestTuple(message string) (string, string, int) {
	return strings.ToUpper(message), strings.ToLower(message), len(message)
}

func (c *chat) DateStream() <-chan string {
	r := make(chan string)
	go func() {
		defer close(r)
		for i := 0; i < 50; i++ {
			r <- fmt.Sprint(time.Now().Clock())
			time.Sleep(time.Second)
		}
	}()
	return r
}

func (c *chat) UploadStream(upload1 <-chan int, factor float64, upload2 <-chan float64) {
	ok1 := true
	ok2 := true
	u1 := 0
	u2 := 0.0
	c.Echo(fmt.Sprintf("f: %v", factor))
	for {
		select {
		case u1, ok1 = <-upload1:
			if ok1 {
				c.Echo(fmt.Sprintf("u1: %v", u1))
			} else if !ok2 {
				c.Echo("Finished")
				return
			}
		case u2, ok2 = <-upload2:
			if ok2 {
				c.Echo(fmt.Sprintf("u2: %v", u2))
			} else if !ok1 {
				c.Echo("Finished")
				return
			}
		}
	}
}

func (c *chat) Abort() {
	fmt.Println("Abort")
	c.Hub.Abort()
}

func TestServer(t *testing.T) {
	skipWithoutIntegration(t)

	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	srv := NewServer(
		WithAddress(":8100"),
		WithCodec("json"),
		WithHub(&chat{}),
	)

	srv.MapHTTP("/chat")

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
