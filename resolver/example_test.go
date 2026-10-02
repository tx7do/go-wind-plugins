package resolver_test

import (
	"google.golang.org/grpc"

	baseRegistry "github.com/tx7do/go-wind-plugins/registry"
	"github.com/tx7do/go-wind-plugins/resolver"
)

// ExampleNewBuilder adapts a registry discovery to a gRPC resolver builder so
// a client can dial a service by name. The builder watches the dialed service
// and pushes instance updates to the channel as instances come and go, and
// the "wind" scheme routes service-name targets to the builder. In an
// application the discovery comes from the configured registry plugin, the
// connection is passed to the generated service stubs, and it is closed on
// shutdown.
func ExampleNewBuilder() {
	// In production: provided by the application's registry plugin.
	var discovery baseRegistry.Discovery

	builder := resolver.NewBuilder(discovery)

	_, err := grpc.NewClient("wind:///user-service",
		grpc.WithResolvers(builder),
	)
	if err != nil {
		return
	}
}
