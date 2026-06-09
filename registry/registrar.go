package registry

import (
	"context"

	"github.com/tx7do/go-wind"
)

// Registrar handles service registration lifecycle — adding and removing an
// [wind.Instance] from a service registry.
//
// A service provider typically calls [Registrar.Register] on startup and
// [Registrar.Deregister] on shutdown.
type Registrar interface {
	// Register adds the given service instance to the registry.
	Register(ctx context.Context, service *wind.Instance) error
	// Deregister removes the given service instance from the registry.
	Deregister(ctx context.Context, service *wind.Instance) error
}
