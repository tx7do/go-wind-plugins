package registry

import (
	"context"

	"github.com/tx7do/go-wind"
)

// Discovery handles service discovery — looking up and watching live
// instances of a named service.
//
// A service consumer uses [Discovery.GetService] for one-shot lookups and
// [Discovery.Watch] for reactive change streams.
type Discovery interface {
	// GetService returns all currently-registered instances for the named
	// service.
	GetService(ctx context.Context, serviceName string) ([]*wind.Instance, error)
	// Watch returns a [Watcher] that delivers subsequent instance changes
	// for the named service.
	Watch(ctx context.Context, serviceName string) (Watcher, error)
}

// Watcher delivers a stream of instance updates for a watched service.
// Each call to [Next] blocks until a new set of instances is available or
// an error occurs.
type Watcher interface {
	// Next blocks until the set of instances changes and returns the latest
	// snapshot.
	Next(ctx context.Context) ([]*wind.Instance, error)
	// Stop terminates the watcher and releases its resources.
	Stop() error
}
