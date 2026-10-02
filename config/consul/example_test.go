package consul_test

import (
	"context"

	"github.com/hashicorp/consul/api"
	"github.com/tx7do/go-wind-plugins/config/consul"
)

// ExampleNew constructs a Consul KV-backed configuration source. Load performs
// a one-shot read of the configured KV path; WatchValue delivers updated
// values on the returned channel whenever the data stored at that path
// changes.
func ExampleNew() {
	client, err := api.NewClient(api.DefaultConfig())
	if err != nil {
		return
	}

	src, err := consul.New(client,
		consul.WithPath("myapp/config"),
	)
	if err != nil {
		return
	}

	raw, err := src.Load(context.Background(), "")
	if err != nil {
		return
	}
	_ = raw

	ch, err := src.WatchValue(context.Background(), "")
	if err != nil {
		return
	}
	_ = ch
}
