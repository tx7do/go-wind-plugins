package servicecomb_test

import (
	"context"

	"github.com/go-chassis/sc-client"
	"github.com/tx7do/go-wind"
	"github.com/tx7do/go-wind-plugins/registry/servicecomb"
)

// ExampleNew constructs a servicecomb-backed service registrar. Instances are
// registered on startup — before the server starts accepting traffic, so
// consumers can discover it immediately — and deregistered in the
// application's BeforeStop hook, which runs before any server shuts down so
// consumers stop seeing the instance before it disappears.
func ExampleNew() {
	client, err := sc.NewClient(sc.Options{
		Endpoints: []string{"localhost:30100"},
	})
	if err != nil {
		return
	}

	reg := servicecomb.New(client)

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
