package polaris_test

import (
	"context"

	"github.com/polarismesh/polaris-go/api"
	"github.com/tx7do/go-wind"
	"github.com/tx7do/go-wind-plugins/registry/polaris"
)

// ExampleNew constructs a polaris-backed service registrar. Instances are
// registered on startup — before the server starts accepting traffic, so
// consumers can discover it immediately — and deregistered in the
// application's BeforeStop hook, which runs before any server shuts down so
// consumers stop seeing the instance before it disappears.
func ExampleNew() {
	provider, err := api.NewProviderAPI()
	if err != nil {
		return
	}
	consumer, err := api.NewConsumerAPI()
	if err != nil {
		return
	}

	reg := polaris.New(provider, consumer,
		polaris.WithNamespace("default"),
		polaris.WithServiceToken("my-service-token"),
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
