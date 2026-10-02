package apollo_test

import (
	"context"

	"github.com/tx7do/go-wind-plugins/config/apollo"
)

// ExampleNewSource constructs an Apollo-backed configuration source. Load
// performs a one-shot read of the configured namespace; WatchValue delivers
// updated namespace content on the returned channel so the application can
// reload configuration without a restart.
func ExampleNewSource() {
	src := apollo.NewSource(
		apollo.WithAppID("my-app-id"),
		apollo.WithCluster("default"),
		apollo.WithEndpoint("http://apollo.example.com:8080"),
		apollo.WithNamespace("application"),
		apollo.WithSecret("placeholder-secret"),
	)

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
