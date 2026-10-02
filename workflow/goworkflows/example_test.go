package goworkflows_test

import (
	"github.com/cschleiden/go-workflows/backend"
	goworkflows "github.com/tx7do/go-wind-plugins/workflow/goworkflows"
)

// ExampleNewClient constructs a workflow client on top of a backend. The
// backend owns the persistence layer that workflow and activity events are
// written to; any backend implementation provided by the workflow engine SDK
// can be passed, with the SDK's mock backend standing in here so the snippet
// stays compile-only. The client must be closed when the application shuts
// down.
func ExampleNewClient() {
	b := backend.NewMockBackend(nil)

	client, err := goworkflows.NewClient(b)
	if err != nil {
		return
	}
	defer client.Close()
}
