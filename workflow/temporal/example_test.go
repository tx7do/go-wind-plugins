package temporal_test

import (
	"github.com/tx7do/go-wind-plugins/workflow/temporal"
)

// ExampleNewClient constructs a Temporal workflow client pointed at a server
// host:port and namespace. Workflow executions started through the client run
// against that server, and the client must be closed when the application
// shuts down.
func ExampleNewClient() {
	client, err := temporal.NewClient(
		temporal.WithClientHostPort("temporal.example.com:7233"),
		temporal.WithClientNamespace("default"),
	)
	if err != nil {
		return
	}
	defer client.Close()
}
