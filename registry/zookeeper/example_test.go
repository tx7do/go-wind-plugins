package zookeeper_test

import (
	"context"
	"time"

	"github.com/go-zookeeper/zk"
	"github.com/tx7do/go-wind"
	"github.com/tx7do/go-wind-plugins/registry/zookeeper"
)

// ExampleNew constructs a zookeeper-backed service registrar. Instances are
// registered on startup — before the server starts accepting traffic, so
// consumers can discover it immediately — and deregistered in the
// application's BeforeStop hook, which runs before any server shuts down so
// consumers stop seeing the instance before it disappears.
func ExampleNew() {
	conn, _, err := zk.Connect([]string{"localhost:2181"}, 5*time.Second)
	if err != nil {
		return
	}
	defer conn.Close()

	reg := zookeeper.New(conn,
		zookeeper.WithRootPath("/microservices"),
		zookeeper.WithDigestACL("my-username", "my-password"),
	)

	instance := &wind.Instance{
		ID:        "registry-demo-001",
		Name:      "demo-service",
		Version:   "1.0.0",
		Endpoints: []string{"http://localhost:8080"},
		Metadata:  map[string]string{"protocol": "http"},
	}

	_ = reg.Register(context.Background(), instance)
	_ = reg.Deregister(context.Background(), instance)
}
