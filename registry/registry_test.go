package registry

import (
	"context"
	"testing"

	"github.com/tx7do/go-wind"
)

// --- compile-time interface assertions ---

type mockRegistrar struct{}

func (mockRegistrar) Register(context.Context, *wind.Instance) error   { return nil }
func (mockRegistrar) Deregister(context.Context, *wind.Instance) error { return nil }

type mockDiscovery struct{}

func (mockDiscovery) GetService(context.Context, string) ([]*wind.Instance, error) {
	return nil, nil
}
func (mockDiscovery) Watch(context.Context, string) (Watcher, error) {
	return nil, nil
}

type mockWatcher struct{}

func (mockWatcher) Next(context.Context) ([]*wind.Instance, error) { return nil, nil }
func (mockWatcher) Stop() error                                    { return nil }

var (
	_ Registrar = mockRegistrar{}
	_ Discovery = mockDiscovery{}
	_ Watcher   = mockWatcher{}
)

// --- functional tests ---

func TestInterfaces_Independent(t *testing.T) {
	// Registrar and Discovery are independent — a pure service provider
	// only needs Registrar; a pure client only needs Discovery.
	var r Registrar = mockRegistrar{}
	_ = r

	var d Discovery = mockDiscovery{}
	_ = d
}
