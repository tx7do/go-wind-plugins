package selector_test

import (
	"context"
	"fmt"

	wind "github.com/tx7do/go-wind"
	baseRegistry "github.com/tx7do/go-wind-plugins/registry"
	"github.com/tx7do/go-wind-plugins/selector"
)

// ExamplePick applies the first-instance strategy to a fixed snapshot. The
// first strategy serves setups where the registry already performs load
// balancing, so every pick returns the head of the snapshot and the choice
// stays stable across calls. In an application the snapshot comes from the
// live view the balancer maintains for the named service.
func ExamplePick() {
	snapshot := []*wind.Instance{
		{ID: "user-service-1", Name: "user-service"},
		{ID: "user-service-2", Name: "user-service"},
	}

	inst, err := selector.Pick(selector.First(), snapshot)
	if err != nil {
		return
	}

	fmt.Println(inst.ID)
	// Output: user-service-1
}

// ExampleNewWatchedBalancer constructs a balancer that keeps a live view of
// each watched service from a registry discovery and picks instances from
// that view with the round-robin strategy. In an application the discovery
// comes from the configured registry plugin, every call site picks through
// the balancer instead of caching addresses, and Close is called on shutdown
// to release the watchers.
func ExampleNewWatchedBalancer() {
	// In production: provided by the application's registry plugin.
	var discovery baseRegistry.Discovery

	b := selector.NewWatchedBalancer(discovery, selector.RoundRobin())
	defer func() { _ = b.Close() }()

	_, _ = b.Pick(context.Background(), "user-service")
}
