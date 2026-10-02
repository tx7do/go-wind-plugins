package consul_test

import (
	"context"

	"github.com/hashicorp/consul/api"
	"github.com/tx7do/go-wind"
	"github.com/tx7do/go-wind-plugins/registry/consul"
)

// ExampleNew constructs a consul-backed service registrar. Instances are
// registered on startup — before the server starts accepting traffic, so
// consumers can discover it immediately — and deregistered in the
// application's BeforeStop hook, which runs before any server shuts down so
// consumers stop seeing the instance before it disappears.
func ExampleNew() {
	client, err := api.NewClient(&api.Config{
		Address: "localhost:8500",
	})
	if err != nil {
		return
	}

	reg := consul.New(client,
		consul.WithDatacenter(consul.SingleDatacenter),
		consul.WithHealthCheck(false),
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
