package polaris_test

import (
	"context"

	polarissdk "github.com/polarismesh/polaris-go"
	"github.com/tx7do/go-wind-plugins/config/polaris"
)

// ExampleNew constructs a Polaris-backed configuration source. Load performs
// a one-shot read of the configured config file within its file group;
// WatchValue delivers updated file content on the returned channel so the
// application can reload configuration without a restart.
func ExampleNew() {
	client, err := polarissdk.NewConfigAPI()
	if err != nil {
		return
	}

	src, err := polaris.New(client,
		polaris.WithFileGroup("myapp"),
		polaris.WithFileName("config.properties"),
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
