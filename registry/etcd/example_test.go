package etcd_test

import (
	"context"
	"time"

	"github.com/tx7do/go-wind"
	"github.com/tx7do/go-wind-plugins/registry/etcd"
	clientv3 "go.etcd.io/etcd/client/v3"
)

// ExampleNew constructs an etcd-backed service registrar. Instances are
// registered on startup — before the server starts accepting traffic, so
// consumers can discover it immediately — and deregistered in the
// application's BeforeStop hook, which runs before any server shuts down so
// consumers stop seeing the instance before it disappears.
func ExampleNew() {
	client, err := clientv3.New(clientv3.Config{
		Endpoints:   []string{"localhost:2379"},
		DialTimeout: 5 * time.Second,
	})
	if err != nil {
		return
	}
	defer client.Close()

	reg := etcd.New(client,
		etcd.Namespace("/microservices"),
		etcd.RegisterTTL(15*time.Second),
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
